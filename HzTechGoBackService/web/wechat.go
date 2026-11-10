package web

import (
	"automoney/service/wechat"
	"github.com/gin-gonic/gin"
	"strings"
)

// RegisterWechatRoutes shares the local Host/origin guards from NewRouter.
func RegisterWechatRoutes(r *gin.Engine, login *wechat.Login) {
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/wechat/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	})
	r.GET("/api/v1/wechat/connection", func(c *gin.Context) {
		data, err := login.Status()
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		ok(c, data)
	})
	r.POST("/api/v1/wechat/login", func(c *gin.Context) {
		data, err := login.Start(c.Request.Context())
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		ok(c, data)
	})
	r.POST("/api/v1/wechat/login/poll", func(c *gin.Context) {
		var in struct {
			ID string `json:"id"`
		}
		if c.ShouldBindJSON(&in) != nil || in.ID == "" || len(in.ID) > 128 {
			fail(c, 400, "无效登录任务")
			return
		}
		data, err := login.Poll(c.Request.Context(), in.ID)
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		ok(c, data)
	})
	r.POST("/api/v1/wechat/login/cancel", func(c *gin.Context) {
		var in struct {
			ID string `json:"id"`
		}
		if c.ShouldBindJSON(&in) != nil || len(in.ID) > 128 {
			fail(c, 400, "无效登录任务")
			return
		}
		_ = login.Cancel(in.ID)
		ok(c, gin.H{"cancelled": true})
	})
	r.POST("/api/v1/wechat/connection/verify", func(c *gin.Context) {
		data, err := login.Verify(c.Request.Context())
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		ok(c, data)
	})
	r.DELETE("/api/v1/wechat/connection", func(c *gin.Context) {
		if err := login.Disconnect(); err != nil {
			fail(c, 500, err.Error())
			return
		}
		ok(c, gin.H{"disconnected": true})
	})
}
