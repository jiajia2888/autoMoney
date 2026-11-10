package monitor

import (
	"automoney/dao"
	"automoney/service/benchmark"
	"testing"
	"time"
)

func TestFirstOver10kDateAndHistory(t *testing.T) {
	db := testDB(t)
	m := New(db, nil)
	a := dao.BenchmarkAccount{PlatformAccountID: KnownID, IntervalMinutes: 60, Threshold: 10000}
	db.Create(&a)
	now := time.Date(2026, 11, 8, 15, 59, 0, 0, time.UTC)
	apply := func(likes int64) {
		t.Helper()
		run := dao.CrawlRun{AccountID: a.ID}
		db.Create(&run)
		if err := m.Apply(a.ID, run.ID, benchmark.Result{Posts: []benchmark.Post{{PlatformID: "123", Likes: likes}}, Complete: true}, now); err != nil {
			t.Fatal(err)
		}
	}
	get := func() dao.Video { var v dao.Video; db.First(&v); return v }
	apply(10000)
	if get().FirstOver10kAt != nil {
		t.Fatal("10000 must not create an over-10k date")
	}
	now = now.Add(2 * time.Minute)
	crossing := now
	apply(10001)
	v := get()
	if v.FirstOver10kAt == nil || !v.FirstOver10kAt.Equal(crossing) || v.AlreadyOver10k {
		t.Fatal("crossing must use first observed date and previous exact count")
	}
	now = now.Add(24 * time.Hour)
	apply(9999)
	apply(12000)
	if !get().FirstOver10kAt.Equal(crossing) {
		t.Fatal("falling/re-crossing changed the original date")
	}
	// A newly discovered high-like video has an observation date, while the
	// actual historic crossing remains unknown.
	run := dao.CrawlRun{AccountID: a.ID}
	db.Create(&run)
	if err := m.Apply(a.ID, run.ID, benchmark.Result{Posts: []benchmark.Post{{PlatformID: "456", Likes: 20000}}}, now); err != nil {
		t.Fatal(err)
	}
	db.Where("platform_id = ?", "456").First(&v)
	var high dao.Video
	db.Where("platform_id = ?", "456").First(&high)
	if high.FirstOver10kAt == nil || !high.AlreadyOver10k {
		t.Fatal("historical high-like video must be marked already over threshold")
	}
}
