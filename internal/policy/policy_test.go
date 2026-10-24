package policy

import "testing"

func TestMatchAndStrip(t *testing.T) {
	s := Default()
	r, ok := s.Match("api.anthropic.com")
	if !ok {
		t.Fatal("应匹配 api.anthropic.com")
	}
	if got := r.Strip("/anthropic/v1/messages"); got != "/v1/messages" {
		t.Fatalf("strip = %q", got)
	}
	if _, ok := s.Match("evil.example"); ok {
		t.Fatal("未授权主机不应匹配")
	}
}

func TestAllowed(t *testing.T) {
	r := Rule{Allow: []string{"POST /v1/messages", "GET /v1/*"}}
	cases := []struct {
		m, p string
		want bool
	}{
		{"POST", "/v1/messages", true},
		{"POST", "/v1/other", false},
		{"GET", "/v1/models", true},
		{"DELETE", "/v1/messages", false},
	}
	for _, c := range cases {
		if got := r.Allowed(c.m, c.p); got != c.want {
			t.Errorf("Allowed(%s %s) = %v, want %v", c.m, c.p, got, c.want)
		}
	}
}

func TestEmptyAllowMeansAll(t *testing.T) {
	r := Rule{}
	if !r.Allowed("DELETE", "/anything") {
		t.Fatal("空 allow 应表示全部允许")
	}
}
