// Package store 管理宿主侧的密钥存储。值只存在于宿主，永不进入沙箱。
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store 是一个极简的键值存储，落盘为 0600 的 JSON。
type Store struct {
	path string
	data map[string]string
}

// DefaultPath 返回默认存储路径。
func DefaultPath() string {
	if v := os.Getenv("TWINBOX_SECRETS"); v != "" {
		return v
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "secrets.json"
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "twinbox", "secrets.json")
}

// Load 读取存储；文件不存在时返回空存储。
func Load(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Get 读取一个值。
func (s *Store) Get(name string) (string, bool) {
	v, ok := s.data[name]
	return v, ok
}

// Set 写入并落盘。
func (s *Store) Set(name, value string) error {
	s.data[name] = value
	return s.flush()
}

// SetVolatile 只在内存中注入，不落盘。用于从现有配置派生出的密钥。
func (s *Store) SetVolatile(name, value string) {
	s.data[name] = value
}

// Delete 删除一个值。
func (s *Store) Delete(name string) error {
	delete(s.data, name)
	return s.flush()
}

// Names 返回已存在的键名（排序）。
func (s *Store) Names() []string {
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Resolve 解析 "@name" 形式的引用。
func (s *Store) Resolve(ref string) (string, bool) {
	if !strings.HasPrefix(ref, "@") {
		return ref, true
	}
	return s.Get(strings.TrimPrefix(ref, "@"))
}

func (s *Store) flush() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
