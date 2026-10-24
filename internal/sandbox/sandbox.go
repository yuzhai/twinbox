// Package sandbox 负责生成 bwrap 的启动参数。它不执行任何东西，只描述边界。
package sandbox

import (
	"os"
	"path/filepath"
	"sort"
)

// Manifest 描述一次沙箱运行看到的世界。
type Manifest struct {
	Profile   string            // rootfs 目录；为空则用 HostRoots
	HostRoots []string          // 例如 /usr /bin /lib /lib64
	ReadOnly  []string          // 额外只读路径
	Workspace []string          // 宿主目录，挂到 /workspace/<base>
	Home      string            // 持久 home（宿主路径）
	Sockets   map[string]string // 宿主 socket -> 沙箱内路径
	Bins      map[string]string // 宿主二进制 -> 沙箱内路径
	Env       map[string]string
	Command   []string
	HostName  string
	Cwd       string
	AllowNet  bool
}

// Default 返回一份最小可用的清单：宿主只读根 + 空 home。
func Default() Manifest {
	return Manifest{
		HostRoots: []string{"/usr", "/bin", "/lib", "/lib64"},
		ReadOnly: []string{
			"/etc/ssl", "/etc/ld.so.cache", "/etc/localtime",
			"/etc/passwd", "/etc/group", "/etc/nsswitch.conf",
		},
		Sockets: map[string]string{},
		Bins:    map[string]string{},
		Env:     map[string]string{},
	}
}

// Args 生成 bwrap 参数（不含 bwrap 本身）。
func Args(m Manifest) []string {
	var a []string
	add := func(s ...string) { a = append(a, s...) }

	add("--die-with-parent", "--new-session", "--clearenv")
	add("--unshare-all", "--cap-drop", "ALL")
	if m.Profile != "" {
		// 镜像根以 overlay 挂载：可创建挂载点与临时写入，改动退出即丢弃。
		add("--overlay-src", m.Profile, "--tmp-overlay", "/")
	}
	add("--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp")
	add("--tmpfs", "/run", "--dir", "/run/twinbox")

	roots := m.HostRoots
	if m.Profile != "" {
		roots = nil
	}
	for _, r := range roots {
		if exists(r) {
			add("--ro-bind", r, r)
		}
	}
	// profile 模式下根来自镜像，宿主侧的只读补丁不再挂载（镜像自带 /etc/ssl 等）。
	if m.Profile == "" {
		for _, r := range m.ReadOnly {
			if exists(r) {
				add("--ro-bind", r, r)
			}
		}
	}

	path := "/usr/local/bin:/usr/bin:/bin"
	dirs := []string{"/run/twinbox/bin"}
	if m.Profile == "" {
		dirs = append([]string{"/opt/toolchain/bin"}, dirs...)
	}
	for _, dir := range dirs {
		add("--dir", dir)
		path = dir + ":" + path
	}
	for host, in := range m.Bins {
		add("--ro-bind", host, in)
	}

	for _, w := range m.Workspace {
		base := filepath.Base(w)
		dest := "/workspace/" + base
		add("--dir", "/workspace", "--dir", dest, "--bind", w, dest)
	}
	if len(m.Workspace) == 0 {
		add("--dir", "/workspace")
	}

	home := m.Home
	if home != "" {
		add("--bind", home, "/home/agent")
	} else {
		add("--tmpfs", "/home/agent")
	}

	keys := make([]string, 0, len(m.Sockets))
	for k := range m.Sockets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add("--bind", k, m.Sockets[k])
	}

	name := m.HostName
	if name == "" {
		name = "twinbox"
	}
	add("--hostname", name)
	add("--setenv", "HOME", "/home/agent")
	add("--setenv", "PATH", path)
	add("--setenv", "TERM", "xterm-256color")
	add("--setenv", "TWINBOX", "1")
	if m.Cwd != "" {
		add("--chdir", m.Cwd)
	}

	envKeys := make([]string, 0, len(m.Env))
	for k := range m.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		add("--setenv", k, m.Env[k])
	}

	add("--")
	add(m.Command...)
	return a
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
