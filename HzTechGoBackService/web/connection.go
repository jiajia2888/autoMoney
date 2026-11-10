package web

import (
	"automoney/service/benchmark"
	"github.com/gin-gonic/gin"
)

// RegisterConnectionRoutes must be called on NewRouter so the same loopback,
// origin and body-size guards apply to every credential operation.
func RegisterConnectionRoutes(r *gin.Engine, store benchmark.CredentialStore) {
	r.GET("/api/v1/collector/connection", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		status, err := store.Status()
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		ok(c, status)
	})
	r.PUT("/api/v1/collector/connection", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var in struct {
			Mode      string `json:"mode"`
			Cookie    string `json:"cookie"`
			UserAgent string `json:"user_agent"`
		}
		if c.ShouldBindJSON(&in) != nil {
			fail(c, 400, "登录配置格式无效")
			return
		}
		if in.Mode == "browser" {
			if err := store.UseBrowser(); err != nil {
				fail(c, 500, err.Error())
				return
			}
			ok(c, benchmark.ConnectionStatus{Mode: "browser"})
			return
		}
		if in.Mode != "cookie" {
			fail(c, 400, "请选择扫码登录或手动 Cookie")
			return
		}
		creds := benchmark.Credentials{Cookie: in.Cookie, UserAgent: in.UserAgent}
		if err := creds.Validate(); err != nil {
			fail(c, 400, err.Error())
			return
		}
		status, err := store.Save(creds)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		ok(c, status)
	})
}
