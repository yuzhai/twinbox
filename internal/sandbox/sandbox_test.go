package sandbox

import (
	"strings"
	"testing"
)

func TestArgsContainHardBoundaries(t *testing.T) {
	m := Default()
	m.Workspace = []string{"/tmp/proj"}
	m.Home = "/tmp/home"
	m.Cwd = "/workspace/proj"
	m.Command = []string{"/bin/true"}
	args := Args(m)
	joined := strings.Join(args, " ")

	must := []string{"--unshare-all", "--clearenv", "--cap-drop ALL", "--die-with-parent", "--chdir"}
	for _, s := range must {
		if !strings.Contains(joined, s) {
			t.Errorf("缺少边界参数 %q", s)
		}
	}
}

func TestArgsDoNotMountHostHome(t *testing.T) {
	m := Default()
	m.Command = []string{"/bin/true"}
	joined := strings.Join(Args(m), " ")
	if strings.Contains(joined, "--bind /home") || strings.Contains(joined, "--ro-bind /home ") {
		t.Fatalf("不应挂载宿主 home: %s", joined)
	}
	if strings.Contains(joined, "docker.sock") {
		t.Fatal("不应挂载 docker.sock")
	}
}

func TestWorkspaceIsMountedReadWrite(t *testing.T) {
	m := Default()
	m.Workspace = []string{"/tmp/proj"}
	m.Command = []string{"/bin/true"}
	joined := strings.Join(Args(m), " ")
	if !strings.Contains(joined, "--bind /tmp/proj /workspace/proj") {
		t.Fatalf("工作区挂载缺失: %s", joined)
	}
}
