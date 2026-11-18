package apps

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

func aiRec(c *Catalog, name string, cfg map[string]string) (*App, Record) {
	a, _ := c.Get(name)
	return a, Record{App: name, Type: TypeNpm, Instance: name, Version: a.Npm.Version, Bins: a.Npm.Bins, Config: cfg}
}

func TestAIConfigClaude(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	a, rec := aiRec(c, "claude-code", nil)
	if a.AI == nil || len(a.AI.Providers) < 5 || a.AI.Login == "" {
		t.Fatalf("Claude Code 应有供应商预置和订阅登录：%+v", a.AI)
	}
	// 第三方供应商：Anthropic 兼容接口 + AUTH_TOKEN（从标准输入写进 secret.env），模型写进 config.env。
	p, err := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", Model: "deepseek-v4-pro", SmallModel: "deepseek-flash", Secret: "deepseek-key"})
	if err != nil {
		t.Fatal(err)
	}
	if p.KeyEnv != "ANTHROPIC_AUTH_TOKEN" || p.KeyRef != "app/deepseek-key" || !p.Dotenv {
		t.Errorf("密钥送法不对：%+v", p)
	}
	for _, s := range []string{`cat > "$a/secret.env"`, `ANTHROPIC_BASE_URL=`, `api.deepseek.com/anthropic`, `ANTHROPIC_DEFAULT_HAIKU_MODEL=deepseek-flash`, `ANTHROPIC_API_KEY=`, `> "$a/config.json"`, `hostower-app claude-code`} {
		if !strings.Contains(p.Command, s) {
			t.Errorf("命令里缺少 %q：\n%s", s, p.Command)
		}
	}
	if strings.Contains(p.Command, "deepseek-key") && !strings.Contains(p.Command, `"secret":"deepseek-key"`) {
		t.Error("命令里只能出现密钥的名字（在 config.json 里），不能有别的")
	}
	if got := e.Classify(p.Command).Level; got != risk.L1 {
		t.Errorf("配置命令应是 L1，得到 %s：%+v", got, e.Classify(p.Command).Findings)
	}
	// 只改模型、不换 Key：沿用已有的 secret.env，不清空也不重写。
	_, rec2 := aiRec(c, "claude-code", map[string]string{"provider": "deepseek", "key_env": "ANTHROPIC_AUTH_TOKEN", "secret": "deepseek-key"})
	p, err = a.PlanAIConfig(rec2, AIConfig{Provider: "deepseek", Model: "deepseek-flash"})
	if err != nil || p.KeyRef != "" || strings.Contains(p.Command, "secret.env\"; ") && strings.Contains(p.Command, `: > "$a/secret.env"`) {
		t.Errorf("不换 Key 时不应动 secret.env：%v\n%s", err, p.Command)
	}
	// 换到官方、不填 Key（用订阅登录）：清掉旧的第三方 Key，不设接口地址。
	p, err = a.PlanAIConfig(rec2, AIConfig{Provider: "anthropic"})
	if err != nil || !strings.Contains(p.Command, `: > "$a/secret.env"`) || strings.Contains(p.Command, "ANTHROPIC_BASE_URL") {
		t.Errorf("换到官方订阅时应清掉旧 Key：%v\n%s", err, p.Command)
	}
	// 换到另一家第三方（Key 的变量名一样）但没给新 Key：旧 Key 用不了，拒绝。
	if _, err := a.PlanAIConfig(rec2, AIConfig{Provider: "kimi"}); err == nil {
		t.Error("换了供应商应要求重新选择 Key")
	}
	// 第三方供应商没给 Key：拒绝。
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "kimi"}); err == nil {
		t.Error("需要 Key 的供应商没选密钥时应拒绝")
	}
	// 自定义地址必须是 https，不能夹带引号或命令。
	for _, bad := range []string{"http://198.51.100.7/v1", "https://example.com/'; reboot; '", "https://user:pass@example.com", "ftp://example.com"} {
		if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "custom", BaseURL: bad, Secret: "k"}); err == nil {
			t.Errorf("不合法的地址 %q 应被拒绝", bad)
		}
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", Model: "x'; reboot", Secret: "k"}); err == nil {
		t.Error("不合法的模型名应被拒绝")
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "nope"}); err == nil {
		t.Error("不认识的供应商应被拒绝")
	}
}

