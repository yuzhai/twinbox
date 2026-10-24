// Package scan 提供密钥模式扫描，供 prove 与入箱审查共用。
package scan

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding 是一处命中。
type Finding struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Match string `json:"match"`
	Kind  string `json:"kind"` // content | filename
}

// Patterns 是高置信度的密钥模式。
var Patterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|passwd)\s*[:=]\s*["']?[A-Za-z0-9_\-\.]{12,}`),
}

// DeniedNames 是默认拒绝进入沙箱的文件名模式。
var DeniedNames = []string{
	".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx",
	"id_rsa", "id_ed25519", "id_ecdsa",
	"credentials.json", "auth.json", ".npmrc", ".netrc", ".git-credentials",
}

// Dir 扫描目录，返回命中。跳过 .git、node_modules、bin 与超大文件。
func Dir(dir string) ([]Finding, error) {
	var out []Finding
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2<<20 || !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		line := 0
		for sc.Scan() {
			line++
			text := sc.Text()
			for _, re := range Patterns {
				m := re.FindString(text)
				if m == "" || strings.Contains(strings.ToLower(m), "placeholder") {
					continue
				}
				out = append(out, Finding{Path: p, Line: line, Match: m, Kind: "content"})
				break
			}
		}
		return nil
	})
	return out, err
}

// Names 检查文件名黑名单（只看顶层与一层子目录，避免遍历成本）。
func Names(dirs []string) []Finding {
	var out []Finding
	for _, d := range dirs {
		for _, pat := range DeniedNames {
			matches, _ := filepath.Glob(filepath.Join(d, pat))
			for _, m := range matches {
				out = append(out, Finding{Path: m, Kind: "filename"})
			}
			nested, _ := filepath.Glob(filepath.Join(d, "*", pat))
			for _, m := range nested {
				out = append(out, Finding{Path: m, Kind: "filename"})
			}
		}
	}
	return out
}
