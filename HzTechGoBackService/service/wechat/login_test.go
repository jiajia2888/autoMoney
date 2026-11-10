package wechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loginFixture(t *testing.T, handler http.HandlerFunc) (*Login, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "session.enc")
	l := NewLogin(path)
	l.factory = func() *client {
		c := newClient()
		c.base = server.URL
		c.http.Timeout = time.Second
		return c
	}
	return l, path
}
func TestLoginPersistsOnlyAfterProfileAndRestoresEncryptedSession(t *testing.T) {
	status := 0
	l, path := loginFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Query().Get("_pageUrl") == "" {
			t.Error("invalid request contract")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "auth_login_token"):
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"data":{"token":"secret-token","futureField":"preserve"}}`))
		case strings.HasSuffix(r.URL.Path, "auth_login_status"):
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["futureField"] != "preserve" {
				t.Error("challenge fields lost")
			}
			if status == 1 {
				http.SetCookie(w, &http.Cookie{Name: "promotewebsessionid", Value: "private-session", Path: "/promote"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": status, "acctStatus": 1}})
		default:
			cookie, err := r.Cookie("promotewebsessionid")
			if err != nil || cookie.Value != "private-session" {
				t.Error("profile missing session")
			}
			_, _ = w.Write([]byte(`{"data":{"selfAccountInfo":{"username":"test@finder","nickName":"测试投手","cookie":"never-forward"}}}`))
		}
	})
	ctx := context.Background()
	qr, err := l.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(qr.QRImage, "data:image/png;base64,") {
		t.Fatal("missing qr")
	}
	for _, step := range []struct {
		status int
		want   string
	}{{0, "waiting"}, {5, "scanned"}, {1, "success"}} {
		status = step.status
		l.pending.lastPoll = time.Time{}
		result, err := l.Poll(ctx, qr.ID)
		if err != nil || result.State != step.want {
			t.Fatalf("%+v %v", result, err)
		}
		if status != 1 {
			if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("saved before confirmation")
			}
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "private-session") {
		t.Fatal("plaintext session")
	}
	for _, name := range []string{path, path + ".key"} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("bad file permissions %s", name)
		}
	}
	restored := NewLogin(path)
	restored.factory = l.factory
	connection, err := restored.Status()
	if err != nil || !connection.Configured || connection.Identity.Username != "test@finder" {
		t.Fatalf("restore failed: %+v %v", connection, err)
	}
	body, _ := json.Marshal(connection)
	if strings.Contains(string(body), "private-session") || strings.Contains(string(body), "never-forward") {
		t.Fatal("credentials leaked")
	}
	if _, err = restored.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	if err = restored.Disconnect(); err != nil {
		t.Fatal(err)
	}
	connection, err = restored.Status()
	if err != nil || connection.Configured {
		t.Fatal("disconnect failed")
	}
}
func TestLoginRejectsMissingSessionOrIdentity(t *testing.T) {
	for _, cookie := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-cookie", true: "no-identity"}[cookie], func(t *testing.T) {
			l, path := loginFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "auth_login_token"):
					_, _ = w.Write([]byte(`{"data":{"token":"token"}}`))
				case strings.HasSuffix(r.URL.Path, "auth_login_status"):
					if cookie {
						http.SetCookie(w, &http.Cookie{Name: "promotewebsessionid", Value: "private", Path: "/"})
					}
					_, _ = w.Write([]byte(`{"data":{"status":1,"acctStatus":1}}`))
				default:
					_, _ = w.Write([]byte(`{"data":{"selfAccountInfo":{"nickName":"no username"}}}`))
				}
			})
			qr, err := l.Start(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = l.Poll(context.Background(), qr.ID); err == nil {
				t.Fatal("false success")
			}
			if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("saved invalid session")
			}
		})
	}
}
func TestExpiryCancelAndStaleAttempt(t *testing.T) {
	l, _ := loginFixture(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":{"token":"token"}}`)) })
	ctx := context.Background()
	first, _ := l.Start(ctx)
	second, _ := l.Start(ctx)
	if _, err := l.Poll(ctx, first.ID); err == nil {
		t.Fatal("old attempt accepted")
	}
	_ = l.Cancel(first.ID)
	l.pending.expires = time.Now().Add(-time.Second)
	result, err := l.Poll(ctx, second.ID)
	if err != nil || result.State != "expired" || l.pending.client != nil {
		t.Fatal("expiry failed")
	}
	_ = l.Cancel(second.ID)
	if _, err = l.Poll(ctx, second.ID); err == nil {
		t.Fatal("cancelled attempt accepted")
	}
}
func TestBusinessErrorsAreSafeAndNested(t *testing.T) {
	for _, body := range []string{`{"error":"private-session"}`, `{"errCode":17}`, `{"data":{"baseResp":{"errcode":3}}}`, `{"data":{"baseResponse":{"Ret":-333}}}`} {
		var value map[string]any
		_ = json.Unmarshal([]byte(body), &value)
		err := checkResponse(value)
		if err == nil || strings.Contains(err.Error(), "private-session") {
			t.Fatal("unsafe success or error")
		}
	}
}
func TestVerifyDistinguishesNetworkAndExpired(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "network", true: "expired"}[expired], func(t *testing.T) {
			l, _ := loginFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if expired {
					_, _ = w.Write([]byte(`{"data":{"baseResp":{"errcode":-330}}}`))
				} else {
					w.WriteHeader(502)
				}
			})
			if err := l.store.save(credential{Session: "private-session", Identity: Identity{Username: "id"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := l.Verify(context.Background()); err == nil {
				t.Fatal("unexpected success")
			}
			c, err := l.Status()
			if err != nil || c.Configured == expired {
				t.Fatal("wrong session invalidation")
			}
		})
	}
}

