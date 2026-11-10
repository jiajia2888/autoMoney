package yuanbao

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestLoginTunnelRejectsOtherHostsAndPorts(t *testing.T) {
	s := &Service{DNS: "127.0.0.1:53"}
	address, stop, err := s.loginProxy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for _, target := range []string{"example.org:443", "yuanbao.tencent.com.evil.test:443", "127.0.0.1:443", "yuanbao.tencent.com:80"} {
		c, err := net.Dial("tcp", strings.TrimPrefix(address, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		r, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
		if err != nil {
			c.Close()
			t.Fatal(err)
		}
		if r.StatusCode != 403 {
			t.Fatal("non-login tunnel allowed", target, r.StatusCode)
		}
		r.Body.Close()
		c.Close()
	}
}
