package frames

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxFrames = 1000
const MaxBytes int64 = 1 << 30

var ErrBusy = errors.New("另一条视频正在切片，请完成后重试")

type Options struct {
	Mode     string  `json:"mode"`
	Count    int     `json:"count"`
	Interval float64 `json:"interval"`
}

func Normalize(o Options) (Options, error) {
	if o.Mode == "" {
		o.Mode = "count"
	}
	if o.Mode == "count" {
		if o.Count == 0 {
			o.Count = 30
		}
		if o.Count < 1 || o.Count > 300 {
			return o, errors.New("提取数量应为1到300张")
		}
		o.Interval = 0
	} else if o.Mode == "interval" {
		if math.IsNaN(o.Interval) || math.IsInf(o.Interval, 0) || o.Interval < 0.1 || o.Interval > 3600 {
			return o, errors.New("间隔应为0.1到3600秒")
		}
		o.Count = 0
	} else {
		return o, errors.New("无效提取方式")
	}
	return o, nil
}
func option(args []Options) Options {
	if len(args) > 0 {
		return args[0]
	}
	o, _ := Normalize(Options{})
	return o
}
func key(id uint64, o Options) string {
	return fmt.Sprintf("%d-%s-%d-%s", id, o.Mode, o.Count, strconv.FormatFloat(o.Interval, 'f', -1, 64))
}

type Job struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Bytes  int64  `json:"bytes"`
	Error  string `json:"error,omitempty"`
}
type Service struct {
	Ctx     context.Context
	Root    string
	Prepare func(context.Context, uint64) (string, error)
	mu      sync.Mutex
	jobs    map[string]Job
	active  uint64
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func (s *Service) dir(id uint64, args ...Options) string {
	return filepath.Join(s.Root, key(id, option(args)))
}
func (s *Service) get(id uint64, args ...Options) Job {
	o := option(args)
	k := key(id, o)
	if j, ok := s.jobs[k]; ok {
		return j
	}
	var j Job
	b, e := os.ReadFile(filepath.Join(s.dir(id, o), "manifest.json"))
	if e == nil && json.Unmarshal(b, &j) == nil && j.Status == "success" && j.Count > 0 {
		if st, e := os.Stat(filepath.Join(s.dir(id, option(args)), "frames.zip")); e == nil && st.Size() > 0 {
			return j
		}
	}
	return Job{Status: "not_started"}
}
func (s *Service) Get(id uint64, args ...Options) Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(id, args...)
}
func (s *Service) Start(id uint64, args ...Options) (Job, error) {
	o, err := Normalize(option(args))
	if err != nil {
		return Job{}, err
	}
	k := key(id, o)
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.get(id, o)
	if j.Status == "running" || j.Status == "success" {
		return j, nil
	}
	if s.active != 0 {
		return Job{}, ErrBusy
	}
	if s.Ctx.Err() != nil {
		return Job{}, errors.New("切片服务正在停止")
	}
	if s.jobs == nil {
		s.jobs = map[string]Job{}
	}
	ctx, cancel := context.WithTimeout(s.Ctx, 20*time.Minute)
	s.active = id
	s.cancel = cancel
	j = Job{Status: "running"}
	s.jobs[k] = j
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		result, err := s.run(ctx, id, o)
		if err != nil {
			result = Job{Status: "error", Error: err.Error()}
			if ctx.Err() != nil {
				result.Error = "切片已取消或超时，可重新开始"
			}
		}
		s.mu.Lock()
		s.jobs[k] = result
		s.active = 0
		s.cancel = nil
		s.mu.Unlock()
	}()
	return j, nil
}
func (s *Service) Cancel(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == id && s.cancel != nil {
		s.cancel()
	}
}
func (s *Service) Wait() { s.wg.Wait() }
func (s *Service) Image(id uint64, n int, args ...Options) (string, error) {
	j := s.Get(id, args...)
	if j.Status != "success" || n < 1 || n > j.Count {
		return "", errors.New("切片未完成或图片不存在")
	}
	return filepath.Join(s.dir(id, option(args)), fmt.Sprintf("frame-%06d.jpg", n)), nil
}
func (s *Service) Archive(id uint64, args ...Options) (string, error) {
	if s.Get(id, args...).Status != "success" {
		return "", errors.New("切片尚未完成")
	}
	return filepath.Join(s.dir(id, option(args)), "frames.zip"), nil
}
func (s *Service) run(ctx context.Context, id uint64, o Options) (Job, error) {
	k := key(id, o)
	media, err := s.Prepare(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if err = os.MkdirAll(s.Root, 0700); err != nil {
		return Job{}, errors.New("无法创建切片目录")
	}
	tmp, err := os.MkdirTemp(s.Root, ".frames-")
	if err != nil {
		return Job{}, err
	}
	defer os.RemoveAll(tmp)
	progress := func(n int, b int64) {
		s.mu.Lock()
		s.jobs[k] = Job{Status: "running", Count: n, Bytes: b}
		s.mu.Unlock()
	}
	j, err := extract(ctx, media, tmp, progress, o)
	if err != nil {
		return Job{}, err
	}
	if err = pack(ctx, tmp, j.Count); err != nil {
		return Job{}, err
	}
	j.Status = "success"
	raw, _ := json.Marshal(j)
	if err = os.WriteFile(filepath.Join(tmp, "manifest.json"), raw, 0600); err != nil {
		return Job{}, err
	}
	if err = ctx.Err(); err != nil {
		return Job{}, err
	}
	if err = os.Rename(tmp, s.dir(id, o)); err != nil {
		return Job{}, errors.New("保存切片失败，请检查目录和剩余空间")
	}
	return j, nil
}
func scan(dir string) (int, int64, error) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return 0, 0, e
	}
	var bytes int64
	var n int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".jpg" {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return 0, 0, e
		}
		n++
		bytes += info.Size()
	}
	return n, bytes, nil
}

