// twinbox-shim 运行在沙箱内：把 127.0.0.1 上的请求接到宿主 broker，
// 自身不持有任何秘密。
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: twinbox-shim <bridge|mcp|version>\n")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "bridge":
		if err := bridge(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "twinbox-shim: %v\n", err)
			os.Exit(1)
		}
	case "mcp":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "用法: twinbox-shim mcp <name>")
			os.Exit(2)
		}
		if err := mcpCmd(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "twinbox-shim: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

// bridge 监听 TWINBOX_ROUTES 中声明的端口，转发到 unix socket。
// 路由格式："8899=api.anthropic.com,8898=registry.npmjs.org"
func bridge(rest []string) error {
	sock := os.Getenv("TWINBOX_SOCK")
	if sock == "" {
		return fmt.Errorf("TWINBOX_SOCK 未设置")
	}
	routes := os.Getenv("TWINBOX_ROUTES")
	if routes == "" {
		routes = "8899=api.anthropic.com"
	}

	for _, r := range strings.Split(routes, ",") {
		parts := strings.SplitN(strings.TrimSpace(r), "=", 2)
		if len(parts) != 2 {
			continue
		}
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return err
		}
		host := parts[1]
		l, err := net.Listen("tcp", "127.0.0.1:"+parts[0])
		if err != nil {
			return fmt.Errorf("listen %d: %w", port, err)
		}
		go serve(l, sock, host)
	}
	if len(rest) > 0 && rest[0] == "--" && len(rest) > 1 {
		c := exec.Command(rest[1], rest[2:]...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	select {}
}

func serve(l net.Listener, sock, virtualHost string) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go handle(c, sock, virtualHost)
	}
}

func handle(c net.Conn, sock, virtualHost string) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	// CONNECT：直接隧道到宿主 broker，由它做策略判定。
	if req.Method == http.MethodConnect {
		up, err := net.DialTimeout("unix", sock, 5*time.Second)
		if err != nil {
			return
		}
		defer up.Close()
		fmt.Fprintf(up, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", req.URL.Host, req.URL.Host)
		ubr := bufio.NewReader(up)
		resp, err := http.ReadResponse(ubr, req)
		if err != nil {
			return
		}
		if resp.StatusCode != 200 {
			fmt.Fprintf(c, "HTTP/1.1 %d %s\r\n\r\n", resp.StatusCode, resp.Status)
			return
		}
		fmt.Fprint(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go io.Copy(up, br)
		_, _ = io.Copy(c, ubr)
		return
	}

	body := req.Body
	target := fmt.Sprintf("http://%s%s", virtualHost, req.URL.RequestURI())
	out, err := http.NewRequest(req.Method, target, body)
	if err != nil {
		return
	}
	out.Host = virtualHost
	for k, vs := range req.Header {
		for _, v := range vs {
			out.Header.Add(k, v)
		}
	}
	client := &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}
	resp, err := client.Do(out)
	if err != nil {
		fmt.Fprint(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer resp.Body.Close()
	_ = resp.Write(c)
}

// mcpCmd 把 stdin/stdout 经 unix socket 接到宿主的 MCP 网关。
func mcpCmd(name string) error {
	sock := os.Getenv("TWINBOX_MCP_SOCK")
	if sock == "" {
		return fmt.Errorf("TWINBOX_MCP_SOCK 未设置")
	}
	c, err := net.DialTimeout("unix", sock, 5*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := fmt.Fprintf(c, "%s\n", name); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(c, os.Stdin)
		done <- struct{}{}
	}()
	_, err = io.Copy(os.Stdout, c)
	return err
}
