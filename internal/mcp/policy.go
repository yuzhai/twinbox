package mcp

import (
	"path"
	"strings"
)

// ToolAllowed 判断一次工具调用是否放行。拒绝规则优先于允许规则。
// 空 allow 表示不限制。
func ToolAllowed(tool string, allow, deny []string) bool {
	for _, p := range deny {
		if ok, _ := path.Match(p, tool); ok {
			return false
		}
	}
	if len(allow) == 0 {
		return true
	}
	for _, p := range allow {
		if ok, _ := path.Match(p, tool); ok {
			return true
		}
	}
	return false
}

// Describe 返回一行策略摘要，供 doctor 与审计使用。
func Describe(allow, deny []string) string {
	var parts []string
	if len(allow) > 0 {
		parts = append(parts, "allow="+strings.Join(allow, ","))
	}
	if len(deny) > 0 {
		parts = append(parts, "deny="+strings.Join(deny, ","))
	}
	if len(parts) == 0 {
		return "allow=*"
	}
	return strings.Join(parts, " ")
}