// Select source frames without generating duplicate images.
func extract(ctx context.Context, media, dir string, progress func(int, int64), args ...Options) (Job, error) {
	filter, err := selection(ctx, media, option(args))
	if err != nil {
		return Job{}, err
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, "ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", media, "-map", "0:v:0", "-an", "-sn", "-vf", filter, "-fps_mode", "passthrough", "-frames:v", fmt.Sprint(MaxFrames+1), "-q:v", "3", filepath.Join(dir, "frame-%06d.jpg"))
	if err := cmd.Start(); err != nil {
		return Job{}, errors.New("无法启动FFmpeg切片，请检查安装")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	var limit error
	for {
		select {
		case err := <-done:
			n, b, e := scan(dir)
			if limit != nil {
				return Job{}, limit
			}
			if ctx.Err() != nil {
				return Job{}, ctx.Err()
			}
			if err != nil || e != nil || n == 0 {
				return Job{}, errors.New("视频切片失败，请检查视频和剩余磁盘空间")
			}
			if n > MaxFrames || b > MaxBytes {
				return Job{}, errors.New("切片超出单次上限（1000张或1GB图片），未保存不完整结果")
			}
			progress(n, b)
			return Job{Count: n, Bytes: b}, nil
		case <-ticker.C:
			n, b, e := scan(dir)
			progress(n, b)
			if e != nil || n > MaxFrames || b > MaxBytes {
				limit = errors.New("切片超出单次上限（1000张或1GB图片），未保存不完整结果")
				cancel()
			}
		}
	}
}
func pack(ctx context.Context, dir string, count int) error {
	f, err := os.OpenFile(filepath.Join(dir, "frames.zip"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	for n := 1; n <= count; n++ {
		if err = ctx.Err(); err != nil {
			break
		}
		name := fmt.Sprintf("frame-%06d.jpg", n)
		var out io.Writer
		out, err = z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			break
		}
		var in *os.File
		in, err = os.Open(filepath.Join(dir, name))
		if err != nil {
			break
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			break
		}
	}
	ze := z.Close()
	fe := f.Close()
	if err != nil {
		return err
	}
	if ze != nil {
		return ze
	}
	return fe
}

func selection(ctx context.Context, media string, o Options) (string, error) {
	if o.Mode == "interval" {
		return fmt.Sprintf("select=isnan(prev_selected_t)+gte(t-prev_selected_t\\,%g)", o.Interval), nil
	}
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_entries", "stream=nb_read_frames", "-of", "json", media).Output()
	var probe struct {
		Streams []struct {
			Frames string `json:"nb_read_frames"`
		} `json:"streams"`
	}
	if err != nil || json.Unmarshal(raw, &probe) != nil || len(probe.Streams) != 1 {
		return "", errors.New("无法读取视频帧数")
	}
	total, e := strconv.Atoi(probe.Streams[0].Frames)
	if e != nil || total < 1 {
		return "", errors.New("视频没有可提取的帧")
	}
	count := min(o.Count, total)
	terms := make([]string, count)
	for i := 0; i < count; i++ {
		n := 0
		if count > 1 {
			n = i * (total - 1) / (count - 1)
		}
		terms[i] = fmt.Sprintf("eq(n\\,%d)", n)
	}
	return "select=" + strings.Join(terms, "+"), nil
}
