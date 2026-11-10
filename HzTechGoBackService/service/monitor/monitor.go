package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"automoney/dao"
	"automoney/service/benchmark"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrBusy = errors.New("已有任务执行或排队，请等待当前任务结束")
var validID = regexp.MustCompile(`^MS4wLj[A-Za-z0-9_-]{20,180}$`)

const KnownID = "MS4wLjABAAAAovmrcujSuO71ql7DfF0X6PymlOfV5komvBZscsJ75H-TaRccS6pAudUnWcYAuX0E"

func Normalize(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "80185543890" {
		return KnownID, nil
	}
	if validID.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(input)
	if err != nil || u.Scheme != "https" || (u.Host != "www.douyin.com" && u.Host != "m.douyin.com") {
		return "", errors.New("请粘贴抖音完整主页链接；普通抖音号暂仅支持已核验的 80185543890")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "user" && validID.MatchString(parts[len(parts)-1]) {
		return parts[len(parts)-1], nil
	}
	return "", errors.New("无法识别主页，请使用 https://www.douyin.com/user/… 链接")
}

type AccountInput struct {
	Input           string `json:"input"`
	DisplayName     string `json:"display_name"`
	IntervalMinutes int    `json:"interval_minutes"`
	Threshold       int64  `json:"threshold"`
	MaxItems        int    `json:"max_items"`
	Enabled         bool   `json:"enabled"`
	NotifyNew       bool   `json:"notify_new"`
	NotifyThreshold bool   `json:"notify_threshold"`
}

func (v AccountInput) Validate() error {
	if v.IntervalMinutes < 5 || v.IntervalMinutes > 10080 {
		return errors.New("检查间隔需为 5 至 10080 分钟")
	}
	if v.Threshold < 0 || v.Threshold > 1000000000 {
		return errors.New("点赞阈值需为 0 至 10 亿")
	}
	if v.MaxItems < 1 || v.MaxItems > 500 {
		return errors.New("每次扫描上限需为 1 至 500 条")
	}
	if len([]rune(strings.TrimSpace(v.DisplayName))) < 1 || len([]rune(strings.TrimSpace(v.DisplayName))) > 80 {
		return errors.New("账号名称需为 1 至 80 个字")
	}
	return nil
}

type work struct {
	RunID, AccountID uint64
	Kind             string
}
type Monitor struct {
	DB            *gorm.DB
	Crawler       benchmark.Crawler
	mu            sync.Mutex
	busy          bool
	activeRunID   uint64
	currentCancel context.CancelFunc
	cancelPending bool
	queue         chan work
}

func New(db *gorm.DB, c benchmark.Crawler) *Monitor {
	return &Monitor{DB: db, Crawler: c, queue: make(chan work, 1)}
}

func (m *Monitor) Start(ctx context.Context) {
	// Interrupted runs are terminal and retry on the next due tick.
	now := time.Now().UTC()
	m.DB.Model(&dao.CrawlRun{}).Where("status IN ?", []string{"queued", "running"}).Updates(map[string]any{"status": "interrupted", "message": "服务重启，任务中断；等待下一轮检查", "finished_at": now})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case w := <-m.queue:
				if ctx.Err() != nil {
					m.release()
					return
				}
				m.execute(ctx, w)
				m.release()
			case <-ticker.C:
				var a dao.BenchmarkAccount
				err := m.DB.Where("enabled = ? AND next_check_at <= ?", true, time.Now().UTC()).Order("next_check_at ASC").First(&a).Error
				if err == nil {
					_, _ = m.Enqueue(a.ID, "collect")
				}
			}
		}
	}()
}
func (m *Monitor) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.busy = false
	m.currentCancel = nil
	m.activeRunID = 0
	m.cancelPending = false
}
func (m *Monitor) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.busy }
func (m *Monitor) Enqueue(accountID uint64, kind string) (dao.CrawlRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return dao.CrawlRun{}, ErrBusy
	}
	if kind == "collect" {
		var a dao.BenchmarkAccount
		if err := m.DB.First(&a, accountID).Error; err != nil {
			return dao.CrawlRun{}, err
		}
	}
	run := dao.CrawlRun{AccountID: accountID, Kind: kind, Status: "queued", Message: "等待执行"}
	if err := m.DB.Create(&run).Error; err != nil {
		return run, err
	}
	m.busy = true
	m.activeRunID = run.ID
	m.cancelPending = false
	m.queue <- work{run.ID, accountID, kind}
	return run, nil
}
func (m *Monitor) Cancel(runID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.busy || m.activeRunID != runID {
		return errors.New("任务已经结束或不存在")
	}
	m.cancelPending = true
	if m.currentCancel != nil {
		m.currentCancel()
	}
	return nil
}

