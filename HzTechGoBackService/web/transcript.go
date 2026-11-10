package web

import (
	"automoney/service/transcript"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterTranscriptRoutes(r *gin.Engine, service *transcript.Service) {
	r.GET("/api/v1/videos/:id/transcript", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		video, job, err := service.Get(n)
		if err != nil {
			fail(c, 404, "视频不存在或读取失败")
			return
		}
		toolsError := ""
		if err := service.Ready(); err != nil {
			toolsError = err.Error()
		}
		caption := job.Caption
		if caption == "" && video.LikesExact {
			caption = video.Title
		}
		ok(c, gin.H{"video": video, "job": job, "caption": caption, "tools_ready": toolsError == "", "tools_error": toolsError})
	})
	r.POST("/api/v1/videos/:id/transcript", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		job, err := service.Enqueue(n)
		if err != nil {
			status := 503
			if errors.Is(err, gorm.ErrRecordNotFound) {
				status = 404
			}
			if errors.Is(err, transcript.ErrBusy) {
				status = 409
			}
			fail(c, status, err.Error())
			return
		}
		ok(c, job)
	})
	r.POST("/api/v1/videos/:id/transcript/cancel", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		if !service.Cancel(n) {
			fail(c, 409, "该视频没有正在运行的提取任务")
			return
		}
		ok(c, gin.H{"cancel_requested": true})
	})
	r.GET("/api/v1/videos/:id/transcript/download", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		_, job, err := service.Get(n)
		if err != nil {
			fail(c, 404, "视频不存在")
			return
		}
		if job.Status != "success" {
			fail(c, 409, "文案尚未提取完成")
			return
		}
		text, ext := job.Text, "txt"
		if c.Query("format") == "srt" {
			text, ext = job.SRT, "srt"
		}
		if text == "" {
			fail(c, 404, "没有可下载的正文或字幕")
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="transcript-%d.%s"`, n, ext))
		c.Header("Cache-Control", "no-store")
		c.Data(200, "text/plain; charset=utf-8", []byte(text))
	})
}
