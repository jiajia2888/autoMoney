package monitor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"automoney/dao"
	"automoney/service/benchmark"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	return db
}
func TestBaselineCrossingAndNewDedup(t *testing.T) {
	db := testDB(t)
	m := New(db, nil)
	now := time.Now().UTC()
	published := now.Add(-time.Hour)
	a := dao.BenchmarkAccount{PlatformAccountID: KnownID, DisplayName: "测试", IntervalMinutes: 60, Threshold: 10000, NotifyNew: true, NotifyThreshold: true}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	apply := func(posts ...benchmark.Post) {
		t.Helper()
		run := dao.CrawlRun{AccountID: a.ID, Kind: "collect", Status: "running"}
		db.Create(&run)
		if err := m.Apply(a.ID, run.ID, benchmark.Result{Posts: posts, Complete: true, StopReason: "end"}, now); err != nil {
			t.Fatal(err)
		}
	}
	post := func(id string, likes int64) benchmark.Post {
		return benchmark.Post{PlatformID: id, URL: "https://www.douyin.com/video/" + id, Title: id, Likes: likes, PublishedAt: &published}
	}
	apply(post("1", 9999), post("2", 10000), post("3", 10001))
	count := func() int64 {
		var n int64
		db.Model(&dao.Notification{}).Where("kind <> ?", "baseline").Count(&n)
		return n
	}
	if count() != 0 {
		t.Fatal("first scan must not alert history")
	}
	now = now.Add(time.Minute)
	published = now
	apply(post("1", 10001), post("2", 10000), post("3", 12000), post("4", 15000), post("4", 15000))
	if count() != 3 {
		t.Fatalf("want threshold 1 + new 4 + threshold 4, got %d", count())
	}
	apply(post("1", 10001), post("2", 10000), post("3", 12000), post("4", 15000))
	if count() != 3 {
		t.Fatal("duplicate alerts")
	}
	apply(post("1", 9999), post("4", 15000))
	apply(post("1", 11000), post("4", 15000))
	if count() != 3 {
		t.Fatal("crossing again must not resend")
	}
	var videos int64
	db.Model(&dao.Video{}).Count(&videos)
	if videos != 4 {
		t.Fatal("duplicate posts not deduped")
	}
	if err := m.Apply(a.ID, 1, benchmark.Result{}, now); err == nil {
		t.Fatal("empty must be error")
	}
}
func TestNormalize(t *testing.T) {
	for _, input := range []string{KnownID, "80185543890", "https://www.douyin.com/user/" + KnownID + "?a=b", "https://m.douyin.com/share/user/" + KnownID} {
		id, err := Normalize(input)
		if err != nil || id != KnownID {
			t.Fatalf("%s: %s %v", input, id, err)
		}
	}
	for _, input := range []string{"徐晶说", "123456", "https://evil.example/user/" + KnownID, "https://www.douyin.com@evil.test/user/" + KnownID, "https://www.douyin.com/video/123"} {
		if _, err := Normalize(input); err == nil {
			t.Fatal(fmt.Sprintf("accepted %s", input))
		}
	}
}
func TestFailedApplyRollsBack(t *testing.T) {
	db := testDB(t)
	m := New(db, nil)
	a := dao.BenchmarkAccount{PlatformAccountID: KnownID}
	db.Create(&a)
	err := m.Apply(a.ID, 1, benchmark.Result{Posts: []benchmark.Post{{PlatformID: "ok", Likes: 100}, {PlatformID: "bad", Likes: -1}}}, time.Now())
	if err == nil {
		t.Fatal("invalid must fail")
	}
	var n int64
	db.Model(&dao.Video{}).Count(&n)
	if n != 0 {
		t.Fatal("partial writes survived")
	}
}

type blockingCrawler struct{ started chan struct{} }

func (b blockingCrawler) FetchPosts(ctx context.Context, r benchmark.Request) (benchmark.Result, error) {
	close(b.started)
	<-ctx.Done()
	return benchmark.Result{}, ctx.Err()
}
func (b blockingCrawler) Login(ctx context.Context) error {
	close(b.started)
	<-ctx.Done()
	return ctx.Err()
}
func TestWorkerCancellationAndSingleFlight(t *testing.T) {
	db := testDB(t)
	started := make(chan struct{})
	m := New(db, blockingCrawler{started})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	run, err := m.Enqueue(0, "login")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker not started")
	}
	if _, err = m.Enqueue(0, "login"); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent run accepted")
	}
	if err = m.Cancel(run.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for m.Busy() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.Busy() {
		t.Fatal("worker failed to cancel")
	}
	var saved dao.CrawlRun
	db.First(&saved, run.ID)
	if saved.Status != "error" || saved.FinishedAt == nil {
		t.Fatalf("cancel not persisted: %#v", saved)
	}
}
