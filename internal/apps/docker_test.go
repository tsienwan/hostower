package apps

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 检查命令必须是只读的（L0）：它走只读查询。
func TestDockerStatusCommandReadOnly(t *testing.T) {
	if r := risk.New().Classify(DockerStatusCommand); r.Level != risk.L0 {
		t.Fatalf("检查命令应是 L0，得到 %s：%+v", r.Level, r.Findings)
	}
}

func TestDockerInstallCommand(t *testing.T) {
	e := risk.New()
	cases := []struct {
		name, out string
		want      []string
		not       []string
	}{
		{"ubuntu 用 sudo", "sudo\nhas apt-get\nhas systemctl\nhas curl\narch amd64\nos ID=ubuntu\nos ID_LIKE=debian\nos VERSION_CODENAME=noble\nos UBUNTU_CODENAME=noble\n",
			[]string{"sudo -n env DEBIAN_FRONTEND=noninteractive apt-get update -q", "https://download.docker.com/linux/ubuntu noble stable", "arch=amd64", "apt-get install -y -q " + dockerCE, "sudo -n systemctl enable --now docker"}, nil},
		{"debian root", "root\nhas apt-get\nhas systemctl\narch arm64\nos ID=debian\nos VERSION_CODENAME=bookworm\n",
			[]string{"linux/debian bookworm stable", "arch=arm64", "systemctl enable --now docker"}, []string{"sudo"}},
		{"已有 docker 只缺 compose", "docker\nroot\nhas apt-get\nhas systemctl\narch amd64\nos ID=debian\nos VERSION_CODENAME=bookworm\n",
			[]string{"apt-get install -y -q docker-compose-plugin"}, []string{"docker-ce ", "systemctl enable"}},
		{"rocky 用 dnf", "root\nhas dnf\nhas systemctl\nos ID=\"rocky\"\nos ID_LIKE=\"rhel centos fedora\"\n",
			[]string{"dnf install -y -q curl", "linux/centos/docker-ce.repo -o /etc/yum.repos.d/docker-ce.repo", "dnf install -y -q " + dockerCE}, nil},
		{"alpine", "root\nhas apk\nhas rc-service\nos ID=alpine\n",
			[]string{"apk add -q docker docker-cli-compose", "rc-update add docker default", "rc-service docker start"}, nil},
	}
	for _, c := range cases {
		st := ParseDockerStatus(c.out)
		cmd, err := DockerInstallCommand(st)
		if err != nil {
			t.Fatalf("%s：%v", c.name, err)
		}
		for _, w := range c.want {
			if !strings.Contains(cmd, w) {
				t.Errorf("%s：命令里没有 %q：\n%s", c.name, w, cmd)
			}
		}
		for _, w := range c.not {
			if strings.Contains(cmd, w) {
				t.Errorf("%s：命令里不该有 %q：\n%s", c.name, w, cmd)
			}
		}
		if r := e.Classify(cmd); r.Level != risk.L1 {
			t.Errorf("%s：安装命令应是 L1，得到 %s：%+v", c.name, r.Level, r.Findings)
		}
	}
	for _, out := range []string{
		"has apt-get\nos ID=ubuntu\nos VERSION_CODENAME=noble\narch amd64\n", // 不是 root 也不能 sudo
		"root\nhas apt-get\nos ID=debian\n",                                  // 没有版本代号
		"root\nhas emerge\nos ID=gentoo\n",                                   // 不认识
		"docker\ncompose\nroot\n",                                            // 已经齐了
		"root\nhas apt-get\narch amd64\nos ID=debian\nos VERSION_CODENAME=book'worm\n",
	} {
		if cmd, err := DockerInstallCommand(ParseDockerStatus(out)); err == nil {
			t.Errorf("应该拒绝：%q → %s", out, cmd)
		}
	}
}
