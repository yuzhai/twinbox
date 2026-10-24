package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuzhai/twinbox/internal/agent"
	"github.com/yuzhai/twinbox/internal/store"
)

type check struct {
	name   string
	ok     bool
	detail string
	hard   bool
}

func cmdDoctor(args []string) error {
	var cs []check

	cs = append(cs, checkBwrap())
	cs = append(cs, checkDir("运行时目录", filepath.Join(os.TempDir())))
	cs = append(cs, checkDir("配置目录", filepath.Dir(store.DefaultPath())))
	cs = append(cs, checkPath("审计目录", filepath.Dir(defaultAuditPath())))

	if _, err := os.Stat(store.DefaultPath()); err == nil {
		st, err := store.Load(store.DefaultPath())
		if err != nil {
			cs = append(cs, check{"密钥库", false, err.Error(), false})
		} else {
			cs = append(cs, check{"密钥库", true, fmt.Sprintf("%d 项", len(st.Names())), false})
		}
	} else {
		cs = append(cs, check{"密钥库", false, "尚未创建（首次 run 时会用到）", false})
	}

	cs = append(cs, checkBinary("twinbox-shim"))
	for _, name := range []string{"claude", "codex", "opencode"} {
		if ad, ok := agent.Get(name); ok {
			cs = append(cs, checkBinary(ad.Binary))
		}
	}

	failed := false
	hardFailed := false
	for _, c := range cs {
		mark := "PASS"
		if !c.ok {
			mark = "FAIL"
			failed = true
			if c.hard {
				hardFailed = true
			}
		}
		fmt.Printf("%-5s %-16s %s\n", mark, c.name, c.detail)
	}
	if hardFailed {
		return fmt.Errorf("存在必须修复的问题")
	}
	if failed {
		fmt.Println("\n以上 FAIL 不阻塞运行，但会影响对应功能。")
	}
	return nil
}

func checkBwrap() check {
	p, err := exec.LookPath("bwrap")
	if err != nil {
		return check{"bubblewrap", false, "未安装", true}
	}
	out, err := exec.Command(p, "--version").CombinedOutput()
	if err != nil {
		return check{"bubblewrap", false, err.Error(), true}
	}
	// 关键能力：非特权 user namespace + 网络命名空间
	probe := exec.Command(p, "--unshare-user", "--unshare-net", "--ro-bind", "/", "/", "--", "/bin/true")
	if err := probe.Run(); err != nil {
		return check{"bubblewrap", false, strings.TrimSpace(string(out)) + "（userns 不可用）", true}
	}
	return check{"bubblewrap", true, strings.TrimSpace(string(out)), true}
}

func checkDir(name, p string) check {
	if err := os.MkdirAll(p, 0o700); err != nil {
		return check{name, false, err.Error(), false}
	}
	f, err := os.CreateTemp(p, ".probe-*")
	if err != nil {
		return check{name, false, "不可写: " + p, false}
	}
	f.Close()
	os.Remove(f.Name())
	return check{name, true, p, false}
}

func checkPath(name, p string) check {
	if err := os.MkdirAll(p, 0o700); err != nil {
		return check{name, false, err.Error(), false}
	}
	return check{name, true, p, false}
}

func checkBinary(name string) check {
	if name == "" {
		return check{"agent", false, "未配置", false}
	}
	if p, err := exec.LookPath(name); err == nil {
		return check{name, true, p, false}
	}
	return check{name, false, "未安装（不影响其他 agent）", false}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|passwd)\s*[:=]\s*["']?[A-Za-z0-9_\-\.]{12,}`),
}

// cmdProve 扫描目录，判断是否可以公开。
func cmdProve(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	var hits int
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2<<20 {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		line := 0
		for sc.Scan() {
			line++
			for _, re := range secretPatterns {
				m := re.FindString(sc.Text())
				if m == "" {
					continue
				}
				if strings.Contains(strings.ToLower(m), "placeholder") {
					continue // 占位符是有意为之，不算命中
				}
				fmt.Printf("%s:%d 命中 %s\n", p, line, m)
				hits++
				break
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if hits > 0 {
		fmt.Printf("\n发现 %d 处疑似密钥，不可公开\n", hits)
		return fmt.Errorf("可公开性检查未通过")
	}
	fmt.Println("可公开性检查通过：未发现疑似密钥")
	return nil
}
