package yuanbao

// A per-login HTTPS tunnel uses the same dedicated DNS as the Go API client.
// It does not decrypt TLS, install certificates, or change system proxy settings.
import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func loginHostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, root := range []string{"tencent.com", "qq.com", "gtimg.com", "qpic.cn", "myqcloud.com", "cdn-go.cn", "tencent-cloud.net"} {
		if host == root || strings.HasSuffix(host, "."+root) {
			return true
		}
	}
	return false
}
func (s *Service) loginProxy(ctx context.Context) (string, func(), error) {
	if s.DNS == "" {
		return "", func() {}, nil
	}
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "udp", s.DNS)
	}}
	dialer := &net.Dialer{Timeout: 8 * time.Second, Resolver: resolver}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, errors.New("无法启动元宝登录专用网络通道")
	}
	tunnelCtx, cancel := context.WithCancel(ctx)
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.Host)
		if r.Method != "CONNECT" || err != nil || port != "443" || !loginHostAllowed(host) {
			http.Error(w, "Forbidden", 403)
			return
		}
		upstream, err := dialer.DialContext(tunnelCtx, "tcp", net.JoinHostPort(host, port))
		if err != nil {
			http.Error(w, "Login network unavailable", 502)
			return
		}
		downstream, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		mu.Lock()
		connections[upstream] = true
		connections[downstream] = true
		mu.Unlock()
		defer func() {
			upstream.Close()
			downstream.Close()
			mu.Lock()
			delete(connections, upstream)
			delete(connections, downstream)
			mu.Unlock()
		}()
		if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if buffer.Flush() != nil {
			return
		}
		done := make(chan struct{}, 2)
		go func() { io.Copy(upstream, buffer); done <- struct{}{} }()
		go func() { io.Copy(downstream, upstream); done <- struct{}{} }()
		select {
		case <-done:
		case <-tunnelCtx.Done():
		}
	})
	go srv.Serve(ln)
	close := func() {
		cancel()
		srv.Close()
		mu.Lock()
		for c := range connections {
			c.Close()
		}
		mu.Unlock()
	}
	return "http://" + ln.Addr().String(), close, nil
}