func (m *Monitor) execute(parent context.Context, w work) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
	defer cancel()
	m.mu.Lock()
	m.currentCancel = cancel
	if m.cancelPending {
		cancel()
	}
	m.mu.Unlock()
	m.DB.Model(&dao.CrawlRun{}).Where("id = ?", w.RunID).Updates(map[string]any{"status": "running", "message": "正在检查；如需扫码/验证，请查看专用浏览器窗口"})
	if w.Kind == "login" {
		err := m.Crawler.Login(ctx)
		m.finish(w.RunID, err)
		return
	}
	var a dao.BenchmarkAccount
	if err := m.DB.First(&a, w.AccountID).Error; err != nil {
		m.finish(w.RunID, err)
		return
	}
	request := benchmark.Request{Platform: a.Platform, AccountID: a.PlatformAccountID, HomepageURL: a.HomepageURL, MaxItems: a.MaxItems}
	if a.Platform == "wx" && !a.HistoryComplete {
		request.Cursor = a.HistoryCursor
		if a.HistoryEnd != nil {
			request.EndTs = a.HistoryEnd.Unix()
		}
	}
	result, err := m.Crawler.FetchPosts(ctx, request)
	result.Backfill = a.Platform == "wx" && !a.HistoryComplete
	if err == nil {
		err = m.Apply(a.ID, w.RunID, result, time.Now().UTC())
	}
	if err != nil {
		now := time.Now().UTC()
		m.DB.Model(&dao.BenchmarkAccount{}).Where("id = ?", a.ID).Updates(map[string]any{"last_status": "error", "last_error": err.Error(), "last_checked_at": now, "next_check_at": now.Add(time.Duration(a.IntervalMinutes) * time.Minute)})
		m.finish(w.RunID, err)
	}
}
func (m *Monitor) finish(runID uint64, err error) {
	now := time.Now().UTC()
	status, msg := "success", "登录已完成"
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	m.DB.Model(&dao.CrawlRun{}).Where("id = ?", runID).Updates(map[string]any{"status": status, "message": msg, "finished_at": now})
}

