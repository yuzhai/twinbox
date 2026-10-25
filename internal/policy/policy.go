// Package policy 决定一次出站请求是否被允许，以及需要注入哪些凭据。
package policy

import (
	"regexp"
	"strings"
)

// match 实现 glob：* 匹配任意字符（含 /），? 匹配单个字符。
// 不用 path.Match，因为它的 * 不跨目录分隔符。
func match(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	ok, _ := regexp.MatchString(b.String(), s)
	return ok
}

// Rule 描述一个上游目标的访问策略。
type Rule struct {
	Host     string            // 虚拟主机名，例如 api.example.com
	Prefix   string            // 沙箱侧路径前缀，例如 /anthropic
	Inject   map[string]string // header -> "@secret-name"
	Allow    []string          // "METHOD /path/glob"，为空表示全部允许
	Upstream string            // 上游地址覆盖（测试用），为空则用 Host
}

// Set 是规则集合，按精确主机、再按后缀主机匹配。
type Set struct {
	Rules []Rule
}

// Match 返回匹配的规则。
func (s Set) Match(host string) (Rule, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	for _, r := range s.Rules {
		if r.Host != "" && strings.EqualFold(r.Host, host) {
			return r, true
		}
	}
	for _, r := range s.Rules {
		if strings.HasPrefix(r.Host, ".") && strings.HasSuffix(host, r.Host) {
			return r, true
		}
	}
	return Rule{}, false
}

// Strip 去掉沙箱侧前缀。
func (r Rule) Strip(p string) string {
	if r.Prefix == "" {
		return p
	}
	if strings.HasPrefix(p, r.Prefix) {
		p = strings.TrimPrefix(p, r.Prefix)
		if p == "" {
			return "/"
		}
	}
	return p
}

// Allowed 判断方法与路径是否在允许范围内。
func (r Rule) Allowed(method, p string) bool {
	if len(r.Allow) == 0 {
		return true
	}
	for _, a := range r.Allow {
		parts := strings.Fields(a)
		if len(parts) != 2 {
			continue
		}
		if parts[0] != "*" && !strings.EqualFold(parts[0], method) {
			continue
		}
		if match(parts[1], p) {
			return true
		}
	}
	return false
}

// Default 返回内置规则。不使用任何配置文件。
func Default() Set {
	return Set{Rules: []Rule{
		{
			Host:   "api.anthropic.com",
			Prefix: "/anthropic",
			Inject: map[string]string{"x-api-key": "@anthropic"},
			Allow:  []string{"POST /v1/messages", "GET /v1/models", "POST /v1/messages/count_tokens"},
		},
		{
			Host:   "api.openai.com",
			Prefix: "/openai",
			Inject: map[string]string{"authorization": "Bearer @openai"},
			Allow:  []string{"POST /v1/*", "GET /v1/*"},
		},
		{
			Host:   "registry.npmjs.org",
			Prefix: "/npm",
			Allow:  []string{"GET *", "HEAD *", "CONNECT *"},
		},
		{
			Host:  "github.com",
			Allow: []string{"CONNECT *"},
		},
		{
			Host:  "api.github.com",
			Allow: []string{"GET /repos/*", "POST /repos/*", "GET /user"},
		},
	}}
}

// WithCompat 追加一个 OpenAI 兼容网关（主机名由调用方推导，不写死）。
func (s Set) WithCompat(host string) Set {
	if host == "" {
		return s
	}
	out := Set{Rules: append([]Rule{}, s.Rules...)}
	out.Rules = append(out.Rules, Rule{
		Host:   host,
		Prefix: "/compat",
		Inject: map[string]string{"authorization": "Bearer @openai"},
		Allow:  []string{"POST /chat/completions", "POST /v1/chat/completions", "GET /models", "GET /v1/models"},
	})
	return out
}
