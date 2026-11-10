package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"automoney/service/benchmark"
)

func TestConnectionPersistenceAndRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local", "credentials.json")
	router := NewRouter(nil, t.TempDir())
	RegisterConnectionRoutes(router, benchmark.CredentialStore{Path: path})
	const secret = "test-only-session-secret"
	call := func(method, body, origin string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "/api/v1/collector/connection", bytes.NewBufferString(body))
		r.Host = "127.0.0.1:9010"
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("status %d want %d", w.Code, want)
		}
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("credential exposed in API response")
		}
		if w.Header().Get("Cache-Control") != "no-store" && want == 200 {
			t.Fatal("credential metadata may be cached")
		}
		return w.Body.String()
	}
	call("GET", "", "", 200)
	body, _ := json.Marshal(map[string]string{"mode": "cookie", "cookie": "sessionid=" + secret + "; msToken=test-signing-value", "user_agent": "Mozilla/5.0 TestBrowser"})
	call("PUT", string(body), "https://foreign.example", 403)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cross-origin mutation saved credentials")
	}
	call("PUT", string(body), "", 200)
	if result := call("GET", "", "", 200); !strings.Contains(result, `"configured":true`) || strings.Contains(result, `"cookie":`) || strings.Contains(result, `"user_agent":`) {
		t.Fatal("status must return metadata only")
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("credential file must be owner-readable only")
	}
	// A new store simulates restart; changing UI views or restarting must not
	// require another credential paste.
	if status, err := (benchmark.CredentialStore{Path: path}).Status(); err != nil || !status.Configured {
		t.Fatal("saved configuration did not survive restart")
	}
	call("PUT", `{"mode":"cookie","cookie":"msToken=only","user_agent":"Mozilla/5.0 TestBrowser"}`, "", 400)
	call("PUT", `{"mode":"cookie","cookie":"sessionid=x\r\nInjected: bad","user_agent":"Mozilla/5.0 TestBrowser"}`, "", 400)
	call("PUT", `{"mode":"cookie","cookie":"sessionid=x; sessionid=y","user_agent":"Mozilla/5.0 TestBrowser"}`, "", 400)
	call("PUT", `{"mode":"browser"}`, "", 200)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("switching to browser did not remove manual credentials")
	}
	call("GET", "", "", 200)
}
