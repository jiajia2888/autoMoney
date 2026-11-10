package configs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalConfigAndWechatDNSValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AUTOMONEY_WECHAT_DNS_SERVER", "")
	if err := os.WriteFile("config.json", []byte(`{"wechat_dns_server":"223.5.5.5:53"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil || cfg.WechatDNSServer != "223.5.5.5:53" || cfg.WechatSessionFile == "" {
		t.Fatalf("local config failed %v", err)
	}
	path := filepath.Join(t.TempDir(), "other.json")
	_ = os.WriteFile(path, []byte(`{"wechat_dns_server":""}`), 0600)
	cfg, err = Load(path)
	if err != nil || cfg.WechatDNSServer != "" {
		t.Fatal("explicit config not preferred")
	}
	for _, value := range []string{"example.com:53", "223.5.5.5:80", "223.5.5.5"} {
		t.Setenv("AUTOMONEY_WECHAT_DNS_SERVER", value)
		if _, err = Load(path); err == nil {
			t.Fatalf("invalid DNS accepted %s", value)
		}
	}
}
