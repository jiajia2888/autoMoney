package web

import (
	"automoney/service/download"
	"automoney/service/yuanbao"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

func RegisterDownloadRoutes(r *gin.Engine, s *download.Service) {
	respondError := func(c *gin.Context, err error) {
		status := 400
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = 404
		}
		if errors.Is(err, download.ErrBusy) {
			status = 409
		}
		fail(c, status, err.Error())
	}

	r.GET("/api/v1/media", func(c *gin.Context) {
		ids := strings.Split(c.Query("ids"), ",")
		if len(ids) > 500 {
			fail(c, 400, "最多查询500条视频")
			return
		}
		result := map[uint64]download.Job{}
		for _, raw := range ids {
			n, e := strconv.ParseUint(raw, 10, 64)
			if e != nil || n == 0 {
				fail(c, 400, "无效视频ID")
				return
			}
			j, e := s.Get(n)
			if e != nil {
				j = download.Job{Status: "error", Error: e.Error()}
			}
			result[n] = j
		}
		c.Header("Cache-Control", "no-store")
		ok(c, result)
	})
	r.GET("/api/v1/videos/:id/media/play", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		p, _, err := s.File(n)
		if err != nil {
			respondError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Disposition", "inline")
		c.File(p)
	})
	r.GET("/api/v1/videos/:id/media/poster", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		p, err := s.Poster(c.Request.Context(), n)
		if err != nil {
			respondError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", "image/jpeg")
		c.File(p)
	})
	r.GET("/api/v1/videos/:id/media", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		c.Header("Cache-Control", "no-store")
		j, err := s.Get(n)
		if err != nil {
			respondError(c, err)
			return
		}
		ok(c, j)
	})
	r.POST("/api/v1/videos/:id/media", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		c.Header("Cache-Control", "no-store")
		j, err := s.Start(n)
		if err != nil {
			respondError(c, err)
			return
		}
		ok(c, j)
	})
	r.GET("/api/v1/videos/:id/media/file", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		p, name, err := s.File(n)
		if err != nil {
			respondError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Type", "video/mp4")
		c.FileAttachment(p, name)
	})
}

func RegisterYuanbaoRoutes(r *gin.Engine, s *yuanbao.Service) {
	r.GET("/api/v1/yuanbao/login/qr", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		raw, err := s.QR()
		if err != nil {
			fail(c, 404, err.Error())
			return
		}
		c.Data(200, "image/png", raw)
	})
	r.GET("/api/v1/yuanbao/connection", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); ok(c, s.Status()) })
	r.POST("/api/v1/yuanbao/login", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); ok(c, s.Start()) })
	r.POST("/api/v1/yuanbao/login/cancel", func(c *gin.Context) { s.Cancel(); ok(c, s.Status()) })
}
