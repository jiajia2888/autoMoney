package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"automoney/dao"
	"automoney/service/monitor"
)

func TestLocalAPI(t *testing.T) {
	db, err := dao.OpenLocal("", filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err = dao.Migrate(db); err != nil {
		t.Fatal(err)
	}
	r := NewRouter(db, t.TempDir(), monitor.New(db, nil))
	request := func(method, path, body string, want int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Host = "127.0.0.1:9010"
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	request("GET", "/healthz", "", 200)
	request("GET", "/api/v1/status", "", 200)
	valid := `{"input":"80185543890","display_name":"徐晶说","enabled":false,"interval_minutes":60,"threshold":10000,"max_items":100,"notify_new":true,"notify_threshold":true}`
	request("POST", "/api/v1/accounts", valid, 200)
	request("POST", "/api/v1/accounts", valid, 409)
	request("POST", "/api/v1/accounts", `{"input":"123","display_name":"test","interval_minutes":0}`, 400)
	result := request("GET", "/api/v1/accounts", "", 200)
	if len(result["data"].([]any)) != 1 {
		t.Fatal("account not saved")
	}
	request("GET", "/api/v1/videos?min_likes=abc", "", 400)
	request("GET", "/api/v1/videos?min_likes=10000", "", 200)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/collector/login", nil)
	req.Host = "127.0.0.1:9010"
	req.Header.Set("Origin", "https://foreign.example")
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross origin write allowed")
	}
	request("DELETE", "/api/v1/accounts/1", "", 200)
	request("DELETE", "/api/v1/accounts/1", "", 404)
	request("GET", "/api/v1/unknown", "", 404)
}
