package web

import (
	"automoney/dao"
	"automoney/service/frames"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
)

func RegisterFrameRoutes(r *gin.Engine, db *gorm.DB, s *frames.Service) {
	group := r.Group("/api/v1/videos/:id/frames")
	group.Use(func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			c.Abort()
			return
		}
		var v dao.Video
		if db.First(&v, n).Error != nil {
			fail(c, 404, "视频不存在")
			c.Abort()
			return
		}
		o := frames.Options{Mode: c.DefaultQuery("mode", "count")}
		var err error
		if c.Query("count") != "" {
			o.Count, err = strconv.Atoi(c.Query("count"))
			if err != nil {
				fail(c, 400, "无效数量")
				c.Abort()
				return
			}
		}
		if c.Query("interval") != "" {
			o.Interval, err = strconv.ParseFloat(c.Query("interval"), 64)
			if err != nil {
				fail(c, 400, "无效间隔")
				c.Abort()
				return
			}
		}
		o, err = frames.Normalize(o)
		if err != nil {
			fail(c, 400, err.Error())
			c.Abort()
			return
		}
		c.Set("frameOptions", o)
		c.Header("Cache-Control", "no-store")
	})
	group.GET("", func(c *gin.Context) { n, _ := id(c); ok(c, s.Get(n, c.MustGet("frameOptions").(frames.Options))) })
	group.POST("", func(c *gin.Context) {
		n, _ := id(c)
		j, e := s.Start(n, c.MustGet("frameOptions").(frames.Options))
		if e != nil {
			status := 400
			if errors.Is(e, frames.ErrBusy) {
				status = 409
			}
			fail(c, status, e.Error())
			return
		}
		ok(c, j)
	})
	group.POST("/cancel", func(c *gin.Context) {
		n, _ := id(c)
		s.Cancel(n)
		ok(c, s.Get(n, c.MustGet("frameOptions").(frames.Options)))
	})
	group.GET("/images/:frame", func(c *gin.Context) {
		n, _ := id(c)
		index, e := strconv.Atoi(c.Param("frame"))
		if e != nil {
			fail(c, 400, "无效帧序号")
			return
		}
		p, e := s.Image(n, index, c.MustGet("frameOptions").(frames.Options))
		if e != nil {
			fail(c, 404, e.Error())
			return
		}
		c.Header("Content-Type", "image/jpeg")
		c.File(p)
	})
	group.GET("/archive", func(c *gin.Context) {
		n, _ := id(c)
		p, e := s.Archive(n, c.MustGet("frameOptions").(frames.Options))
		if e != nil {
			fail(c, 400, e.Error())
			return
		}
		c.FileAttachment(p, fmt.Sprintf("video-%d-frames.zip", n))
	})
}
