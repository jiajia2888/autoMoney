package web

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"automoney/base"
	"automoney/dao"
	"automoney/service/monitor"
	"automoney/service/wechat"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, base.APIResponse{Code: 0, Message: "ok", Data: data})
}
func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, base.APIResponse{Code: status, Message: msg})
}
func id(c *gin.Context) (uint64, bool) {
	n, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || n == 0 {
		fail(c, 400, "无效 ID")
		return 0, false
	}
	return n, true
}

func NewRouter(db *gorm.DB, pagesDir string, monitors ...*monitor.Monitor) *gin.Engine {
	var m *monitor.Monitor
	if len(monitors) > 0 {
		m = monitors[0]
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	// Local personal app: mutations from other origins are rejected; only loopback listening is allowed by main.
	r.Use(func(c *gin.Context) {
		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "localhost" {
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				fail(c, 403, "本地版仅接受本机访问")
				c.Abort()
				return
			}
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" {
			if origin := c.GetHeader("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != c.Request.Host {
					fail(c, 403, "请求来源不匹配")
					c.Abort()
					return
				}
			}
			if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
				fail(c, 403, "不接受跨站写入")
				c.Abort()
				return
			}
		}
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) { ok(c, gin.H{"service": "autoMoney"}) })
	r.GET("/api/v1/status", func(c *gin.Context) {
		busy := false
		if m != nil {
			busy = m.Busy()
		}
		ok(c, gin.H{"database_configured": db != nil, "benchmark_crawler": "python_expert_adapter", "busy": busy, "local_only": true})
	})
	r.StaticFile("/", filepath.Join(pagesDir, "index.html"))
	r.Static("/assets", filepath.Join(pagesDir, "assets"))
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		if db == nil || m == nil {
			fail(c, 503, "监控服务未初始化")
			c.Abort()
			return
		}
		c.Next()
	})
	api.GET("/accounts", func(c *gin.Context) {
		var rows []dao.BenchmarkAccount
		if err := platformQuery(db.Model(&dao.BenchmarkAccount{}), c, true).Order("id ASC").Find(&rows).Error; err != nil {
			fail(c, 500, "读取账号失败")
			return
		}
		ok(c, rows)
	})
	api.POST("/accounts", func(c *gin.Context) {
		var in monitor.AccountInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, 400, "无效 JSON")
			return
		}
		if err := in.Validate(); err != nil {
			fail(c, 400, err.Error())
			return
		}
		platform := c.DefaultQuery("platform", "dy")
		var sec string
		var err error
		if platform == "wx" {
			sec = strings.TrimSpace(in.Input)
			if !wechat.ValidFinderID(sec) {
				err = errors.New("请先搜索并选择视频号作者")
			}
		} else if platform == "dy" {
			sec, err = monitor.Normalize(in.Input)
		} else {
			err = errors.New("无效平台")
		}
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		a := dao.BenchmarkAccount{Platform: platform, PlatformAccountID: sec, HomepageURL: "https://www.douyin.com/user/" + sec, DisplayName: strings.TrimSpace(in.DisplayName), Enabled: in.Enabled, IntervalMinutes: in.IntervalMinutes, Threshold: in.Threshold, MaxItems: in.MaxItems, NotifyNew: in.NotifyNew, NotifyThreshold: in.NotifyThreshold, NextCheckAt: time.Now().UTC(), LastStatus: "pending"}
		if platform == "wx" {
			a.HomepageURL = ""
			a.Threshold = 10000
		}
		if sec == monitor.KnownID {
			a.DouyinNumber = "80185543890"
		}
		var existing dao.BenchmarkAccount
		if db.Where("platform_account_id = ?", sec).First(&existing).Error == nil {
			fail(c, 409, "该账号已在监控列表中")
			return
		}
		if err := db.Create(&a).Error; err != nil {
			fail(c, 500, "保存账号失败")
			return
		}
		ok(c, a)
	})
	api.PATCH("/accounts/:id", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		var a dao.BenchmarkAccount
		if db.First(&a, n).Error != nil {
			fail(c, 404, "账号不存在")
			return
		}
		var in monitor.AccountInput
		if c.ShouldBindJSON(&in) != nil {
			fail(c, 400, "无效 JSON")
			return
		}
		if err := in.Validate(); err != nil {
			fail(c, 400, err.Error())
			return
		}
		a.DisplayName = strings.TrimSpace(in.DisplayName)
		a.Enabled = in.Enabled
		a.IntervalMinutes = in.IntervalMinutes
		a.Threshold = in.Threshold
		if a.Platform == "wx" {
			a.Threshold = 10000
		}
		a.MaxItems = in.MaxItems
		a.NotifyNew = in.NotifyNew
		a.NotifyThreshold = in.NotifyThreshold
		a.NextCheckAt = time.Now().UTC().Add(time.Duration(a.IntervalMinutes) * time.Minute)
		if err := db.Save(&a).Error; err != nil {
			fail(c, 500, "保存规则失败")
			return
		}
		ok(c, a)
	})
	api.DELETE("/accounts/:id", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		if m.Busy() {
			fail(c, 409, "任务执行中，请结束后再删除账号")
			return
		}
		var extracting int64
		if db.Model(&dao.VideoTranscript{}).Where("account_id = ? AND status IN ?", n, []string{"queued", "running"}).Count(&extracting).Error != nil || extracting > 0 {
			fail(c, 409, "该账号有文案提取任务，请结束后再删除")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			var a dao.BenchmarkAccount
			if err := tx.First(&a, n).Error; err != nil {
				return err
			}
			for _, model := range []any{&dao.Video{}, &dao.Notification{}, &dao.CrawlRun{}, &dao.VideoTranscript{}} {
				if err := tx.Where("account_id = ?", n).Delete(model).Error; err != nil {
					return err
				}
			}
			return tx.Delete(&a).Error
		})
		if err != nil {
			fail(c, 404, "删除失败或账号不存在")
			return
		}
		ok(c, gin.H{"deleted": true})
	})
	api.POST("/accounts/:id/check", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		run, err := m.Enqueue(n, "collect")
		if err != nil {
			status := 409
			if errors.Is(err, gorm.ErrRecordNotFound) {
				status = 404
			}
			fail(c, status, err.Error())
			return
		}
		ok(c, run)
	})
	api.POST("/collector/login", func(c *gin.Context) {
		run, err := m.Enqueue(0, "login")
		if err != nil {
			fail(c, 409, err.Error())
			return
		}
		ok(c, run)
	})
	api.GET("/videos", func(c *gin.Context) {
		q := platformQuery(db.Model(&dao.Video{}), c, false)
		if v := c.Query("account_id"); v != "" {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				fail(c, 400, "账号筛选无效")
				return
			}
			q = q.Where("account_id = ?", n)
		}
		if v := c.Query("min_likes"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				fail(c, 400, "点赞条件无效")
				return
			}
			q = q.Where("likes > ?", n)
		}
		if c.Query("exact_only") == "true" {
			q = q.Where("likes_exact = ?", true)
		}
		if c.Query("over_10k") == "true" {
			q = q.Where("(first_over_10k_at IS NOT NULL OR likes > ?)", 10000)
		}
		shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
		var dateFrom, dateTo time.Time
		for _, filter := range []struct{ key, operator string }{{"over_date_from", ">="}, {"over_date_to", "<"}} {
			if value := c.Query(filter.key); value != "" {
				d, err := time.ParseInLocation("2006-01-02", value, shanghai)
				if err != nil {
					fail(c, 400, "发现日期格式应为 YYYY-MM-DD")
					return
				}
				if filter.key == "over_date_from" {
					dateFrom = d
				} else {
					dateTo = d
					d = d.AddDate(0, 0, 1)
				}
				q = q.Where("first_over_10k_at "+filter.operator+" ?", d.UTC())
			}
		}
		if !dateFrom.IsZero() && !dateTo.IsZero() && dateFrom.After(dateTo) {
			fail(c, 400, "开始日期不能晚于结束日期")
			return
		}
		if c.Query("cohort") == "new" {
			q = q.Where("published_at >= (SELECT baseline_at FROM benchmark_accounts WHERE benchmark_accounts.id = videos.account_id)")
		}
		if c.Query("cohort") == "historical" {
			q = q.Where("published_at < (SELECT baseline_at FROM benchmark_accounts WHERE benchmark_accounts.id = videos.account_id)")
		}
		for _, filter := range []struct{ key, op string }{{"published_from", ">="}, {"published_to", "<"}} {
			if value := c.Query(filter.key); value != "" {
				d, err := time.ParseInLocation("2006-01-02", value, shanghai)
				if err != nil {
					fail(c, 400, "发布日期格式应为 YYYY-MM-DD")
					return
				}
				if filter.key == "published_to" {
					d = d.AddDate(0, 0, 1)
				}
				q = q.Where("published_at "+filter.op+" ?", d.UTC())
			}
		}
		if v := c.Query("q"); v != "" {
			q = q.Where("title LIKE ?", "%"+v+"%")
		}
		var total int64
		if q.Count(&total).Error != nil {
			fail(c, 500, "统计视频失败")
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		if page > 100000 {
			page = 100000
		}
		order := "likes DESC, id DESC"
		if c.Query("sort") == "newest" {
			order = "published_at DESC, id DESC"
		}
		if c.Query("sort") == "seen" {
			order = "first_seen_at DESC, id DESC"
		}
		if c.Query("sort") == "over_date" {
			order = "first_over_10k_at IS NULL ASC, first_over_10k_at DESC, id DESC"
		}
		var rows []dao.Video
		query := q.Order(order)
		showAll := c.Query("all") == "true"
		if showAll {
			page = 1
		} else {
			query = query.Limit(24).Offset((page - 1) * 24)
		}
		if query.Find(&rows).Error != nil {
			fail(c, 500, "读取视频失败")
			return
		}
		pageSize := 24
		if showAll {
			pageSize = len(rows)
		}
		ok(c, gin.H{"items": rows, "total": total, "page": page, "page_size": pageSize, "all": showAll})
	})
	api.GET("/notifications", func(c *gin.Context) {
		var rows []dao.Notification
		q := platformQuery(db.Model(&dao.Notification{}), c, false).Where("kind <> ?", "baseline")
		if c.Query("unread") == "true" {
			q = q.Where("read = ?", false)
		}
		if q.Order("id DESC").Limit(100).Find(&rows).Error != nil {
			fail(c, 500, "读取提醒失败")
			return
		}
		ok(c, rows)
	})
	api.POST("/notifications/read", func(c *gin.Context) {
		if platformQuery(db.Model(&dao.Notification{}), c, false).Where("read = ?", false).Update("read", true).Error != nil {
			fail(c, 500, "更新失败")
			return
		}
		ok(c, gin.H{"read": true})
	})
	api.POST("/runs/:id/cancel", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		if err := m.Cancel(n); err != nil {
			fail(c, 409, err.Error())
			return
		}
		ok(c, gin.H{"cancel_requested": true})
	})

	api.GET("/runs", func(c *gin.Context) {
		var rows []dao.CrawlRun
		if platformQuery(db.Model(&dao.CrawlRun{}), c, false).Order("id DESC").Limit(100).Find(&rows).Error != nil {
			fail(c, 500, "读取任务失败")
			return
		}
		ok(c, rows)
	})
	api.GET("/overview", func(c *gin.Context) {
		counts := map[string]int64{}
		for name, model := range map[string]any{"accounts": &dao.BenchmarkAccount{}, "videos": &dao.Video{}, "alerts": &dao.Notification{}} {
			q := platformQuery(db.Model(model), c, name == "accounts")
			if name == "alerts" {
				q = q.Where("kind <> ? AND read = ?", "baseline", false)
			}
			var n int64
			if q.Count(&n).Error != nil {
				fail(c, 500, "统计失败")
				return
			}
			counts[name] = n
		}
		var enabled int64
		platformQuery(db.Model(&dao.BenchmarkAccount{}), c, true).Where("enabled = ?", true).Count(&enabled)
		counts["enabled"] = enabled
		var hits int64
		platformQuery(db.Model(&dao.Video{}), c, false).Where("likes > ?", 10000).Count(&hits)
		counts["hits"] = hits
		ok(c, counts)
	})
	r.NoRoute(func(c *gin.Context) { fail(c, 404, "route not found") })
	return r
}

// Optional platform filters preserve older clients while preventing mixed UI data.
func platformQuery(q *gorm.DB, c *gin.Context, account bool) *gorm.DB {
	p := c.Query("platform")
	if p == "" {
		return q
	}
	if account {
		return q.Where("platform = ?", p)
	}
	return q.Where("account_id IN (?)", q.Session(&gorm.Session{NewDB: true}).Model(&dao.BenchmarkAccount{}).Select("id").Where("platform = ?", p))
}
