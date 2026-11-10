package configs

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

type Config struct {
	WechatDNSServer   string `json:"wechat_dns_server"`
	WechatSessionFile string `json:"wechat_session_file"`
	Address           string `json:"address"`
	PagesDir          string `json:"pages_dir"`
	MySQLDSN          string `json:"mysql_dsn"`
	SQLitePath        string `json:"sqlite_path"`
	Python            string `json:"python"`
	CollectorScript   string `json:"collector_script"`
	ProfileDir        string `json:"profile_dir"`
	CredentialsFile   string `json:"credentials_file"`
	TranscriptScript  string `json:"transcript_script"`
	WhisperModel      string `json:"whisper_model"`
}

func Load(path string) (Config, error) {
	cfg := Config{WechatSessionFile: "../.local/wechat-session.enc", Address: "127.0.0.1:9010", PagesDir: "../HxTechPages", SQLitePath: "../.local/monitor.db", Python: "../.venv/bin/python", CollectorScript: "../scripts/douyin_collector.py", ProfileDir: "../.local/douyin-profile", CredentialsFile: "../.local/douyin-credentials.json"}
	home, _ := os.UserHomeDir()
	cfg.TranscriptScript = "../scripts/extract_copy.py"
	cfg.WhisperModel = home + "/.cache/whisper/ggml-large-v3-turbo-q5_0.bin"
	if path == "" {
		if _, err := os.Stat("config.json"); err == nil {
			path = "config.json"
		}
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err = json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config: %w", err)
		}
	}
	if v := os.Getenv("AUTOMONEY_TRANSCRIPT_SCRIPT"); v != "" {
		cfg.TranscriptScript = v
	}
	if v := os.Getenv("AUTOMONEY_WHISPER_MODEL"); v != "" {
		cfg.WhisperModel = v
	}
	for name, ptr := range map[string]*string{"AUTOMONEY_WECHAT_DNS_SERVER": &cfg.WechatDNSServer, "AUTOMONEY_WECHAT_SESSION_FILE": &cfg.WechatSessionFile, "AUTOMONEY_ADDR": &cfg.Address, "AUTOMONEY_MYSQL_DSN": &cfg.MySQLDSN, "AUTOMONEY_PAGES_DIR": &cfg.PagesDir, "AUTOMONEY_SQLITE_PATH": &cfg.SQLitePath, "AUTOMONEY_PYTHON": &cfg.Python, "AUTOMONEY_COLLECTOR_SCRIPT": &cfg.CollectorScript, "AUTOMONEY_PROFILE_DIR": &cfg.ProfileDir, "AUTOMONEY_CREDENTIALS_FILE": &cfg.CredentialsFile} {
		if v := os.Getenv(name); v != "" {
			*ptr = v
		}
	}
	if cfg.WechatSessionFile == "" || cfg.Address == "" || cfg.PagesDir == "" || cfg.SQLitePath == "" || cfg.Python == "" || cfg.CredentialsFile == "" {
		return cfg, fmt.Errorf("address, pages_dir, sqlite_path, python credentials_file and wechat_session_file are required")
	}
	if cfg.WechatDNSServer != "" {
		host, port, err := net.SplitHostPort(cfg.WechatDNSServer)
		if err != nil || net.ParseIP(host) == nil || port != "53" {
			return cfg, fmt.Errorf("wechat_dns_server must be an IP address with port 53")
		}
	}
	return cfg, nil
}
