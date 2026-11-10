package wechat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"automoney/service/benchmark"
)

const createPage = origin + "/promote/pages/platform/create-order"

var finderID = regexp.MustCompile(`^[A-Za-z0-9_@.-]{5,191}$`)

func ValidFinderID(id string) bool {
	return finderID.MatchString(id) && strings.HasSuffix(id, "@finder")
}

type Author struct {
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Signature string `json:"signature"`
}
type Authors struct {
	Items []Author `json:"items"`
	Total int64    `json:"total"`
	Page  int      `json:"page"`
}

func (l *Login) form(ctx context.Context, path string, values url.Values) (map[string]any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	saved, err := l.store.load()
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, errors.New("尚未连接视频号，请先扫码")
	}
	c := l.factory()
	c.setSession(saved.Session)
	data, err := c.form(ctx, path, createPage, values)
	if errors.Is(err, ErrExpired) {
		if e := l.store.clear(); e != nil {
			return nil, e
		}
	}
	return data, err
}
func (l *Login) Search(ctx context.Context, query string, page int) (Authors, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 1 || len([]rune(query)) > 80 || page < 1 || page > 50 {
		return Authors{}, errors.New("请输入 1 至 80 字的作者昵称，页码为 1 至 50")
	}
	data, err := l.form(ctx, "transfer/MmFinderPromotionApiSvr/searchAccount", url.Values{"query": {query}, "flag": {"1"}, "currentPage": {strconv.Itoa(page)}, "pageSize": {"5"}})
	if err != nil {
		return Authors{}, err
	}
	list, ok := data["list"].([]any)
	if !ok {
		return Authors{}, errors.New("微信作者响应缺少列表")
	}
	result := Authors{Items: []Author{}, Page: page}
	total, ok := integer(data["totalCount"])
	if !ok {
		return result, errors.New("微信作者响应缺少总数")
	}
	result.Total = total
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return Authors{}, errors.New("微信作者结构异常")
		}
		a := Author{Username: str(m, "username"), Nickname: str(m, "nickName"), Avatar: str(m, "headImgUrl"), Signature: str(m, "signature")}
		if !ValidFinderID(a.Username) || a.Nickname == "" {
			return Authors{}, errors.New("微信作者缺少有效身份或昵称")
		}
		result.Items = append(result.Items, a)
	}
	return result, nil
}
func str(m map[string]any, key string) string { v, _ := m[key].(string); return v }
func integer(v any) (int64, bool) {
	n, ok := v.(float64)
	if !ok || n < 0 || n > 9007199254740991 || n != float64(int64(n)) {
		return 0, false
	}
	return int64(n), true
}

type Crawler struct {
	LoginService *Login
	PageDelay    time.Duration
}

