package apps

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// AI 命令行工具的配置：API Key、接口地址（模型供应商）、模型。
//
// 做法对每个工具都一样：我们装的启动脚本（~/.local/bin/<命令>）每次运行时读两个文件——
//
//	~/.hostower/apps/<实例>/config.env  接口地址、模型这些普通设置（命令里看得到，审批时看得到）
//	~/.hostower/apps/<实例>/secret.env  API Key（只有登录账号可读；内容由塔台从标准输入写进去，不进命令和审计）
//
// Codex 和 DeepSeek Harness 的配置目录用 CODEX_HOME / DSH_HOME 指到实例目录下的 home，不碰用户自己的 ~/.codex、~/.dsh。
// DeepSeek Harness 的第三方提供商写在 $DSH_HOME/cordis.patch.yml（它的 home 级配置层）：
// 内置目录里的提供商（pi-ai 目录）只写路由名，自定义模型 API 写协议、地址和模型；默认模型写在 agent-default-model 一行。
// 订阅登录（ChatGPT、Claude 订阅）要在浏览器里授权，走交互终端：开终端本身按 L2 审批、全程录制。

// Configure 是清单里的 configure 一节。
type Configure struct {
	Kind      string `yaml:"kind" json:"kind"`                     // codex | claude | dsh
	Login     string `yaml:"login" json:"login,omitempty"`         // 订阅登录的命令（在终端里运行）
	LoginHint string `yaml:"loginHint" json:"loginHint,omitempty"` // 登录时会看到什么
}

// Provider 是一个模型供应商的预置。接口地址以供应商的文档为准。
type Provider struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	BaseURL string   `json:"baseUrl,omitempty"`
	Models  []string `json:"models,omitempty"`
	// Key 表示这个供应商要不要 API Key：required 必须；optional 可选（官方：不填就用订阅登录）。
	Key  string `json:"key"`
	Note string `json:"note,omitempty"`
	// Custom 表示接口地址由用户填。
	Custom bool `json:"custom,omitempty"`
	// APIs 是自定义接口可选的协议（只有 DeepSeek Harness 的自定义模型 API 用）。
	APIs []string `json:"apis,omitempty"`
}

// dshAPIs 是 DeepSeek Harness 自定义模型 API 支持的协议。
var dshAPIs = []string{"openai-completions", "openai-responses", "anthropic-messages"}