// This is the lifetime returned by the real confirmed-login response; the
// credential is synthetic. A large Max-Age overflows cookiejar's duration math.
func TestConfirmedLoginWithWechatCookieLifetime(t *testing.T) {
	l, _ := loginFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "auth_login_token"):
			_, _ = w.Write([]byte(`{"data":{"token":"synthetic-token"}}`))
		case strings.HasSuffix(r.URL.Path, "auth_login_status"):
			w.Header().Add("Set-Cookie", "promotewebsessionid=synthetic-session; Max-Age=253402300000; Path=/; Expires=Wed, 08 Nov 10056 12:45:06 GMT")
			_, _ = w.Write([]byte(`{"data":{"status":1,"acctStatus":1}}`))
		default:
			cookie, err := r.Cookie("promotewebsessionid")
			if err != nil || cookie.Value != "synthetic-session" {
				t.Error("profile did not receive confirmed session")
			}
			_, _ = w.Write([]byte(`{"data":{"selfAccountInfo":{"username":"synthetic@finder","nickName":"测试"}}}`))
		}
	})
	qr, err := l.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := l.Poll(context.Background(), qr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "success" {
		t.Fatalf("confirmed login did not succeed: %s", result.State)
	}
}

func TestSessionJarBoundsLifetimeWithoutWeakeningCookieRules(t *testing.T) {
	jar := newSessionJar()
	u, _ := url.Parse("https://channels.weixin.qq.com/promote/api/web/auth/auth_login_status")
	cookie := &http.Cookie{Name: "promotewebsessionid", Value: "synthetic", Path: "/promote", Secure: true, MaxAge: 253402300000}
	jar.SetCookies(u, []*http.Cookie{cookie})
	if cookie.MaxAge != 253402300000 {
		t.Fatal("mutated caller cookie")
	}
	profile, _ := url.Parse("https://channels.weixin.qq.com/promote/api/web/" + profilePath)
	if len(jar.Cookies(profile)) != 1 {
		t.Fatal("large lifetime expired new cookie")
	}
	for _, raw := range []string{"http://channels.weixin.qq.com/promote/api/web/", "https://channels.weixin.qq.com/other", "https://evil.example/promote/api/web/"} {
		target, _ := url.Parse(raw)
		if len(jar.Cookies(target)) != 0 {
			t.Fatal("cookie scope weakened")
		}
	}
	jar.SetCookies(u, []*http.Cookie{{Name: cookie.Name, Value: "", Path: cookie.Path, Secure: true, MaxAge: -1}})
	if len(jar.Cookies(profile)) != 0 {
		t.Fatal("cookie deletion ignored")
	}
}
