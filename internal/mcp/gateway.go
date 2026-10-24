// Package mcp 在宿主侧托管 MCP server 进程并持有它们的凭据。
// 沙箱内只有 twinbox-shim，通过 unix socket 转发 stdio。
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuzhai/twinbox/internal/audit"
	"github.com/yuzhai/twinbox/internal/store"
)

// ServerSpec 描述一个 MCP server 的启动方式。
type ServerSpec struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
	Allow   []string          `json:"allow,omitempty"` // 允许的工具名 glob
	Deny    []string          `json:"deny,omitempty"`  // 拒绝的工具名 glob，优先
}

// Gateway 监听 unix socket，按名字拉起对应的 MCP server。
type Gateway struct {
	Servers map[string]ServerSpec
	Secrets *store.Store
	Audit   *audit.Log
}

type rpcMsg struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		Name string `json:"name"`
	} `json:"params"`
}

// ServeUnix 在 socket 上接受连接。每条连接的首行是 server 名。
func (g *Gateway) ServeUnix(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go g.handle(c)
	}
}

func (g *Gateway) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Time{})

	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	name := strings.TrimSpace(line)
	spec, ok := g.Servers[name]
	if !ok || len(spec.Command) == 0 {
		fmt.Fprintf(c, "{\"error\":\"unknown mcp server\"}\n")
		return
	}

	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + os.TempDir()}
	for k, v := range spec.Env {
		if strings.HasPrefix(v, "@") {
			if g.Secrets == nil {
				continue
			}
			val, ok := g.Secrets.Get(strings.TrimPrefix(v, "@"))
			if !ok {
				fmt.Fprintf(c, "{\"error\":\"missing credential for %s\"}\n", name)
				return
			}
			v = val
		}
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(c, "{\"error\":\"start failed\"}\n")
		return
	}
	go func() {
		sc := bufio.NewScanner(br)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for sc.Scan() {
			line := sc.Bytes()
			var m rpcMsg
			if json.Unmarshal(line, &m) == nil && m.Method == "tools/call" {
				if !ToolAllowed(m.Params.Name, spec.Allow, spec.Deny) {
					g.audit(name, m.Params.Name, "deny")
					fmt.Fprintf(c, "{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":-32601,\"message\":\"twinbox: tool not allowed: %s\"}}\n", idOrNull(m.ID), m.Params.Name)
					continue
				}
				g.audit(name, m.Params.Name, "allow")
			}
			if _, err := stdin.Write(append(line, '\n')); err != nil {
				break
			}
		}
		_ = stdin.Close()
	}()
	_, _ = io.Copy(c, stdout)
	_ = cmd.Wait()
}

func (g *Gateway) audit(server, tool, decision string) {
	if g.Audit == nil {
		return
	}
	_ = g.Audit.Write(audit.Record{
		Host:     "mcp:" + server,
		Method:   "tools/call",
		Path:     tool,
		Decision: decision,
	})
}

func idOrNull(id json.RawMessage) string {
	if len(id) == 0 {
		return "null"
	}
	return string(id)
}