func (c Crawler) Login(context.Context) error {
	return errors.New("视频号请使用微信扫码登录入口")
}
func (c Crawler) FetchPosts(ctx context.Context, r benchmark.Request) (benchmark.Result, error) {
	result := benchmark.Result{Posts: []benchmark.Post{}}
	if !ValidFinderID(r.AccountID) || r.MaxItems < 1 || r.MaxItems > 500 {
		return result, errors.New("视频号作者或扫描上限无效")
	}
	cursor := r.Cursor
	seenCursor := map[string]bool{}
	seen := map[string]bool{}
	endTs := r.EndTs
	if endTs <= 0 {
		endTs = time.Now().Unix()
	}
	end := strconv.FormatInt(endTs, 10)
	endTime := time.Unix(endTs, 0).UTC()
	result.RangeEnd = &endTime
	for page := 0; page < 100; page++ {
		if err := ctx.Err(); err != nil {
			return benchmark.Result{}, errors.New("视频号采集已取消或超时")
		}
		limit := 5
		if remaining := r.MaxItems - len(result.Posts); remaining < limit {
			limit = remaining
		}
		values := url.Values{"encryptedUsername": {r.AccountID}, "startTs": {"0"}, "endTs": {end}, "limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			values.Set("lastBuf", cursor)
		}
		data, err := c.LoginService.form(ctx, "transfer/MmFinderPromotionApiSvr/getFinderObject", values)
		if err != nil {
			return benchmark.Result{}, err
		}
		items, ok := data["objects"].([]any)
		if !ok || len(items) == 0 {
			return benchmark.Result{}, errors.New("微信未返回作品，无法确认账号无作品；请核验会话和作者")
		}
		if len(items) > limit {
			return benchmark.Result{}, errors.New("微信返回作品超过请求页上限，已停止避免漏记")
		}
		more, ok := data["continueFlag"].(bool)
		if !ok {
			return benchmark.Result{}, errors.New("微信作品响应缺少有效分页状态")
		}
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				return benchmark.Result{}, errors.New("微信作品结构异常")
			}
			p, err := normalizePost(m, r.AccountID)
			if err != nil {
				return benchmark.Result{}, err
			}
			if seen[p.PlatformID] {
				continue
			}
			seen[p.PlatformID] = true
			if len(result.Posts) < r.MaxItems {
				result.Posts = append(result.Posts, p)
			}
		}
		if !more {
			result.Complete = true
			return result, nil
		}
		next := str(data, "lastBuf")
		if next == "" || next == cursor || seenCursor[next] {
			return benchmark.Result{}, errors.New("微信作品分页游标缺失或重复，已停止本轮")
		}
		seenCursor[next] = true
		cursor = next
		result.NextCursor = next
		if len(result.Posts) >= r.MaxItems {
			result.StopReason = "item_limit"
			return result, nil
		}
		delay := c.PageDelay
		if delay <= 0 {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return benchmark.Result{}, errors.New("视频号采集已取消或超时")
		case <-timer.C:
		}
	}
	result.StopReason = "page_limit"
	return result, nil
}
func normalizePost(m map[string]any, target string) (benchmark.Post, error) {
	p := benchmark.Post{PlatformID: str(m, "exportId"), Title: str(m, "description"), CoverURL: str(m, "coverUrl"), Nonce: str(m, "nonce")}
	if info, ok := m["accountInfo"].(map[string]any); ok {
		if id := str(info, "username"); id != "" && id != target {
			return p, errors.New("微信返回了其他作者的作品，已停止")
		}
	}
	if p.PlatformID == "" || len(p.PlatformID) > 191 {
		return p, errors.New("微信作品缺少有效标识")
	}
	thumbs, ok := integer(m["favCount"])
	if !ok {
		return p, errors.New("微信作品缺少有效大拇指数，未将缺失值当作零")
	}
	p.Likes = thumbs
	hearts, ok := integer(m["likeCount"])
	if !ok {
		return p, errors.New("微信作品缺少有效爱心数")
	}
	p.Hearts = &hearts
	if p.Comments, ok = integer(m["commentCount"]); !ok {
		return p, errors.New("微信作品缺少评论数")
	}
	if p.Shares, ok = integer(m["forwardCount"]); !ok {
		return p, errors.New("微信作品缺少转发数")
	}
	ts, ok := integer(m["createTime"])
	if !ok || ts == 0 || ts > time.Now().Add(24*time.Hour).Unix() {
		return p, errors.New("微信作品发布时间无效")
	}
	t := time.Unix(ts, 0).UTC()
	p.PublishedAt = &t
	// Share links are fetched on demand; no cover or identifier is a video URL.
	p.URL = ""
	if p.Title == "" {
		p.Title = fmt.Sprintf("视频号作品 %s", p.PlatformID)
	}
	return p, nil
}
func (l *Login) Share(ctx context.Context, exportID, nonce string) (string, error) {
	if exportID == "" || nonce == "" {
		return "", errors.New("作品缺少分享标识")
	}
	data, err := l.form(ctx, "transfer/MmFinderAssistantBroker/getObjectShortLink", url.Values{"exportId": {exportID}, "nonceId": {nonce}})
	if err != nil {
		return "", err
	}
	link := str(data, "shortUrl")
	u, e := url.Parse(link)
	if e != nil || u.Scheme != "https" || u.Host != "weixin.qq.com" || !strings.HasPrefix(u.Path, "/sph/") {
		return "", errors.New("微信未返回有效分享链接")
	}
	return link, nil
}

// ExpandShare resolves only the official public short link, without login cookies
// or automatic redirects. Browser proxies can fail on the short-link hostname.
func (l *Login) ExpandShare(ctx context.Context, link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "weixin.qq.com" || !strings.HasPrefix(u.Path, "/sph/") {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	c := l.factory()
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return ""
	}
	res, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != 301 && res.StatusCode != 302 && res.StatusCode != 307 && res.StatusCode != 308 {
		return ""
	}
	dest, err := url.Parse(res.Header.Get("Location"))
	if err != nil || dest.Scheme != "https" || dest.Host != "channels.weixin.qq.com" || dest.Path != "/finder-preview/pages/sph" || dest.User != nil {
		return ""
	}
	return dest.String()
}
