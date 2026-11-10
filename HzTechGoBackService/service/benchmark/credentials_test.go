package benchmark

import (
	"strings"
	"testing"
)

func TestCredentialFormatDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, cookie, want string }{
		{"missing separator", "sessionid=test-secret; invalid bare value", "Cookie 第 2 项缺少 ="},
		{"invalid name", "sessionid=test-secret; invalid name=secret", "Cookie 第 2 项字段名无效"},
		{"duplicate", "sessionid=test-secret; sessionid=other-secret", "Cookie 第 2 项与第 1 项字段重复"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Credentials{Cookie: tc.cookie, UserAgent: "Mozilla/5.0 TestBrowser"}
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("diagnostic leaked credential content")
			}
		})
	}
}

func TestBrowserNamelessCookiePreservesHeader(t *testing.T) {
	const raw = "sessionid=test-secret; browserBare123; msToken=test-token"
	c := Credentials{Cookie: raw, UserAgent: "Mozilla/5.0 TestBrowser"}
	if err := c.Validate(); err != nil {
		t.Fatalf("browser header rejected: %v", err)
	}
	if c.Cookie != raw {
		t.Fatal("browser header was rewritten")
	}
	c.Cookie = "sessionid; msToken=test-token"
	if c.Validate() == nil {
		t.Fatal("bare sessionid must not count as authentication")
	}
}
