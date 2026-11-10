package frames

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEveryFrameAndPersistentArchive(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg not installed")
	}
	media := filepath.Join(t.TempDir(), "sample.mp4")
	if out, e := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=96x64:rate=12:duration=2", "-pix_fmt", "yuv420p", media).CombinedOutput(); e != nil {
		t.Fatalf("fixture: %v %s", e, out)
	}
	s := &Service{Ctx: context.Background(), Root: t.TempDir(), Prepare: func(context.Context, uint64) (string, error) { return media, nil }}
	if _, e := s.Start(1); e != nil {
		t.Fatal(e)
	}
	s.Wait()
	j := s.Get(1)
	if j.Status != "success" || j.Count != 24 {
		t.Fatalf("every frame must survive: %+v", j)
	}
	p, e := s.Archive(1)
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(p)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 24 {
		t.Fatalf("archive has %d images", len(z.File))
	}
	restored := &Service{Root: s.Root}
	if restored.Get(1).Count != 24 {
		t.Fatal("restart lost images")
	}
	if _, e = s.Image(1, 25); e == nil {
		t.Fatal("out of range accepted")
	}
	if _, e = s.Start(1); e != nil {
		t.Fatal("reuse", e)
	}
}
func TestCancelAndBusy(t *testing.T) {
	s := &Service{Ctx: context.Background(), Root: t.TempDir(), Prepare: func(ctx context.Context, _ uint64) (string, error) { <-ctx.Done(); return "", ctx.Err() }}
	if _, e := s.Start(1); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Start(2); e != ErrBusy {
		t.Fatalf("busy: %v", e)
	}
	s.Cancel(1)
	s.Wait()
	if s.Get(1).Status != "error" {
		t.Fatal("cancel reported success")
	}
	if _, e := os.Stat(s.dir(1)); !os.IsNotExist(e) {
		t.Fatal("cancel published partial output")
	}
}
func TestDecodeFailureDoesNotPublish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := &Service{Ctx: ctx, Root: t.TempDir(), Prepare: func(context.Context, uint64) (string, error) { return "/nonexistent-video.mp4", nil }}
	s.Start(1)
	s.Wait()
	if s.Get(1).Status != "error" {
		t.Fatal("invalid video accepted")
	}
	if _, e := s.Archive(1); e == nil {
		t.Fatal("failed archive exposed")
	}
}

func TestVariableFrameRateDoesNotDuplicateFrames(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg")
	}
	media := filepath.Join(t.TempDir(), "vfr.mp4")
	out, e := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=96x64:rate=12:duration=2", "-vf", `select=eq(n\,0)+eq(n\,5)+eq(n\,23)`, "-fps_mode", "vfr", "-pix_fmt", "yuv420p", media).CombinedOutput()
	if e != nil {
		t.Fatalf("vfr fixture: %v %s", e, out)
	}
	j, e := extract(context.Background(), media, t.TempDir(), func(int, int64) {})
	if e != nil || j.Count != 3 {
		t.Fatalf("expected 3 source frames, no duplicated gap frames: %+v %v", j, e)
	}
}

func TestSamplingCountIntervalAndCacheIsolation(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg")
	}
	media := filepath.Join(t.TempDir(), "sample.mp4")
	if out, e := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=96x64:rate=12:duration=4", "-pix_fmt", "yuv420p", media).CombinedOutput(); e != nil {
		t.Fatalf("fixture %v %s", e, out)
	}
	s := &Service{Ctx: context.Background(), Root: t.TempDir(), Prepare: func(context.Context, uint64) (string, error) { return media, nil }}
	s.Start(1)
	s.Wait()
	if j := s.Get(1); j.Status != "success" || j.Count != 30 {
		t.Fatalf("default: %+v", j)
	}
	o, _ := Normalize(Options{Mode: "interval", Interval: 1})
	s.Start(1, o)
	s.Wait()
	if j := s.Get(1, o); j.Status != "success" || j.Count != 4 {
		t.Fatalf("interval: %+v", j)
	}
	if s.Get(1).Count != 30 {
		t.Fatal("different settings overwrote default cache")
	}
	for _, o := range []Options{{Mode: "count", Count: -1}, {Mode: "count", Count: 301}, {Mode: "interval", Interval: 0}, {Mode: "bad"}} {
		if _, e := Normalize(o); e == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
}
