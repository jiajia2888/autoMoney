package web

import (
	"automoney/dao"
	"automoney/service/download"
	"automoney/service/monitor"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalMediaPlaybackRangePosterAndReuse(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, e := exec.LookPath(tool); e != nil {
			t.Skip(tool)
		}
	}
	root := t.TempDir()
	db, e := dao.OpenLocal("", filepath.Join(root, "media.db"))
	if e != nil {
		t.Fatal(e)
	}
	dao.Migrate(db)
	sql, _ := db.DB()
	defer sql.Close()
	a := dao.BenchmarkAccount{Platform: "dy", PlatformAccountID: "test"}
	db.Create(&a)
	v := dao.Video{AccountID: a.ID, PlatformID: "7639685139231984939", URL: "https://www.douyin.com/video/7639685139231984939"}
	db.Create(&v)
	p := filepath.Join(root, v.PlatformID+".mp4")
	if out, e := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=32x32:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", p).CombinedOutput(); e != nil {
		t.Fatalf("fixture %v %s", e, out)
	}
	s := &download.Service{DB: db, Root: root, Ctx: context.Background(), Run: func(context.Context, dao.Video) error {
		t.Error("cached video downloaded again")
		return fmt.Errorf("unexpected download")
	}}
	got, e := s.Ensure(context.Background(), v.ID)
	if e != nil || got != p {
		t.Fatal(got, e)
	}
	r := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	RegisterDownloadRoutes(r, s)
	request := func(path string, rangeHeader string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		q := httptest.NewRequest("GET", path, nil)
		q.Host = "127.0.0.1:9010"
		if rangeHeader != "" {
			q.Header.Set("Range", rangeHeader)
		}
		r.ServeHTTP(w, q)
		return w
	}
	prefix := fmt.Sprintf("/api/v1/videos/%d/media", v.ID)
	w := request(prefix+"/play", "bytes=0-63")
	if w.Code != 206 || w.Body.Len() != 64 || w.Header().Get("Content-Disposition") != "inline" {
		t.Fatal("range/inline playback", w.Code, w.Header())
	}
	w = request(prefix+"/poster", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/jpeg") || w.Body.Len() == 0 {
		t.Fatal("poster", w.Code)
	}
	w = request("/api/v1/media?ids="+fmt.Sprint(v.ID), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"success"`) {
		t.Fatal("download status", w.Body.String())
	}
	w = request("/api/v1/videos/999/media/play", "")
	if w.Code != 404 {
		t.Fatal("missing file exposed", w.Code)
	}
	os.Remove(p)
	if _, _, e = s.File(v.ID); e == nil {
		t.Fatal("deleted cache exposed")
	}
}
