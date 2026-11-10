package wechat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

type Connection struct {
	Configured bool       `json:"configured"`
	Identity   *Identity  `json:"identity,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}
type LoginResult struct {
	ID        string    `json:"id,omitempty"`
	State     string    `json:"state"`
	QRImage   string    `json:"qr_image,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Identity  *Identity `json:"identity,omitempty"`
}
type attempt struct {
	id        string
	challenge map[string]any
	expires   time.Time
	client    *client
	lastPoll  time.Time
	result    LoginResult
}
type Login struct {
	mu      sync.Mutex
	store   store
	pending *attempt
	factory func() *client
}

func NewLogin(path string) *Login { return &Login{store: store{path: path}, factory: newClient} }
func (l *Login) Status() (Connection, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, err := l.store.load()
	if err != nil {
		return Connection{}, err
	}
	if c == nil {
		return Connection{}, nil
	}
	return Connection{Configured: true, Identity: &c.Identity, VerifiedAt: &c.VerifiedAt}, nil
}
func (l *Login) Start(ctx context.Context) (LoginResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.factory()
	data, err := c.post(ctx, "auth/auth_login_token", loginPage, map[string]any{})
	if err != nil {
		return LoginResult{}, err
	}
	token, ok := data["token"].(string)
	if !ok || token == "" || len(token) > 4096 {
		return LoginResult{}, errors.New("微信未返回有效二维码 token")
	}
	png, err := qrcode.Encode(origin+"/promote/pages/mobile_login?token="+url.QueryEscape(token), qrcode.Medium, 280)
	if err != nil {
		return LoginResult{}, errors.New("生成二维码失败")
	}
	id := make([]byte, 24)
	if _, err = rand.Read(id); err != nil {
		return LoginResult{}, errors.New("创建登录任务失败")
	}
	result := LoginResult{ID: base64.RawURLEncoding.EncodeToString(id), State: "waiting", QRImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), ExpiresAt: time.Now().Add(5 * time.Minute)}
	l.pending = &attempt{id: result.ID, challenge: data, expires: result.ExpiresAt, client: c, result: LoginResult{ID: result.ID, State: "waiting", ExpiresAt: result.ExpiresAt}}
	return result, nil
}
func (l *Login) Poll(ctx context.Context, id string) (LoginResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.pending
	if a == nil || id != a.id {
		return LoginResult{}, errors.New("登录任务已关闭或被更新，请刷新二维码")
	}
	if a.result.State == "success" || a.result.State == "expired" {
		return a.result, nil
	}
	if time.Now().After(a.expires) {
		a.result.State = "expired"
		a.challenge = nil
		a.client = nil
		return a.result, nil
	}
	if time.Since(a.lastPoll) < 2*time.Second {
		return a.result, nil
	}
	a.lastPoll = time.Now()
	data, err := a.client.post(ctx, "auth/auth_login_status", loginPage, a.challenge)
	if err != nil {
		return LoginResult{}, err
	}
	status, validStatus := data["status"].(float64)
	acct, _ := data["acctStatus"].(float64)
	if !validStatus {
		a.result.State = "unknown"
		return a.result, nil
	}
	switch {
	case status == 4:
		a.result.State = "expired"
		a.challenge = nil
		a.client = nil
	case status == 5 && acct == 1:
		a.result.State = "scanned"
	case status == 1 && acct == 1:
		session := a.client.session()
		if session == "" {
			return LoginResult{}, errors.New("微信已确认但未返回投手会话，请刷新二维码重试")
		}
		identity, err := a.client.profile(ctx)
		if err != nil {
			return LoginResult{}, err
		}
		if err = l.store.save(credential{Session: a.client.session(), Identity: identity, VerifiedAt: time.Now().UTC()}); err != nil {
			return LoginResult{}, err
		}
		a.result.State = "success"
		a.result.Identity = &identity
		a.challenge = nil
		a.client = nil
	case status == 0:
		a.result.State = "waiting"
	default:
		a.result.State = "unknown"
	}
	return a.result, nil
}
func (l *Login) Verify(ctx context.Context) (Connection, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	saved, err := l.store.load()
	if err != nil {
		return Connection{}, err
	}
	if saved == nil {
		return Connection{}, errors.New("尚未连接视频号，请先扫码")
	}
	c := l.factory()
	c.setSession(saved.Session)
	identity, err := c.profile(ctx)
	if err != nil {
		if errors.Is(err, ErrExpired) {
			if clearErr := l.store.clear(); clearErr != nil {
				return Connection{}, clearErr
			}
		}
		return Connection{}, err
	}
	saved.Identity = identity
	saved.VerifiedAt = time.Now().UTC()
	if value := c.session(); value != "" {
		saved.Session = value
	}
	if err = l.store.save(*saved); err != nil {
		return Connection{}, err
	}
	return Connection{Configured: true, Identity: &identity, VerifiedAt: &saved.VerifiedAt}, nil
}
func (l *Login) Cancel(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending != nil && l.pending.id == id {
		l.pending = nil
	}
	return nil
}
func (l *Login) Disconnect() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = nil
	return l.store.clear()
}

func NewLoginWithDNS(path, server string) *Login {
	l := NewLogin(path)
	l.factory = func() *client { return clientWithDNS(server) }
	return l
}