// Apply atomically updates videos and creates deduplicated alerts. Imported display-only samples never reach this method.
func (m *Monitor) Apply(accountID, runID uint64, result benchmark.Result, now time.Time) error {
	if len(result.Posts) == 0 {
		return errors.New("空响应未写入：无法确认是否为正常无作品")
	}
	return m.DB.Transaction(func(tx *gorm.DB) error {
		var a dao.BenchmarkAccount
		if err := tx.First(&a, accountID).Error; err != nil {
			return err
		}
		first := a.BaselineAt == nil
		baseline := first || result.Backfill
		count, alerts := 0, 0
		seen := map[string]bool{}
		for _, p := range result.Posts {
			if seen[p.PlatformID] {
				continue
			}
			seen[p.PlatformID] = true
			if p.PlatformID == "" || p.Likes < 0 {
				return errors.New("作品缺少有效 ID 或点赞数")
			}
			var v dao.Video
			err := tx.Where("account_id = ? AND platform_id = ?", a.ID, p.PlatformID).First(&v).Error
			fresh := errors.Is(err, gorm.ErrRecordNotFound)
			if err != nil && !fresh {
				return err
			}
			if fresh {
				v = dao.Video{AccountID: a.ID, PlatformID: p.PlatformID, FirstSeenAt: now}
				count++
			}
			if p.Likes > 10000 && v.FirstOver10kAt == nil {
				v.FirstOver10kAt = &now
				// A previous exact observation at <=10000 brackets a crossing.
				// Otherwise the actual crossing predates our first observation.
				v.AlreadyOver10k = fresh || !v.LikesExact || v.Likes > 10000
			}
			v.Title = p.Title
			v.URL = p.URL
			v.CoverURL = p.CoverURL
			v.Likes = p.Likes
			v.LikesExact = true
			v.LikesDisplay = ""
			v.Comments = p.Comments
			v.Favorites = p.Favorites
			v.Shares = p.Shares
			v.PublishedAt = p.PublishedAt
			v.LastSeenAt = now
			v.Source = "douyin_api"
			if a.Platform == "wx" {
				v.Source = "wechat_promote_web"
			}
			v.Hearts = p.Hearts
			v.Nonce = p.Nonce
			if err := tx.Save(&v).Error; err != nil {
				return err
			}
			send := func(kind, title, key string) error {
				n := dao.Notification{EventKey: key, AccountID: a.ID, VideoID: v.ID, Kind: kind, Title: title, URL: v.URL, CreatedAt: now}
				res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&n)
				if res.Error == nil {
					alerts += int(res.RowsAffected)
				}
				return res.Error
			}
			// First import is a baseline; older videos discovered later are not reported as newly published.
			if !baseline && fresh && a.NotifyNew && p.PublishedAt != nil && !p.PublishedAt.Before(*a.BaselineAt) {
				if err := send("new", a.DisplayName+" 发布了新视频："+p.Title, fmt.Sprintf("new:%d:%s", a.ID, p.PlatformID)); err != nil {
					return err
				}
			}
			if !baseline && a.NotifyThreshold && p.Likes > a.Threshold {
				if err := send("threshold", fmt.Sprintf("%s 视频点赞超过 %d：%s", a.DisplayName, a.Threshold, p.Title), fmt.Sprintf("threshold:%d:%s:%d", a.ID, p.PlatformID, a.Threshold)); err != nil {
					return err
				}
			}
			// Remember baseline hits without notifying, so later refreshes don't announce existing hits.
			if baseline && p.Likes > a.Threshold {
				key := fmt.Sprintf("threshold:%d:%s:%d", a.ID, p.PlatformID, a.Threshold)
				n := dao.Notification{EventKey: key, AccountID: a.ID, VideoID: v.ID, Kind: "baseline", Read: true, Title: "历史基线", URL: v.URL, CreatedAt: now}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{"last_status": "success", "last_error": "", "last_checked_at": now, "next_check_at": now.Add(time.Duration(a.IntervalMinutes) * time.Minute)}
		if first {
			updates["baseline_at"] = now
		}
		if result.Backfill {
			updates["history_cursor"] = result.NextCursor
			updates["history_end"] = result.RangeEnd
			updates["history_complete"] = result.Complete
			if !result.Complete {
				if result.NextCursor == "" {
					return errors.New("历史扫描未完成且缺少续页游标")
				}
				updates["next_check_at"] = now.Add(30 * time.Second)
			}
		}
		if err := tx.Model(&a).Updates(updates).Error; err != nil {
			return err
		}
		message := fmt.Sprintf("已检查 %d 条视频，新增收录 %d 条，提醒 %d 条", len(seen), count, alerts)
		if baseline {
			message += "；历史基线批次，不推送历史视频"
			if result.Backfill && !result.Complete {
				message += "；30 秒后继续扫描更早的历史作品"
			}
		}
		if !result.Complete {
			message += "；仅覆盖本次扫描范围"
		}
		return tx.Model(&dao.CrawlRun{}).Where("id = ?", runID).Updates(map[string]any{"status": "success", "scanned": len(seen), "new_count": count, "alert_count": alerts, "complete": result.Complete, "stop_reason": result.StopReason, "message": message, "finished_at": now}).Error
	})
}
