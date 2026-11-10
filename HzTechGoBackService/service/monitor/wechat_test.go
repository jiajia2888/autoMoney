package monitor

import (
	"automoney/dao"
	"automoney/service/benchmark"
	"testing"
	"time"
)

func TestWechatBaselineAndLaterThumbCrossing(t *testing.T) {
	db := testDB(t)
	m := New(db, nil)
	a := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "target@finder", DisplayName: "作者", Threshold: 10000, IntervalMinutes: 60, NotifyNew: true, NotifyThreshold: true}
	db.Create(&a)
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	hearts := int64(99999)
	apply := func(posts []benchmark.Post) {
		t.Helper()
		r := dao.CrawlRun{AccountID: a.ID}
		db.Create(&r)
		if err := m.Apply(a.ID, r.ID, benchmark.Result{Posts: posts, Complete: true}, now); err != nil {
			t.Fatal(err)
		}
	}
	apply([]benchmark.Post{{PlatformID: "old-high", Likes: 10001, Hearts: &hearts, PublishedAt: &old}, {PlatformID: "old-low", Likes: 10000, Hearts: &hearts, PublishedAt: &old}})
	var n int64
	db.Model(&dao.Notification{}).Where("kind <> ?", "baseline").Count(&n)
	if n != 0 {
		t.Fatal("history alerted")
	}
	var low dao.Video
	db.Where("platform_id = ?", "old-low").First(&low)
	if low.FirstOver10kAt != nil || low.Hearts == nil || *low.Hearts != hearts || low.Source != "wechat_promote_web" {
		t.Fatal("incorrect thumb boundary or source")
	}
	now = now.Add(time.Hour)
	published := now.Add(-time.Minute)
	apply([]benchmark.Post{{PlatformID: "old-high", Likes: 12000, Hearts: &hearts, PublishedAt: &old}, {PlatformID: "old-low", Likes: 10001, Hearts: &hearts, PublishedAt: &old}, {PlatformID: "new", Likes: 10001, Hearts: &hearts, PublishedAt: &published}})
	db.Model(&dao.Notification{}).Where("kind <> ?", "baseline").Count(&n)
	if n != 3 {
		t.Fatalf("want crossing + new + new threshold, got %d", n)
	}
	apply([]benchmark.Post{{PlatformID: "new", Likes: 15000, Hearts: &hearts, PublishedAt: &published}})
	db.Model(&dao.Notification{}).Where("kind <> ?", "baseline").Count(&n)
	if n != 3 {
		t.Fatal("repeat alerted")
	}
}

func TestWechatHistoryContinuesWithoutResettingBaselineOrAlerting(t *testing.T) {
	db := testDB(t)
	m := New(db, nil)
	a := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "history@finder", Threshold: 10000, IntervalMinutes: 60, NotifyThreshold: true}
	db.Create(&a)
	now := time.Now().UTC()
	rangeEnd := now.Add(-time.Minute)
	run := dao.CrawlRun{AccountID: a.ID}
	db.Create(&run)
	if err := m.Apply(a.ID, run.ID, benchmark.Result{Backfill: true, RangeEnd: &rangeEnd, NextCursor: "opaque", Posts: []benchmark.Post{{PlatformID: "first", Likes: 10001}}}, now); err != nil {
		t.Fatal(err)
	}
	db.First(&a, a.ID)
	if a.HistoryCursor != "opaque" || a.HistoryComplete || a.HistoryEnd == nil || a.BaselineAt == nil {
		t.Fatal("history progress lost")
	}
	baseline := *a.BaselineAt
	run = dao.CrawlRun{AccountID: a.ID}
	db.Create(&run)
	if err := m.Apply(a.ID, run.ID, benchmark.Result{Backfill: true, RangeEnd: &rangeEnd, Complete: true, Posts: []benchmark.Post{{PlatformID: "older", Likes: 20000}}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	db.First(&a, a.ID)
	if !a.HistoryComplete || !a.BaselineAt.Equal(baseline) {
		t.Fatal("history completion or stable baseline lost")
	}
	var n int64
	db.Model(&dao.Notification{}).Where("kind <> ?", "baseline").Count(&n)
	if n != 0 {
		t.Fatal("historical continuation emitted alerts")
	}
}
