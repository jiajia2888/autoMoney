package download

import (
	"automoney/dao"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"
)

var ErrBusy = errors.New("正在下载另一条视频，请完成后重试")
var numericID = regexp.MustCompile(`^[0-9]{10,30}$`)

type Job struct {
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	Filename string `json:"filename,omitempty"`
}
type Service struct {
	DB                                         *gorm.DB
	Python, Script, Root, Credentials, Profile string
	Ctx                                        context.Context
	Run                                        func(context.Context, dao.Video) error
	WechatRun                                  func(context.Context, dao.Video, string) error
	mu                                         sync.Mutex
	jobs                                       map[uint64]Job
	busy                                       bool
	wg                                         sync.WaitGroup
}

func (s *Service) video(id uint64) (dao.Video, error) {
	var v dao.Video
	var a dao.BenchmarkAccount
	if s.DB.First(&v, id).Error != nil || s.DB.First(&a, v.AccountID).Error != nil {
		return v, gorm.ErrRecordNotFound
	}
	if a.Platform == "wx" && s.WechatRun != nil && v.PlatformID != "" && len(v.PlatformID) <= 191 && v.Nonce != "" {
		return v, nil
	}
	if a.Platform != "dy" || !numericID.MatchString(v.PlatformID) || v.URL != "https://www.douyin.com/video/"+v.PlatformID {
		return v, errors.New("仅支持已收录的抖音视频")
	}
	return v, nil
}
func (s *Service) isWechat(v dao.Video) bool {
	var a dao.BenchmarkAccount
	return s.DB.First(&a, v.AccountID).Error == nil && a.Platform == "wx"
}
func (s *Service) path(v dao.Video) string {
	if s.isWechat(v) {
		return filepath.Join(filepath.Dir(s.Root), "wechat", fmt.Sprintf("%d-%x.mp4", v.ID, sha256.Sum256([]byte(v.PlatformID))))
	}
	return filepath.Join(s.Root, v.PlatformID+".mp4")
}
func (s *Service) verified(v dao.Video) (Job, error) {
	p := s.path(v)
	info, err := os.Lstat(p)
	if errors.Is(err, os.ErrNotExist) {
		return Job{Status: "not_started"}, nil
	}
	if err != nil {
		return Job{}, errors.New("无法读取本机视频文件")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 150*1024*1024 {
		return Job{}, errors.New("本机视频文件无效")
	}
	ctx, cancel := context.WithTimeout(s.Ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", p).Output()
	var probe struct {
		Streams []struct {
			Type string `json:"codec_type"`
		} `json:"streams"`
	}
	if err == nil && json.Unmarshal(out, &probe) == nil {
		for _, stream := range probe.Streams {
			if stream.Type == "video" {
				return Job{Status: "success", Bytes: info.Size(), Filename: filepath.Base(p)}, nil
			}
		}
	}
	return Job{}, errors.New("本机视频未通过完整性校验")
}
func (s *Service) Get(id uint64) (Job, error) {
	v, err := s.video(id)
	if err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		if j.Status == "success" {
			info, e := os.Lstat(s.path(v))
			if errors.Is(e, os.ErrNotExist) {
				delete(s.jobs, id)
				return Job{Status: "not_started"}, nil
			}
			if e != nil || !info.Mode().IsRegular() || info.Size() != j.Bytes {
				return Job{}, errors.New("本机视频文件已变动，请检查后重试")
			}
		}
		return j, nil
	}
	j, err := s.verified(v)
	if err != nil {
		return Job{}, err
	}
	if j.Status == "success" {
		if s.jobs == nil {
			s.jobs = map[uint64]Job{}
		}
		s.jobs[id] = j
	}
	return j, nil
}
func (s *Service) Start(id uint64) (Job, error) {
	if _, err := s.Get(id); err != nil {
		return Job{}, err
	}
	v, err := s.video(id)
	if err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok && (j.Status == "success" || j.Status == "running") {
		return j, nil
	}
	if s.busy {
		return Job{}, ErrBusy
	}
	if s.Ctx.Err() != nil {
		return Job{}, errors.New("下载服务正在停止")
	}
	if s.jobs == nil {
		s.jobs = map[uint64]Job{}
	}
	job := Job{Status: "running"}
	s.jobs[id] = job
	s.busy = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(s.Ctx, 4*time.Minute)
		defer cancel()
		run := s.Run
		if run == nil {
			run = s.runPython
		}
		var err error
		if s.isWechat(v) {
			err = s.WechatRun(ctx, v, s.path(v))
		} else {
			err = run(ctx, v)
		}
		result := Job{Status: "error", Error: "下载未完成，请重试"}
		if err != nil {
			result.Error = err.Error()
		} else {
			result, err = s.verified(v)
			if err != nil || result.Status != "success" {
				result = Job{Status: "error", Error: "未取得有效视频文件"}
			}
		}
		s.mu.Lock()
		s.jobs[id] = result
		s.busy = false
		s.mu.Unlock()
	}()
	return job, nil
}
func (s *Service) File(id uint64) (string, string, error) {
	j, err := s.Get(id)
	if err != nil {
		return "", "", err
	}
	if j.Status != "success" {
		return "", "", errors.New("视频尚未下载完成")
	}
	v, err := s.video(id)
	if err != nil {
		return "", "", err
	}
	p := s.path(v)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() != j.Bytes {
		return "", "", errors.New("视频文件已变动，请重新检查")
	}
	return p, j.Filename, nil
}
func (s *Service) Wait() { s.wg.Wait() }
func (s *Service) runPython(ctx context.Context, v dao.Video) error {
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	cmd := exec.CommandContext(ctx, abs(s.Python), abs(s.Script), "--url", v.URL, "--output-dir", abs(s.Root), "--credentials-file", abs(s.Credentials), "--profile", abs(s.Profile))
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "PYTHONPATH=") && !strings.HasPrefix(item, "PYTHONHOME=") && !strings.HasPrefix(item, "VIRTUAL_ENV=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	if ctx.Err() != nil {
		return errors.New("下载超时或服务已停止，请重试")
	}
	var result struct {
		Error string `json:"error"`
		Bytes int64  `json:"bytes"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil {
		return errors.New("下载进程未返回有效结果")
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	if err != nil || result.Bytes <= 0 {
		return errors.New("下载未完成")
	}
	return nil
}
