// Package broker 是沙箱唯一的出站通道：按规则改写请求并注入凭据。
package broker

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuzhai/twinbox/internal/audit"
	"github.com/yuzhai/twinbox/internal/policy"
	"github.com/yuzhai/twinbox/internal/store"
)

// Broker 实现出站代理。
type Broker struct {
	Rules   policy.Set
	Secrets *store.Store
	Audit   *audit.Log
	Client  *http.Client
	Scheme  string // 默认 https；测试用 http
	Session string
	// DefaultAllow 为 true 时，未命中规则的普通主机默认放行（只审计、不注入凭据）。
	// 凭据仍然只对显式规则注入，元数据地址始终拒绝。
	DefaultAllow bool
	DenyHosts    []string
}

// New 构造一个带默认超时的 broker。
func New(rules policy.Set, secrets *store.Store, log *audit.Log) *Broker {
	return &Broker{
		Rules:   rules,
		Secrets: secrets,
		Audit:   log,
		Scheme:       "https",
		DefaultAllow: true,
		DenyHosts:    []string{"169.254.169.254", "metadata.google.internal", "100.100.100.200", "127.0.0.1", "localhost", "::1"},
		Client: &http.Client{
			Timeout: 120 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ServeUnix 在 unix socket 上提供服务。
func (b *Broker) ServeUnix(path string) error {
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
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 30 * time.Second}
	return srv.Serve(l)
}

func (b *Broker) record(r audit.Record) {
	if b.Audit != nil {
		r.Session = b.Session
		_ = b.Audit.Write(r)
	}
}

func hostOnly(h string) string {
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		return h[:i]
	}
	return h
}

// ServeHTTP 处理一条请求。
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		b.handleConnect(w, r)
		return
	}

	host := hostOnly(r.Host)
	rule, ok := b.Rules.Match(host)
	if !ok {
		if b.denied(host) {
			b.record(audit.Record{Host: host, Method: r.Method, Path: r.URL.Path, Status: 403, Decision: "deny", Reason: "denylisted host"})
			writeErr(w, http.StatusForbidden, "twinbox: host denied: "+host)
			return
		}
		if !b.DefaultAllow {
			b.record(audit.Record{Host: host, Method: r.Method, Path: r.URL.Path, Status: 403, Decision: "deny", Reason: "no rule"})
			writeErr(w, http.StatusForbidden, "twinbox: host not allowed: "+host)
			return
		}
		b.forwardPlain(w, r, host)
		return
	}

	outPath := rule.Strip(r.URL.Path)
	if !rule.Allowed(r.Method, outPath) {
		b.record(audit.Record{Host: host, Method: r.Method, Path: outPath, Status: 403, Decision: "deny", Reason: "path not allowed"})
		writeErr(w, http.StatusForbidden, "twinbox: path not allowed: "+r.Method+" "+outPath)
		return
	}

	upHost := rule.Upstream
	if upHost == "" {
		upHost = host
	}
	scheme := b.Scheme
	if scheme == "" {
		scheme = "https"
	}
	if strings.HasPrefix(upHost, "http://") {
		scheme, upHost = "http", strings.TrimPrefix(upHost, "http://")
	} else if strings.HasPrefix(upHost, "https://") {
		scheme, upHost = "https", strings.TrimPrefix(upHost, "https://")
	}
	target := scheme + "://" + upHost + outPath
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	var body io.Reader
	if r.Body != nil {
		body = r.Body
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "twinbox: bad request")
		return
	}
	for k, vs := range r.Header {
		if isHopByHop(k) || strings.EqualFold(k, "Host") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, v := range rule.Inject {
		val, ok := b.injectValue(v)
		if !ok {
			b.record(audit.Record{Host: host, Method: r.Method, Path: outPath, Status: 403, Decision: "deny", Reason: "missing secret for " + k})
			writeErr(w, http.StatusForbidden, "twinbox: missing credential for "+host)
			return
		}
		req.Header.Set(k, val)
	}

	resp, err := b.Client.Do(req)
	if err != nil {
		b.record(audit.Record{Host: host, Method: r.Method, Path: outPath, Status: 502, Decision: "allow", Reason: "upstream error: " + err.Error()})
		writeErr(w, http.StatusBadGateway, "twinbox: upstream error")
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		if isHopByHop(k) {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	b.record(audit.Record{Host: host, Method: r.Method, Path: outPath, Status: resp.StatusCode, BytesOut: n, Decision: "allow"})
}

// forwardPlain 处理未命中规则的主机：只转发、不注入，并记录原因。
func (b *Broker) forwardPlain(w http.ResponseWriter, r *http.Request, host string) {
	scheme := r.Header.Get("X-Twinbox-Scheme")
	if scheme == "" {
		scheme = b.Scheme
		if scheme == "" {
			scheme = "https"
		}
	}
	target := scheme + "://" + host + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "twinbox: bad request")
		return
	}
	for k, vs := range r.Header {
		if isHopByHop(k) || strings.EqualFold(k, "Host") || strings.HasPrefix(strings.ToLower(k), "x-twinbox-") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := b.Client.Do(req)
	if err != nil {
		b.record(audit.Record{Host: host, Method: r.Method, Path: r.URL.Path, Status: 502, Decision: "allow", Reason: "default-allow; upstream error"})
		writeErr(w, http.StatusBadGateway, "twinbox: upstream error")
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		if isHopByHop(k) {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	b.record(audit.Record{Host: host, Method: r.Method, Path: r.URL.Path, Status: resp.StatusCode, BytesOut: n, Decision: "allow", Reason: "default-allow"})
}

func (b *Broker) denied(host string) bool {
	h := strings.ToLower(host)
	for _, d := range b.DenyHosts {
		d = strings.ToLower(d)
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// handleConnect 只做透传，不注入：用于包管理器这类不需要凭据的出站。
func (b *Broker) handleConnect(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	host := hostOnly(target)
	rule, ok := b.Rules.Match(host)
	if b.denied(host) || (ok && !rule.Allowed("CONNECT", "/")) || (!ok && !b.DefaultAllow) {
		b.record(audit.Record{Host: host, Method: "CONNECT", Status: 403, Decision: "deny", Reason: "connect not allowed"})
		writeErr(w, http.StatusForbidden, "twinbox: connect not allowed")
		return
	}
	up := target
	if !strings.Contains(up, ":") {
		up += ":443"
	}
	conn, err := net.DialTimeout("tcp", up, 15*time.Second)
	if err != nil {
		b.record(audit.Record{Host: host, Method: "CONNECT", Status: 502, Decision: "allow", Reason: err.Error()})
		writeErr(w, http.StatusBadGateway, "twinbox: dial failed")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		conn.Close()
		writeErr(w, http.StatusInternalServerError, "twinbox: cannot hijack")
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		conn.Close()
		return
	}
	defer client.Close()
	defer conn.Close()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	b.record(audit.Record{Host: host, Method: "CONNECT", Status: 200, Decision: "allow"})
	go io.Copy(conn, client)
	_, _ = io.Copy(client, conn)
}

func (b *Broker) injectValue(v string) (string, bool) {
	parts := strings.Split(v, " ")
	for i, p := range parts {
		if strings.HasPrefix(p, "@") {
			if b.Secrets == nil {
				return "", false
			}
			val, ok := b.Secrets.Get(strings.TrimPrefix(p, "@"))
			if !ok {
				return "", false
			}
			parts[i] = val
		}
	}
	return strings.Join(parts, " "), true
}

func isHopByHop(k string) bool {
	switch strings.ToLower(k) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	}
	return false
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
