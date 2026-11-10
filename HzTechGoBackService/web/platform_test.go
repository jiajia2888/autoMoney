package web

import (
	"automoney/dao"
	"automoney/service/monitor"
	"automoney/service/wechat"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlatformAndNewVideoDateFilters(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	dao.Migrate(db)
	baseline := time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)
	old := baseline.Add(-time.Hour)
	newDate := baseline.Add(time.Hour)
	wx := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "author@finder", BaselineAt: &baseline}
	dy := dao.BenchmarkAccount{Platform: "dy", PlatformAccountID: monitor.KnownID}
	db.Create(&wx)
	db.Create(&dy)
	for _, v := range []dao.Video{{AccountID: wx.ID, PlatformID: "history", Likes: 20000, PublishedAt: &old}, {AccountID: wx.ID, PlatformID: "new", Likes: 10001, PublishedAt: &newDate}, {AccountID: wx.ID, PlatformID: "boundary", Likes: 10000, PublishedAt: &newDate}, {AccountID: dy.ID, PlatformID: "dy", Likes: 20000, PublishedAt: &newDate}} {
		db.Create(&v)
	}
	db.Create(&dao.CrawlRun{AccountID: wx.ID})
	db.Create(&dao.CrawlRun{AccountID: dy.ID})
	router := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	get := func(path string) any {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/"+path, nil)
		r.Host = "127.0.0.1:9010"
		router.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return body["data"]
	}
	videos := get("videos?platform=wx&min_likes=10000&cohort=new&published_from=2026-11-09&published_to=2026-11-09").(map[string]any)
	if videos["total"] != float64(1) || videos["items"].([]any)[0].(map[string]any)["platform_id"] != "new" {
		t.Fatal("wrong platform, date, thumb boundary or cohort")
	}
	if len(get("accounts?platform=wx").([]any)) != 1 || len(get("runs?platform=wx").([]any)) != 1 {
		t.Fatal("cross-platform list leakage")
	}
	counts := get("overview?platform=wx").(map[string]any)
	if counts["accounts"] != float64(1) || counts["videos"] != float64(3) || counts["hits"] != float64(2) {
		t.Fatal("mixed overview")
	}
}

func TestWechatShareOffersQRForCachedOfficialLink(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "share.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	dao.Migrate(db)
	a := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "target@finder"}
	db.Create(&a)
	v := dao.Video{AccountID: a.ID, PlatformID: "one", URL: "https://weixin.qq.com/sph/test"}
	db.Create(&v)
	router := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	// A canceled request skips live URL expansion while exercising cached link QR.
	RegisterWechatCollectionRoutes(router, db, wechat.NewLogin(filepath.Join(t.TempDir(), "session.enc")))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/wechat/videos/%d/share", v.ID), nil)
	r.Host = "127.0.0.1:9010"
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	r = r.WithContext(ctx)
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out struct {
		Data struct {
			URL string `json:"url"`
			QR  string `json:"qr_image"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.URL != v.URL || !strings.HasPrefix(out.Data.QR, "data:image/png;base64,") {
		t.Fatal("missing cached link or QR")
	}
}

func TestShowAllVideosPreservesPlatformAndThreshold(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "all.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err := dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	wx := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "target@finder"}
	dy := dao.BenchmarkAccount{Platform: "dy", PlatformAccountID: "douyin"}
	db.Create(&wx)
	db.Create(&dy)
	for i := 0; i < 30; i++ {
		db.Create(&dao.Video{AccountID: wx.ID, PlatformID: fmt.Sprint(i), Likes: 10001})
	}
	db.Create(&dao.Video{AccountID: wx.ID, PlatformID: "below", Likes: 10000})
	db.Create(&dao.Video{AccountID: dy.ID, PlatformID: "other", Likes: 999999})
	router := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	for _, all := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/videos?platform=wx&min_likes=10000&all=%t&page=1", all), nil)
		r.Host = "127.0.0.1:9010"
		router.ServeHTTP(w, r)
		var body struct {
			Data struct {
				Items []dao.Video `json:"items"`
				Total int         `json:"total"`
				All   bool        `json:"all"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
			t.Fatalf("invalid response: %s", w.Body)
		}
		expected := 24
		if all {
			expected = 30
		}
		if len(body.Data.Items) != expected || body.Data.Total != 30 || body.Data.All != all {
			t.Fatalf("wrong pagination: %+v", body.Data)
		}
		for _, v := range body.Data.Items {
			if v.AccountID != wx.ID || v.Likes <= 10000 {
				t.Fatal("filter lost")
			}
		}
	}
}
