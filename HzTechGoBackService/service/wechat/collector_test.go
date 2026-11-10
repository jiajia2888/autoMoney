package wechat

import (
	"automoney/service/benchmark"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func samplePost(id string, thumbs int) map[string]any {
	return map[string]any{"exportId": id, "nonce": "nonce", "description": "作品", "createTime": 1700000000, "favCount": thumbs, "likeCount": 90000, "commentCount": 2, "forwardCount": 3, "accountInfo": map[string]any{"username": "target@finder"}}
}
func TestCollectorUsesThumbsAndOpaquePagination(t *testing.T) {
	calls := 0
	l, _ := loginFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("form required")
		}
		if c, err := r.Cookie("promotewebsessionid"); err != nil || c.Value != "test-session" {
			t.Error("missing session")
		}
		r.ParseForm()
		calls++
		if r.Form.Get("encryptedUsername") != "target@finder" || r.Form.Get("startTs") != "0" {
			t.Error("wrong target or time units")
		}
		cursor := "opaque+/= cursor"
		more := calls == 1
		if calls == 2 && r.Form.Get("lastBuf") != cursor {
			t.Error("cursor changed")
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"objects": []any{samplePost(string(rune('a'+calls)), 9999+calls)}, "continueFlag": more, "lastBuf": cursor}})
	})
	if err := l.store.save(credential{Session: "test-session", Identity: Identity{Username: "executor@finder"}}); err != nil {
		t.Fatal(err)
	}
	result, err := (Crawler{LoginService: l, PageDelay: time.Nanosecond}).FetchPosts(context.Background(), benchmark.Request{AccountID: "target@finder", MaxItems: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Posts) != 2 || result.Posts[0].Likes != 10000 || result.Posts[1].Likes != 10001 || *result.Posts[0].Hearts != 90000 || result.Posts[0].Favorites != 0 {
		t.Fatalf("incorrect metric mapping: %+v", result)
	}
}
func TestCollectorRejectsMissingCountsEmptyRepeatedCursorAndBusinessFailure(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "repeated", "expired", "other-author"} {
		t.Run(mode, func(t *testing.T) {
			l, _ := loginFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if mode == "expired" {
					w.Write([]byte(`{"errCode":-330}`))
					return
				}
				post := samplePost("same", 10001)
				items := []any{post}
				more := false
				switch mode {
				case "missing":
					delete(post, "favCount")
				case "empty":
					items = []any{}
				case "repeated":
					more = true
				case "other-author":
					post["accountInfo"] = map[string]any{"username": "other@finder"}
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"objects": items, "continueFlag": more, "lastBuf": "same"}})
			})
			l.store.save(credential{Session: "secret", Identity: Identity{Username: "executor@finder"}})
			_, err := (Crawler{LoginService: l, PageDelay: time.Nanosecond}).FetchPosts(context.Background(), benchmark.Request{AccountID: "target@finder", MaxItems: 5})
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected safe error: %v", err)
			}
			if mode == "expired" {
				status, _ := l.Status()
				if status.Configured {
					t.Fatal("expired session retained")
				}
			}
		})
	}
}

type shareTransport func(*http.Request) (*http.Response, error)

func (f shareTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestExpandShareAllowsOnlyOfficialDestinationWithoutCredentials(t *testing.T) {
	for _, dest := range []string{"https://channels.weixin.qq.com/finder-preview/pages/sph?x=test", "https://evil.example/finder-preview/pages/sph", "http://channels.weixin.qq.com/finder-preview/pages/sph", "https://channels.weixin.qq.com/other"} {
		t.Run(dest, func(t *testing.T) {
			called := 0
			l := NewLogin("unused")
			l.factory = func() *client {
				c := newClient()
				c.http.Transport = shareTransport(func(r *http.Request) (*http.Response, error) {
					called++
					if r.Header.Get("Cookie") != "" || r.URL.Host != "weixin.qq.com" {
						t.Error("credentials or wrong host")
					}
					return &http.Response{StatusCode: 301, Header: http.Header{"Location": {dest}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				})
				return c
			}
			got := l.ExpandShare(context.Background(), "https://weixin.qq.com/sph/test")
			if (got != "") != (dest == "https://channels.weixin.qq.com/finder-preview/pages/sph?x=test") || called != 1 {
				t.Fatalf("unsafe redirect or follow: %q calls=%d", got, called)
			}
		})
	}
}
