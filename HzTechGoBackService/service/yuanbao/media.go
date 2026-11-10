package yuanbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const parseURL = "https://yuanbao.tencent.com/api/weixin/get_parse_result"
const previewURL = "https://channels.weixin.qq.com/finder-preview/api/feed/get_feed_info"
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func (s *Service) client() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if s.DNS != "" {
		resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "udp", s.DNS)
		}}
		t.Proxy = nil
		t.DialContext = (&net.Dialer{Timeout: 6 * time.Second, Resolver: resolver}).DialContext
	}
	return &http.Client{Transport: t, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func obj(m map[string]any, key string) map[string]any { r, _ := m[key].(map[string]any); return r }
func str(m map[string]any, key string) string         { r, _ := m[key].(string); return r }
func post(ctx context.Context, h *http.Client, endpoint, referer string, body any, a *credential) (map[string]any, error) {
	restricted := *h
	restricted.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	h = &restricted
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("生成媒体解析请求失败")
	}
	origin := "https://channels.weixin.qq.com"
	ua := userAgent
	if a != nil {
		origin = "https://yuanbao.tencent.com"
		if a.UserAgent != "" {
			ua = a.UserAgent
		}
		req.Header.Set("Cookie", a.Cookie)
		req.Header.Set("x-source", "web")
		for _, k := range []string{"authorization", "t-userid", "x-id", "x-device-id", "x-hy92", "x-hy93"} {
			v := a.Headers[k]
			if v != "" && !strings.ContainsAny(v, "\r\n") {
				req.Header.Set(k, v)
			}
		}
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", referer)
	res, err := h.Do(req)
	if err != nil {
		return nil, errors.New("元宝或微信媒体解析网络失败，请稍后重试")
	}
	defer res.Body.Close()
	if a != nil && (res.StatusCode == 401 || res.StatusCode == 403) {
		return nil, ErrLogin
	}
	if res.StatusCode == 429 {
		return nil, errors.New("媒体接口请求过于频繁，请稍后重试")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, errors.New("媒体解析接口请求失败，HTTP " + strconv.Itoa(res.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
	if err != nil || len(data) > 2<<20 {
		return nil, errors.New("媒体解析响应超出限制或读取失败")
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil || result == nil {
		return nil, errors.New("媒体解析接口未返回有效JSON")
	}
	return result, nil
}
func previewParts(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "channels.weixin.qq.com" || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/finder-preview/pages/") {
		return "", "", errors.New("元宝未返回有效的官方播放预览地址")
	}
	token, eid := u.Query().Get("token"), u.Query().Get("eid")
	if token == "" || eid == "" || len(token) > 8192 || len(eid) > 1024 {
		return "", "", errors.New("元宝播放预览缺少token或作品标识")
	}
	return token, eid, nil
}
func mediaAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, root := range []string{"video.qq.com", "tc.qq.com", "qpic.cn", "myqcloud.com"} {
		if host == root || strings.HasSuffix(host, "."+root) {
			return true
		}
	}
	return false
}

// SourceVideo binds a freshly generated share link to the collected work.
// Export IDs are opaque and may be reissued across Tencent services.
type SourceVideo struct {
	ExportID, Title, Author string
	PublishedAt             int64
}

func matchesSource(parsed, feed map[string]any, source SourceVideo) bool {
	return source.ExportID != "" && source.Title != "" && source.Author != "" && source.PublishedAt > 0 &&
		str(parsed, "desc") == source.Title && str(parsed, "author") == source.Author &&
		str(feed, "description") == source.Title && feed["createtime"] == float64(source.PublishedAt)
}

func resolve(ctx context.Context, h *http.Client, a credential, share string, source SourceVideo) (string, error) {
	u, err := url.Parse(share)
	if err != nil || u.Scheme != "https" || u.Host != "weixin.qq.com" || u.User != nil || !strings.HasPrefix(u.Path, "/sph/") || u.RawQuery != "" {
		return "", errors.New("视频号分享链接不是允许的官方地址")
	}
	r, err := post(ctx, h, parseURL, "https://yuanbao.tencent.com/", map[string]any{"type": "video_channel_url", "url": share, "scene": 1}, &a)
	if err != nil {
		return "", err
	}
	if r["code"] != float64(0) {
		return "", errors.New("元宝解析未成功，请核对元宝登录状态及作品可见性")
	}
	data := obj(r, "data")
	parsed := data
	preview := str(data, "playable_url")
	token, eid, err := previewParts(preview)
	if err != nil {
		return "", err
	}
	r, err = post(ctx, h, previewURL, preview, map[string]any{"baseReq": map[string]any{"generalToken": token}, "exportId": eid}, nil)
	if err != nil {
		return "", err
	}
	data = obj(r, "data")
	e := obj(data, "errMsg")
	if code, ok := r["errCode"]; ok && code != float64(0) {
		return "", errors.New("微信预览接口返回业务错误")
	}
	if data == nil || e != nil && (e["type"] != nil && e["type"] != float64(0) || str(e, "title") != "" || str(e, "content") != "") {
		return "", errors.New("微信预览暂时无法播放该作品，未取得视频文件")
	}
	feed := obj(data, "feedInfo")
	if !matchesSource(parsed, feed, source) {
		return "", errors.New("作品作者、文案或发布时间不匹配，已停止下载")
	}
	if feed["mediaType"] != float64(4) {
		return "", errors.New("微信未返回普通视频，暂不支持图集或直播")
	}
	for _, candidate := range []string{str(obj(feed, "h264VideoInfo"), "videoUrl"), str(feed, "videoUrl"), str(obj(feed, "h265VideoInfo"), "videoUrl")} {
		if candidate != "" {
			if !mediaAllowed(candidate) {
				return "", errors.New("媒体地址不在允许的腾讯HTTPS域名范围内")
			}
			return candidate, nil
		}
	}
	return "", errors.New("微信预览未返回视频媒体地址")
}