// 各工具的供应商预置。Claude Code 接的是各家的 Anthropic 兼容接口；Codex 只接 Responses API。
var aiProviders = map[string][]Provider{
	"claude": {
		{ID: "anthropic", Title: "Anthropic 官方", Key: "optional", Note: "填 API Key 按量计费；不填就在终端里用 Claude 订阅登录"},
		{ID: "deepseek", Title: "DeepSeek", BaseURL: "https://api.deepseek.com/anthropic", Models: []string{"deepseek-v4-pro", "deepseek-flash"}, Key: "required"},                  // hygiene:allow 供应商公开的接口地址
		{ID: "kimi", Title: "Kimi（国际）", BaseURL: "https://api.moonshot.ai/anthropic", Models: []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed"}, Key: "required"},                // hygiene:allow 供应商公开的接口地址
		{ID: "kimi-cn", Title: "Kimi（中国）", BaseURL: "https://api.moonshot.cn/anthropic", Models: []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed"}, Key: "required"},             // hygiene:allow 供应商公开的接口地址
		{ID: "glm-cn", Title: "智谱 GLM", BaseURL: "https://open.bigmodel.cn/api/anthropic", Models: []string{"glm-5.3", "glm-5.3-flash"}, Key: "required"},                           // hygiene:allow 供应商公开的接口地址
		{ID: "zai", Title: "Z.ai（智谱国际）", BaseURL: "https://api.z.ai/api/anthropic", Models: []string{"glm-5.3", "glm-5.3-flash"}, Key: "required"},                                  // hygiene:allow 供应商公开的接口地址
		{ID: "qwen-coding", Title: "通义千问 Coding Plan（国际）", BaseURL: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic", Models: []string{"qwen3.7-plus"}, Key: "required"}, // hygiene:allow 供应商公开的接口地址
		{ID: "minimax", Title: "MiniMax", BaseURL: "https://api.minimax.io/anthropic", Models: []string{"MiniMax-M3", "MiniMax-M3.1-Flash"}, Key: "required"},                       // hygiene:allow 供应商公开的接口地址
		{ID: "openrouter", Title: "OpenRouter", BaseURL: "https://openrouter.ai/api", Key: "required", Note: "模型写 OpenRouter 上的完整名字，例如 anthropic/claude-sonnet-4.5"},                // hygiene:allow 供应商公开的接口地址
		{ID: "custom", Title: "自定义（Anthropic 兼容接口）", Key: "required", Custom: true},
	},
	"codex": {
		{ID: "openai", Title: "OpenAI 官方", Key: "optional", Note: "填 API Key 按量计费；不填就在终端里用 ChatGPT 订阅登录"},
		{ID: "custom", Title: "自定义（需兼容 Responses API）", Key: "required", Custom: true, Note: "Codex 现在只支持 Responses API；只有 Chat Completions 接口的供应商接不上"},
	},
	// DeepSeek Harness：DeepSeek 走它自带的直连路由；其余是它内置的 pi-ai 提供商目录（ID 就是目录里的路由名，
	// 接口地址和协议由目录给出），模型名是目录里的，供选择时参考。
	"dsh": {
		{ID: "deepseek", Title: "DeepSeek", Models: []string{"deepseek-v4-pro", "deepseek-flash"}, Key: "required", Note: "接口地址留空就用 DeepSeek 官方的"},
		{ID: "openai", Title: "OpenAI", Models: []string{"gpt-6.1-sol", "gpt-6.1-terra", "gpt-5.5"}, Key: "required"},
		{ID: "anthropic", Title: "Anthropic", Models: []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"}, Key: "required"},
		{ID: "google", Title: "Google Gemini", Models: []string{"gemini-pro-latest", "gemini-flash-latest"}, Key: "required"},
		{ID: "xai", Title: "xAI Grok", Models: []string{"grok-4.7", "grok-4.6"}, Key: "required"},
		{ID: "openrouter", Title: "OpenRouter", Models: []string{"~anthropic/claude-sonnet-latest", "~openai/gpt-sol-latest", "~deepseek/deepseek-pro-latest"}, Key: "required", Note: "模型写 OpenRouter 上的完整名字"},
		{ID: "moonshotai", Title: "Kimi（国际）", Models: []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed", "kimi-k3"}, Key: "required"},
		{ID: "moonshotai-cn", Title: "Kimi（中国）", Models: []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed", "kimi-k3"}, Key: "required"},
		{ID: "kimi-coding", Title: "Kimi Coding 订阅", Models: []string{"kimi-for-coding", "kimi-for-coding-highspeed"}, Key: "required"},
		{ID: "zai", Title: "Z.ai（智谱国际）", Models: []string{"glm-5.3", "glm-5.3-flash"}, Key: "required"},
		{ID: "zai-coding-cn", Title: "智谱 Coding Plan（中国）", Models: []string{"glm-5.3", "glm-5.3-flash"}, Key: "required"},
		{ID: "minimax", Title: "MiniMax", Models: []string{"MiniMax-M3", "MiniMax-M3.1-Flash"}, Key: "required"},
		{ID: "minimax-cn", Title: "MiniMax（中国）", Models: []string{"MiniMax-M3", "MiniMax-M3.1-Flash"}, Key: "required"},
		{ID: "qwen-token-plan", Title: "通义千问 Token Plan（国际）", Models: []string{"qwen3.8-max", "qwen3.8-flash"}, Key: "required"},
		{ID: "qwen-token-plan-cn", Title: "通义千问 Token Plan（中国）", Models: []string{"qwen3.8-max", "qwen3.8-flash"}, Key: "required"},
		{ID: "xiaomi", Title: "小米 MiMo", Models: []string{"mimo-v2.6-pro", "mimo-v2.6-flash"}, Key: "required"},
		{ID: "ant-ling", Title: "蚂蚁百灵", Models: []string{"Ling-2.6-1T", "Ling-2.6-flash"}, Key: "required"},
		{ID: "mistral", Title: "Mistral", Models: []string{"mistral-large-latest", "devstral-latest", "codestral-latest"}, Key: "required"},
		{ID: "groq", Title: "Groq", Models: []string{"openai/gpt-oss-120b", "qwen/qwen3.8-27b"}, Key: "required"},
		{ID: "cerebras", Title: "Cerebras", Models: []string{"gpt-oss-120b", "qwen-3.8-27b"}, Key: "required"},
		{ID: "fireworks", Title: "Fireworks", Models: []string{"accounts/fireworks/routers/kimi-latest", "accounts/fireworks/routers/deepseek-flash-latest"}, Key: "required"},
		{ID: "together", Title: "Together AI", Models: []string{"moonshotai/Kimi-K3.1-Pro", "zai-org/GLM-5.3", "Qwen/Qwen3.7-Max"}, Key: "required"},
		{ID: "huggingface", Title: "Hugging Face", Models: []string{"zai-org/GLM-5.3"}, Key: "required"},
		{ID: "nvidia", Title: "NVIDIA", Models: []string{"deepseek-ai/deepseek-v4.1-flash", "z-ai/glm-5.3-flash"}, Key: "required"},
		{ID: "baseten", Title: "Baseten", Models: []string{"zai-org/GLM-5.3", "openai/gpt-oss-120b"}, Key: "required"},
		{ID: "vercel-ai-gateway", Title: "Vercel AI Gateway", Models: []string{"anthropic/claude-sonnet-5.5", "zai/glm-5.3"}, Key: "required"},
		{ID: "opencode", Title: "OpenCode Zen", Models: []string{"claude-sonnet-5-5", "glm-5.3", "kimi-k3"}, Key: "required"},
		{ID: "custom", Title: "自定义模型 API", Key: "required", Custom: true, APIs: dshAPIs, Note: "接 OpenAI 兼容网关或自己部署的模型服务：选协议、填地址和模型名"},
	},
}

// AIView 是给页面看的配置能力：有哪些供应商、怎么登录订阅。
type AIView struct {
	Configure
	Providers []Provider `json:"providers"`
}

// AIConfig 是一次配置。Secret 是应用密钥的名字：空表示 API Key 不变。
type AIConfig struct {
	Provider   string `json:"provider"`
	BaseURL    string `json:"baseUrl"`
	Model      string `json:"model"`
	SmallModel string `json:"smallModel"`
	Secret     string `json:"secret"`
	// API 是自定义接口的协议（DeepSeek Harness 的自定义模型 API 用）。
	API string `json:"api"`
}

// AIPlan 是渲染好的配置命令。KeyRef 不为空时，API Key 从标准输入送进去：
// Dotenv 为真时写成 secret.env 的一行（变量名 KeyEnv），否则原样送给 codex login --with-api-key。
type AIPlan struct {
	Command string
	KeyEnv  string
	KeyRef  string
	Dotenv  bool
	Params  map[string]string
}

var (
	// 模型名：OpenRouter 的 ~ 开头别名、Cloudflare 的 @cf/ 开头名字都要能写。值写进配置时一律加引号。
	aiModelRe = regexp.MustCompile(`^[A-Za-z0-9._:/@~\[\]-]{1,120}$`)
)

func (a *App) aiView() *AIView {
	if a.Configure == nil {
		return nil
	}
	return &AIView{Configure: *a.Configure, Providers: aiProviders[a.Configure.Kind]}
}

func findProvider(kind, id string) (Provider, bool) {
	for _, p := range aiProviders[kind] {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

func validBaseURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(s) > 300 ||
		strings.ContainsAny(s, "'\"`$\\ \n\r\t") {
		return errors.New("接口地址要是 https:// 开头的完整地址")
	}
	return nil
}

// envFileLine 渲染 config.env 里的一行：值加单引号，被 sh 读入时原样生效。
func envFileLine(k, v string) string { return k + "=" + snippet.Quote(v) }

// PlanAIConfig 渲染配置命令。rec 是主机上记的现状（rec.Config 来自 config.json：用来知道 API Key 的变量名变没变）。
// 配置记在 config.json 里，不动 app.json（那里记的是装了哪个版本）。
func (a *App) PlanAIConfig(rec Record, c AIConfig) (*AIPlan, error) {
	if a.Configure == nil || a.Type != TypeNpm {
		return nil, errors.New("这个应用没有可以配置的模型和 API Key")
	}
	if rec.App != a.Name || !InstanceRe.MatchString(rec.Instance) {
		return nil, errors.New("实例和应用对不上")
	}
	kind := a.Configure.Kind
	p, ok := findProvider(kind, c.Provider)
	if !ok {
		return nil, errors.New("不认识的模型供应商")
	}
	base := p.BaseURL
	if p.Custom || (kind == "dsh" && c.BaseURL != "") {
		if err := validBaseURL(c.BaseURL); err != nil {
			return nil, err
		}
		base = c.BaseURL
	}
	for _, m := range []string{c.Model, c.SmallModel} {
		if m != "" && !aiModelRe.MatchString(m) {
			return nil, fmt.Errorf("模型名 %q 的格式不对", m)
		}
	}
	if c.Secret != "" && !SecretNameRe.MatchString(c.Secret) {
		return nil, errors.New("应用密钥的名字不合法")
	}
	if kind == "codex" && p.Custom && c.Model == "" {
		return nil, errors.New("自定义供应商要填模型名")
	}
	// DeepSeek Harness 走第三方提供商时，模型就是默认模型，必须填；自定义接口还要选协议。
	dshRoute := kind == "dsh" && p.ID != "deepseek"
	if dshRoute && c.Model == "" {
		return nil, errors.New("第三方提供商要填模型名（作为默认模型）")
	}
	if p.Custom && len(p.APIs) > 0 && !slices.Contains(p.APIs, c.API) {
		return nil, errors.New("请选择接口协议")
	}

	// API Key 用哪个变量、怎么送：
	plan := &AIPlan{Params: map[string]string{"provider": p.ID}}
	keyEnv, dotenv := "", true
	switch kind {
	case "claude":
		keyEnv = "ANTHROPIC_AUTH_TOKEN"
		if p.ID == "anthropic" {
			keyEnv = "ANTHROPIC_API_KEY"
		}
	case "codex":
		if p.ID == "openai" {
			keyEnv, dotenv = "OPENAI_API_KEY", false // 交给 codex login --with-api-key，存在 CODEX_HOME/auth.json
		} else {
			keyEnv = "HOSTOWER_PROVIDER_API_KEY"
		}
	case "dsh":
		keyEnv = "DEEPSEEK_API_KEY"
		if dshRoute {
			keyEnv = "HOSTOWER_PROVIDER_API_KEY" // cordis.patch.yml 里的 apiKeyEnv 指向它
		}
	}
	keepKey := false
	// 沿用主机上已有的 Key：必须是同一个供应商（同一种 Key）。换了供应商，旧 Key 用不了。
	sameKey := rec.Config["provider"] == p.ID && rec.Config["key_env"] == keyEnv && rec.Config["secret"] != ""
	switch {
	case c.Secret != "":
		plan.KeyEnv, plan.KeyRef, plan.Dotenv = keyEnv, "app/"+c.Secret, dotenv
		plan.Params["secret"], plan.Params["key_env"] = c.Secret, keyEnv
	case p.Key == "required" && !sameKey:
		return nil, errors.New("这个供应商需要 API Key：请选择一个应用密钥")
	case sameKey:
		keepKey = true // 沿用主机上已有的 Key
		plan.Params["secret"], plan.Params["key_env"] = rec.Config["secret"], keyEnv
	}

	// config.env：普通设置。
	inst := rec.Instance
	var env []string
	switch kind {
	case "claude":
		env = append(env, envFileLine("DISABLE_AUTOUPDATER", "1")) // 更新由 Hostower 管（应用市场的“更新”）
		if base != "" {
			env = append(env, envFileLine("ANTHROPIC_BASE_URL", base), envFileLine("ANTHROPIC_API_KEY", ""))
		}
		if c.Model != "" {
			small := c.SmallModel
			if small == "" {
				small = c.Model
			}
			env = append(env, envFileLine("ANTHROPIC_MODEL", c.Model), envFileLine("ANTHROPIC_DEFAULT_OPUS_MODEL", c.Model),
				envFileLine("ANTHROPIC_DEFAULT_SONNET_MODEL", c.Model), envFileLine("ANTHROPIC_DEFAULT_HAIKU_MODEL", small))
		}
	case "codex":
		env = append(env, `CODEX_HOME="$HOME/.hostower/apps/`+inst+`/home"`)
	case "dsh":
		env = append(env, `DSH_HOME="$HOME/.hostower/apps/`+inst+`/home"`, envFileLine("DSH_TELEMETRY_DISABLED", "1"))
		if base != "" && !dshRoute {
			env = append(env, envFileLine("DEEPSEEK_BASE_URL", base))
		}
	}
	if p.Custom && len(p.APIs) > 0 {
		plan.Params["api"] = c.API
	}
	for k, v := range map[string]string{"base_url": base, "model": c.Model, "small_model": c.SmallModel} {
		if v != "" {
			plan.Params[k] = v
		}
	}

	var b strings.Builder
	w := func(s string) { b.WriteString(s) }
	w(`umask 077; a="$HOME/.hostower/apps/` + inst + `"; `)
	w(`[ -r "$a/app.json" ] || { echo 'hostower: 这台主机上还没有装这个工具'; exit 3; }; `)
	w(`mkdir -p "$a/home" || exit 1; `)
	// API Key：写进 secret.env（从标准输入），或者交给 codex login；不需要 Key 时清掉旧的。
	switch {
	case plan.KeyRef != "" && plan.Dotenv:
		w(`cat > "$a/secret.env" || exit 1; `)
	case plan.KeyRef != "":
		w(`: > "$a/secret.env"; `)
	case !keepKey:
		w(`: > "$a/secret.env"; `)
	}
	w(`printf '%s\n'`)
	for _, l := range env {
		w(" " + snippet.Quote(l))
	}
	w(` > "$a/config.env" || exit 1; `)
	if kind == "codex" {
		w(`cat > "$a/home/config.toml" <<'` + heredocTag + `' || exit 1` + "\n")
		w(codexTOML(p, base, c.Model))
		w(heredocTag + "\n")
	}
	if kind == "dsh" {
		w(`cat > "$a/home/cordis.patch.yml" <<'` + heredocTag + `' || exit 1` + "\n")
		w(dshPatch(p, base, c.Model, c.API))
		w(heredocTag + "\n")
	}
	if plan.KeyRef != "" && !plan.Dotenv {
		// codex login --with-api-key 从标准输入读 Key，存进 CODEX_HOME/auth.json。
		w(`nb=; [ -r "$a/nodebin" ] && read -r nb < "$a/nodebin"; P="$PATH"; [ -z "$nb" ] || P="$nb:$PATH"; `)
		w(`CODEX_HOME="$a/home" PATH="$P" "$HOME/.hostower/npm/bin/codex" login --with-api-key || exit 1; `)
	}
	// 启动脚本：老版本装的脚本不读配置，这里一并改成新的。
	w(wrapperScript(inst, rec.Bins))
	cfg, _ := json.Marshal(struct {
		Instance string            `json:"instance"`
		Config   map[string]string `json:"config"`
	}{inst, plan.Params})
	w(`printf '%s\n' ` + snippet.Quote(string(cfg)) + ` > "$a/config.json"; `)
	w(`echo 'hostower: 配置已写入，下次运行 ` + strings.Join(rec.Bins, "、") + ` 时生效'`)
	plan.Command = b.String()
	return plan, nil
}

func codexTOML(p Provider, base, model string) string {
	var t strings.Builder
	t.WriteString("# 由 Hostower 生成：在“应用 → 已安装 → 配置”里修改，手动改的内容下次配置时会被覆盖。\n")
	t.WriteString("cli_auth_credentials_store = \"file\"\n")
	t.WriteString("check_for_update_on_startup = false\n")
	if model != "" {
		t.WriteString("model = " + tomlString(model) + "\n")
	}
	if p.Custom {
		t.WriteString("model_provider = \"hostower\"\n\n[model_providers.hostower]\n")
		t.WriteString("name = \"Hostower 配置的供应商\"\n")
		t.WriteString("base_url = " + tomlString(base) + "\n")
		t.WriteString("env_key = \"HOSTOWER_PROVIDER_API_KEY\"\n")
		t.WriteString("wire_api = \"responses\"\n")
	}
	return t.String()
}

// dshRouteCustom 是自定义模型 API 在 DeepSeek Harness 里的路由名。
const dshRouteCustom = "hostower-custom"

// dshPatch 渲染 DeepSeek Harness 的 home 级配置层（$DSH_HOME/cordis.patch.yml）。
// 按 id 改写的行会整体替换那一行的配置；这个文件由 Hostower 管，每次配置整个重写。
// 没有要改的（DeepSeek 官方、没指定模型）时写 []：空文件会让 dsh 启动失败。
func dshPatch(p Provider, base, model, api string) string {
	var t strings.Builder
	t.WriteString("# 由 Hostower 生成：在“应用 → 已安装 → 配置”里修改，手动改的内容下次配置时会被覆盖。\n")
	route := ""
	switch {
	case p.ID == "deepseek":
		if model == "" {
			t.WriteString("[]\n")
			return t.String()
		}
		route = "deepseek-official"
	case p.Custom:
		route = dshRouteCustom
		t.WriteString("- id: llm-pi-ai\n  config:\n    providers:\n      " + route + ":\n")
		t.WriteString("        displayName: \"Custom (Hostower)\"\n")
		t.WriteString("        apiKeyEnv: HOSTOWER_PROVIDER_API_KEY\n")
		t.WriteString("        api: " + yamlString(api) + "\n")
		t.WriteString("        baseURL: " + yamlString(base) + "\n")
		t.WriteString("        models:\n          - id: " + yamlString(model) + "\n")
	default:
		route = p.ID
		t.WriteString("- id: llm-pi-ai\n  config:\n    providers:\n      " + yamlString(route) + ":\n")
		t.WriteString("        apiKeyEnv: HOSTOWER_PROVIDER_API_KEY\n")
		if base != "" {
			t.WriteString("        baseURL: " + yamlString(base) + "\n")
		}
	}
	t.WriteString("- id: agent-default-model\n  config:\n    provider: " + yamlString(route) + "\n    model: " + yamlString(model) + "\n")
	return t.String()
}

// yamlString 把校验过的值写成 YAML 双引号字符串（JSON 字符串也是合法的 YAML；模型名可能以 @、~ 开头）。
func yamlString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// tomlString 把校验过的值写成 TOML 字符串（值里没有引号和反斜杠，前面的校验保证了）。
func tomlString(s string) string { return `"` + s + `"` }

// wrapperScript 渲染（覆盖）命令行工具的启动脚本：读 config.env、secret.env，用装时选的 Node.js，再运行真正的程序。
// 脚本内容是固定的，不含任何配置值；不是 Hostower 装的同名文件不动。
func wrapperScript(inst string, bins []string) string {
	var b strings.Builder
	for _, bin := range bins {
		if !binRe.MatchString(bin) {
			continue
		}
		lines := []string{
			"#!/bin/sh",
			"# hostower-app " + inst,
			`a="$HOME/.hostower/apps/` + inst + `"`,
			`set -a; [ -r "$a/config.env" ] && . "$a/config.env"; [ -r "$a/secret.env" ] && . "$a/secret.env"; set +a`,
			`nb=; [ -r "$a/nodebin" ] && read -r nb < "$a/nodebin"; [ -n "$nb" ] && PATH="$nb:$PATH"`,
			`exec "$HOME/.hostower/npm/bin/` + bin + `" "$@"`,
		}
		f := `"$HOME/.local/bin/` + bin + `"`
		b.WriteString(`mkdir -p "$HOME/.local/bin"; if [ ! -e ` + f + ` ] || grep -q 'hostower-app ` + inst + `' ` + f + ` 2>/dev/null; then printf '%s\n'`)
		for _, l := range lines {
			b.WriteString(" " + snippet.Quote(l))
		}
		b.WriteString(` > ` + f + ` && chmod 755 ` + f + `; else echo "hostower: ` + "$HOME/.local/bin/" + bin + ` 已经存在（不是 Hostower 装的），没有覆盖它"; fi; `)
	}
	return b.String()
}
