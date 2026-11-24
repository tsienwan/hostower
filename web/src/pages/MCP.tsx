// MCP 接入：给 Claude Code、Codex 这类 AI 助手签发令牌，让它们通过 MCP 查看主机、提交请求。
// AI 提交的请求照常判级，L1、L2 仍然要人在网页上批准；令牌只在签发时显示一次。
import { useCallback, useEffect, useState } from "react";
import { Bot, Check, Copy, Plus, Trash2, TriangleAlert, X } from "lucide-react";
import { api, type MCPToken } from "../api";
import { askConfirm } from "../dialogs";
import { fail, toast } from "../store";
import { stamp } from "../ui";
import { Select } from "../controls";
import { VerifyCancelled, withVerify } from "../verify";
import { useT } from "../i18n";

const NAME_RE = /^[A-Za-z0-9._-]{1,40}$/;

function CopyBlock({ text, label }: { text: string; label: string }) {
  const t = useT();
  return (
    <div className="col" style={{ gap: 6 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <span className="section-title">{label}</span>
        <button className="btn ghost icon sm" title={t("复制")} aria-label={t("复制")} onClick={() => navigator.clipboard.writeText(text).then(() => toast(t("已复制")))}><Copy /></button>
      </div>
      <pre className="code">{text}</pre>
    </div>
  );
}

export function MCPSettings({ owner }: { owner: boolean }) {
  const t = useT();
  const [tokens, setTokens] = useState<MCPToken[] | null>(null);
  const [endpoint, setEndpoint] = useState("");
  const [name, setName] = useState("claude-code");
  const [days, setDays] = useState("30");
  const [busy, setBusy] = useState(false);
  const [issued, setIssued] = useState<{ token: string; name: string } | null>(null);

  const load = useCallback(() => {
    if (owner) api.mcpTokens().then((v) => { setTokens(v.tokens); setEndpoint(v.endpoint); }).catch(fail);
  }, [owner]);
  useEffect(() => { load(); }, [load]);
  if (!owner || !tokens) return null;

  const nameOk = NAME_RE.test(name.trim());
  const create = async () => {
    setBusy(true);
    try {
      const v = await withVerify(() => api.mcpTokenCreate(name.trim(), Number(days)));
      setIssued({ token: v.token, name: v.info.name });
      setEndpoint(v.endpoint);
      load();
    } catch (e) {
      if (!(e instanceof VerifyCancelled)) fail(e);
    } finally { setBusy(false); }
  };
  const revoke = async (tk: MCPToken) => {
    if (!(await askConfirm(t("吊销后，用令牌 {name} 的 AI 助手立刻连不上 Hostower。", { name: tk.name })))) return;
    setBusy(true);
    try {
      await api.mcpTokenRevoke(tk.id);
      if (issued?.name === tk.name) setIssued(null);
      toast(t("已吊销"));
      load();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  const shown = issued?.token ?? "<token>";
  const claude = `claude mcp add --transport http hostower ${endpoint} --header "Authorization: Bearer ${shown}"`;
  const codex = `# ~/.codex/config.toml\n[mcp_servers.hostower]\nurl = "${endpoint}"\nbearer_token_env_var = "HOSTOWER_MCP_TOKEN"\n\n# ${t("启动 Codex 之前：")}\nexport HOSTOWER_MCP_TOKEN=${shown}`;
  const now = Date.now();

  return (
    <div className="card">
      <div className="card-head">
        <h2>{t("MCP 接入")}</h2>
        <span className="muted hide-sm">{t("让 Claude Code、Codex 这类 AI 助手查看主机、提交请求")}</span>
      </div>
      <div className="card-body">
        <div className="setting-row">
          <div>
            <div className="setting-name">{t("签发令牌")}</div>
            <div className="setting-desc">{t("每个 AI 助手用一个令牌，名称会出现在请求和审计里。令牌只显示一次，签发前要先验证身份。")}</div>
          </div>
          <div className="col" style={{ gap: 8 }}>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <input className="input mono" style={{ width: 180 }} value={name} aria-label={t("令牌名称")} placeholder="claude-code" onChange={(e) => setName(e.target.value)} />
              <Select value={days} disabled={busy} onChange={setDays} options={[
                { value: "7", label: t("{n} 天", { n: 7 }) }, { value: "30", label: t("{n} 天", { n: 30 }) },
                { value: "90", label: t("{n} 天", { n: 90 }) }, { value: "365", label: t("{n} 天", { n: 365 }) }, { value: "0", label: t("永久") },
              ]} />
              <button className="btn primary sm" disabled={busy || !nameOk} onClick={create}><Plus />{t("签发")}</button>
            </div>
            {!nameOk && name !== "" && <div className="hint">{t("名称只能用字母、数字、点、横线和下划线，最多 40 个字符")}</div>}
          </div>
        </div>

        {issued && (
          <div className="note warn" style={{ marginTop: 14 }}>
            <TriangleAlert />
            <div className="col" style={{ gap: 8, minWidth: 0, flex: 1 }}>
              <span>{t("令牌 {name} 已签发。它只显示这一次，关掉这个提示就再也看不到了；丢了就吊销重新签发。", { name: issued.name })}</span>
              <div className="cb-code" style={{ maxWidth: "100%" }}>
                <span className="mono ellipsis" style={{ fontSize: 13, letterSpacing: 0 }}>{issued.token}</span>
                <button className="btn ghost icon sm" title={t("复制")} aria-label={t("复制")} onClick={() => navigator.clipboard.writeText(issued.token).then(() => toast(t("已复制")))}><Copy /></button>
              </div>
              <div><button className="btn sm" onClick={() => setIssued(null)}>{t("我已保存好")}</button></div>
            </div>
          </div>
        )}

        {tokens.length > 0 && (
          <div className="cb-bind">
            {tokens.map((tk) => {
              const expired = !!tk.expiresAt && new Date(tk.expiresAt).getTime() < now;
              return (
                <div className="nt-row" key={tk.id}>
                  <span className="cb-key"><Bot /></span>
                  <div className="cell-2" style={{ minWidth: 0, flex: 1 }}>
                    <b className="ellipsis mono">{tk.name}</b>
                    <small className="ellipsis">
                      {[t("代表 {user} 发起", { user: tk.userName }),
                        expired ? t("已过期") : tk.expiresAt ? t("{time} 到期", { time: stamp(tk.expiresAt) }) : t("永久有效"),
                        tk.lastUsedAt ? t("最后使用 {time}", { time: stamp(tk.lastUsedAt) }) : t("还没用过")].join(" · ")}
                    </small>
                  </div>
                  {expired && <span className="badge neutral">{t("已过期")}</span>}
                  <button className="btn sm danger" disabled={busy} onClick={() => revoke(tk)}><Trash2 />{t("吊销")}</button>
                </div>
              );
            })}
          </div>
        )}

        <div className="col" style={{ gap: 14, marginTop: 18 }}>
          <CopyBlock label="Claude Code" text={claude} />
          <CopyBlock label="Codex" text={codex} />
        </div>

        <div className="cb-scope">
          <div>
            <div className="cb-scope-title ok"><Check />{t("AI 助手能做")}</div>
            <ul>
              <li>{t("列出主机、看指标、预估一条命令的风险等级")}</li>
              <li>{t("提交命令：只读的（L0）直接执行，其他的生成请求，等你在这里批准")}</li>
              <li>{t("查询、取消自己提交的请求")}</li>
            </ul>
          </div>
          <div>
            <div className="cb-scope-title crit"><X />{t("AI 助手不能做")}</div>
            <ul>
              <li>{t("批准任何请求（L1、L2 都要回网页批准，L3 一律拒绝）")}</li>
              <li>{t("开终端、读写文件、改凭据、暂停或恢复执行、改设置")}</li>
              <li>{t("看到别人提交的请求")}</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
