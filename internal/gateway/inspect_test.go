package gateway

import "testing"

func TestMaskEnv(t *testing.T) {
	cases := []struct {
		in, value string
		masked    bool
	}{
		{"TZ=Asia/Shanghai", "Asia/Shanghai", false},
		{"MYSQL_ROOT_PASSWORD=hunter2", "", true},
		{"API_TOKEN=abc", "", true},
		{"SECRET_KEY_BASE=abc", "", true},
		{"DATABASE_URL=postgres://app:hunter2@db.example.com/app", "postgres://app:••••••@db.example.com/app", true},
		{"EMPTY_PASSWORD=", "", false},
		{"PATH=/usr/bin:/bin", "/usr/bin:/bin", false},
	}
	for _, c := range cases {
		got := maskEnv(c.in)
		if got.Value != c.value || got.Masked != c.masked {
			t.Errorf("%s → %+v", c.in, got)
		}
	}
}

func TestParseInspect(t *testing.T) {
	out := `[{"Id":"abc123","Name":"/web","Created":"2026-01-02T03:04:05Z","Image":"sha256:feed","RestartCount":2,
	"State":{"Status":"running","StartedAt":"2026-01-02T03:04:06Z","ExitCode":0,"Health":{"Status":"healthy"}},
	"Config":{"Image":"nginx:1.27","Env":["B=2","A_PASSWORD=x"],"Cmd":["nginx","-g","daemon off;"],"Entrypoint":["/docker-entrypoint.sh"],
	  "Labels":{"com.docker.compose.project":"stack","com.docker.compose.service":"web"}},
	"HostConfig":{"RestartPolicy":{"Name":"unless-stopped"},"Memory":1073741824,"NanoCpus":1500000000},
	"Mounts":[{"Type":"volume","Name":"cache","Source":"/var/lib/docker/volumes/cache/_data","Destination":"/cache","RW":true},
	          {"Type":"bind","Source":"/opt/app","Destination":"/app","RW":false}],
	"NetworkSettings":{"Ports":{"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"8080"},{"HostIp":"::","HostPort":"8080"}],"443/tcp":null},
	  "Networks":{"stack_default":{"IPAddress":"192.0.2.2","Aliases":["web"]}}}}]`
	d, ok := parseInspect(out)
	if !ok {
		t.Fatal("解析失败")
	}
	if d.Name != "web" || d.Image != "nginx:1.27" || d.Health != "healthy" || d.Restart != "unless-stopped" || d.RestartCount != 2 {
		t.Errorf("基本字段不对：%+v", d)
	}
	if d.MemoryLimit != 1<<30 || d.CPULimit != 1.5 || d.Project != "stack" || d.Service != "web" {
		t.Errorf("限制或 Compose 信息不对：%+v", d)
	}
	if len(d.Command) != 4 || d.Command[0] != "/docker-entrypoint.sh" {
		t.Errorf("命令不对：%v", d.Command)
	}
	// 对外映射的端口排前面，没有映射的也要列出来。
	if len(d.Ports) != 3 || d.Ports[0].HostPort != "8080" || d.Ports[2].Container != "443" || d.Ports[2].HostPort != "" {
		t.Errorf("端口不对：%+v", d.Ports)
	}
	if len(d.Mounts) != 2 || d.Mounts[0].Destination != "/app" || !d.Mounts[0].ReadOnly || d.Mounts[1].Source != "cache" {
		t.Errorf("挂载不对：%+v", d.Mounts)
	}
	if len(d.Env) != 2 || d.Env[0].Key != "A_PASSWORD" || !d.Env[0].Masked || d.Env[0].Value != "" || d.Env[1].Value != "2" {
		t.Errorf("环境变量不对：%+v", d.Env)
	}
	// 和镜像里一模一样的变量标成“镜像自带”，被改过的不标。
	markImageEnv(d, `["B=2","A_PASSWORD=default"]`)
	if d.Env[0].FromImage || !d.Env[1].FromImage {
		t.Errorf("镜像自带的标记不对：%+v", d.Env)
	}
	if _, ok := parseInspect("no such container"); ok {
		t.Error("不是 JSON 时应该失败")
	}
}
