// 通知渠道：Telegram 机器人、飞书机器人、Discord、邮件。
// 令牌、Webhook 地址、密码只写不读：编辑时留空表示保持原来的。
import { useCallback, useEffect, useMemo, useState } from "react";
import { Bell, Pencil, Plus, Send, Trash2, TriangleAlert } from "lucide-react";
import { BrandIcon } from "../brand";
import { api, type NotifyChannel, type NotifyEvent, type NotifyStatus } from "../api";
import { fail, toast, useStore } from "../store";
import { Modal, ago } from "../ui";
import { Select, TagInput } from "../controls";
import { lang as uiLang, languages, ts, useT } from "../i18n";

type Kind = NotifyChannel["kind"];

// 值是中文原文，显示时翻译。
const kindNames: Record<Kind, string> = { telegram: "Telegram 机器人", feishu: "飞书机器人", discord: "Discord", email: "邮件" };
const eventNames: Record<string, [string, string]> = {
  "approval.pending": ["有请求等待批准", "L1、L2 的变更提交后，等人批准时"],
  "request.failed": ["执行失败", "有主机执行失败"],
  "request.done": ["执行完成", "变更执行成功（只读操作不通知）"],
  "request.rejected": ["禁止的操作被拒绝", "有人或 AI 提交了 L3 操作"],
  "host.offline": ["主机离线", "连不上超过 2 分钟"],
  "host.online": ["主机恢复在线", "离线通知发出后又连上了"],
  "auth.failed": ["登录失败", "Passkey 登录、二次验证、初始化失败，聊天账号配对码输错"],
  "terminal.opened": ["打开终端", "有人打开了交互终端"],
  "system.paused": ["紧急开关", "暂停、恢复执行，吊销所有会话"],
  "secret.changed": ["凭据改动", "SSH 凭据、镜像仓库、应用密钥的新增和删除"],
  "system.update": ["更新", "Hostower 更新完成，或更新因签名不对被拒绝"],
};
const groupNames: Record<string, string> = { approval: "审批", run: "执行", host: "主机", security: "安全", system: "系统" };


