package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type PythonExtractor struct{ Python, Script, Model, CredentialsFile, ProfileDir string }

func (p PythonExtractor) Ready() error {
	for _, path := range []string{p.Python, p.Script, p.Model} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("本地转写环境不完整，请检查 Python、提取脚本与 Whisper 模型")
		}
	}
	for _, name := range []string{"ffmpeg", "ffprobe", "whisper-cli"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("本机缺少 %s，请安装后重试", name)
		}
	}
	return nil
}

var videoURL = regexp.MustCompile(`^https://www\.douyin\.com/video/[0-9]{10,30}$`)

func (p PythonExtractor) Extract(ctx context.Context, url string) (Result, error) {
	if !videoURL.MatchString(url) {
		return Result{}, fmt.Errorf("视频链接无效")
	}

	return p.run(ctx, "--url", url)
}
func (p PythonExtractor) ExtractFile(ctx context.Context, path string) (Result, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 150*1024*1024 {
		return Result{}, fmt.Errorf("本地视频文件无效")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Result{}, err
	}
	return p.run(ctx, "--input", absolute)
}
func (p PythonExtractor) run(ctx context.Context, kind, input string) (Result, error) {
	if err := p.Ready(); err != nil {
		return Result{}, err
	}
	python, _ := filepath.Abs(p.Python)
	script, _ := filepath.Abs(p.Script)
	model, _ := filepath.Abs(p.Model)
	credentials, _ := filepath.Abs(p.CredentialsFile)
	profile, _ := filepath.Abs(p.ProfileDir)
	cmd := exec.CommandContext(ctx, python, script, kind, input, "--model", model, "--credentials-file", credentials, "--profile", profile)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PYTHONPATH=") && !strings.HasPrefix(value, "PYTHONHOME=") && !strings.HasPrefix(value, "VIRTUAL_ENV=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("本次文案提取已取消或超时")
	}
	var envelope struct {
		Result
		Error string `json:"error"`
	}
	if json.Unmarshal(out.Bytes(), &envelope) != nil {
		return Result{}, fmt.Errorf("本地文案提取进程失败，未取得有效结果")
	}
	if envelope.Error != "" {
		return Result{}, fmt.Errorf("%s", envelope.Error)
	}
	if err != nil || envelope.Text == "" {
		return Result{}, fmt.Errorf("本地文案提取未完成")
	}
	return envelope.Result, nil
}
