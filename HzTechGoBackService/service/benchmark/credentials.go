package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Credentials are stored only in a local, owner-readable file. Never return them
// from a GET endpoint or place them in subprocess arguments or application logs.
type Credentials struct {
	Cookie    string    `json:"cookie"`
	UserAgent string    `json:"user_agent"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConnectionStatus struct {
	Mode       string     `json:"mode"`
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type CredentialStore struct{ Path string }

var cookieName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func (c *Credentials) Validate() error {
	c.Cookie = strings.TrimSpace(c.Cookie)
	if strings.HasPrefix(strings.ToLower(c.Cookie), "cookie:") {
		c.Cookie = strings.TrimSpace(c.Cookie[len("cookie:"):])
	}
	c.UserAgent = strings.TrimSpace(c.UserAgent)
	for _, r := range c.Cookie + c.UserAgent {
		if r < 32 || r > 126 {
			return fmt.Errorf("Cookie 和 User-Agent 应为单行请求头值，请勿包含换行或其他字符")
		}
	}
	if len(c.Cookie) == 0 || len(c.Cookie) > 32768 || strings.ContainsAny(c.Cookie, "\r\n\x00") {
		return fmt.Errorf("请粘贴完整、单行的抖音 Cookie（最多 32KB）")
	}
	if len(c.UserAgent) < 10 || len(c.UserAgent) > 1024 || strings.ContainsAny(c.UserAgent, "\r\n\x00") {
		return fmt.Errorf("请填写取得 Cookie 时同一浏览器的 User-Agent")
	}
	seen := map[string]int{}
	auth := false
	for index, part := range strings.Split(c.Cookie, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		// Chromium can send a nameless cookie as a bare token. Preserve the
		// original header, but never count this value as a named auth cookie.
		if !ok && cookieName.MatchString(part) {
			continue
		}
		if !ok {
			return fmt.Errorf("Cookie 第 %d 项缺少 =，请重新复制 Network → Request Headers 中完整的 Cookie 值", index+1)
		}
		if !cookieName.MatchString(name) {
			return fmt.Errorf("Cookie 第 %d 项字段名无效，请复制完整的 Cookie 请求头值", index+1)
		}
		if first := seen[name]; first != 0 {
			return fmt.Errorf("Cookie 第 %d 项与第 %d 项字段重复，请从同一个请求复制完整 Cookie，不要拼接多次内容", index+1, first)
		}
		seen[name] = index + 1
		if (name == "sessionid" || name == "sessionid_ss") && value != "" {
			auth = true
		}
	}
	if !auth {
		return fmt.Errorf("Cookie 缺少 sessionid 登录字段；单个 msToken 或 API Token 不能用于此入口")
	}
	return nil
}

func (s CredentialStore) Status() (ConnectionStatus, error) {
	status := ConnectionStatus{Mode: "browser"}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, fmt.Errorf("无法读取本地登录配置")
	}
	var c Credentials
	if len(data) > 65536 || json.Unmarshal(data, &c) != nil || c.Validate() != nil {
		return status, fmt.Errorf("本地登录配置损坏，请重新保存")
	}
	return ConnectionStatus{Mode: "cookie", Configured: true, UpdatedAt: &c.UpdatedAt}, nil
}

func (s CredentialStore) Save(c Credentials) (ConnectionStatus, error) {
	if err := c.Validate(); err != nil {
		return ConnectionStatus{}, err
	}
	c.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(c)
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("生成登录配置失败")
	}
	if err = os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return ConnectionStatus{}, fmt.Errorf("创建登录配置目录失败")
	}
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".credentials-*")
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("保存登录配置失败")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), s.Path)
	}
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("保存登录配置失败")
	}
	return ConnectionStatus{Mode: "cookie", Configured: true, UpdatedAt: &c.UpdatedAt}, nil
}

func (s CredentialStore) UseBrowser() error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("切换扫码登录失败")
	}
	return nil
}
