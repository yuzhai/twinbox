package main

import (
	"crypto/sha1"
	"encoding/json"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuzhai/twinbox/internal/agent"
	"github.com/yuzhai/twinbox/internal/audit"
	"github.com/yuzhai/twinbox/internal/broker"
	"github.com/yuzhai/twinbox/internal/mcp"
	"github.com/yuzhai/twinbox/internal/policy"
	"github.com/yuzhai/twinbox/internal/sandbox"
	"github.com/yuzhai/twinbox/internal/store"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var workspace multiFlag
	fs.Var(&workspace, "workspace", "允许进入沙箱的目录（可重复）")
	home := fs.String("home", "", "沙箱 home 的宿主路径（默认按工作区派生）")
	shimPath := fs.String("shim", "", "twinbox-shim 路径（默认与 twinbox 同目录）")
	execCmd := fs.String("exec", "", "用 shell 命令代替 agent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 允许 "run <agent> --flag" 的顺序：先取出 agent 名，再解析其余参数。
	name := "fake"
	if fs.NArg() > 0 {
		name = fs.Arg(0)
		rest := fs.Args()[1:]
		if len(rest) > 0 {
			if err := fs.Parse(rest); err != nil {
				return err
			}
		}
	}
	if fs.NArg() > 1 && *execCmd == "" {
		return fmt.Errorf("用法: twinbox run <agent> [--workspace DIR]")
	}

	ad, ok := agent.Get(name)
	if !ok {
		return fmt.Errorf("不支持的 agent %q（可选: %s）", name, strings.Join(agent.List(), ", "))
	}

	shim := *shimPath
	if shim == "" {
		shim = os.Getenv("TWINBOX_SHIM")
	}
	if shim == "" {
		self, err := os.Executable()
		if err == nil {
			cand := filepath.Join(filepath.Dir(self), "twinbox-shim")
			if _, err := os.Stat(cand); err == nil {
				shim = cand
			}
		}
	}
	if shim == "" {
		return fmt.Errorf("找不到 twinbox-shim，请用 --shim 指定或设置 TWINBOX_SHIM")
	}

	ws := []string(workspace)
	if len(ws) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ws = []string{cwd}
	}

	// 运行时目录与 broker
	rt, err := os.MkdirTemp("", "twinbox-rt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rt)
	sock := filepath.Join(rt, "broker.sock")

	st, err := store.Load(store.DefaultPath())
	if err != nil {
		return err
	}
	al, err := audit.Open(defaultAuditPath())
	if err != nil {
		return err
	}
	defer al.Close()

	rules := policy.Default()
	if h := os.Getenv("TWINBOX_COMPAT_HOST"); h != "" {
		rules = rules.WithCompat(h)
	}
	// 测试与离线演示用：把内置上游指向本地替身
	if up := os.Getenv("TWINBOX_UPSTREAM_ANTHROPIC"); up != "" {
		rs := append([]policy.Rule{}, rules.Rules...)
		for i := range rs {
			if rs[i].Host == "api.anthropic.com" {
				rs[i].Upstream = up
			}
		}
		rules = policy.Set{Rules: rs}
	}
	b := broker.New(rules, st, al)
	go func() {
		if err := b.ServeUnix(sock); err != nil {
			fmt.Fprintf(os.Stderr, "twinbox: broker: %v\n", err)
		}
	}()
	// 等待 socket 就绪
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	homeDir := *home
	if homeDir == "" {
		homeDir = defaultHome(ws[0])
	}
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		return err
	}

	var mcpNames []string
	var mcpSockHost string
	if cfg := os.Getenv("TWINBOX_MCP_CONFIG"); cfg != "" {
		raw, err := os.ReadFile(cfg)
		if err != nil {
			return fmt.Errorf("读取 MCP 配置失败: %w", err)
		}
		var doc struct {
			Servers map[string]mcp.ServerSpec `json:"servers"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("解析 MCP 配置失败: %w", err)
		}
		if len(doc.Servers) > 0 {
			gw := &mcp.Gateway{Servers: doc.Servers, Secrets: st}
			mcpSockHost = filepath.Join(rt, "mcp.sock")
			go func() {
				if err := gw.ServeUnix(mcpSockHost); err != nil {
					fmt.Fprintf(os.Stderr, "twinbox: mcp gateway: %v\n", err)
				}
			}()
			for i := 0; i < 100; i++ {
				if _, err := os.Stat(mcpSockHost); err == nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			for name := range doc.Servers {
				mcpNames = append(mcpNames, name)
			}
			sort.Strings(mcpNames)
		}
	}

	ctx := agent.Context{Home: homeDir, BaseURL: "http://127.0.0.1:8899", Compat: "compat.local", MCP: mcpNames}
	env := map[string]string{
		"TWINBOX":        "1",
		"TWINBOX_SOCK":   "/run/twinbox/broker.sock",
		"TWINBOX_ROUTES": "8899=api.anthropic.com",
	}
	if mcpSockHost != "" {
		env["TWINBOX_MCP_SOCK"] = "/run/twinbox/mcp.sock"
	}
	for k, v := range ad.Env(ctx) {
		env[k] = v
	}
	if ad.Write != nil {
		if err := ad.Write(ctx); err != nil {
			return fmt.Errorf("生成 %s 配置失败: %w", ad.Name, err)
		}
	}

	m := sandbox.Default()
	m.Workspace = ws
	m.Home = homeDir
	m.Cwd = "/workspace/" + filepath.Base(ws[0])
	m.Sockets[sock] = "/run/twinbox/broker.sock"
	if mcpSockHost != "" {
		m.Sockets[mcpSockHost] = "/run/twinbox/mcp.sock"
	}
	m.Bins[shim] = "/run/twinbox/bin/twinbox-shim"
	m.Env = env

	var cmd []string
	if *execCmd != "" {
		cmd = []string{"/bin/sh", "-c", *execCmd}
	} else if ad.Name == "fake" {
		fake := os.Getenv("TWINBOX_FAKE_AGENT")
		if fake == "" {
			fake = "testdata/fake-agent.sh"
		}
		cmd = []string{"/run/twinbox/bin/twinbox-shim", "bridge", "--", "/bin/sh", fake}
	} else {
		bin, err := exec.LookPath(ad.Binary)
		if err != nil {
			return fmt.Errorf("找不到 %s 可执行文件（沙箱内需要它）", ad.Binary)
		}
		m.Bins[bin] = "/run/twinbox/bin/" + filepath.Base(bin)
		cmd = []string{"/run/twinbox/bin/twinbox-shim", "bridge", "--", "/run/twinbox/bin/" + filepath.Base(bin)}
	}

	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return fmt.Errorf("找不到 bwrap，请先安装 bubblewrap")
	}
	argv := cmdArgs(sandbox.Args(m), cmd)
	c := exec.Command(bwrap, argv...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// cmdArgs 把 bwrap 参数与要执行的命令拼起来（sandbox.Args 已以 "--" 结尾）。
func cmdArgs(args []string, cmd []string) []string {
	out := make([]string, 0, len(args)+len(cmd))
	out = append(out, args...)
	out = append(out, cmd...)
	return out
}

func defaultHome(workspace string) string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/twinbox-home"
		}
		base = filepath.Join(home, ".local", "share")
	}
	sum := sha1.Sum([]byte(workspace))
	return filepath.Join(base, "twinbox", "homes", hex.EncodeToString(sum[:6]))
}

