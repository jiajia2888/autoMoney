package web

import (
	"automoney/dao"
	"automoney/service/monitor"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoveryDateUsesShanghaiAndKeepsHistoricalHits(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "date.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err := dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, 11, 8, 15, 59, 0, 0, time.UTC)
	inside := before.Add(time.Minute)
	after := inside.Add(24 * time.Hour)
	for i, row := range []dao.Video{{PlatformID: "a", Likes: 20000, FirstOver10kAt: &before}, {PlatformID: "b", Likes: 9000, FirstOver10kAt: &inside}, {PlatformID: "c", Likes: 20000, FirstOver10kAt: &after}, {PlatformID: "sample", Likes: 30000, LikesExact: false}} {
		row.AccountID = uint64(i + 1)
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	get := func(query string, status int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/videos?"+query, nil)
		r.Host = "127.0.0.1:9010"
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
		var data map[string]any
		json.Unmarshal(w.Body.Bytes(), &data)
		return data
	}
	data := get("over_10k=true&sort=over_date&over_date_from=2026-11-09&over_date_to=2026-11-09", 200)["data"].(map[string]any)
	if data["total"].(float64) != 1 || data["items"].([]any)[0].(map[string]any)["platform_id"] != "b" {
		t.Fatal("date range must use Shanghai and include a previously recorded hit whose likes fell")
	}
	get("over_date_from=2026-11-10&over_date_to=2026-11-09", 400)
	get("over_date_from=2026-13-99", 400)
	data = get("over_10k=true&sort=over_date", 200)["data"].(map[string]any)
	items := data["items"].([]any)
	if len(items) != 4 || items[3].(map[string]any)["platform_id"] != "sample" {
		t.Fatal("unknown sample dates should remain unknown and sort last")
	}
}
