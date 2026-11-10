package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type PythonCrawler struct{ Python, Script, Profile, CredentialsFile string }

func (p PythonCrawler) execute(ctx context.Context, args ...string) ([]byte, error) {
	python, err := filepath.Abs(p.Python)
	if err != nil {
		return nil, err
	}
	script, err := filepath.Abs(p.Script)
	if err != nil {
		return nil, err
	}
	profile, err := filepath.Abs(p.Profile)
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(python); err != nil {
		return nil, fmt.Errorf("采集环境未安装，请运行 make collector-setup")
	}
	cmd := exec.CommandContext(ctx, python, append([]string{script, "--profile", profile}, args...)...)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PYTHONPATH=") && !strings.HasPrefix(v, "PYTHONHOME=") && !strings.HasPrefix(v, "VIRTUAL_ENV=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PYTHONUNBUFFERED=1")
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
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("采集超时或任务取消")
	}
	var envelope struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(out.Bytes(), &envelope)
	if err != nil {
		if envelope.Error != "" {
			return nil, fmt.Errorf("%s", envelope.Error)
		}
		return nil, fmt.Errorf("采集进程失败，请运行 make collector-doctor 检查环境")
	}
	if envelope.Error != "" {
		return nil, fmt.Errorf("%s", envelope.Error)
	}
	return out.Bytes(), nil
}
func (p PythonCrawler) FetchPosts(ctx context.Context, r Request) (Result, error) {
	args := []string{"--mode", "collect", "--url", r.HomepageURL, "--max-items", strconv.Itoa(r.MaxItems)}
	if p.CredentialsFile != "" {
		path, err := filepath.Abs(p.CredentialsFile)
		if err != nil {
			return Result{}, fmt.Errorf("登录配置路径无效")
		}
		args = append(args, "--credentials-file", path)
	}
	out, err := p.execute(ctx, args...)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err = json.Unmarshal(out, &result); err != nil {
		return result, fmt.Errorf("采集响应格式错误")
	}
	if len(result.Posts) == 0 {
		return result, fmt.Errorf("未获取作品，无法确认账号无作品；请检查登录或验证提示")
	}
	if len(result.Posts) > r.MaxItems {
		return result, fmt.Errorf("采集器超过任务数量上限")
	}
	for _, v := range result.Posts {
		if v.PlatformID == "" || v.Likes < 0 || !strings.HasPrefix(v.URL, "https://www.douyin.com/video/") {
			return result, fmt.Errorf("采集响应含无效作品")
		}
	}
	return result, nil
}
func (p PythonCrawler) Login(ctx context.Context) error {
	_, err := p.execute(ctx, "--mode", "login")
	return err
}
