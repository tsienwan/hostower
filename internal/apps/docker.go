package apps

import (
	"errors"
	"regexp"
	"strings"
)

// 容器应用要靠 Docker 和 Docker Compose v2。没有的时候，面板可以替你生成一个“安装 Docker”的请求：
// 先用一条固定的只读命令（DockerStatusCommand）看清这台主机的情况，再按它的系统拼出具体的安装命令。
// 安装命令是给这台主机写死的（系统、版本代号、处理器架构都在命令里），审批时看到的就是实际要跑的每一步；
// 软件来自 Docker 官方的软件源（Debian、Ubuntu、CentOS 系、Fedora）或系统自带的软件源（Alpine、Arch、openSUSE），
// 不下载脚本来执行。
//
// 不是 root 的账号用 sudo -n（需要免密 sudo）。装好之后不把账号加进 docker 组：那等于给面板自己提权，
// 只能由人在主机上做（容器页会给出命令）。

// DockerStatusCommand 是检查 Docker 情况用的固定只读命令。每行一项：
// docker、compose（各自装了才有）、root 或 sudo（能不能以 root 运行）、has <命令>、arch <dpkg 架构>、os <os-release 里的一行>。
const DockerStatusCommand = `if command -v docker >/dev/null 2>&1; then echo docker; fi; if docker compose version >/dev/null 2>&1; then echo compose; fi; ` +
	`if [ "$(id -u)" = 0 ]; then echo root; elif sudo -n true 2>/dev/null; then echo sudo; fi; ` +
	`for c in apt-get dnf yum apk pacman zypper systemctl rc-service curl; do if command -v "$c" >/dev/null 2>&1; then echo "has $c"; fi; done; ` +
	`if command -v dpkg >/dev/null 2>&1; then echo "arch $(dpkg --print-architecture)"; fi; ` +
	`while IFS= read -r l || [ -n "$l" ]; do case $l in ID=*|ID_LIKE=*|VERSION_CODENAME=*|UBUNTU_CODENAME=*) echo "os $l";; esac; done </etc/os-release`

// DockerStatus 是 DockerStatusCommand 的结果。
type DockerStatus struct {
	Docker  bool `json:"docker"`  // 有 docker 命令
	Compose bool `json:"compose"` // 有 docker compose（v2）
	Root    bool `json:"root"`    // 登录账号就是 root
	Sudo    bool `json:"sudo"`    // 能免密 sudo
	// Distro 是认出来的系统（debian、ubuntu、centos、fedora、alpine、arch、opensuse…）；空表示不认识，装不了。
	Distro string `json:"distro,omitempty"`
	// OS 是 os-release 里的 ID，给页面显示用。
	OS string `json:"os,omitempty"`

	has      map[string]bool
	id, like string
	codename string
	arch     string
}

// Ready 表示容器应用可以直接装。
func (s DockerStatus) Ready() bool { return s.Docker && s.Compose }

var dockerTokenRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,40}$`)

// ParseDockerStatus 解析 DockerStatusCommand 的输出。
func ParseDockerStatus(out string) DockerStatus {
	s := DockerStatus{has: map[string]bool{}}
	var ubuntuCodename, versionCodename string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "docker":
			s.Docker = true
		case line == "compose":
			s.Compose = true
		case line == "root":
			s.Root = true
		case line == "sudo":
			s.Sudo = true
		case strings.HasPrefix(line, "has "):
			s.has[strings.TrimPrefix(line, "has ")] = true
		case strings.HasPrefix(line, "arch "):
			s.arch = strings.TrimSpace(strings.TrimPrefix(line, "arch "))
		case strings.HasPrefix(line, "os "):
			k, v, _ := strings.Cut(strings.TrimPrefix(line, "os "), "=")
			v = strings.ToLower(strings.Trim(strings.TrimSpace(v), `"'`))
			switch k {
			case "ID":
				s.id = v
			case "ID_LIKE":
				s.like = v
			case "VERSION_CODENAME":
				versionCodename = v
			case "UBUNTU_CODENAME":
				ubuntuCodename = v
			}
		}
	}
	if !dockerTokenRe.MatchString(s.id) {
		s.id = ""
	}
	s.OS = s.id
	like := " " + s.id + " " + s.like + " "
	is := func(names ...string) bool {
		for _, n := range names {
			if strings.Contains(like, " "+n+" ") {
				return true
			}
		}
		return false
	}
	switch {
	case s.has["apt-get"] && (s.id == "ubuntu" || (is("ubuntu") && ubuntuCodename != "")):
		s.Distro, s.codename = "ubuntu", ubuntuCodename
		if s.codename == "" {
			s.codename = versionCodename
		}
	case s.has["apt-get"] && (s.id == "debian" || s.id == "raspbian"):
		s.Distro, s.codename = s.id, versionCodename
	case (s.has["dnf"] || s.has["yum"]) && s.id == "fedora":
		s.Distro = "fedora"
	case (s.has["dnf"] || s.has["yum"]) && s.id == "rhel":
		s.Distro = "rhel"
	case (s.has["dnf"] || s.has["yum"]) && is("centos", "rhel", "rocky", "almalinux", "ol"):
		s.Distro = "centos"
	case s.has["apk"] && s.id == "alpine":
		s.Distro = "alpine"
	case s.has["pacman"] && is("arch"):
		s.Distro = "arch"
	case s.has["zypper"] && is("opensuse", "suse", "sles"):
		s.Distro = "opensuse"
	}
	if s.codename != "" && !dockerTokenRe.MatchString(s.codename) {
		s.codename = ""
	}
	if s.arch != "" && !dockerTokenRe.MatchString(s.arch) {
		s.arch = ""
	}
	if (s.Distro == "debian" || s.Distro == "ubuntu" || s.Distro == "raspbian") && (s.codename == "" || s.arch == "") {
		s.Distro = "" // 拼不出软件源那一行
	}
	return s
}

