import { useCallback, useEffect, useState } from "react";
import { askConfirm } from "../dialogs";
import { Archive, Bell, OctagonPause, Play, RefreshCw, RotateCcw, Server, Shield, ShieldCheck, SlidersHorizontal, TriangleAlert, UserX } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { api, type AuditArchiveView, type UpdateStatus, type PanelView, type RetentionView } from "../api";
import { fail, loadBoot, refreshHosts, setPaused, toast, useStore } from "../store";
import { cached, remember, useCached } from "../cache";
import { Modal, ago, bytes, stamp, uptime } from "../ui";
import { withVerify } from "../verify";
import { Select } from "../controls";
import { NotifySettings } from "./Notify";
import { ChatBotSettings } from "./ChatBot";
import { OIDCSettings } from "./OIDC";
import { MCPSettings } from "./MCP";
import { LoginSecurity } from "./Security";
import { langSetting, languages, setLang, setTimeZone, systemTimeZone, timeZoneOffset, timeZoneSetting, timeZones, ts, useT, type LangMode } from "../i18n";

type Sys = Awaited<ReturnType<typeof api.system>>;

// 设置分区。标签是中文原文，显示时翻译。
const sections = [
  { key: "general", label: "通用", icon: SlidersHorizontal },
  { key: "panel", label: "面板", icon: Server },
  { key: "notify", label: "通知与机器人", icon: Bell },
  { key: "security", label: "安全", icon: Shield },
] as const;

