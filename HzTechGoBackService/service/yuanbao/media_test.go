package yuanbao

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}
func TestResolveProtocolAndCookieIsolation(t *testing.T) {
	calls := 0
	h := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			if r.URL.String() != parseURL || r.Header.Get("Cookie") != "hy_user=test; hy_token=secret" || body["scene"] != float64(1) || body["type"] != "video_channel_url" {
				t.Fatal("incorrect Yuanbao request")
			}
			return response(`{"code":0,"data":{"wx_export_id":"export/test","desc":"work","author":"author","playable_url":"https://channels.weixin.qq.com/finder-preview/pages/feed?token=private-token&eid=export%2Ftest"}}`), nil
		}
		if r.URL.String() != previewURL || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || obj(body, "baseReq")["generalToken"] != "private-token" || body["exportId"] != "export/test" {
			t.Fatal("preview isolation/contract failed")
		}
		return response(`{"errCode":0,"data":{"feedInfo":{"description":"work","createtime":100,"mediaType":4,"h264VideoInfo":{"videoUrl":"https://finder.video.qq.com/video.mp4?sig=x"}}}}`), nil
	})}
	media, err := resolve(context.Background(), h, credential{Cookie: "hy_user=test; hy_token=secret", Headers: map[string]string{"authorization": "secret"}}, "https://weixin.qq.com/sph/Test", SourceVideo{ExportID: "export/test", Title: "work", Author: "author", PublishedAt: 100})
	if err != nil || media != "https://finder.video.qq.com/video.mp4?sig=x" || calls != 2 {
		t.Fatalf("%s %v %d", media, err, calls)
	}
}
func TestResolveRejectsMismatchedIdentityPreviewAndBusinessError(t *testing.T) {
	for _, tc := range []struct{ parse, preview string }{
		{`{"code":0,"data":{"wx_export_id":"other","playable_url":"https://channels.weixin.qq.com/finder-preview/pages/feed?token=x&eid=y"}}`, ""},
		{`{"code":0,"data":{"playable_url":"https://channels.weixin.qq.com.evil.test/finder-preview/pages/feed?token=x&eid=y"}}`, ""},
		{`{"code":1,"data":{}}`, ""},
		{`{"code":0,"data":{"playable_url":"https://channels.weixin.qq.com/finder-preview/pages/feed?token=x&eid=y"}}`, `{"errCode":0,"data":{"errMsg":{"type":2,"title":"此内容暂时无法播放"}}}`},
	} {
		h := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() == parseURL {
				return response(tc.parse), nil
			}
			return response(tc.preview), nil
		})}
		if _, err := resolve(context.Background(), h, credential{}, "https://weixin.qq.com/sph/Test", SourceVideo{ExportID: "export/test", Title: "work", Author: "author", PublishedAt: 100}); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
func TestStreamBoundaries(t *testing.T) {
	for _, kind := range []string{"redirect", "html", "truncated", "valid"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			h := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
					t.Fatal("CDN credentials leaked")
				}
				res := response("video")
				res.Header.Set("Content-Type", "video/mp4")
				switch kind {
				case "redirect":
					res.StatusCode = 302
					res.Header.Set("Location", "https://example.org/video.mp4")
				case "html":
					res.Header.Set("Content-Type", "text/html")
				case "truncated":
					res.ContentLength = 20
				}
				return res, nil
			})}
			var out strings.Builder
			n, err := stream(context.Background(), h, "https://finder.video.qq.com/a", &out)
			if kind == "valid" {
				if err != nil || n != 5 {
					t.Fatal(n, err)
				}
			} else if err == nil {
				t.Fatal("unsafe media accepted")
			}
			if calls != 1 {
				t.Fatal("unexpected redirect request")
			}
		})
	}
}
func TestCredentialsNotExposedByConnection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	s := &Service{Credentials: p}
	if s.Status().Configured {
		t.Fatal("missing credentials accepted")
	}
	os.WriteFile(p, []byte(`{"cookie":"hy_user=user; hy_token=secret","saved_at":123}`), 0600)
	c := s.Status()
	raw, _ := json.Marshal(c)
	if !c.Configured || c.SavedAt != 123 || strings.Contains(string(raw), "secret") {
		t.Fatal(string(raw))
	}
	os.WriteFile(p, []byte(`{"cookie":"tracking=anonymous"}`), 0600)
	if s.Status().Configured {
		t.Fatal("anonymous cookie accepted")
	}
}

func TestSourceIdentityAcrossReissuedExportIDs(t *testing.T) {
	source := SourceVideo{ExportID: "collected-id", Title: "work", Author: "author", PublishedAt: 100}
	parsed := map[string]any{"wx_export_id": "reissued-id", "desc": "work", "author": "author"}
	feed := map[string]any{"description": "work", "createtime": float64(100)}
	if !matchesSource(parsed, feed, source) {
		t.Fatal("same work with reissued export ID rejected")
	}
	for _, field := range []string{"author", "desc"} {
		wrong := map[string]any{"desc": "work", "author": "author"}
		wrong[field] = "other"
		if matchesSource(wrong, feed, source) {
			t.Fatal("mismatched parse accepted", field)
		}
	}
	for _, wrong := range []map[string]any{{"description": "other", "createtime": float64(100)}, {"description": "work", "createtime": float64(101)}, {"description": "work"}} {
		if matchesSource(parsed, wrong, source) {
			t.Fatal("mismatched preview accepted")
		}
	}
	if matchesSource(parsed, feed, SourceVideo{ExportID: "x", Title: "work", PublishedAt: 100}) {
		t.Fatal("missing author accepted")
	}
}
