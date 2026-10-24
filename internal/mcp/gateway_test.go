package mcp

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuzhai/twinbox/internal/store"
)

func TestGatewayInjectsCredentialAndTunnels(t *testing.T) {
	dir := t.TempDir()
	fake := "../../testdata/fake-mcp.sh"
	if _, err := os.Stat(fake); err != nil {
		t.Skip("缺少 testdata/fake-mcp.sh")
	}

	sp := filepath.Join(dir, "secrets.json")
	st, err := store.Load(sp)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set("github", "secret-token"); err != nil {
		t.Fatal(err)
	}

	gw := &Gateway{
		Servers: map[string]ServerSpec{
			"echo": {Command: []string{"/bin/sh", fake}, Env: map[string]string{"GITHUB_TOKEN": "@github"}},
		},
		Secrets: st,
	}
	sock := filepath.Join(dir, "mcp.sock")
	go func() { _ = gw.ServeUnix(sock) }()
	for i := 0; i < 100; i++ {
		if _, err := net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("echo\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("{\"method\":\"initialize\"}\n")); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `"hasCredential":true`) {
		t.Fatalf("凭据未注入: %s", line)
	}
}

func TestGatewayRejectsUnknownServer(t *testing.T) {
	dir := t.TempDir()
	gw := &Gateway{Servers: map[string]ServerSpec{}}
	sock := filepath.Join(dir, "mcp.sock")
	go func() { _ = gw.ServeUnix(sock) }()
	for i := 0; i < 100; i++ {
		if _, err := net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = c.Write([]byte("nope\n"))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, _ := bufio.NewReader(c).ReadString('\n')
	if !strings.Contains(line, "unknown") {
		t.Fatalf("应拒绝未知 server，得到 %q", line)
	}
}
