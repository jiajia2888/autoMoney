package download

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Ensure joins an existing download or starts one, and returns only a verified
// project-owned file. Cancelling transcription does not cancel a shared download.
func (s *Service) Ensure(ctx context.Context, id uint64) (string, error) {
	job, err := s.Start(id)
	if err != nil {
		return "", err
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if job.Status == "success" {
			p, _, err := s.File(id)
			return p, err
		}
		if job.Status != "running" {
			return "", errors.New(job.Error)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
		job, err = s.Get(id)
		if err != nil {
			return "", err
		}
	}
}

// Poster is derived from the first decoded frame of the local, verified MP4.
func (s *Service) Poster(ctx context.Context, id uint64) (string, error) {
	media, _, err := s.File(id)
	if err != nil {
		return "", err
	}
	dst := media + ".jpg"
	videoInfo, err := os.Stat(media)
	if err != nil {
		return "", err
	}
	if info, e := os.Lstat(dst); e == nil && info.Mode().IsRegular() && info.Size() > 0 && !info.ModTime().Before(videoInfo.ModTime()) {
		return dst, nil
	}
	temp, err := os.CreateTemp(filepath.Dir(media), ".poster-*.jpg")
	if err != nil {
		return "", errors.New("无法创建首帧封面")
	}
	tmp := temp.Name()
	temp.Close()
	defer os.Remove(tmp)
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, "ffmpeg", "-nostdin", "-v", "error", "-y", "-i", media, "-frames:v", "1", "-vf", "scale=240:-2", "-q:v", "3", tmp)
	if cmd.Run() != nil {
		return "", errors.New("视频首帧提取失败")
	}
	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return "", errors.New("视频首帧为空")
	}
	if err = os.Rename(tmp, dst); err != nil {
		return "", errors.New("保存首帧封面失败")
	}
	return dst, nil
}
