package web

import (
	"automoney/dao"
	"automoney/service/wechat"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"
	"net/url"
	"strconv"
	"strings"
)

func RegisterWechatCollectionRoutes(r *gin.Engine, db *gorm.DB, l *wechat.Login) {
	r.POST("/api/v1/wechat/authors/search", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var in struct {
			Query string `json:"query"`
			Page  int    `json:"page"`
		}
		if c.ShouldBindJSON(&in) != nil {
			fail(c, 400, "无效搜索请求")
			return
		}
		if in.Page == 0 {
			in.Page = 1
		}
		data, err := l.Search(c.Request.Context(), in.Query, in.Page)
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		ok(c, data)
	})
	r.POST("/api/v1/wechat/videos/:id/share", func(c *gin.Context) {
		n, yes := id(c)
		if !yes {
			return
		}
		var v dao.Video
		var a dao.BenchmarkAccount
		if db == nil || db.First(&v, n).Error != nil || db.First(&a, v.AccountID).Error != nil || a.Platform != "wx" {
			fail(c, 404, "视频号作品不存在")
			return
		}
		if v.URL != "" {
			wechatShareResponse(c, v.URL, n, l.ExpandShare(c.Request.Context(), v.URL))
			return
		}
		link, err := l.Share(c.Request.Context(), v.PlatformID, v.Nonce)
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		if db.Model(&v).Update("url", link).Error != nil {
			fail(c, 500, "保存分享链接失败")
			return
		}
		wechatShareResponse(c, link, n, l.ExpandShare(c.Request.Context(), link))
	})
}

func wechatShareResponse(c *gin.Context, link string, n uint64, direct string) {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "weixin.qq.com" || !strings.HasPrefix(u.Path, "/sph/") {
		fail(c, 400, "微信分享链接格式无效")
		return
	}
	png, err := qrcode.Encode(link, qrcode.Medium, 260)
	if err != nil {
		fail(c, 500, "生成分享二维码失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	ok(c, gin.H{"url": link, "video_id": strconv.FormatUint(n, 10), "direct_url": direct, "qr_image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}
