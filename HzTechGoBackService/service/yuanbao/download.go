package yuanbao

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxBytes = int64(150 * 1024 * 1024)

func stream(ctx context.Context, h *http.Client, raw string, out io.Writer) (int64, error) {
	restricted := *h
	restricted.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	h = &restricted
	for i := 0; i < 6; i++ {
		if !mediaAllowed(raw) {
			return 0, errors.New("视频重定向到不允许的媒体域名，已停止")
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept-Encoding", "identity")
		res, err := h.Do(req)
		if err != nil {
			return 0, errors.New("视频媒体网络失败或超时")
		}
		if res.StatusCode == 301 || res.StatusCode == 302 || res.StatusCode == 303 || res.StatusCode == 307 || res.StatusCode == 308 {
			location := res.Header.Get("Location")
			res.Body.Close()
			base, _ := url.Parse(raw)
			u, err := url.Parse(location)
			if err != nil || location == "" {
				return 0, errors.New("视频媒体重定向无效")
			}
			raw = base.ResolveReference(u).String()
			continue
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return 0, errors.New("媒体请求失败，地址可能过期")
		}
		kind := strings.ToLower(strings.Split(res.Header.Get("Content-Type"), ";")[0])
		if kind != "video/mp4" && kind != "application/octet-stream" && kind != "binary/octet-stream" && kind != "application/mp4" {
			res.Body.Close()
			return 0, errors.New("媒体接口未返回MP4文件")
		}
		length := res.ContentLength
		if length > maxBytes {
			res.Body.Close()
			return 0, errors.New("视频超过150MB")
		}
		n, err := io.Copy(out, io.LimitReader(res.Body, maxBytes+1))
		res.Body.Close()
		if err != nil || n == 0 || n > maxBytes || length >= 0 && n != length {
			return 0, errors.New("视频文件为空、超限或传输不完整")
		}
		return n, nil
	}
	return 0, errors.New("视频媒体重定向过多")
}
func validate(ctx context.Context, path string) error {
	c, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(c, "ffprobe", "-v", "error", "-show_entries", "format=duration:stream=codec_type", "-of", "json", path).Output()
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type string `json:"codec_type"`
		} `json:"streams"`
	}
	if err != nil || json.Unmarshal(raw, &p) != nil {
		return errors.New("视频校验失败，媒体可能需要解密或格式异常")
	}
	duration, err := strconv.ParseFloat(p.Format.Duration, 64)
	hasVideo := false
	for _, s := range p.Streams {
		hasVideo = hasVideo || s.Type == "video"
	}
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 1800 || !hasVideo {
		return errors.New("视频缺少有效画面、时长异常或超过30分钟")
	}
	c2, cancel2 := context.WithTimeout(ctx, 90*time.Second)
	defer cancel2()
	cmd := exec.CommandContext(c2, "ffmpeg", "-v", "error", "-xerror", "-i", path, "-f", "null", "-")
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return errors.New("视频未通过完整解码检查，未保存成品")
	}
	return nil
}
func (s *Service) Download(ctx context.Context, share string, source SourceVideo, destination string) error {
	a, err := s.auth()
	if err != nil {
		return err
	}
	h := s.client()
	defer h.CloseIdleConnections()
	media, err := resolve(ctx, h, a, share, source)
	if err != nil {
		return err
	}
	h.Timeout = 180 * time.Second
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return errors.New("无法创建本机视频目录")
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".wechat-*.part")
	if err != nil {
		return errors.New("无法创建视频临时文件")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = stream(ctx, h, media, f); err != nil {
		return err
	}
	if f.Sync() != nil || f.Close() != nil {
		return errors.New("本机视频保存失败")
	}
	if err = validate(ctx, f.Name()); err != nil {
		return err
	}
	if os.Link(f.Name(), destination) != nil {
		return errors.New("发布视频文件失败，目标可能已存在")
	}
	return nil
}
