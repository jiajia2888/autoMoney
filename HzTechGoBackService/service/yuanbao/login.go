package yuanbao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrLogin = errors.New("请先登录元宝下载会话，再重试视频号下载")

type credential struct {
	Cookie    string            `json:"cookie"`
	UserAgent string            `json:"user_agent"`
	Headers   map[string]string `json:"headers"`
	SavedAt   int64             `json:"saved_at"`
}
type Connection struct {
	Configured bool   `json:"configured"`
	QRReady    bool   `json:"qr_ready"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	SavedAt    int64  `json:"saved_at,omitempty"`
}
type Service struct {
	Python, Script, Credentials, DNS string
	Ctx                              context.Context
	mu                               sync.Mutex
	state, message                   string
	cancel                           context.CancelFunc
	wg                               sync.WaitGroup
}

func (s *Service) auth() (credential, error) {
	var a credential
	f, err := os.Open(s.Credentials)
	if err != nil {
		return a, ErrLogin
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &a) != nil {
		return a, ErrLogin
	}
	names := map[string]bool{}
	for _, p := range strings.Split(a.Cookie, ";") {
		n, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok && v != "" {
			names[n] = true
		}
	}
	if !names["hy_user"] || !names["hy_token"] || strings.ContainsAny(a.Cookie, "\r\n") || strings.ContainsAny(a.UserAgent, "\r\n") {
		return credential{}, ErrLogin
	}
	return a, nil
}
func (s *Service) Status() Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.auth()
	state := s.state
	if state == "" {
		state = "idle"
	}
	qr := false
	if state == "starting" || state == "waiting" {
		_, e := s.readQR()
		qr = e == nil
		if qr {
			state = "waiting"
		}
	}
	return Connection{Configured: err == nil, State: state, Error: s.message, SavedAt: a.SavedAt, QRReady: qr}
}
func (s *Service) Start() Connection {
	s.mu.Lock()
	if s.state == "waiting" || s.state == "starting" {
		s.mu.Unlock()
		return s.Status()
	}
	ctx, cancel := context.WithTimeout(s.Ctx, 6*time.Minute)
	s.cancel = cancel
	s.state = "starting"
	os.Remove(s.Credentials + ".qr.png")
	s.message = ""
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer os.Remove(s.Credentials + ".qr.png")
		defer cancel()
		abs := func(p string) string { a, _ := filepath.Abs(p); return a }
		proxy, closeProxy, proxyErr := s.loginProxy(ctx)
		if proxyErr != nil {
			s.mu.Lock()
			s.state = "error"
			s.message = proxyErr.Error()
			s.cancel = nil
			s.mu.Unlock()
			return
		}
		defer closeProxy()
		args := []string{abs(s.Script), "--credentials-file", abs(s.Credentials)}
		if proxy != "" {
			args = append(args, "--proxy-server", proxy)
		}
		cmd := exec.CommandContext(ctx, abs(s.Python), args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		cmd.WaitDelay = 3 * time.Second
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "PYTHONPATH=") && !strings.HasPrefix(e, "PYTHONHOME=") && !strings.HasPrefix(e, "VIRTUAL_ENV=") {
				cmd.Env = append(cmd.Env, e)
			}
		}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		err := cmd.Run()
		var result struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		parse := json.Unmarshal(out.Bytes(), &result)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cancel = nil
		if ctx.Err() != nil {
			s.state = "cancelled"
			s.message = "元宝登录已取消或超时"
			return
		}
		if err != nil || parse != nil || result.Status != "saved" {
			s.state = "error"
			s.message = "元宝登录未完成，请重试"
			if result.Error != "" {
				s.message = result.Error
			}
			return
		}
		if _, err = s.auth(); err != nil {
			s.state = "error"
			s.message = "元宝会话未成功保存"
			return
		}
		s.state = "saved"
		s.message = ""
	}()
	return s.Status()
}
func (s *Service) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) readQR() ([]byte, error) {
	f, err := os.Open(s.Credentials + ".qr.png")
	if err != nil {
		return nil, errors.New("元宝二维码尚未就绪")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 512*1024+1))
	if err != nil || len(raw) < 8 || len(raw) > 512*1024 || !bytes.Equal(raw[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("元宝二维码尚未就绪")
	}
	return raw, nil
}
func (s *Service) QR() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != "starting" && s.state != "waiting" {
		return nil, errors.New("元宝登录未在等待扫码，请重新获取")
	}
	return s.readQR()
}
