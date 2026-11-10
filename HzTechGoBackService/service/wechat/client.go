package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const origin = "https://channels.weixin.qq.com"
const loginPage = origin + "/promote/pages/platform/login-iframe"
const profilePath = "transfer/MMFinderPromotionLiveDspApiSvr/getLivePromotionUserProfile"

var ErrExpired = errors.New("视频号会话已失效，请重新扫码")

type client struct {
	http *http.Client
	base string
}

// WeChat currently returns Max-Age=253402300000. Converting that many
// seconds to time.Duration overflows inside the standard cookiejar and expires
// the fresh session. Bound positive lifetimes before cookiejar sees them;
// preserve deletion, domain, path and Secure semantics.
type sessionJar struct{ http.CookieJar }

func newSessionJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return sessionJar{CookieJar: jar}
}
func (j sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	const maxAge = 400 * 24 * 60 * 60
	bounded := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		copy := *c
		if copy.MaxAge > maxAge {
			copy.MaxAge = maxAge
		}
		bounded = append(bounded, &copy)
	}
	j.CookieJar.SetCookies(u, bounded)
}

func newClient() *client { return clientWithDNS("") }

// An optional dedicated resolver avoids broken local proxy DNS; TLS still verifies
// the official hostname and redirects remain disabled. No system settings change.
func clientWithDNS(server string) *client {
	jar := newSessionJar()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if server != "" {
		resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", server)
		}}
		transport.Proxy = nil
		transport.DialContext = (&net.Dialer{Timeout: 4 * time.Second, Resolver: resolver}).DialContext
	}
	return &client{http: &http.Client{Transport: transport, Jar: jar, Timeout: 6 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, base: origin}
}
func checkResponse(v map[string]any) error {
	for _, key := range []string{"error", "errCode", "errcode", "ret", "Ret"} {
		if value, ok := v[key]; ok && value != nil && value != false && value != float64(0) && value != "0" && value != "" {
			if value == float64(-330) || value == float64(-333) || value == float64(-334) || value == "-330" || value == "-333" || value == "-334" {
				return ErrExpired
			}
			return errors.New("微信接口返回业务错误，请稍后重试或重新核验登录")
		}
	}
	for _, key := range []string{"data", "baseResp", "baseResponse", "BaseResponse"} {
		if child, ok := v[key].(map[string]any); ok {
			if err := checkResponse(child); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *client) post(ctx context.Context, path, page string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("生成微信请求失败")
	}
	return c.request(ctx, path, page, bytes.NewReader(raw), "application/json")
}
func (c *client) form(ctx context.Context, path, page string, values url.Values) (map[string]any, error) {
	return c.request(ctx, path, page, strings.NewReader(values.Encode()), "application/x-www-form-urlencoded")
}
func (c *client) request(ctx context.Context, path, page string, body io.Reader, contentType string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/promote/api/web/"+path+"?_pageUrl="+url.QueryEscape(page), body)
	if err != nil {
		return nil, errors.New("生成微信请求失败")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", page)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("微信接口网络失败或超时，请稍后重试")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, ErrExpired
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, errors.New("微信接口 HTTP 请求失败")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("微信响应读取失败或超出限制")
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil || result == nil {
		return nil, errors.New("微信接口返回非 JSON 响应")
	}
	if err := checkResponse(result); err != nil {
		return nil, err
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, errors.New("微信接口响应缺少 data")
	}
	return data, nil
}
func (c *client) session() string {
	u, _ := url.Parse(c.base + "/promote/api/web/" + profilePath)
	for _, cookie := range c.http.Jar.Cookies(u) {
		if cookie.Name == "promotewebsessionid" {
			return cookie.Value
		}
	}
	return ""
}
func (c *client) setSession(value string) {
	u, _ := url.Parse(c.base)
	c.http.Jar.SetCookies(u, []*http.Cookie{{Name: "promotewebsessionid", Value: value, Path: "/", Secure: u.Scheme == "https"}})
}
func (c *client) profile(ctx context.Context) (Identity, error) {
	data, err := c.post(ctx, profilePath, origin+"/promote/pages/platform/home", map[string]any{})
	if err != nil {
		return Identity{}, err
	}
	info, ok := data["selfAccountInfo"].(map[string]any)
	if !ok {
		return Identity{}, errors.New("微信响应缺少投手身份，尚未登录成功")
	}
	id, _ := info["username"].(string)
	name, _ := info["nickName"].(string)
	if id == "" {
		return Identity{}, errors.New("微信未返回有效投手身份，尚未登录成功")
	}
	return Identity{Username: id, Nickname: name}, nil
}
