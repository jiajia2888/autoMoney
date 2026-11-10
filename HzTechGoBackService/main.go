package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"automoney/configs"
	"automoney/dao"
	"automoney/service/benchmark"
	"automoney/service/download"
	"automoney/service/frames"
	"automoney/service/monitor"
	"automoney/service/transcript"
	"automoney/service/wechat"
	"automoney/service/yuanbao"
	"automoney/web"
)

func main() {
	migrate := flag.Bool("migrate", false, "显式初始化/更新数据库结构后退出")
	seed := flag.Bool("seed-samples", false, "导入已验证的徐晶说页面样本后退出（非实时数据）")
	flag.Parse()
	cfg, err := configs.Load(os.Getenv("AUTOMONEY_CONFIG"))
	if err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		log.Fatal("address must include loopback host and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("本地版只允许监听 127.0.0.1 或 ::1")
	}
	db, err := dao.OpenLocal(cfg.MySQLDSN, cfg.SQLitePath)
	if err != nil {
		log.Fatal("database initialization failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	if *migrate {
		if err := dao.Migrate(db); err != nil {
			log.Fatal(err)
		}
		log.Print("数据库迁移完成")
		return
	}
	if err := dao.CheckSchema(db); err != nil {
		log.Fatal(err)
	}
	if *seed {
		if err := monitor.SeedSamples(db, "../docs/samples/xujingshuo-browser-top20.json"); err != nil {
			log.Fatal(err)
		}
		log.Print("已导入 20 条浏览器历史样本，自动监控默认关闭")
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	wxLogin := wechat.NewLoginWithDNS(cfg.WechatSessionFile, cfg.WechatDNSServer)
	m := monitor.New(db, benchmark.PlatformCrawler{Douyin: benchmark.PythonCrawler{Python: cfg.Python, Script: cfg.CollectorScript, Profile: cfg.ProfileDir, CredentialsFile: cfg.CredentialsFile}, Wechat: wechat.Crawler{LoginService: wxLogin}})
	m.Start(ctx)
	router := web.NewRouter(db, cfg.PagesDir, m)
	web.RegisterWechatRoutes(router, wxLogin)
	web.RegisterWechatCollectionRoutes(router, db, wxLogin)
	web.RegisterConnectionRoutes(router, benchmark.CredentialStore{Path: cfg.CredentialsFile})
	transcripts := transcript.New(db, transcript.PythonExtractor{Python: cfg.Python, Script: cfg.TranscriptScript, Model: cfg.WhisperModel, CredentialsFile: cfg.CredentialsFile, ProfileDir: cfg.ProfileDir})
	transcripts.Start(ctx)
	defer transcripts.Wait()
	web.RegisterTranscriptRoutes(router, transcripts)
	downloads := &download.Service{DB: db, Ctx: ctx, Python: cfg.Python, Script: filepath.Join(filepath.Dir(cfg.CollectorScript), "download_douyin.py"), Root: filepath.Join(filepath.Dir(cfg.SQLitePath), "downloads", "douyin"), Credentials: cfg.CredentialsFile, Profile: cfg.ProfileDir}
	defer downloads.Wait()
	yb := &yuanbao.Service{Ctx: ctx, Python: cfg.Python, Script: filepath.Join(filepath.Dir(cfg.CollectorScript), "yuanbao_login.py"), Credentials: filepath.Join(filepath.Dir(cfg.SQLitePath), "yuanbao-credentials.json"), DNS: cfg.WechatDNSServer}
	defer yb.Wait()
	web.RegisterYuanbaoRoutes(router, yb)
	downloads.WechatRun = func(ctx context.Context, v dao.Video, path string) error {
		if !yb.Status().Configured {
			return yuanbao.ErrLogin
		}
		share, err := wxLogin.Share(ctx, v.PlatformID, v.Nonce)
		if err != nil {
			return err
		}
		var account dao.BenchmarkAccount
		if v.PublishedAt == nil || db.First(&account, v.AccountID).Error != nil {
			return errors.New("作品缺少作者或发布时间，无法核验下载身份")
		}
		return yb.Download(ctx, share, yuanbao.SourceVideo{ExportID: v.PlatformID, Title: v.Title, Author: account.DisplayName, PublishedAt: v.PublishedAt.Unix()}, path)
	}
	transcripts.Prepare = func(ctx context.Context, v dao.Video) (string, error) { return downloads.Ensure(ctx, v.ID) }
	web.RegisterDownloadRoutes(router, downloads)
	frameService := &frames.Service{Ctx: ctx, Root: filepath.Join(filepath.Dir(cfg.SQLitePath), "frames"), Prepare: downloads.Ensure}
	defer frameService.Wait()
	web.RegisterFrameRoutes(router, db, frameService)
	srv := &http.Server{Addr: cfg.Address, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("autoMoney: http://%s", cfg.Address)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		log.Print(err)
	}
}
