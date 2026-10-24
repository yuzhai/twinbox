// Package derive 从用户已有的客户端配置里读出 base URL 与密钥，避免重复录入。
// 读取到的密钥只在内存中使用，不落盘、不打印。
package derive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Provider 是从现有配置推导出的接入信息。
type Provider struct {
	BaseURL string
	APIKey  string
	Models  []string
}

var (
	lineComment  = regexp.MustCompile(`(?m)^\s*//.*$`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// Opencode 读取 opencode 的配置文件（JSONC）。
func Opencode(path string) (*Provider, error) { return OpencodeProvider(path, "") }

// OpencodeProvider 优先返回指定名字的 provider；未指定时按名字排序取第一个可用的。
func OpencodeProvider(path, want string) (*Provider, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := blockComment.ReplaceAllString(string(raw), "")
	text = lineComment.ReplaceAllString(text, "")

	var doc struct {
		Providers map[string]struct {
			Settings struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"settings"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	if want != "" {
		if p, ok := doc.Providers[want]; ok && p.Settings.BaseURL != "" && p.Settings.APIKey != "" {
			return toProvider(p.Settings.BaseURL, p.Settings.APIKey, p.Models), nil
		}
	}
	names := make([]string, 0, len(doc.Providers))
	for name := range doc.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := doc.Providers[name]
		if p.Settings.BaseURL == "" || p.Settings.APIKey == "" {
			continue
		}
		return toProvider(p.Settings.BaseURL, p.Settings.APIKey, p.Models), nil
	}
	return nil, os.ErrNotExist
}

func toProvider(base, key string, models map[string]json.RawMessage) *Provider {
	out := &Provider{BaseURL: base, APIKey: key}
	names := make([]string, 0, len(models))
	for n := range models {
		names = append(names, n)
	}
	sort.Strings(names)
	out.Models = names
	return out
}

// Host 返回 base URL 的主机名（含端口）。
func (p *Provider) Host() string {
	s := p.BaseURL
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/"); i >= 0 {
		s = s[:i]
	}
	return s
}