export function SettingsPage() {
  const t = useT();
  const boot = useStore((s) => s.boot)!;
  const paused = useStore((s) => s.paused);
  const [sys, setSys] = useState<Sys | null>(null);
  const [confirm, setConfirm] = useState<"pause" | "revoke" | null>(null);
  const [busy, setBusy] = useState(false);
  const [sp, setSp] = useSearchParams();
  const tab = sections.find((x) => x.key === sp.get("tab"))?.key ?? "general";
  const [notifyVersion, setNotifyVersion] = useState(0);
  const bumpNotify = useCallback(() => setNotifyVersion((v) => v + 1), []);

  const load = useCallback(() => {
    api.system().then(setSys).catch(fail);
  }, []);
  useEffect(load, [load]);

  const act = async (fn: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    try { await fn(); if (done) toast(done); load(); } catch (e) { fail(e); } finally { setBusy(false); setConfirm(null); }
  };

  return (
    <>
      <div className="topbar"><h1>{t("设置")}</h1></div>
      <div className="content">
        <div className="page narrow settings-layout">
          <nav className="settings-nav" aria-label={t("设置分区")}>
            {sections.map((x) => (
              <button key={x.key} className={tab === x.key ? "on" : ""} aria-current={tab === x.key ? "page" : undefined}
                onClick={() => setSp(x.key === "general" ? {} : { tab: x.key }, { replace: true })}><x.icon />{t(x.label)}</button>
            ))}
          </nav>
          <div className="settings-main">
            {/* 四个分区一直挂着，只切换显示：数据在打开设置页时一次取完，切换时不会先空、再一张张卡片冒出来把下面的往下挤。 */}
            <section className="settings-section" hidden={tab !== "general"}>
                <div className="card">
                  <div className="card-head"><h2>{t("关于")}</h2></div>
                  <div className="card-body">
                    <dl className="kv">
                      <dt>{t("版本")}</dt><dd className="mono">Hostower {sys?.version ?? boot.version}</dd>
                      <dt>{t("已运行")}</dt><dd>{uptime(Math.max(60, (Date.now() - boot.started) / 1000))}<span className="muted">　{t("启动于 {time}", { time: stamp(boot.started) })}</span></dd>
                      <dt>{t("访问地址")}</dt><dd className="mono">{sys?.publicURL}</dd>
                      <dt>{t("模式")}</dt><dd>{boot.dev ? t("开发模式（模拟主机）") : t("正常")}</dd>
                      <dt>{t("构建")}</dt><dd><BuildBadge build={boot.build} /></dd>
                      {boot.build?.fingerprint && <><dt>{t("发布公钥指纹")}</dt><dd className="mono">{boot.build.fingerprint}<div className="hint">{t("和官方公布的指纹一致，才是官方构建。也可以在主机上运行 hostower verify <官方公钥> 离线校验。")}</div></dd></>}
                      <dt>{t("许可证")}</dt><dd>GNU AGPL-3.0{boot.source && <> · <a href={boot.source} target="_blank" rel="noreferrer noopener" style={{ color: "var(--accent)" }}>{t("源代码")}</a></>}<div className="hint">{t("Hostower 名称和图标是项目的商标（™），改版后再发布请换名字和图标，见仓库里的 TRADEMARKS.md。")}</div></dd>
                      <dt>{t("清单")}</dt><dd>{sys ? t("{hosts} 台主机 · {groups} 个分组 · {snippets} 个 Snippet", { hosts: sys.hosts, groups: sys.groups, snippets: sys.snippets }) : "—"}</dd>
                    </dl>
                    <div className="row" style={{ marginTop: 16 }}>
                      <button className="btn sm" disabled={busy} onClick={() => act(async () => { await api.reload(); await refreshHosts(); }, t("配置已重新加载"))}><RefreshCw />{t("重新加载配置文件")}</button>
                      <span className="muted" style={{ fontSize: 12.5 }}>{t("主机清单和 Snippet 文件改动后点这里；规则包只在启动时加载。")}</span>
                    </div>
                  </div>
                </div>
                <UpdateSettings />
                <InterfaceSettings />
                <RetentionSettings owner={boot.user?.role === "owner"} />
            </section>

            <section className="settings-section" hidden={tab !== "panel"}><PanelSettings dev={boot.dev} /></section>

            <section className="settings-section" hidden={tab !== "notify"}>
                <NotifySettings onChange={bumpNotify} />
                <ChatBotSettings version={notifyVersion} />
            </section>

            <section className="settings-section" hidden={tab !== "security"}>
                <LoginSecurity />
                <OIDCSettings owner={boot.user?.role === "owner"} />
                <MCPSettings owner={boot.user?.role === "owner"} />

                <div className="card">
                  <div className="card-head"><h2>{t("紧急开关")}</h2></div>
                  <div className="card-body">
                    <div className="setting-row">
                      <div><div className="setting-name">{paused ? t("执行已暂停") : t("暂停全部执行")}</div><div className="setting-desc">{t("立刻取消正在运行的操作，并拒绝新的提交、批准和指标采样，直到恢复。")}</div></div>
                      <div>{paused
                        ? <button className="btn primary" disabled={busy} onClick={() => act(async () => { await api.resume(); setPaused(false); }, t("已恢复执行"))}><Play />{t("恢复执行")}</button>
                        : <button className="btn danger" disabled={busy} onClick={() => setConfirm("pause")}><OctagonPause />{t("暂停全部执行")}</button>}</div>
                    </div>
                    <div className="setting-row">
                      <div><div className="setting-name">{t("吊销所有会话")}</div><div className="setting-desc">{t("所有已登录的会话（包括你自己的）立刻失效，需要重新登录；聊天账号的绑定和 MCP 令牌也一起解除。")}</div></div>
                      <div><button className="btn danger" disabled={busy} onClick={() => setConfirm("revoke")}><UserX />{t("吊销所有会话")}</button></div>
                    </div>
                  </div>
                </div>

                <AuditSettings />
            </section>
          </div>
        </div>
      </div>

      {confirm === "pause" && (
        <Modal title={t("暂停全部执行？")} onClose={() => setConfirm(null)}
          footer={<><button className="btn" onClick={() => setConfirm(null)}>{t("取消")}</button><button className="btn danger-solid" disabled={busy} onClick={() => act(async () => { await api.pause(); setPaused(true); }, t("已暂停全部执行"))}>{t("暂停")}</button></>}>
          <p className="dim">{t("正在运行的操作会被取消，新的提交和批准会被拒绝。这个动作会写入审计，可以随时恢复。")}</p>
        </Modal>
      )}
      {confirm === "revoke" && (
        <Modal title={t("吊销所有会话？")} onClose={() => setConfirm(null)}
          footer={<><button className="btn" onClick={() => setConfirm(null)}>{t("取消")}</button><button className="btn danger-solid" disabled={busy} onClick={() => act(async () => { await api.revokeSessions(); await loadBoot(); })}>{t("吊销并退出")}</button></>}>
          <p className="dim">{t("包括当前会话在内，所有人都要重新登录。")}{boot.devSession ? t("开发模式下需要再次输入启动令牌。") : ""}</p>
        </Modal>
      )}
    </>
  );
}

const split = (listen: string): [string, string] => {
  const i = listen.lastIndexOf(":");
  return i < 0 ? ["", listen] : [listen.slice(0, i), listen.slice(i + 1)];
};

