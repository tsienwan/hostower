// 聊天命令：把所有者的 Telegram 账号绑定到 Hostower，私聊机器人查状态、提交命令、紧急暂停。
// 绑定只能在这里用一次性配对码发起；聊天里不能批准、不能恢复执行、不能改设置。
import { useCallback, useEffect, useState } from "react";
import { askConfirm } from "../dialogs";
import { Check, Copy, KeyRound, Link2Off, RefreshCw, TriangleAlert, X } from "lucide-react";
import { api, type ChatBot } from "../api";
import { fail, toast, useStore } from "../store";
import { stamp } from "../ui";
import { Select } from "../controls";
import { BrandIcon } from "../brand";
import { ts, useT } from "../i18n";

export function ChatBotSettings({ version }: { version: number }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [bot, setBot] = useState<ChatBot | null>(null);
  const [ttl, setTtl] = useState("");
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());

  // ttlDays 为 0 表示永久：输入框里留着上次的天数，方便取消“永久”时直接用。
  const apply = useCallback((b: ChatBot) => { setBot(b); if (b.ttlDays) setTtl(String(b.ttlDays)); else setTtl((v) => v || "30"); }, []);
  const load = useCallback(() => { if (owner) api.chatBot().then(apply).catch(fail); }, [owner, apply]);
  // version 变了说明通知渠道有改动（新增、删除了 Telegram 渠道），重新取一次。
  useEffect(() => { load(); }, [load, version]);

  // 有配对码时每秒走一次倒计时，每 3 秒看一眼是不是已经绑定好了。
  const pairing = bot?.pairing;
  useEffect(() => {
    if (!pairing) return;
    const tick = setInterval(() => setNow(Date.now()), 1000);
    const poll = setInterval(() => api.chatBot().then(apply).catch(() => {}), 3000);
    return () => { clearInterval(tick); clearInterval(poll); };
  }, [pairing, apply]);
  // 没有配对码时也偶尔刷新一下运行状态（机器人连上、出错）。
  useEffect(() => {
    if (!bot?.enabled || pairing) return;
    const poll = setInterval(() => api.chatBot().then(apply).catch(() => {}), 10000);
    return () => clearInterval(poll);
  }, [bot?.enabled, pairing, apply]);

  if (!owner || !bot) return null;

  const run = async (fn: () => Promise<ChatBot>, done?: string) => {
    setBusy(true);
    try { apply(await fn()); if (done) toast(done); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const save = (patch: Partial<Pick<ChatBot, "enabled" | "channel" | "ttlDays">>, done?: string) =>
    run(() => api.setChatBot({ enabled: bot.enabled, channel: bot.channel, ttlDays: bot.ttlDays, ...patch }), done);
  const saveTtl = () => {
    const n = Number(ttl);
    if (!Number.isInteger(n) || n < 1 || n > 365) { toast(t("有效期要在 1 到 365 天之间")); setTtl(String(bot.ttlDays || 30)); return; }
    if (n !== bot.ttlDays) save({ ttlDays: n }, t("已保存"));
  };

  const hasChannels = bot.channels.length > 0;
  const left = pairing ? Math.max(0, Math.round((new Date(pairing.expiresAt).getTime() - now) / 1000)) : 0;
  const bindCmd = pairing ? `/bind ${pairing.code}` : "";
  const state = !bot.enabled ? { cls: "neutral", text: t("未启用") }
    : bot.lastError ? { cls: "crit", text: t("出错") }
    : bot.running ? { cls: "ok", text: t("运行中") } : { cls: "neutral", text: t("正在连接") };

  return (
    <div className="card">
      <div className="card-head">
        <h2>{t("聊天命令")}</h2>
        <span className="muted hide-sm">{t("私聊 Telegram 机器人查状态、提交命令、紧急暂停")}</span>
        <span className="grow" />
        <span className={"badge " + state.cls}>{state.text}</span>
      </div>
      <div className="card-body">
        <div className="setting-row">
          <div><div className="setting-name">{t("启用聊天命令")}</div><div className="setting-desc">{t("默认关闭。关闭时机器人只发通知，不接收任何消息。")}</div></div>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button role="switch" aria-checked={bot.enabled} aria-label={t("启用聊天命令")} className={"switch" + (bot.enabled ? " on" : "")}
              disabled={busy || (!bot.enabled && !bot.channel && !hasChannels)}
              onClick={() => save({ enabled: !bot.enabled, channel: bot.channel || bot.channels[0]?.id || "" }, bot.enabled ? t("已关闭聊天命令") : t("已打开聊天命令"))}><span /></button>
          </div>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("使用的机器人")}</div><div className="setting-desc">{t("用哪个 Telegram 通知渠道的机器人收命令。建议给 Hostower 单独建一个机器人：同一个机器人不能同时被两个程序接收消息。")}</div></div>
          {hasChannels
            ? <Select value={bot.channel} disabled={busy} onChange={async (v) => { if (!bot.binding || (await askConfirm(t("换成别的机器人会解除现在的绑定，继续吗？")))) save({ channel: v }, t("已保存")); }}
                options={[...(bot.channel ? [] : [{ value: "", label: t("请选择") }]), ...bot.channels.map((c) => ({ value: c.id, label: c.name, hint: c.enabled ? undefined : t("通知已停用") }))]} />
            : <div className="note"><TriangleAlert /><span>{t("还没有 Telegram 通知渠道。先在上面添加一个，再回来选择它的机器人。")}</span></div>}
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("绑定有效期")}</div><div className="setting-desc">{t("过期后要回这里重新配对。缩短会立即对现有绑定生效。")}</div></div>
          <div className="col" style={{ gap: 10 }}>
            <div className="row" style={{ gap: 8, flexWrap: "nowrap" }}>
              <input className="input mono" style={{ width: 96 }} inputMode="numeric" value={ttl} disabled={busy || bot.ttlDays === 0} aria-label={t("绑定有效期（天）")}
                onChange={(e) => setTtl(e.target.value.replace(/\D/g, ""))} onBlur={saveTtl} onKeyDown={(e) => { if (e.key === "Enter") saveTtl(); }} />
              <span className="muted">{t("天")}</span>
              <label className="check" style={{ marginLeft: 8 }}>
                <input type="checkbox" checked={bot.ttlDays === 0} disabled={busy}
                  onChange={(e) => save({ ttlDays: e.target.checked ? 0 : Number(ttl) || 30 }, t("已保存"))} />{t("永久")}
              </label>
            </div>
            {bot.ttlDays === 0 && <div className="note warn"><TriangleAlert /><span>{t("永久绑定不会过期：只要这个 Telegram 账号还在，就一直能查状态、提交命令和紧急暂停（仍然不能批准）。手机丢了或账号可能被盗时，记得来这里解除绑定，或者在“安全”里吊销所有会话。")}</span></div>}
          </div>
        </div>

        {bot.lastError && bot.enabled && <div className="note crit" style={{ marginTop: 14 }}><TriangleAlert /><span>{ts(bot.lastError)}</span></div>}

        {bot.enabled && bot.channel && (
          <div className="cb-bind">
            {bot.binding ? (
              <div className="nt-row">
                <BrandIcon kind="telegram" />
                <div className="cell-2" style={{ minWidth: 0, flex: 1 }}>
                  <b className="ellipsis">{bot.binding.chatName}</b>
                  <small className="ellipsis">{bot.binding.expiresAt ? t("代表 {user} 操作 · {time} 到期", { user: bot.binding.userName, time: stamp(bot.binding.expiresAt) }) : t("代表 {user} 操作 · 永久有效", { user: bot.binding.userName })}</small>
                </div>
                <button className="btn sm" disabled={busy} onClick={() => run(api.chatPair)}><RefreshCw />{t("重新配对")}</button>
                <button className="btn sm danger" disabled={busy} onClick={async () => (await askConfirm(t("解除绑定后，这个 Telegram 账号发的命令都不再生效。"))) && run(api.chatUnbind, t("已解除绑定"))}><Link2Off />{t("解除绑定")}</button>
              </div>
            ) : !pairing && (
              <div className="nt-row">
                <span className="cb-key"><KeyRound /></span>
                <div className="cell-2" style={{ minWidth: 0, flex: 1 }}>
                  <b>{t("还没有绑定 Telegram 账号")}</b>
                  <small>{t("生成一个一次性配对码，在 Telegram 里私聊机器人发送它，就能把你的账号绑定上。")}</small>
                </div>
                <button className="btn primary sm" disabled={busy || !bot.running} onClick={() => run(api.chatPair)}>{t("生成配对码")}</button>
              </div>
            )}
            {pairing && left > 0 && (
              <div className="cb-pair">
                <div className="cb-step">{t("在 Telegram 里私聊")} {bot.botUsername
                  ? <a href={`https://t.me/${bot.botUsername}`} target="_blank" rel="noreferrer">@{bot.botUsername}</a>
                  : t("机器人")}{t("，发送：")}</div>
                <div className="cb-code">
                  <span className="mono">{bindCmd}</span>
                  <button className="btn ghost icon sm" title={t("复制")} aria-label={t("复制")} onClick={() => navigator.clipboard.writeText(bindCmd).then(() => toast(t("已复制")))}><Copy /></button>
                </div>
                <div className="hint">{t("配对码 {m}:{s} 后失效，只能用一次，输错 5 次作废。绑定成功后这里会自动刷新。", { m: Math.floor(left / 60), s: String(left % 60).padStart(2, "0") })}</div>
              </div>
            )}
          </div>
        )}

        <div className="cb-scope">
          <div>
            <div className="cb-scope-title ok"><Check />{t("在聊天里能做")}</div>
            <ul>
              <li><span className="mono">/status</span> {t("主机在线情况、待批准数量")}</li>
              <li><span className="mono">/pending</span> · <span className="mono">/failed</span> {t("等待批准的请求、最近失败的执行")}</li>
              <li><span className="mono">/run web-1 df -h</span> {t("提交命令：只读的直接执行并回传结果，其他的生成请求")}</li>
              <li><span className="mono">/pause</span> {t("紧急暂停全部执行")}</li>
            </ul>
          </div>
          <div>
            <div className="cb-scope-title crit"><X />{t("在聊天里不能做")}</div>
            <ul>
              <li>{t("批准任何请求（L1、L2 都要回网页批准）")}</li>
              <li>{t("恢复执行、开终端、改设置、改凭据")}</li>
              <li>{t("在群组里使用；别人发的消息一律不回应")}</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
