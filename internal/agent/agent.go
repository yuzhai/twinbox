// Package agent 生成各 agent 的沙箱侧配置。配置里只出现占位符与本地地址。
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Context 是生成配置所需的上下文。
type Context struct {
	Home    string // 沙箱内 home（宿主路径）
	BaseURL string // 例如 http://127.0.0.1:8899
	Compat  string // OpenAI 兼容网关的虚拟主机名
}

// Adapter 描述一个 agent 的接入方式。
type Adapter struct {
	Name   string
	Binary string
	Env    func(Context) map[string]string
	Write  func(Context) error
}

// Placeholder 是沙箱内唯一允许出现的"密钥"形状。
const Placeholder = "sk-twinbox-placeholder-0000"

// Get 返回指定 agent 的适配器。
func Get(name string) (Adapter, bool) {
	switch strings.ToLower(name) {
	case "claude", "claude-code":
		return Adapter{Name: "claude", Binary: "claude", Env: claudeEnv, Write: claudeWrite}, true
	case "codex":
		return Adapter{Name: "codex", Binary: "codex", Env: codexEnv, Write: codexWrite}, true
	case "opencode", "opencode2":
		return Adapter{Name: "opencode", Binary: "opencode", Env: opencodeEnv, Write: opencodeWrite}, true
	case "fake":
		return Adapter{Name: "fake", Binary: "", Env: fakeEnv}, true
	}
	return Adapter{}, false
}

// List 返回内置适配器名。
func List() []string { return []string{"claude", "codex", "opencode", "fake"} }

func claudeEnv(c Context) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL": c.BaseURL + "/anthropic",
		"ANTHROPIC_API_KEY":  Placeholder,
	}
}

func claudeWrite(c Context) error {
	dir := filepath.Join(c.Home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// 只写非敏感的客户端设置；密钥通过环境变量传入占位符。
	return os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{\n  \"includeCoAuthoredBy\": false\n}\n"), 0o600)
}

func codexEnv(c Context) map[string]string {
	return map[string]string{
		"OPENAI_API_KEY":  Placeholder,
		"OPENAI_BASE_URL": c.BaseURL + "/openai",
	}
}

func codexWrite(c Context) error {
	dir := filepath.Join(c.Home, ".codex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	conf := fmt.Sprintf(`model_provider = "twinbox"

[model_providers.twinbox]
name = "twinbox"
base_url = "%s/openai"
wire_api = "responses"
`, c.BaseURL)
	return os.WriteFile(filepath.Join(dir, "config.toml"), []byte(conf), 0o600)
}

func opencodeEnv(c Context) map[string]string {
	return map[string]string{"TWINBOX_BASE_URL": c.BaseURL}
}

func opencodeWrite(c Context) error {
	dir := filepath.Join(c.Home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	conf := fmt.Sprintf(`{
  "$schema": "https://opencode.ai/config.json",
  "providers": {
    "twinbox": {
      "name": "TwinBox",
      "package": "aisdk:@ai-sdk/openai-compatible",
      "settings": {
        "baseURL": "%s/compat",
        "apiKey": "%s"
      }
    }
  },
  "mcp": { "servers": {} }
}
`, c.BaseURL, Placeholder)
	return os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte(conf), 0o600)
}

func fakeEnv(c Context) map[string]string {
	return map[string]string{
		"TWINBOX_BASE_URL":  c.BaseURL + "/anthropic",
		"ANTHROPIC_API_KEY": Placeholder,
	}
}
