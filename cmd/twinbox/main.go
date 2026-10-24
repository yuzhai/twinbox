// twinbox — 桌面级 agent 沙箱。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yuzhai/twinbox/internal/audit"
	"github.com/yuzhai/twinbox/internal/store"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("twinbox " + version)
	case "run":
		err = cmdRun(args)
	case "doctor":
		err = cmdDoctor(args)
	case "secret":
		err = cmdSecret(args)
	case "log":
		err = cmdLog(args)
	case "prove":
		err = cmdProve(args)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "twinbox: 未知命令 %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "twinbox: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `twinbox — 桌面级 agent 沙箱

用法:
  twinbox run <agent> [--workspace DIR]... [--exec CMD...]
  twinbox doctor
  twinbox secret set <name> <value> | list | rm <name>
  twinbox log [N]
  twinbox prove <dir>
  twinbox version

agent: claude | codex | opencode | fake
`)
}

func cmdSecret(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: twinbox secret <set|list|rm>")
	}
	st, err := store.Load(store.DefaultPath())
	if err != nil {
		return err
	}
	switch args[0] {
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("用法: twinbox secret set <name> <value>")
		}
		if err := st.Set(args[1], args[2]); err != nil {
			return err
		}
		fmt.Printf("已保存 %s（仅宿主可见）\n", args[1])
	case "list":
		names := st.Names()
		if len(names) == 0 {
			fmt.Println("（空）")
			return nil
		}
		for _, n := range names {
			fmt.Println(n)
		}
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("用法: twinbox secret rm <name>")
		}
		if err := st.Delete(args[1]); err != nil {
			return err
		}
		fmt.Printf("已删除 %s\n", args[1])
	default:
		return fmt.Errorf("未知子命令 %q", args[0])
	}
	return nil
}

func cmdLog(args []string) error {
	path := defaultAuditPath()
	recs, err := audit.ReadAll(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("（暂无审计记录）")
			return nil
		}
		return err
	}
	n := len(recs)
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &n)
	}
	if n > len(recs) {
		n = len(recs)
	}
	start := len(recs) - n
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].TS < recs[j].TS })
	for _, r := range recs[start:] {
		fmt.Printf("%s %-6s %-6s %-40s %3d %s %s\n", r.TS, r.Decision, r.Method, r.Host+r.Path, r.Status, humanBytes(r.BytesOut), r.Reason)
	}
	fmt.Printf("\n共 %d 条，显示最近 %d 条\n", len(recs), n)
	return nil
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1fK", float64(n)/1024)
}

func defaultAuditPath() string {
	if v := os.Getenv("TWINBOX_AUDIT"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "twinbox", "audit", "audit.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "audit.jsonl"
	}
	return filepath.Join(home, ".local", "state", "twinbox", "audit", "audit.jsonl")
}
