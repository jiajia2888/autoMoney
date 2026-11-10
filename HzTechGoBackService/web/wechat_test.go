package web

import (
	"automoney/service/wechat"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWechatRoutesGuardAndNoStore(t *testing.T) {
	r := NewRouter(nil, t.TempDir())
	RegisterWechatRoutes(r, wechat.NewLogin(filepath.Join(t.TempDir(), "session.enc")))
	request := httptest.NewRequest("GET", "http://localhost/api/v1/wechat/connection", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"configured":false`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/v1/wechat/login", "/api/v1/wechat/login/poll", "/api/v1/wechat/connection/verify", "/api/v1/wechat/login/cancel"} {
		request = httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader(`{}`))
		request.Header.Set("Origin", "https://evil.example")
		w = httptest.NewRecorder()
		r.ServeHTTP(w, request)
		if w.Code != 403 {
			t.Fatalf("cross-origin accepted: %s", path)
		}
	}
	request = httptest.NewRequest("DELETE", "http://localhost/api/v1/wechat/connection", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != 403 {
		t.Fatal("cross-site deletion accepted")
	}
}
