package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKey 在运行时拼出密钥形状的字符串，避免仓库里出现字面量。
func fakeKey() string { return "sk-" + strings.Repeat("A1", 12) }

func TestDirFindsKeyAndSkipsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "leak.txt"), []byte("x\n"+fakeKey()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("sk-twinbox-placeholder-0000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("命中数 = %d，期望 1: %+v", len(found), found)
	}
}

func TestNamesDetectsDeniedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Names([]string{dir})
	if len(got) != 1 || got[0].Kind != "filename" {
		t.Fatalf("期望命中 .env: %+v", got)
	}
}

func TestDirSkipsGitDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("token="+fakeKey()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, _ := Dir(dir)
	if len(found) != 0 {
		t.Fatalf(".git 应被跳过: %+v", found)
	}
}
