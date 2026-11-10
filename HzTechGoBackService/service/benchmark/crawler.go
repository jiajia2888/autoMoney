package benchmark

import (
	"context"
	"fmt"
	"time"
)

type Request struct {
	Platform    string
	AccountID   string
	HomepageURL string
	Cursor      string
	EndTs       int64
	MaxItems    int
}
type Post struct {
	PlatformID  string     `json:"platform_id"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	CoverURL    string     `json:"cover_url"`
	Hearts      *int64     `json:"hearts"`
	Nonce       string     `json:"nonce"`
	Likes       int64      `json:"likes"`
	Comments    int64      `json:"comments"`
	Favorites   int64      `json:"favorites"`
	Shares      int64      `json:"shares"`
	PublishedAt *time.Time `json:"published_at"`
}
type Result struct {
	Backfill   bool       `json:"-"`
	RangeEnd   *time.Time `json:"-"`
	NextCursor string     `json:"-"`
	Posts      []Post     `json:"posts"`
	Complete   bool       `json:"complete"`
	StopReason string     `json:"stop_reason"`
}
type Crawler interface {
	FetchPosts(context.Context, Request) (Result, error)
	Login(context.Context) error
}

// PlatformCrawler keeps login and collection isolated while sharing one queue.
type PlatformCrawler struct{ Douyin, Wechat Crawler }

func (p PlatformCrawler) FetchPosts(ctx context.Context, r Request) (Result, error) {
	if r.Platform == "wx" {
		return p.Wechat.FetchPosts(ctx, r)
	}
	if r.Platform != "" && r.Platform != "dy" {
		return Result{}, fmt.Errorf("不支持的监控平台")
	}
	return p.Douyin.FetchPosts(ctx, r)
}
func (p PlatformCrawler) Login(ctx context.Context) error { return p.Douyin.Login(ctx) }
