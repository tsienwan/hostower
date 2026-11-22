package hostenv

import (
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 检测用的命令都走内核的只读查询：必须是 L0，否则页面上查不出来。
func TestCommandsAreReadOnly(t *testing.T) {
	e := risk.New()
	cmds := map[string]string{"tools": ToolsCommand, "net": NetCommand}
	for name, c := range cmds {
		if v := e.Classify(c); v.Level != risk.L0 {
			t.Errorf("%s 的检测命令被判为 %s：%+v\n%s", name, v.Level, v.Findings, c)
		}
	}
}

func TestParseTools(t *testing.T) {
	out := "== tools\nclaude\t/home/dev/.local/bin/claude\ncodex\t/home/dev/.local/bin/codex\ndocker\t/usr/bin/docker\nevil\t$(reboot)\n" +
		"== versions\n/home/dev/.hostower/npm/lib/node_modules/@openai/codex/package.json:  \"version\": \"0.169.0\",\n" +
		"/usr/lib/node_modules/@openai/codex/package.json:  \"version\": \"0.1.0\",\n" +
		"== claude-native\n2.1.9\n2.1.30\nnot-a-version\n" +
		"== auth\n/home/dev/.codex/auth.json\n== end\n"
	by := map[string]ToolStatus{}
	for _, s := range ParseTools(out) {
		by[s.Name] = s
	}
	if len(by) != len(Tools) {
		t.Fatalf("应该每个命令一项，得到 %d 项", len(by))
	}
	if c := by["codex"]; !c.Installed || c.Version != "0.169.0" || !c.Signed {
		t.Errorf("codex：%+v", c)
	}
	if c := by["claude"]; !c.Installed || c.Version != "2.1.30" || c.Signed {
		t.Errorf("claude 的原生安装应该取最新的版本目录：%+v", c)
	}
	if c := by["docker"]; !c.Installed || c.Path != "/usr/bin/docker" {
		t.Errorf("docker：%+v", c)
	}
	if c := by["gemini"]; c.Installed || c.Version != "" {
		t.Errorf("没装的命令不该有版本：%+v", c)
	}
}

func TestParseService(t *testing.T) {
	by := map[string]Service{}
	for _, s := range Services {
		by[s.ID] = s
	}
	cases := []struct {
		id, out, want string
		code          int
	}{
		{"openai", "{\"error\":{\"message\":\"Missing bearer authentication in header\"}}|code=401", StateOK, 401},
		{"openai", "{\"error\":{\"code\":\"unsupported_country_region_territory\"}}|code=403", StateBlocked, 403},
		{"anthropic", "{\"type\":\"error\",\"error\":{\"type\":\"authentication_error\"}}|code=401", StateOK, 401},
		{"anthropic", "{\"error\":{\"type\":\"forbidden\",\"message\":\"Request not allowed\"}}|code=403", StateBlocked, 403},
		{"gemini", "\"message\": \"API key not valid. Please pass a valid API key.\"|code=400", StateOK, 400},
		{"gemini", "\"message\": \"User location is not supported for the API use.\"|code=400", StateBlocked, 400},
		{"gemini", "bad request|code=400", StateUnknown, 400},
		{"xai", "|code=000", StateUnreachable, 0},
		{"deepseek", "", StateUnreachable, 0},
		{"deepseek", "<html>502</html>|code=502", StateUnknown, 502},
	}
	for _, c := range cases {
		got := ParseService(by[c.id], c.out)
		if got.State != c.want || got.Code != c.code {
			t.Errorf("%s %q：期望 %s/%d，得到 %s/%d", c.id, c.out, c.want, c.code, got.State, got.Code)
		}
	}
}

func TestParseNet(t *testing.T) {
	out := "site google |204|\n" +
		"egress ip=203.0.113.9 loc=SG colo=SIN warp=off \n" +
		"egress6 \n" +
		"svc openai {\"error\":{\"message\":\"Missing bearer authentication in header\"}}|code=401\n" +
		"svc anthropic {\"error\":{\"type\":\"forbidden\"}}|code=403\n" +
		"ipinfo {    \"status\": \"ok\",    \"203.0.113.9\": {        \"asn\": \"AS64500\",        \"provider\": \"Example <b>Hosting</b> Ltd\",        \"country\": \"Singapore\",        \"city\": \"Singapore\",        \"proxy\": \"no\",        \"type\": \"Hosting\",        \"risk\": 33    }}\n" +
		"site netflix </html>|200|https://www.netflix.com/sg/title/81280792\n" +
		"site disney |302|https://www.disneyplus.com/unavailable\n" +
		"site youtube |200|\n" +
		"region youtube \"countryCode\":\"SG\"\n" +
		"site x |000|\n" +
		"site reddit |403|\n" +
		"== end\n"
	n := ParseNet(out)
	e := n.Egress
	if e.IP != "203.0.113.9" || e.Country != "SG" || e.Colo != "SIN" || e.Warp || e.IPv6 != "" {
		t.Errorf("出口：%+v", e)
	}
	if e.ASN != "AS64500" || e.Type != "hosting" || e.Risk != 33 || e.Proxy || e.Provider != "Example bHosting/b Ltd" {
		t.Errorf("IP 信息：%+v", e)
	}
	if len(n.Services) != len(Services) || len(n.Sites) != len(Sites) {
		t.Fatalf("每家接口、每个网站都要有一项：%d %d", len(n.Services), len(n.Sites))
	}
	svc := map[string]ServiceStatus{}
	for _, s := range n.Services {
		svc[s.ID] = s
	}
	if svc["openai"].State != StateOK || svc["anthropic"].State != StateBlocked || svc["gemini"].State != StateUnreachable {
		t.Errorf("接口：%+v", svc)
	}
	site := map[string]SiteStatus{}
	for _, s := range n.Sites {
		site[s.ID] = s
	}
	if s := site["netflix"]; s.State != StateOK || s.Region != "SG" || s.Note != "full" {
		t.Errorf("netflix：%+v", s)
	}
	if s := site["youtube"]; s.State != StateOK || s.Region != "SG" {
		t.Errorf("youtube：%+v", s)
	}
	if site["google"].State != StateOK || site["disney"].State != StateBlocked || site["x"].State != StateUnreachable || site["reddit"].State != StateBlocked || site["github"].State != StateUnreachable {
		t.Errorf("网站：%+v", site)
	}
	// 只看得到自制剧。
	if s := parseSite(Sites[2], "x|404|https://www.netflix.com/jp/title/81280792", ""); s.State != StateOK || s.Note != "originals" || s.Region != "JP" {
		t.Errorf("netflix 自制剧：%+v", s)
	}
	// 什么都没查到：没有风险分，不是 0 分。
	if z := ParseNet(""); z.Egress.Risk != -1 || z.Egress.IP != "" {
		t.Errorf("空输出：%+v", z.Egress)
	}
}