export function NotifySettings({ onChange }: { onChange?: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [data, setData] = useState<{ channels: NotifyChannel[]; status: Record<string, NotifyStatus>; events: NotifyEvent[]; defaults: string[]; available: boolean } | null>(null);
  const [editing, setEditing] = useState<NotifyChannel | "new" | null>(null);
  const [busy, setBusy] = useState("");
  const load = useCallback(() => api.notifyList().then((d) => { setData(d); onChange?.(); }).catch(fail), [onChange]);
  useEffect(() => { load(); }, [load]);
  if (!data) return null;

  const test = async (c: NotifyChannel) => {
    setBusy(c.id);
    try { await api.notifyTest(c.id); toast(t("测试通知已发出，去 {name} 看看", { name: c.name })); } catch (e) { fail(e); } finally { setBusy(""); load(); }
  };
  const toggle = async (c: NotifyChannel) => {
    setBusy(c.id);
    try { await api.notifySave({ ...c, enabled: !c.enabled }, {}); } catch (e) { fail(e); } finally { setBusy(""); load(); }
  };
  const remove = async (c: NotifyChannel) => {
    if (!confirm(t("删除通知渠道“{name}”？保存的令牌或密码也会一起删掉。", { name: c.name }))) return;
    try { await api.notifyDelete(c.id); toast(t("已删除")); } catch (e) { fail(e); } finally { load(); }
  };

  return (
    <div className="card">
      <div className="card-head">
        <h2>{t("通知")}</h2>
        <span className="muted hide-sm">{t("把需要你知道的事推送到 Telegram、飞书、Discord 或邮箱")}</span>
        <span className="grow" />
        {owner && <button className="btn sm" disabled={!data.available} onClick={() => setEditing("new")}><Plus />{t("添加渠道")}</button>}
      </div>
      <div className="card-body col" style={{ gap: 10 }}>
        {!data.available && <div className="note warn"><TriangleAlert /><span>{t("没有配置主密钥，不能保存令牌和密码，通知渠道暂时用不了。")}</span></div>}
        {data.channels.length === 0 ? (
          <div className="empty sm"><Bell /><span>{t("还没有通知渠道。添加一个之后，审批待办、执行失败、主机离线这些事会推送给你。")}</span></div>
        ) : data.channels.map((c) => {
          const st = data.status[c.id];
          const failed = st?.lastError && (!st.lastOk || (st.lastErrorAt ?? 0) > st.lastOk);
          return (
            <div key={c.id} className="nt-row">
              <BrandIcon kind={c.kind} />
              <div className="cell-2" style={{ minWidth: 0, flex: 1 }}>
                <span className="row" style={{ gap: 8 }}>
                  <b className="ellipsis">{c.name}</b>
                  <span className="muted" style={{ fontSize: 12.5 }}>{t(kindNames[c.kind])}</span>
                  {!c.enabled && <span className="badge neutral">{t("已停用")}</span>}
                </span>
                <small className="ellipsis">
                  {t("订阅 {n} 类通知", { n: c.events.length })}
                  {c.hosts?.length ? " · " + t("只看 {hosts}", { hosts: c.hosts.join(", ") }) : ""}
                  {st?.lastOk ? " · " + t("上次发送成功 {ago}", { ago: ago(new Date(st.lastOk).toISOString()) }) : ""}
                </small>
                {failed && <small className="nt-err" title={ts(st!.lastError!)}>{t("上次发送失败：{err}", { err: ts(st!.lastError!) })}</small>}
              </div>
              {owner && (
                <div className="row" style={{ gap: 4, flexWrap: "nowrap" }}>
                  <button role="switch" aria-checked={c.enabled} aria-label={t("启用")} className={"switch" + (c.enabled ? " on" : "")} disabled={busy === c.id} onClick={() => toggle(c)}><span /></button>
                  <button className="btn ghost icon sm" disabled={busy === c.id} onClick={() => test(c)} title={t("发一条测试通知")} aria-label={t("发一条测试通知")}><Send /></button>
                  <button className="btn ghost icon sm" onClick={() => setEditing(c)} title={t("编辑")} aria-label={t("编辑")}><Pencil /></button>
                  <button className="btn ghost icon sm" onClick={() => remove(c)} title={t("删除")} aria-label={t("删除")}><Trash2 /></button>
                </div>
              )}
            </div>
          );
        })}
        <div className="hint">{t("通知里只有摘要和一个回到页面的链接，不能在聊天软件里批准。同一条通知一分钟内只发一次，每个渠道每分钟最多 20 条。")}</div>
      </div>
      {editing && <ChannelForm initial={editing === "new" ? null : editing} events={data.events} defaults={data.defaults}
        onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </div>
  );
}

function ChannelForm({ initial, events, defaults, onClose, onSaved }: {
  initial: NotifyChannel | null; events: NotifyEvent[]; defaults: string[]; onClose: () => void; onSaved: () => void;
}) {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const editing = !!initial;
  const [kind, setKind] = useState<Kind>(initial?.kind ?? "telegram");
  const [name, setName] = useState(initial?.name ?? "");
  // 新渠道默认用界面当前的语言（设置里选的，或者跟随系统）。
  const [lang, setLang] = useState(initial?.lang ?? uiLang());
  const [picked, setPicked] = useState<string[]>(initial?.events ?? defaults);
  const [onlyHosts, setOnlyHosts] = useState<string[]>(initial?.hosts ?? []);
  const [chatId, setChatId] = useState(initial?.telegram?.chatId ?? "");
  const [token, setToken] = useState("");
  const [webhook, setWebhook] = useState("");
  const [sign, setSign] = useState("");
  const [smtp, setSmtp] = useState(initial?.email ?? { host: "", port: 465, security: "tls" as const, username: "", from: "", to: [] as string[] });
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const groups = useMemo(() => {
    const out: Record<string, NotifyEvent[]> = {};
    for (const e of events) (out[e.group] ??= []).push(e);
    return out;
  }, [events]);

  const keep = editing ? t("留空表示不修改") : "";
  const save = async () => {
    setBusy(true);
    try {
      const body: Partial<NotifyChannel> = { kind, name, lang, events: picked, hosts: onlyHosts, enabled: initial?.enabled ?? true };
      if (kind === "telegram") body.telegram = { chatId };
      if (kind === "email") body.email = smtp;
      await api.notifySave({ ...body, id: initial?.id } as NotifyChannel, { token, webhook, sign, password });
      toast(editing ? t("已保存") : t("已添加，可以点纸飞机按钮发一条测试通知"));
      onSaved();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <Modal wide title={editing ? t("编辑通知渠道") : t("添加通知渠道")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !name.trim()} onClick={save}>{t("保存")}</button></>}>
      {!editing && (
        <div className="nt-kinds" role="radiogroup" aria-label={t("渠道类型")}>
          {(Object.keys(kindNames) as Kind[]).map((k) => (
            <button key={k} role="radio" aria-checked={kind === k} className={"nt-kind" + (kind === k ? " on" : "")} onClick={() => setKind(k)}>
              <BrandIcon kind={k} size={36} />{t(kindNames[k])}
            </button>
          ))}
        </div>
      )}
      <div className="form-grid">
        <div className="field"><label htmlFor="nt-name">{t("名称")}</label><input id="nt-name" className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("例如：运维群")} /></div>
        <div className="field"><label htmlFor="nt-lang">{t("通知语言")}</label>
          <Select id="nt-lang" value={lang} onChange={setLang} options={languages.map((l) => ({ value: l.code, label: l.label }))} />
        </div>
      </div>

      {kind === "telegram" && (
        <>
          <div className="field"><label htmlFor="nt-token">{t("机器人令牌")}</label>
            <input id="nt-token" className="input mono" type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} placeholder={keep || "123456789:AA…"} />
            <span className="hint">{t("在 Telegram 里找 @BotFather，发 /newbot 创建机器人后得到。")}</span>
          </div>
          <div className="field"><label htmlFor="nt-chat">{t("会话编号")}</label>
            <input id="nt-chat" className="input mono" value={chatId} onChange={(e) => setChatId(e.target.value)} placeholder="-1001234567890" />
            <span className="hint">{t("把机器人拉进群（或先给它发一条消息），再打开 https://api.telegram.org/bot<令牌>/getUpdates，找 chat 里的 id。频道可以填 @频道名。")}</span>
          </div>
        </>
      )}
      {kind === "feishu" && (
        <>
          <div className="field"><label htmlFor="nt-hook">{t("Webhook 地址")}</label>
            <input id="nt-hook" className="input mono" type="password" autoComplete="off" value={webhook} onChange={(e) => setWebhook(e.target.value)} placeholder={keep || "https://open.feishu.cn/open-apis/bot/v2/hook/…"} />
            <span className="hint">{t("在飞书群的“设置 → 群机器人 → 添加机器人 → 自定义机器人”里复制。地址本身就是凭据，不要公开。")}</span>
          </div>
          <div className="field"><label htmlFor="nt-sign">{t("签名密钥（可选）")}</label>
            <input id="nt-sign" className="input mono" type="password" autoComplete="off" value={sign} onChange={(e) => setSign(e.target.value)} placeholder={keep || t("机器人安全设置里打开“签名校验”才需要")} />
          </div>
        </>
      )}
      {kind === "discord" && (
        <div className="field"><label htmlFor="nt-hook">{t("Webhook 地址")}</label>
          <input id="nt-hook" className="input mono" type="password" autoComplete="off" value={webhook} onChange={(e) => setWebhook(e.target.value)} placeholder={keep || "https://discord.com/api/webhooks/…"} />
          <span className="hint">{t("在频道的“编辑频道 → 整合 → Webhooks”里新建并复制地址。通知里的 @ 不会真的提醒任何人。")}</span>
        </div>
      )}
      {kind === "email" && (
        <>
          <div className="form-grid">
            <div className="field"><label htmlFor="nt-host">{t("SMTP 服务器")}</label><input id="nt-host" className="input mono" value={smtp.host} onChange={(e) => setSmtp({ ...smtp, host: e.target.value })} placeholder="smtp.example.com" /></div>
            <div className="field"><label htmlFor="nt-sec">{t("加密方式")}</label>
              <Select id="nt-sec" value={smtp.security} onChange={(v) => setSmtp({ ...smtp, security: v as "tls", port: v === "tls" ? 465 : v === "starttls" ? 587 : 25 })}
                options={[{ value: "tls", label: "SSL/TLS", hint: "465" }, { value: "starttls", label: "STARTTLS", hint: "587" }, { value: "none", label: t("不加密"), hint: t("只用于本机的中转") }]} />
            </div>
          </div>
          <div className="form-grid">
            <div className="field"><label htmlFor="nt-port">{t("端口")}</label><input id="nt-port" className="input mono" inputMode="numeric" value={smtp.port} onChange={(e) => setSmtp({ ...smtp, port: Number(e.target.value.replace(/\D/g, "")) || 0 })} /></div>
            <div className="field"><label htmlFor="nt-user">{t("用户名")}</label><input id="nt-user" className="input mono" autoComplete="off" value={smtp.username ?? ""} onChange={(e) => setSmtp({ ...smtp, username: e.target.value })} placeholder={t("通常就是发件邮箱")} /></div>
          </div>
          <div className="form-grid">
            <div className="field"><label htmlFor="nt-pass">{t("密码或授权码")}</label><input id="nt-pass" className="input mono" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={keep || t("很多邮箱要用“授权码”，不是登录密码")} /></div>
            <div className="field"><label htmlFor="nt-from">{t("发件人")}</label><input id="nt-from" className="input mono" value={smtp.from} onChange={(e) => setSmtp({ ...smtp, from: e.target.value })} placeholder="hostower@example.com" /></div>
          </div>
          <div className="field"><label htmlFor="nt-to">{t("收件人")}</label>
            <TagInput id="nt-to" value={smtp.to} onChange={(to) => setSmtp({ ...smtp, to })} suggestions={[]} placeholder={t("输入邮箱后按回车，可以填多个")}
              pattern={/^[^@\s<>,;"]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,24}$/} invalid={t("不是有效的邮箱地址")} keepCase />
          </div>
        </>
      )}

      <div className="field">
        <label>{t("推送哪些事")}</label>
        <div className="nt-events">
          {Object.entries(groups).map(([g, list]) => (
            <div key={g} className="nt-group">
              <div className="nt-group-name">{t(groupNames[g] ?? g)}</div>
              {list.map((e) => (
                <label key={e.key} className="check nt-event">
                  <input type="checkbox" checked={picked.includes(e.key)} onChange={(ev) => setPicked(ev.target.checked ? [...picked, e.key] : picked.filter((x) => x !== e.key))} />
                  <span className="cell-2"><span>{t(eventNames[e.key]?.[0] ?? e.key)}</span><small>{t(eventNames[e.key]?.[1] ?? "")}</small></span>
                </label>
              ))}
            </div>
          ))}
        </div>
      </div>
      <div className="field">
        <label htmlFor="nt-hosts">{t("只关心这些主机（可选）")}</label>
        <TagInput id="nt-hosts" value={onlyHosts} onChange={setOnlyHosts} suggestions={hosts.map((h) => h.name)} placeholder={t("留空表示所有主机")}
          pattern={/^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$/} invalid={t("主机名只能用字母、数字、点、下划线、连字符")} hint={t("主机：")} keepCase />
        <span className="hint">{t("填了之后，只有和这些主机有关的执行、离线通知会推送到这里；登录失败、更新这类与主机无关的照常推送。")}</span>
      </div>
    </Modal>
  );
}
