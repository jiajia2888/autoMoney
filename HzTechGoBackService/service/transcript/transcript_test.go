package transcript

import (
	"automoney/dao"
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type fakeExtractor struct {
	calls   atomic.Int32
	started chan struct{}
	block   bool
}

func (f *fakeExtractor) Ready() error { return nil }
func (f *fakeExtractor) Extract(ctx context.Context, _ string) (Result, error) {
	f.calls.Add(1)
	if f.started != nil {
		close(f.started)
	}
	if f.block {
		<-ctx.Done()
		return Result{}, errors.New("任务已取消")
	}
	return Result{Text: "这是本地转写测试正文", Caption: "发布文案", SRT: "1\n00:00:00,000 --> 00:00:01,000\n测试\n", Engine: "test", Duration: 1}, nil
}
func newTestService(t *testing.T, f Extractor) *Service {
	t.Helper()
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := New(db, f)
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	sql, _ := db.DB()
	t.Cleanup(func() { cancel(); s.Wait(); sql.Close() })
	return s
}
func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task did not reach expected state")
}
func TestSuccessIsPersistedAndNotExtractedTwice(t *testing.T) {
	f := &fakeExtractor{}
	s := newTestService(t, f)
	v := dao.Video{AccountID: 1, PlatformID: "7639685139231984939", URL: "https://www.douyin.com/video/7639685139231984939"}
	s.DB.Create(&v)
	if _, err := s.Enqueue(v.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { _, row, err := s.Get(v.ID); return err == nil && row.Status == "success" })
	row, err := s.Enqueue(v.ID)
	if err != nil || row.Text == "" || row.SRT == "" || f.calls.Load() != 1 {
		t.Fatal("cached success should return stored text without duplicate extraction")
	}
}
func TestSingleFlightCancellationAndRestartState(t *testing.T) {
	f := &fakeExtractor{started: make(chan struct{}), block: true}
	s := newTestService(t, f)
	v := dao.Video{AccountID: 1, PlatformID: "7639685139231984939", URL: "https://www.douyin.com/video/7639685139231984939"}
	s.DB.Create(&v)
	if _, err := s.Enqueue(v.ID); err != nil {
		t.Fatal(err)
	}
	<-f.started
	if _, err := s.Enqueue(v.ID); !errors.Is(err, ErrBusy) {
		t.Fatal("duplicate running task allowed")
	}
	if !s.Cancel(v.ID) {
		t.Fatal("cancel request missing")
	}
	waitFor(t, func() bool { _, row, _ := s.Get(v.ID); return row.Status == "error" && row.Text == "" })
	if _, err := s.Enqueue(999); err == nil {
		t.Fatal("unknown video accepted")
	}
}

type localExtractor struct {
	fakeExtractor
	fileCalls atomic.Int32
}

func (f *localExtractor) ExtractFile(ctx context.Context, path string) (Result, error) {
	if path != "/project/verified.mp4" {
		return Result{}, errors.New("untrusted file")
	}
	f.fileCalls.Add(1)
	return Result{Text: "真实口播结果", SRT: "subtitle"}, nil
}
func TestLocalPreparationIsReusedAndFailureDoesNotTranscribe(t *testing.T) {
	f := &localExtractor{}
	s := newTestService(t, f)
	var prepared atomic.Int32
	s.Prepare = func(context.Context, dao.Video) (string, error) { prepared.Add(1); return "/project/verified.mp4", nil }
	v := dao.Video{AccountID: 1, PlatformID: "opaque-wechat-id", Title: "发布描述", LikesExact: true}
	s.DB.Create(&v)
	if _, err := s.Enqueue(v.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { _, j, _ := s.Get(v.ID); return j.Status == "success" })
	_, j, _ := s.Get(v.ID)
	if j.Caption != "发布描述" || f.fileCalls.Load() != 1 || f.calls.Load() != 0 {
		t.Fatal("local source/caption not used")
	}
	s.Enqueue(v.ID)
	if prepared.Load() != 1 {
		t.Fatal("completed transcript repeated download")
	}
	s.Prepare = func(context.Context, dao.Video) (string, error) {
		return "", errors.New("视频号会话已失效，请重新扫码")
	}
	v.ID = 0
	v.PlatformID = "other"
	s.DB.Create(&v)
	s.Enqueue(v.ID)
	waitFor(t, func() bool { _, j, _ := s.Get(v.ID); return j.Status == "error" })
	_, j, _ = s.Get(v.ID)
	if j.Text != "" || j.Error != "视频号会话已失效，请重新扫码" || f.fileCalls.Load() != 1 {
		t.Fatal("download failure was hidden or transcribed")
	}
}
