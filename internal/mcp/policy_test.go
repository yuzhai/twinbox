package mcp

import "testing"

func TestToolAllowed(t *testing.T) {
	cases := []struct {
		tool  string
		allow []string
		deny  []string
		want  bool
	}{
		{"search", nil, nil, true},
		{"delete_repo", nil, []string{"delete_*"}, false},
		{"search", []string{"search*"}, nil, true},
		{"delete_repo", []string{"search*"}, nil, false},
		{"delete_repo", []string{"*"}, []string{"delete_*"}, false}, // deny 优先
	}
	for _, c := range cases {
		if got := ToolAllowed(c.tool, c.allow, c.deny); got != c.want {
			t.Errorf("ToolAllowed(%q, %v, %v) = %v, want %v", c.tool, c.allow, c.deny, got, c.want)
		}
	}
}
