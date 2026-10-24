// Package admission 实现入箱审查：进入沙箱的内容必须先被检查并留痕。
package admission

import (
	"fmt"
	"strings"

	"github.com/yuzhai/twinbox/internal/scan"
)

// Result 是一次审查的结果。
type Result struct {
	Findings []scan.Finding
}

// Check 对将要挂载进沙箱的目录做检查。
func Check(dirs []string) Result {
	var fs []scan.Finding
	fs = append(fs, scan.Names(dirs)...)
	for _, d := range dirs {
		got, _ := scan.Dir(d)
		fs = append(fs, got...)
	}
	return Result{Findings: fs}
}

// Blocked 表示存在必须显式覆盖才能继续的命中。
func (r Result) Blocked() bool { return len(r.Findings) > 0 }

// Summary 返回适合打印的一行摘要。
func (r Result) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "入箱审查发现 %d 处疑似敏感内容\n", len(r.Findings))
	limit := len(r.Findings)
	if limit > 10 {
		limit = 10
	}
	for _, f := range r.Findings[:limit] {
		if f.Kind == "filename" {
			fmt.Fprintf(&sb, "  - %s（文件名黑名单）\n", f.Path)
			continue
		}
		fmt.Fprintf(&sb, "  - %s:%d  %s\n", f.Path, f.Line, f.Match)
	}
	if len(r.Findings) > limit {
		fmt.Fprintf(&sb, "  …另有 %d 处\n", len(r.Findings)-limit)
	}
	return sb.String()
}
