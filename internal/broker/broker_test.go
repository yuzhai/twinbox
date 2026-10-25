package broker

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yuzhai/twinbox/internal/audit"
	"github.com/yuzhai/twinbox/internal/policy"
	"github.com/yuzhai/twinbox/internal/store"
)

func startBroker(t *testing.T, rules policy.Set, secrets map[string]string) (sock, auditPath string, b *Broker, stop func()) {
	t.Helper()
	dir := t.TempDir()
	sp := filepath.Join(dir, "secrets.json")
	st, err := store.Load(sp)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range secrets {
		if err := st.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	ap := filepath.Join(dir, "audit.jsonl")
	al, err := audit.Open(ap)
	if err != nil {
		t.Fatal(err)
	}
	b = New(rules, st, al)
	b.Scheme = "http"
	s := filepath.Join(dir, "b.sock")
	go func() { _ = b.ServeUnix(s) }()
	for i := 0; i < 100; i++ {
		if _, err := net.Dial("unix", s); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return s, ap, b, func() { al.Close() }
}

func clientFor(sock string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}
}

func TestInjectsCredentialAndAudits(t *testing.T) {
	var gotKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		if r.URL.Path != "/v1/messages" {
			t.Errorf("上游路径 = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()

	rules := policy.Set{Rules: []policy.Rule{{
		Host:     "api.anthropic.com",
		Prefix:   "/anthropic",
		Inject:   map[string]string{"x-api-key": "@anthropic"},
		Allow:    []string{"POST /v1/messages"},
		Upstream: strings.TrimPrefix(up.URL, "http://"),
	}}}
	sock, ap, _, stop := startBroker(t, rules, map[string]string{"anthropic": "REAL-KEY"})
	defer stop()

	req, _ := http.NewRequest("POST", "http://api.anthropic.com/anthropic/v1/messages", strings.NewReader(`{}`))
	resp, err := clientFor(sock).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gotKey != "REAL-KEY" {
		t.Fatalf("上游收到的 key = %q，注入失败", gotKey)
	}

	recs, err := audit.ReadAll(ap)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Decision != "allow" {
		t.Fatalf("审计记录 = %+v", recs)
	}
}

func TestDenylistedHostAlwaysDenied(t *testing.T) {
	rules := policy.Default()
	sock, ap, _, stop := startBroker(t, rules, nil)
	defer stop()

	resp, err := clientFor(sock).Get("http://169.254.169.254/latest/meta-data/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	recs, _ := audit.ReadAll(ap)
	if len(recs) != 1 || recs[0].Decision != "deny" {
		t.Fatalf("审计记录 = %+v", recs)
	}
}

func TestUnknownHostDeniedWhenDefaultAllowOff(t *testing.T) {
	rules := policy.Default()
	sock, _, b, stop := startBroker(t, rules, nil)
	defer stop()
	b.DefaultAllow = false

	resp, err := clientFor(sock).Get("http://unknown.example/x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("DefaultAllow=false 时应拒绝，status=%d", resp.StatusCode)
	}
}

func TestDeniesDisallowedPath(t *testing.T) {
	rules := policy.Default()
	sock, _, _, stop := startBroker(t, rules, map[string]string{"anthropic": "REAL"})
	defer stop()

	resp, err := clientFor(sock).Post("http://api.anthropic.com/anthropic/v1/admin", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestMissingSecretDenies(t *testing.T) {
	rules := policy.Default()
	sock, _, _, stop := startBroker(t, rules, nil)
	defer stop()

	resp, err := clientFor(sock).Post("http://api.anthropic.com/anthropic/v1/messages", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("缺少凭据时应拒绝，status = %d", resp.StatusCode)
	}
}
