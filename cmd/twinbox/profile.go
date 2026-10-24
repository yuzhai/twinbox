package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// profileRoot 返回 profile 存放目录。
func profileRoot() string {
	if v := os.Getenv("TWINBOX_PROFILES"); v != "" {
		return v
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/twinbox/profiles"
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "twinbox", "profiles")
}

type profileMeta struct {
	Image   string `json:"image"`
	Created string `json:"created"`
}

func cmdProfile(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: twinbox profile <install|list>")
	}
	switch args[0] {
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("用法: twinbox profile install <image>")
		}
		dir, err := installProfile(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("已安装 profile: %s\n", dir)
		fmt.Printf("用 --profile %s 启动沙箱\n", dir)
		return nil
	case "list":
		root := profileRoot()
		entries, err := os.ReadDir(root)
		if err != nil {
			fmt.Println("（暂无 profile）")
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name())
			var size int64
			_ = filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
				if err == nil && info.Mode().IsRegular() {
					size += info.Size()
				}
				return nil
			})
			fmt.Printf("%-40s %6.1f MB\n", p, float64(size)/(1<<20))
		}
		return nil
	default:
		return fmt.Errorf("未知子命令 %q", args[0])
	}
}

// installProfile 用容器运行时导出一个 rootfs，成为可复现的环境。
// 容器运行时只出现在这一步，不是运行期依赖。
func installProfile(image string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("需要 docker 来导出镜像（仅安装期使用）")
	}
	sum := sha1.Sum([]byte(image))
	id := unsafeName.ReplaceAllString(image, "-") + "-" + hex.EncodeToString(sum[:4])
	dir := filepath.Join(profileRoot(), id)
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	create := exec.Command("docker", "create", image)
	out, err := create.Output()
	if err != nil {
		return "", fmt.Errorf("docker create 失败: %w", err)
	}
	cid := strings.TrimSpace(string(out))
	defer exec.Command("docker", "rm", "-f", cid).Run()

	exp := exec.Command("docker", "export", cid)
	tar := exec.Command("tar", "-x", "-C", dir)
	pr, pw := io.Pipe()
	exp.Stdout = pw
	tar.Stdin = pr
	if err := exp.Start(); err != nil {
		return "", err
	}
	if err := tar.Start(); err != nil {
		return "", err
	}
	expErr := exp.Wait()
	_ = pw.CloseWithError(expErr)
	if err := tar.Wait(); err != nil {
		return "", fmt.Errorf("解包失败: %w", err)
	}
	if expErr != nil {
		return "", fmt.Errorf("导出失败: %w", expErr)
	}

	meta := profileMeta{Image: image, Created: time.Now().UTC().Format(time.RFC3339)}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// resolveProfile 接受目录或 profile 名。
func resolveProfile(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("profile 名为空")
	}
	if fi, err := os.Stat(name); err == nil && fi.IsDir() {
		return name, nil
	}
	cand := filepath.Join(profileRoot(), name)
	if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
		return cand, nil
	}
	// 允许前缀匹配
	entries, _ := os.ReadDir(profileRoot())
	var matches []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), name) {
			matches = append(matches, filepath.Join(profileRoot(), e.Name()))
		}
	}
	sort.Strings(matches)
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", fmt.Errorf("找不到 profile %q", name)
}