/** 面板设置：监听、访问地址、证书、会话时长。保存前服务端会校验，重启后生效。 */
/** 自动更新：开关、当前和最新版本、手动检查与更新。新版本在服务端验过发布签名才会换上去。 */
function UpdateSettings() {
  const t = useT();
  const [st, setSt] = useCached<UpdateStatus>("settings.update");
  const [busy, setBusy] = useState(false);
  const [switching, setSwitching] = useState("");
  const load = useCallback(() => api.update().then(setSt).catch(() => {}), []);
  useEffect(() => { load(); }, [load]);

  // 下载或切换的过程中轮询；切换时服务会断开几秒，连回来之后版本号变了就刷新页面。
  const active = !!st && st.state !== "idle";
  useEffect(() => {
    if (!active && !switching) return;
    const timer = setInterval(async () => {
      try {
        const next = await api.update();
        setSt(next);
        if (switching && next.current !== switching) location.reload();
        if (switching && next.state === "idle" && next.error) setSwitching("");
      } catch { /* 切换的那几秒连不上是正常的 */ }
    }, 2000);
    return () => clearInterval(timer);
  }, [active, switching]);

  const run = async (fn: () => Promise<UpdateStatus>, done?: string) => {
    setBusy(true);
    try { setSt(await fn()); if (done) toast(done); } catch (e) { fail(e); load(); } finally { setBusy(false); }
  };
  if (!st) return null;
  const working = st.state === "downloading" || st.state === "restarting" || !!switching;
  return (
    <div className="card">
      <div className="card-head"><h2>{t("更新")}</h2>{st.newer && !working && <span className="badge accent">{t("有新版本 {v}", { v: st.latest ?? "" })}</span>}</div>
      <div className="card-body">
        <div className="row" style={{ alignItems: "flex-start", gap: 20 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="setting-name">{t("自动更新")}</div>
            <div className="setting-desc">{t("每隔几小时看一次发布页。有新版本时自动下载，验证发布签名后切换过去；切换时页面会断开几秒，正在执行的操作会被中断。")}</div>
          </div>
          <div>
            <button role="switch" aria-checked={st.auto} aria-label={t("自动更新")} className={"switch" + (st.auto ? " on" : "")} disabled={busy || !st.supported}
              onClick={() => run(() => api.setUpdate(!st.auto), st.auto ? t("已关闭自动更新") : t("已打开自动更新"))}><span /></button>
          </div>
        </div>
        {!st.supported && <div className="note"><TriangleAlert /><span>{ts(st.reason ?? "")}</span></div>}
        <dl className="kv" style={{ marginTop: 14 }}>
          <dt>{t("当前版本")}</dt><dd className="mono">{st.current}</dd>
          <dt>{t("最新版本")}</dt>
          <dd className="mono">{st.latest ? <>{st.latest}{!st.newer && <span className="muted" style={{ fontFamily: "var(--sans)" }}>　{t("已经是最新")}</span>}{st.url && <>　<a href={st.url} target="_blank" rel="noreferrer">{t("发布说明")}</a></>}</> : <span className="muted">—</span>}</dd>
          <dt>{t("上次检查")}</dt><dd>{st.checkedAt ? stamp(st.checkedAt) : <span className="muted">{t("还没有检查过")}</span>}</dd>
        </dl>
        {st.error && !working && <div className="note crit" style={{ marginTop: 14 }}><TriangleAlert /><span>{ts(st.error)}</span></div>}
        {working && <div className="note" style={{ marginTop: 14 }}><RefreshCw className="spin" /><span>{st.state === "downloading" ? t("正在下载并验证新版本…") : t("正在切换到新版本，页面会自动重新连接…")}</span></div>}
        <div className="row" style={{ marginTop: 16 }}>
          <button className="btn sm" disabled={busy || working || !st.supported} onClick={() => run(api.checkUpdate)}><RefreshCw className={st.state === "checking" ? "spin" : ""} />{t("检查更新")}</button>
          {st.newer && <button className="btn primary sm" disabled={busy || working} onClick={() => { setSwitching(st.current); run(api.applyUpdate); }}>{t("更新到 {v}", { v: st.latest ?? "" })}</button>}
        </div>
      </div>
    </div>
  );
}

/** 审计的范围：日常查看要不要也记下来。关键操作永远记录，不受这个开关影响。 */
function AuditSettings() {
  const t = useT();
  const [record, setRecord] = useCached<boolean>("settings.routine");
  const [busy, setBusy] = useState(false);
  useEffect(() => { api.auditRoutine().then((r) => setRecord(r.record)).catch(() => {}); }, []);
  if (record === null) return null;
  const toggle = async () => {
    setBusy(true);
    try {
      const r = await api.setAuditRoutine(!record);
      setRecord(r.record);
      toast(r.record ? t("日常查看开始写入审计") : t("日常查看不再写入审计"));
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <div className="card">
      <div className="card-head"><h2>{t("审计")}</h2></div>
      <div className="card-body">
        <div className="row" style={{ alignItems: "flex-start", gap: 20 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="setting-name">{t("记录日常查看")}</div>
            <div className="setting-desc">{t("翻目录、列容器、看容器日志和配置、搜文件这类只读查看要不要也写进审计。默认不记：它们数量很多，会淹没关键操作，也让审计越长越快。")}</div>
            <div className="setting-desc" style={{ marginTop: 6 }}>{t("不受这个开关影响、永远记录的：登录、提交和批准、每一次执行、打开终端、读取和下载文件内容、上传、改设置。开关本身的每次改动也会记录。")}</div>
          </div>
          <button role="switch" aria-checked={record} aria-label={t("记录日常查看")} className={"switch" + (record ? " on" : "")} disabled={busy} onClick={toggle}><span /></button>
        </div>
        <AuditArchiveSettings />
      </div>
    </div>
  );
}

/** 审计归档：记录多到一定程度后，把旧的写进带摘要的归档文件再从数据库里删掉。 */
function AuditArchiveSettings() {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [v, setV] = useCached<AuditArchiveView>("settings.archive");
  const [busy, setBusy] = useState(false);
  const load = useCallback(() => api.auditArchive().then(setV).catch(() => {}), []);
  useEffect(() => { load(); }, [load]);
  if (!v) return null;
  const count = (n: number) => (n === 0 ? t("不自动归档") : t("超过 {n} 条", { n: n.toLocaleString() }));
  const days = (n: number) => (n >= 365 && n % 365 === 0 ? t("{n} 年", { n: n / 365 }) : t("{n} 天", { n }));
  const save = async (p: { maxRecords: number; keepDays: number }) => {
    setBusy(true);
    try { setV(await api.setAuditArchive(p)); toast(t("已保存")); setTimeout(load, 3000); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const run = async () => {
    setBusy(true);
    try {
      const r = await api.runAuditArchive();
      toast(r.checkpoint ? t("已归档 {n} 条记录", { n: r.checkpoint.records.toLocaleString() }) : t("现在还不需要归档"));
      load();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <>
      <div className="setting-row" style={{ marginTop: 20, borderTop: "1px solid var(--line)", paddingTop: 20 }}>
        <div>
          <div className="setting-name">{t("自动归档")}</div>
          <div className="setting-desc">{t("数据库里的记录超过设定的条数后，把早于设定时长的记录写进数据目录的 audit-archive/（压缩、带摘要），在链里留一个检查点，再从数据库里删掉。归档文件可以离线校验：")}<span className="mono">hostower audit verify</span></div>
        </div>
        <div className="col" style={{ gap: 10 }}>
          <Select value={String(v.policy.maxRecords)} disabled={!owner || busy} onChange={(x) => save({ ...v.policy, maxRecords: Number(x) })}
            options={v.maxRecordChoices.map((n) => ({ value: String(n), label: count(n) }))} />
          {v.policy.maxRecords > 0 && (
            <Select value={String(v.policy.keepDays)} disabled={!owner || busy} onChange={(x) => save({ ...v.policy, keepDays: Number(x) })}
              options={v.keepDayChoices.map((n) => ({ value: String(n), label: t("归档早于 {d} 的", { d: days(n) }) }))} />
          )}
        </div>
      </div>
      <div className="hint" style={{ marginTop: 4 }}>
        {t("数据库里现有 {n} 条；已归档 {a} 条，共 {f} 个文件。归档的记录不在审计页上显示。每 6 小时检查一次。", { n: v.records.toLocaleString(), a: v.archivedRecords.toLocaleString(), f: v.archives })}
        {" "}{t("数据目录里的 audit.jsonl 是另一份独立副本，不随归档清理。")}
      </div>
      {owner && v.policy.maxRecords > 0 && <div className="row" style={{ marginTop: 12 }}><button className="btn sm" disabled={busy} onClick={run}><Archive />{t("现在检查一次")}</button></div>}
      {v.checkpoints.length > 0 && (
        <div className="arch-list">
          {v.checkpoints.slice(0, 5).map((c) => (
            <div key={c.upto} className="arch-row">
              <Archive />
              <span className="mono ellipsis" title={c.file}>{c.file}</span>
              <span className="muted num">{t("第 {a}～{b} 条", { a: c.first.toLocaleString(), b: c.upto.toLocaleString() })}</span>
              <span className="muted" title={c.createdAt}>{ago(c.createdAt)}</span>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function PanelSettings({ dev }: { dev: boolean }) {
  const t = useT();
  // 先用上次的数据把卡片和表单填好；重新取到的和它不一样才换（一样就不动，免得冲掉正在填的内容）。
  const [seen] = useState(() => cached<PanelView>("settings.panel"));
  const stored0 = seen ? Object.fromEntries(seen.fields.map((f) => [f.key, f.stored])) : {};
  const [view, setView] = useState<PanelView | null>(seen ?? null);
  const [v, setV] = useState<Record<string, string>>(stored0);
  const [ip, setIP] = useState(() => split(stored0.listen || "")[0]);
  const [port, setPort] = useState(() => split(stored0.listen || "")[1]);
  const [busy, setBusy] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const owner = useStore((s) => s.boot?.user?.role === "owner");

  const apply = useCallback((pv: PanelView) => {
    setView(remember("settings.panel", pv));
    const stored = Object.fromEntries(pv.fields.map((f) => [f.key, f.stored]));
    setV(stored);
    const [h, p] = split(stored.listen || "");
    setIP(h); setPort(p);
  }, []);
  useEffect(() => { api.panel().then((pv) => { if (JSON.stringify(pv) !== JSON.stringify(seen)) apply(pv); }).catch(fail); }, [apply, seen]);
  if (!view) return null;

  const f = Object.fromEntries(view.fields.map((x) => [x.key, x]));
  // 被环境变量接管的项也能在这里填、保存：去掉环境变量、重启后才生效（保存前服务端会检查去掉之后能不能起来）。
  const locked = (_k: string) => dev;
  const envKeys = view.fields.filter((x) => x.fromEnv);
  /** 把环境变量现在的值填进输入框：先存下来，去掉环境变量后设置不变，不会断开访问。 */
  const adoptEnv = () => {
    const next = { ...v };
    for (const x of envKeys) if (x.key !== "listen") next[x.key] = x.effective;
    if (f.listen.fromEnv) { setIP(effIP); setPort(effPort); }
    setV(next);
  };
  const [effIP, effPort] = split(f.listen.effective);
  // 重启后生效的访问地址：环境变量接管时不变；否则是页面上保存的那个（留空表示不确定，就不提示换地址）。
  const nextURL = f.public_url.fromEnv ? f.public_url.effective : (f.public_url.stored || "").trim();
  const tlsKind = v.tls_mode === "proxy" ? "proxy" : v.tls_cert || v.tls_key ? "cert" : "none";
  const setTLS = (kind: string) => setV({ ...v, tls_mode: kind === "proxy" ? "proxy" : "", tls_cert: kind === "cert" ? v.tls_cert || " " : "", tls_key: kind === "cert" ? v.tls_key : "" });

  const save = async () => {
    setBusy(true);
    try {
      const listen = ip.trim() || port.trim() ? `${ip.trim() || effIP}:${port.trim() || effPort}` : "";
      apply(await api.savePanel({ ...v, listen, tls_cert: v.tls_cert?.trim() ?? "", tls_key: v.tls_key?.trim() ?? "" }));
      toast(t("设置已保存"));
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const envNote = (k: string) => f[k].fromEnv && (
    <div className="setting-desc warn-text">{t("现在生效的是环境变量 ")}<span className="mono">{f[k].env}</span>{t(" 的值")}{f[k].effective && <>（<span className="mono">{f[k].effective}</span>）</>}{t("。这里保存的值要等去掉这个环境变量、重启后才生效。")}</div>
  );

  return (
    <div className="card">
      <div className="card-head"><h2>{t("面板")}</h2><span className="muted hide-sm">{t("监听地址、证书和会话")}</span></div>
      <div className="card-body">
        {dev && <div className="note" style={{ marginBottom: 16 }}><TriangleAlert /><span>{t("开发模式固定只监听本机，这些设置不可修改。用正常模式启动后可以在这里调整。")}</span></div>}
        {view.restartNeeded && (
          <div className="note warn" style={{ marginBottom: 16 }}><RotateCcw /><span>{t("有设置已保存但还没生效：重启塔台后生效。")}</span>
            {owner && !dev && <button className="btn sm" style={{ marginLeft: "auto" }} onClick={() => setRestarting(true)}>{t("立即重启")}</button>}
          </div>
        )}
        {!dev && envKeys.length > 0 && (
          <div className="note" style={{ marginBottom: 16 }}>
            <TriangleAlert />
            <div className="col" style={{ gap: 6 }}>
              <span>{t("有 {n} 项设置由环境变量决定：", { n: envKeys.length })}<span className="mono">{envKeys.map((x) => x.env).join("、")}</span>{t("。环境变量优先于这里保存的值。")}</span>
              <span>{t("想改成在这里管理：先在下面填好并保存（可以点“带入环境变量的值”），再去掉这些环境变量、重启 Hostower。这样顺序做，不会断开访问。")}</span>
              <span className="hint">{t("systemd 部署：运行 ")}<span className="mono">sudo systemctl edit hostower</span>{t(" 删掉对应的 Environment= 行（或检查 /etc/systemd/system/hostower.service），然后 ")}<span className="mono">sudo systemctl restart hostower</span>{t("。Docker 部署：删掉 compose 文件 environment 里对应的行，再 ")}<span className="mono">docker compose up -d</span>{t("。")}</span>
              <div><button className="btn sm" onClick={adoptEnv}>{t("带入环境变量的值")}</button></div>
            </div>
          </div>
        )}

        <div className="setting-row">
          <div><div className="setting-name">{t("监听 IP")}</div><div className="setting-desc">{t("只在本机用填 ")}<span className="mono">127.0.0.1</span>{t("；要对外提供访问填 ")}<span className="mono">0.0.0.0</span>{t("，此时必须配置下面的 TLS。")}</div>{envNote("listen")}</div>
          <input className="input mono" value={ip} onChange={(e) => setIP(e.target.value)} disabled={locked("listen")} placeholder={effIP} spellCheck={false} aria-label={t("监听 IP")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("监听端口")}</div><div className="setting-desc">{t("塔台本身监听的端口，重启后生效。前面有反向代理时，这是代理转发到的端口；浏览器里用的端口以“访问地址”为准。")}</div>{envNote("listen")}</div>
          <input className="input mono" value={port} onChange={(e) => setPort(e.target.value.replace(/\D/g, ""))} disabled={locked("listen")} placeholder={effPort} inputMode="numeric" aria-label={t("监听端口")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("访问地址")}</div><div className="setting-desc">{t("浏览器里打开塔台用的完整地址（含域名）。Passkey 绑定到这个域名，改了它已有的 Passkey 会失效。")}</div>{envNote("public_url")}</div>
          <input className="input mono" value={v.public_url ?? ""} onChange={(e) => setV({ ...v, public_url: e.target.value })} disabled={locked("public_url")} placeholder={f.public_url.effective} spellCheck={false} aria-label={t("访问地址")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">TLS</div><div className="setting-desc">{t("谁来提供 HTTPS。对外监听时必须二选一，否则塔台拒绝启动。选“塔台自己提供”后填证书和私钥文件的路径。")}</div>{envNote("tls_mode")}{envNote("tls_cert")}{envNote("tls_key")}</div>
          <div className="col" style={{ gap: 10 }}>
            <Select value={tlsKind} onChange={setTLS} disabled={locked("tls_mode")} options={[
              { value: "none", label: t("不启用"), hint: t("只在本机访问") },
              { value: "cert", label: t("塔台自己提供"), hint: t("填证书和私钥") },
              { value: "proxy", label: t("前面的反向代理提供") },
            ]} />
            {tlsKind === "cert" && (
              <>
                <input className="input mono" value={(v.tls_cert ?? "").trim()} onChange={(e) => setV({ ...v, tls_cert: e.target.value })} disabled={locked("tls_cert")} placeholder={t("证书文件路径，例如 /config/tls/fullchain.pem")} spellCheck={false} aria-label={t("证书文件路径")} />
                <input className="input mono" value={v.tls_key ?? ""} onChange={(e) => setV({ ...v, tls_key: e.target.value })} disabled={locked("tls_key")} placeholder={t("私钥文件路径，例如 /config/tls/privkey.pem")} spellCheck={false} aria-label={t("私钥文件路径")} />
                <span className="hint">{t("填的是塔台所在机器（容器）里的文件路径。私钥只留在磁盘上，不经过浏览器，也不进数据库。只接受 TLS 1.3。")}</span>
              </>
            )}
            {view.cert && (
              <div className={"note " + (view.cert.daysLeft < 14 ? "warn" : "ok")}>
                <ShieldCheck />
                <span>{t("当前证书：")}<b>{view.cert.subject || view.cert.names?.[0] || t("未命名")}</b>{view.cert.names?.length ? t("（{names}）", { names: view.cert.names.join(t("、")) }) : ""}{t("，由 {issuer} 签发，{time} 到期，还有 {n} 天。", { issuer: view.cert.issuer || t("未知机构"), time: stamp(view.cert.notAfter), n: view.cert.daysLeft })}</span>
              </div>
            )}
            {view.certError && <div className="note crit"><TriangleAlert /><span>{ts(view.certError)}</span></div>}
          </div>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("会话时长")}</div><div className="setting-desc">{t("登录后保持多久，单位分钟。对之后的新登录立即生效。")}</div>{envNote("session_minutes")}</div>
          <input className="input mono" value={v.session_minutes ?? ""} onChange={(e) => setV({ ...v, session_minutes: e.target.value.replace(/\D/g, "") })} disabled={locked("session_minutes")} placeholder={f.session_minutes.effective} inputMode="numeric" aria-label={t("会话时长（分钟）")} />
        </div>
        <div className="row between" style={{ marginTop: 16 }}>
          <span className="muted" style={{ fontSize: 12.5 }}>{t("留空表示用默认值。保存前会检查这样设置能不能正常启动。")}</span>
          <div className="row" style={{ gap: 8 }}>
            {owner && !dev && <button className="btn" disabled={busy} onClick={() => setRestarting(true)}><RotateCcw />{t("重启 Hostower")}</button>}
            <button className="btn primary" disabled={busy || dev} onClick={save}>{t("保存")}</button>
          </div>
        </div>
      </div>
      {restarting && <RestartDialog nextURL={nextURL} onClose={() => setRestarting(false)} />}
    </div>
  );
}

/** 重启 Hostower：确认 → 当场验证身份 → 等它回来后刷新页面。访问地址改了的话，提示到新地址打开。 */
function RestartDialog({ nextURL, onClose }: { nextURL: string; onClose: () => void }) {
  const t = useT();
  const running = useStore((s) => s.requests.filter((r) => r.state === "running").length);
  const [phase, setPhase] = useState<"confirm" | "waiting" | "slow">("confirm");
  const moved = !!nextURL && nextURL.replace(/\/$/, "") !== location.origin;

  const go = async () => {
    try {
      const r = await withVerify(() => api.restart(true));
      setPhase("waiting");
      if (moved) return;
      // 等新进程起来：启动时间变了就说明是新的那个，刷新页面。
      const began = Date.now();
      const poll = async () => {
        try {
          const b = await fetch("/api/v1/bootstrap", { cache: "no-store" }).then((x) => x.json());
          if (b.started && b.started !== r.started) { location.reload(); return; }
        } catch { /* 还没起来 */ }
        if (Date.now() - began > 60000) { setPhase("slow"); return; }
        setTimeout(poll, 1500);
      };
      setTimeout(poll, 1500);
    } catch (e) { fail(e); }
  };

  return (
    <Modal title={t("重启 Hostower")} onClose={phase === "confirm" ? onClose : () => {}}
      footer={phase === "confirm"
        ? <><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" onClick={go}><RotateCcw />{t("重启")}</button></>
        : phase === "slow" ? <button className="btn" onClick={() => location.reload()}>{t("刷新页面")}</button> : null}>
      {phase === "confirm" && (
        <>
          <p className="dim">{t("重启会让面板断开几秒钟，保存过的面板设置（监听地址、访问地址、证书、会话时长）在重启后生效。需要当场验证一次身份。")}</p>
          {running > 0 && <div className="note warn"><TriangleAlert /><span>{t("有 {n} 个请求正在执行，重启会中断它们（之后标成“中断”，不会自动重跑）。", { n: running })}</span></div>}
          {moved && <div className="note"><TriangleAlert /><span>{t("访问地址会变成 ")}<span className="mono">{nextURL}</span>{t("。重启后请到新地址打开；如果前面有反向代理，记得一起改它转发的端口。")}</span></div>}
        </>
      )}
      {phase === "waiting" && (moved
        ? <p>{t("正在重启。稍等几秒后到新地址打开：")}<a className="mono" href={nextURL} style={{ color: "var(--accent)" }}>{nextURL}</a></p>
        : <p className="row" style={{ gap: 8 }}><RefreshCw className="spin" />{t("正在重启，回来后会自动刷新页面…")}</p>)}
      {phase === "slow" && (
        <div className="note crit"><TriangleAlert /><span>{t("一分钟了还没连上。可能是新的面板设置有问题，或者反向代理转发的端口没跟着改。到主机上看日志：")}<span className="mono">journalctl -u hostower -n 30</span>{t("；面板设置写错时，可以临时设置环境变量 HOSTOWER_IGNORE_PANEL=1 跳过它启动。")}</span></div>
      )}
    </Modal>
  );
}

export function AddSecret({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const t = useT();
  const [name, setName] = useState("ssh/ops");
  const [user, setUser] = useState("ops");
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    try { await api.putSSHSecret({ name, user, privateKey: key, passphrase: "" }); onSaved(); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <Modal title={t("添加 SSH 凭据")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !name || !user || !key} onClick={save}>{t("加密保存")}</button></>}>
      <div className="field"><label htmlFor="s-name">{t("名称")}</label><input id="s-name" className="input mono" value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} /><span className="hint">{t("以 ")}<span className="mono">ssh/</span>{t(" 开头，添加主机时按这个名字选。")}</span></div>
      <div className="field"><label htmlFor="s-user">{t("登录用户名")}</label><input id="s-user" className="input mono" value={user} onChange={(e) => setUser(e.target.value)} spellCheck={false} /><span className="hint">{t("用受限账号加 sudo 白名单，不要用 root。")}</span></div>
      <div className="field"><label htmlFor="s-key">{t("私钥（PEM）")}</label><textarea id="s-key" className="textarea mono" rows={6} value={key} onChange={(e) => setKey(e.target.value)} spellCheck={false} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" /><span className="hint">{t("用信封加密保存。也可以用命令行 ")}<span className="mono">hostower secret set-ssh</span>{t("，私钥就不经过浏览器。")}</span></div>
    </Modal>
  );
}

/** 界面：语言和时区。只影响这台设备上的显示，存在浏览器里。 */
function InterfaceSettings() {
  const t = useT();
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(id); }, []);
  const zones = timeZones();
  const tz = timeZoneSetting();
  return (
    <div className="card">
      <div className="card-head"><h2>{t("界面")}</h2><span className="muted hide-sm">{t("只影响这台设备上的显示")}</span></div>
      <div className="card-body">
        <div className="setting-row">
          <div><div className="setting-name">{t("语言")}</div><div className="setting-desc">{t("默认跟随浏览器的语言。")}</div></div>
          <Select value={langSetting()} onChange={(v) => setLang(v as LangMode)}
            options={[{ value: "system", label: t("跟随系统") }, ...languages.map((l) => ({ value: l.code as string, label: l.label }))]} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("时区")}</div><div className="setting-desc">{t("页面上所有时间都按这个时区显示。默认跟随这台设备的系统时区。")}</div></div>
          <div className="col" style={{ gap: 8 }}>
            <Select mono value={tz} onChange={setTimeZone}
              options={[{ value: "system", label: t("跟随系统"), hint: systemTimeZone() }, ...zones.map((z) => ({ value: z, label: z }))]} />
            <span className="hint">{t("现在是")} <span className="mono">{stamp(now)}</span>（{timeZoneOffset()}）</span>
          </div>
        </div>
      </div>
    </div>
  );
}

/** 指标历史保留多久：分钟级和 15 分钟级分开设。缩短后，超出的部分在一小时内清掉。 */
function RetentionSettings({ owner }: { owner: boolean }) {
  const t = useT();
  const [v, setV] = useCached<RetentionView>("settings.retention");
  const [busy, setBusy] = useState(false);
  const hosts = useStore((st) => st.hosts.length) || 1;
  useEffect(() => { api.retention().then(setV).catch(() => {}); }, []);
  if (!v) return null;
  const days = (n: number) => (n >= 365 && n % 365 === 0 ? t("{n} 年", { n: n / 365 }) : t("{n} 天", { n }));
  const save = async (next: { fineDays: number; coarseDays: number }) => {
    const shorter = next.fineDays < v.retention.fineDays || next.coarseDays < v.retention.coarseDays;
    if (shorter && !(await askConfirm(t("缩短后，超出新时长的历史数据会被删除，不能恢复。继续吗？")))) return;
    setBusy(true);
    try { setV(await api.setRetention(next)); toast(t("已保存")); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  // 估算：每台主机每天分钟级 1440 个点、15 分钟级 96 个点，每个点约 120 字节。
  const est = hosts * (1440 * v.retention.fineDays + 96 * v.retention.coarseDays) * 120;
  return (
    <div className="card">
      <div className="card-head"><h2>{t("数据保留")}</h2><span className="muted hide-sm">{t("指标历史在数据库里存多久")}</span></div>
      <div className="card-body">
        <div className="setting-row">
          <div><div className="setting-name">{t("分钟级数据")}</div><div className="setting-desc">{t("每分钟一个点，用来画 1 小时到几天的细图。保留得越久，“7 天”这类图就越细。")}</div></div>
          <Select value={String(v.retention.fineDays)} disabled={!owner || busy} onChange={(x) => save({ ...v.retention, fineDays: Number(x) })}
            options={v.fine.map((n) => ({ value: String(n), label: days(n) }))} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("15 分钟级数据")}</div><div className="setting-desc">{t("每 15 分钟一个平均值，用来看长期趋势。保留一年以上时，主机详情里可以看“90 天”和“1 年”。")}</div></div>
          <Select value={String(v.retention.coarseDays)} disabled={!owner || busy} onChange={(x) => save({ ...v.retention, coarseDays: Number(x) })}
            options={v.coarse.map((n) => ({ value: String(n), label: days(n) }))} />
        </div>
        <div className="hint" style={{ marginTop: 14 }}>
          {t("现在存着 {n} 个点。按 {hosts} 台主机估算，存满后约占 {size}。", { n: v.stored.toLocaleString(), hosts, size: bytes(est) })}
          {!owner && " " + t("只有所有者可以修改。")}
        </div>
      </div>
    </div>
  );
}

/** 构建封签的结论：颜色之外都带文字，状态不只靠颜色表达。 */
function BuildBadge({ build }: { build?: { status: string; fingerprint?: string } }) {
  const t = useT();
  switch (build?.status) {
    case "signed": return <span className="badge ok"><ShieldCheck />{t("发布签名校验通过")}</span>;
    case "invalid": return <span className="badge crit"><TriangleAlert />{t("封签对不上：文件被改过，或者不是用这把公钥签的")}</span>;
    default: return <span className="badge neutral">{t("自行构建（没有发布签名）")}</span>;
  }
}
