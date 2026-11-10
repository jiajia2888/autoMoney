package download

import (
	"automoney/dao"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestQueueDedupBusyAndFailure(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	a := dao.BenchmarkAccount{Platform: "dy", PlatformAccountID: "test"}
	db.Create(&a)
	v := dao.Video{AccountID: a.ID, PlatformID: "7639685139231984939", URL: "https://www.douyin.com/video/7639685139231984939"}
	db.Create(&v)
	v2 := dao.Video{AccountID: a.ID, PlatformID: "7639685139231984940", URL: "https://www.douyin.com/video/7639685139231984940"}
	db.Create(&v2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	s := &Service{DB: db, Root: t.TempDir(), Ctx: ctx, Run: func(context.Context, dao.Video) error {
		close(started)
		<-release
		return errors.New("测试下载失败")
	}}
	j, err := s.Start(v.ID)
	if err != nil || j.Status != "running" {
		t.Fatalf("%+v %v", j, err)
	}
	<-started
	j, err = s.Start(v.ID)
	if err != nil || j.Status != "running" {
		t.Fatalf("repeat %+v %v", j, err)
	}
	if _, err = s.Start(v2.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	if _, _, err = s.File(v.ID); err == nil {
		t.Fatal("running file must not be exposed")
	}
	close(release)
	s.Wait()
	j, err = s.Get(v.ID)
	if err != nil || j.Status != "error" || j.Error != "测试下载失败" {
		t.Fatalf("%+v %v", j, err)
	}
	if _, _, err = s.File(v.ID); err == nil {
		t.Fatal("failed file must not be exposed")
	}
	a.Platform = "wx"
	db.Save(&a)
	if _, err = s.Start(v.ID); err == nil {
		t.Fatal("WeChat must be rejected")
	}
	a.Platform = "dy"
	db.Save(&a)
	v.URL = "https://example.com/video/" + v.PlatformID
	db.Save(&v)
	if _, err = s.Start(v.ID); err == nil {
		t.Fatal("foreign URL must be rejected")
	}
}

func TestWechatDispatchUsesLocalIDPath(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "wx.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	a := dao.BenchmarkAccount{Platform: "wx", PlatformAccountID: "target"}
	db.Create(&a)
	v := dao.Video{AccountID: a.ID, PlatformID: "export/opaque", Nonce: "nonce"}
	db.Create(&v)
	root := filepath.Join(t.TempDir(), "douyin")
	var received string
	s := &Service{DB: db, Ctx: context.Background(), Root: root, WechatRun: func(_ context.Context, _ dao.Video, p string) error { received = p; return errors.New("no session") }}
	if _, err = s.Start(v.ID); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if filepath.Dir(received) != filepath.Join(filepath.Dir(root), "wechat") || filepath.Base(received) != fmt.Sprintf("%d-%x.mp4", v.ID, sha256.Sum256([]byte(v.PlatformID))) {
		t.Fatal(received)
	}
	j, err := s.Get(v.ID)
	if err != nil || j.Status != "error" || j.Error != "no session" {
		t.Fatal(j, err)
	}
}
