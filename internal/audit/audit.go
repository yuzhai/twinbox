// Package audit 记录每一条出站请求与工具调用，仅追加、仅宿主可写。
package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record 是一条审计记录。
type Record struct {
	TS       string `json:"ts"`
	Session  string `json:"session,omitempty"`
	Host     string `json:"host"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Status   int    `json:"status"`
	BytesOut int64  `json:"bytes_out"`
	Decision string `json:"decision"` // allow | deny
	Reason   string `json:"reason,omitempty"`
}

// Log 是追加式日志。
type Log struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

// Open 打开（或创建）审计文件，权限 0600。
func Open(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Log{f: f, path: path}, nil
}

// Write 追加一条记录。
func (l *Log) Write(r Record) error {
	if r.TS == "" {
		r.TS = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// Close 关闭文件。
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}

// ReadAll 读取全部记录。
func ReadAll(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