func TestAIConfigCodex(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	a, rec := aiRec(c, "codex", nil)
	// 官方 API Key：交给 codex login --with-api-key（标准输入），配置目录是 Hostower 管的 CODEX_HOME。
	p, err := a.PlanAIConfig(rec, AIConfig{Provider: "openai", Model: "gpt-5.5-codex", Secret: "openai-key"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Dotenv || p.KeyRef != "app/openai-key" || !strings.Contains(p.Command, "login --with-api-key") || !strings.Contains(p.Command, `CODEX_HOME="$HOME/.hostower/apps/codex/home"`) ||
		!strings.Contains(p.Command, `cli_auth_credentials_store = "file"`) || !strings.Contains(p.Command, `model = "gpt-5.5-codex"`) {
		t.Errorf("官方 Key 的配置不对：%+v\n%s", p, p.Command)
	}
	if got := e.Classify(p.Command).Level; got != risk.L1 {
		t.Errorf("配置命令应是 L1，得到 %s：%+v", got, e.Classify(p.Command).Findings)
	}
	// 自定义供应商：只有 Responses API，模型必填。
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "custom", BaseURL: "https://gw.example.com/v1", Secret: "k"}); err == nil {
		t.Error("自定义供应商没填模型应拒绝")
	}
	p, err = a.PlanAIConfig(rec, AIConfig{Provider: "custom", BaseURL: "https://gw.example.com/v1", Model: "m-1", Secret: "k"})
	if err != nil || !strings.Contains(p.Command, `wire_api = "responses"`) || !strings.Contains(p.Command, `env_key = "HOSTOWER_PROVIDER_API_KEY"`) || p.KeyEnv != "HOSTOWER_PROVIDER_API_KEY" || !p.Dotenv {
		t.Errorf("自定义供应商的配置不对：%v\n%s", err, p.Command)
	}
	if got := e.Classify(p.Command).Level; got != risk.L1 {
		t.Errorf("配置命令应是 L1，得到 %s", got)
	}
	// 官方不填 Key：订阅登录，配置照样写（CODEX_HOME 要指过去，登录才落在同一个目录）。
	if p, err := a.PlanAIConfig(rec, AIConfig{Provider: "openai"}); err != nil || p.KeyRef != "" || !strings.Contains(p.Command, "CODEX_HOME") {
		t.Errorf("订阅登录的配置不对：%v", err)
	}
}