// dockerCE 是 Docker 官方软件源里的那几个包；只缺 Compose 时只装最后一个。
const dockerCE = "docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin"

// DockerInstallCommand 按检查结果拼出安装命令。已经齐了、不能以 root 运行、或者不认识系统时返回错误。
func DockerInstallCommand(s DockerStatus) (string, error) {
	if s.Ready() {
		return "", errors.New("这台主机已经有 Docker 和 Docker Compose 了")
	}
	if !s.Root && !s.Sudo {
		return "", errors.New("安装 Docker 需要 root，或者登录账号能免密 sudo")
	}
	if s.Distro == "" {
		if s.OS != "" {
			return "", errors.New("不认识这台主机的系统（" + s.OS + "），请按 Docker 官方文档手动安装")
		}
		return "", errors.New("不认识这台主机的系统，请按 Docker 官方文档手动安装")
	}
	S := ""
	if !s.Root {
		S = "sudo -n "
	}
	var steps []string
	add := func(cmd ...string) { steps = append(steps, cmd...) }
	pkgs := dockerCE
	switch s.Distro {
	case "debian", "ubuntu", "raspbian":
		if s.Docker {
			pkgs = "docker-compose-plugin"
		}
		apt := S + "env DEBIAN_FRONTEND=noninteractive apt-get"
		add(apt+" update -q",
			apt+" install -y -q ca-certificates curl",
			S+"install -m 0755 -d /etc/apt/keyrings",
			S+"curl -fsSL https://download.docker.com/linux/"+s.Distro+"/gpg -o /etc/apt/keyrings/docker.asc",
			S+"chmod a+r /etc/apt/keyrings/docker.asc",
			"echo 'deb [arch="+s.arch+" signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/"+s.Distro+" "+s.codename+" stable' | "+S+"tee /etc/apt/sources.list.d/docker.list >/dev/null",
			apt+" update -q",
			apt+" install -y -q "+pkgs)
	case "centos", "rhel", "fedora":
		if s.Docker {
			pkgs = "docker-compose-plugin"
		}
		pm := "dnf"
		if !s.has["dnf"] {
			pm = "yum"
		}
		if !s.has["curl"] {
			add(S + pm + " install -y -q curl")
		}
		add(S+"curl -fsSL https://download.docker.com/linux/"+s.Distro+"/docker-ce.repo -o /etc/yum.repos.d/docker-ce.repo",
			S+pm+" install -y -q "+pkgs)
	case "alpine":
		pkgs = "docker docker-cli-compose"
		if s.Docker {
			pkgs = "docker-cli-compose"
		}
		add(S + "apk add -q " + pkgs)
	case "arch":
		pkgs = "docker docker-compose"
		if s.Docker {
			pkgs = "docker-compose"
		}
		add(S + "pacman -S --noconfirm --needed " + pkgs)
	case "opensuse":
		pkgs = "docker docker-compose"
		if s.Docker {
			pkgs = "docker-compose"
		}
		add(S + "zypper -n install " + pkgs)
	}
	// 刚装上 Docker 时让它开机自启并立刻运行。
	if !s.Docker {
		switch {
		case s.has["systemctl"]:
			add(S + "systemctl enable --now docker")
		case s.has["rc-service"]:
			add(S+"rc-update add docker default", S+"rc-service docker start")
		}
	}
	add("docker compose version")
	return strings.Join(steps, " && "), nil
}
