package transcript

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"automoney/dao"
	"gorm.io/gorm"
)

var ErrBusy = errors.New("本机正在提取另一条视频，请稍后重试")

type Result struct {
	Caption  string  `json:"caption"`
	Text     string  `json:"text"`
	SRT      string  `json:"srt"`
	Engine   string  `json:"engine"`
	Duration float64 `json:"duration"`
}
type Extractor interface {
	Ready() error
	Extract(context.Context, string) (Result, error)
}
type FileExtractor interface {
	ExtractFile(context.Context, string) (Result, error)
}
type work struct {
	Video  dao.Video
	Ctx    context.Context
	Cancel context.CancelFunc
}
type Service struct {
	DB        *gorm.DB
	Prepare   func(context.Context, dao.Video) (string, error)
	Extractor Extractor
	queue     chan work
	parent    context.Context
	mu        sync.Mutex
	current   uint64
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func New(db *gorm.DB, extractor Extractor) *Service {
	return &Service{DB: db, Extractor: extractor, queue: make(chan work, 1)}
}
func (s *Service) Start(ctx context.Context) {
	s.parent = ctx
	now := time.Now().UTC()
	s.DB.Model(&dao.VideoTranscript{}).Where("status IN ?", []string{"queued", "running"}).Updates(map[string]any{"status": "interrupted", "stage": "服务重启，本次提取已中断，可重新提取", "error": "服务重启，提取未完成", "finished_at": now})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case w := <-s.queue:
				s.run(w)
			}
		}
	}()
}
func (s *Service) Wait() { s.wg.Wait() }
func (s *Service) Ready() error {
	if s.Extractor == nil {
		return fmt.Errorf("本地转写工具尚未配置")
	}
	return s.Extractor.Ready()
}
func (s *Service) Get(videoID uint64) (dao.Video, dao.VideoTranscript, error) {
	var v dao.Video
	if err := s.DB.First(&v, videoID).Error; err != nil {
		return v, dao.VideoTranscript{}, err
	}
	var row dao.VideoTranscript
	err := s.DB.Where("video_id = ?", videoID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = dao.VideoTranscript{VideoID: videoID, AccountID: v.AccountID, Status: "not_started"}
		err = nil
	}
	return v, row, err
}
func (s *Service) Enqueue(videoID uint64) (dao.VideoTranscript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, row, err := s.Get(videoID)
	if err != nil {
		return row, err
	}
	if row.Status == "success" {
		return row, nil
	}
	if s.current != 0 {
		return row, ErrBusy
	}
	if s.parent == nil || s.parent.Err() != nil {
		return row, fmt.Errorf("文案提取服务未启动")
	}
	if err = s.Ready(); err != nil {
		return row, err
	}
	row.Status, row.Stage, row.Error = "queued", "等待本机提取", ""
	row.FinishedAt = nil
	if err = s.DB.Save(&row).Error; err != nil {
		return row, fmt.Errorf("保存提取任务失败")
	}
	ctx, cancel := context.WithTimeout(s.parent, 35*time.Minute)
	s.current, s.cancel = videoID, cancel
	s.queue <- work{Video: v, Ctx: ctx, Cancel: cancel}
	return row, nil
}
func (s *Service) Cancel(videoID uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != videoID || s.cancel == nil {
		return false
	}
	s.cancel()
	return true
}
func (s *Service) run(w work) {
	defer w.Cancel()
	defer func() { s.mu.Lock(); s.current = 0; s.cancel = nil; s.mu.Unlock() }()
	s.DB.Model(&dao.VideoTranscript{}).Where("video_id = ?", w.Video.ID).Updates(map[string]any{"status": "running", "stage": "正在下载视频并用本机 Whisper 提取口播（最长 30 分钟视频）"})
	var result Result
	var err error
	if s.Prepare != nil {
		var path string
		path, err = s.Prepare(w.Ctx, w.Video)
		if err == nil {
			s.DB.Model(&dao.VideoTranscript{}).Where("video_id = ?", w.Video.ID).Update("stage", "视频已保存，正在本机提取口播；无需重复下载")
			if extractor, ok := s.Extractor.(FileExtractor); ok {
				result, err = extractor.ExtractFile(w.Ctx, path)
			} else {
				err = errors.New("本地文件转写未配置")
			}
		}
	} else {
		result, err = s.Extractor.Extract(w.Ctx, w.Video.URL)
	}
	if result.Caption == "" && w.Video.LikesExact {
		result.Caption = w.Video.Title
	}
	now := time.Now().UTC()
	updates := map[string]any{"finished_at": now, "updated_at": now}
	if err != nil || result.Text == "" {
		message := "未识别出可用口播，没有以标题代替正文"
		if err != nil {
			message = err.Error()
		}
		updates["status"], updates["stage"], updates["error"] = "error", "提取未完成", message
	} else {
		updates["status"], updates["stage"], updates["error"] = "success", "提取完成；语音识别结果请核对", ""
		updates["caption"], updates["text"], updates["srt"] = result.Caption, result.Text, result.SRT
		updates["engine"], updates["duration"] = result.Engine, result.Duration
	}
	s.DB.Model(&dao.VideoTranscript{}).Where("video_id = ?", w.Video.ID).Updates(updates)
}