func TestAIConfigDsh(t *testing.T) {
	e := risk.New()
	c := catalog(t)
	a, rec := aiRec(c, "deepseek-harness", nil)
	p, err := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", Secret: "ds-key"})
	if err != nil || p.KeyEnv != "DEEPSEEK_API_KEY" || !strings.Contains(p.Command, `DSH_HOME="$HOME/.hostower/apps/deepseek-harness/home"`) || strings.Contains(p.Command, "DEEPSEEK_BASE_URL") {
		t.Errorf("dsh 配置不对：%v\n%s", err, p.Command)
	}
	if p, _ := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", BaseURL: "https://gw.example.com/anthropic", Secret: "ds-key"}); !strings.Contains(p.Command, "DEEPSEEK_BASE_URL=") {
		t.Error("填了接口地址应写进 DEEPSEEK_BASE_URL")
	}
	// 没指定模型：配置层写 []（空文件会让 dsh 启动失败）；指定了就写默认模型。
	if p, _ := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", Secret: "ds-key"}); !strings.Contains(p.Command, "cordis.patch.yml") || !strings.Contains(p.Command, "\n[]\n") {
		t.Errorf("DeepSeek 官方不指定模型应写空配置层：\n%s", p.Command)
	}
	if p, _ := a.PlanAIConfig(rec, AIConfig{Provider: "deepseek", Model: "deepseek-v4-pro", Secret: "ds-key"}); !strings.Contains(p.Command, `provider: "deepseek-official"`) {
		t.Errorf("DeepSeek 官方指定模型应写默认模型：\n%s", p.Command)
	}

	// 内置目录的第三方提供商：只写路由名和 Key 的变量名，默认模型指过去。
	p, err = a.PlanAIConfig(rec, AIConfig{Provider: "openrouter", Model: "~anthropic/claude-sonnet-latest", Secret: "or-key"})
	if err != nil || p.KeyEnv != "HOSTOWER_PROVIDER_API_KEY" || !p.Dotenv || strings.Contains(p.Command, "DEEPSEEK_BASE_URL") ||
		!strings.Contains(p.Command, "- id: llm-pi-ai\n  config:\n    providers:\n      \"openrouter\":\n        apiKeyEnv: HOSTOWER_PROVIDER_API_KEY\n") ||
		!strings.Contains(p.Command, "- id: agent-default-model\n  config:\n    provider: \"openrouter\"\n    model: \"~anthropic/claude-sonnet-latest\"\n") {
		t.Errorf("第三方提供商的配置不对：%v\n%s", err, p.Command)
	}
	if got := e.Classify(p.Command).Level; got != risk.L1 {
		t.Errorf("配置命令应是 L1，得到 %s", got)
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "openrouter", Secret: "or-key"}); err == nil {
		t.Error("第三方提供商不填模型应拒绝")
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "nope", Model: "m", Secret: "k"}); err == nil {
		t.Error("不认识的提供商应拒绝")
	}

	// 自定义模型 API：协议必须是支持的那几种，地址必须是 https。
	p, err = a.PlanAIConfig(rec, AIConfig{Provider: "custom", API: "openai-completions", BaseURL: "https://gw.example.com/v1", Model: "@cf/qwen/qwen3.8-27b", Secret: "gw-key"})
	if err != nil || !strings.Contains(p.Command, `api: "openai-completions"`) || !strings.Contains(p.Command, `baseURL: "https://gw.example.com/v1"`) ||
		!strings.Contains(p.Command, `- id: "@cf/qwen/qwen3.8-27b"`) || !strings.Contains(p.Command, `provider: "hostower-custom"`) || p.Params["api"] != "openai-completions" {
		t.Errorf("自定义模型 API 的配置不对：%v\n%s", err, p.Command)
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "custom", API: "bedrock-converse-stream", BaseURL: "https://gw.example.com/v1", Model: "m", Secret: "k"}); err == nil {
		t.Error("不支持的协议应拒绝")
	}
	if _, err := a.PlanAIConfig(rec, AIConfig{Provider: "custom", API: "openai-completions", BaseURL: "http://gw.example.com/v1", Model: "m", Secret: "k"}); err == nil {
		t.Error("非 https 地址应拒绝")
	}

	r, _ := c.Get("redis")
	if _, err := r.PlanAIConfig(Record{App: "redis", Type: TypeCompose, Instance: "redis"}, AIConfig{Provider: "deepseek"}); err == nil {
		t.Error("容器应用没有这种配置")
	}
}

func TestListWithConfig(t *testing.T) {
	out := `{"app":"codex","version":"0.169.0","type":"npm","instance":"codex","bins":["codex"]}
{"instance":"codex","config":{"provider":"openai","secret":"k"}}
{"instance":"ghost","config":{"provider":"x"}}
---
`
	l := ParseList(out)
	if len(l) != 1 || l[0].Config["provider"] != "openai" {
		t.Fatalf("config.json 应并进实例：%+v", l)
	}
}
