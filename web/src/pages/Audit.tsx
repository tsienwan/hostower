import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Link2, Link2Off, RefreshCw, ScrollText, Search, X } from "lucide-react";
import { api, type AuditRecord, type VerifyResult } from "../api";
import { fail } from "../store";
import { cached, remember } from "../cache";
import { Empty, LevelBadge, Pager, stamp } from "../ui";
import { t, ts, useT } from "../i18n";

// 值是中文原文，显示时再翻译：t(typeNames[type] ?? type)。
const typeNames: Record<string, string> = {
  "request.submitted": "提交请求", "request.rejected": "内核拒绝", "request.refused": "暂停期间被拒", "request.cancelled": "取消请求", "request.expired": "请求过期",
  "approval.granted": "批准", "approval.denied": "拒绝", "approval.refused": "批准未通过",
  "exec.host": "主机执行", "exec.finished": "批量结束", "file.list": "列目录", "file.read": "读文件", "file.download": "下载文件",
  "auth.login": "登录", "auth.login.failed": "登录失败", "auth.setup": "初始化", "auth.setup.failed": "初始化失败", "auth.reauth.failed": "二次验证失败", "auth.passkey.added": "新增 Passkey", "auth.passkey.removed": "删除 Passkey", "auth.verify": "身份验证", "auth.verify.failed": "身份验证失败", "auth.methods.updated": "登录方式", "auth.password.changed": "密码已修改", "auth.password.reset": "密码已重置", "auth.totp.added": "绑定动态码", "auth.totp.removed": "解绑动态码",
  "system.started": "塔台启动", "system.paused": "暂停执行", "system.resumed": "恢复执行", "system.sessions.revoked": "吊销会话", "system.config.reloaded": "重载配置",
  "snippet.created": "新建 Snippet", "snippet.updated": "修改 Snippet", "snippet.deleted": "删除 Snippet",
  "terminal.opened": "打开终端", "terminal.closed": "终端结束",
  "query": "只读查询", "upload.stored": "暂存上传的文件", "system.panel.updated": "修改面板设置",
  "system.audit.routine": "审计范围", "system.retention": "指标保留时长", "host.traffic_quota": "设置流量配额", "host.traffic_quota.removed": "取消流量配额", "speedtest.retention": "测速保留规则", "speedtest.deleted": "删除测速记录", "speedtest.routes": "测速追踪目标", "speedtest.geo": "测速节点地区查询", "speedtest.ookla": "Ookla 许可协议", "auth.username.changed": "修改用户名", "apps.autoupdate": "应用自动发起更新", "app.panel": "应用面板地址", "system.restart": "重启 Hostower", "audit.archived": "审计归档", "audit.retention": "审计归档条件", "auth.oidc.updated": "OIDC 设置", "notify.channel.created": "新建通知渠道", "notify.channel.updated": "修改通知渠道", "notify.channel.deleted": "删除通知渠道", "notify.test": "测试通知", "system.update.auto": "自动更新开关", "system.update.requested": "手动更新", "system.update.applied": "已更新", "system.update.rejected": "更新被拒绝",
  "chat.command": "聊天命令", "chat.bound": "绑定聊天账号", "chat.unbound": "解除聊天绑定", "chat.binding.expired": "聊天绑定过期", "chat.bind.failed": "聊天绑定失败", "chat.config.updated": "修改聊天命令设置", "chat.pair.started": "生成配对码",
  "inventory.host.created": "新增主机", "inventory.host.updated": "修改主机", "inventory.host.deleted": "删除主机",
  "secret.updated": "写入凭据", "secret.deleted": "删除凭据", "hostkey.forgotten": "清除主机公钥", "host.reconnected": "重新连接主机",
};

const typeTone = (ty: string) =>
  ty === "request.rejected" || ty.endsWith(".failed") || ty === "approval.refused" || ty === "system.paused" ? "crit"
    : ty === "approval.granted" ? "ok" : ty === "approval.denied" ? "warn" : "neutral";

const SIZE = 20;

export function AuditPage() {
  const t = useT();
  // 切回审计页时先显示上次的第一页和校验结果，后台再刷新。
  const first = cached<{ records: AuditRecord[]; total: number }>("audit.first");
  const [records, setRecords] = useState<AuditRecord[]>(first?.records ?? []);
  const [total, setTotal] = useState(first?.total ?? 0);
  const [verify, setVerify] = useState<VerifyResult | null>(() => cached<VerifyResult>("audit.verify") ?? null);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState<AuditRecord | null>(null);
  const [page, setPage] = useState(0);
  const [scope, setScope] = useState<"key" | "all">("key");
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");

  // 输入停下来一会儿再搜，避免每敲一个字就查一次。
  useEffect(() => {
    const timer = setTimeout(() => { setQuery(input.trim()); setPage(0); }, 300);
    return () => clearTimeout(timer);
  }, [input]);

  const load = useCallback(async () => {
    setBusy(true);
    try {
      // 事件在页面上显示的是名字（“打开终端”），记录里存的是代码：把名字对得上的代码一起发给服务端。
      const w = query.toLowerCase();
      const types = !w ? [] : Object.entries(typeNames).filter(([code, name]) => name.toLowerCase().includes(w) || t(name).toLowerCase().includes(w) || code.includes(w)).map(([code]) => code);
      const r = await api.audit({ q: query || undefined, types: types.length ? types.join(",") : undefined, scope, page, size: SIZE });
      setRecords(r.records);
      setTotal(r.total);
      if (!query && scope === "key" && page === 0) remember("audit.first", { records: r.records, total: r.total });
    } catch (e) { fail(e); } finally { setBusy(false); }
  }, [query, scope, page, t]);
  // 进页面时静默刷新校验结果；手动点“重新校验”时才先清空，让人看到它确实重新算了。
  const refresh = useCallback(() => { api.auditVerify().then((v) => setVerify(remember("audit.verify", v))).catch(fail); }, []);
  const check = useCallback(() => { setVerify(null); refresh(); }, [refresh]);
  useEffect(() => { load(); }, [load]);
  useEffect(() => { refresh(); }, [refresh]);

  const scopes: { key: "key" | "all"; label: string; hint: string }[] = [
    { key: "key", label: t("关键操作"), hint: t("登录、提交、批准、执行、读写文件、改设置这些；不含翻目录和页面上的只读查询") },
    { key: "all", label: t("全部"), hint: t("包括翻目录、列容器这类日常查看（要先在“设置 → 审计”里打开记录）") },
  ];

  return (
    <>
      <div className="topbar">
        <h1>{t("审计")}</h1>
        <span className="grow" />
        <button className="btn" onClick={() => { load(); check(); }} disabled={busy}><RefreshCw className={busy ? "spin" : ""} />{t("刷新并校验")}</button>
      </div>
      <div className="content">
        <div className="page">
          <div className={"note " + (!verify ? "" : verify.ok ? "ok" : "crit")} role="status">
            {verify && !verify.ok ? <Link2Off /> : <Link2 />}
            {!verify ? <span>{t("正在校验哈希链…")}</span> : verify.ok ? (
              <span><b>{t("哈希链完整")}</b>{t("：{n} 条记录，每条都包含上一条的哈希。链头", { n: verify.count.toLocaleString() })} <span className="mono">{verify.head.slice(0, 16)}…</span>{t("　也可以离线运行")} <span className="mono">hostower audit verify</span>{verify.archived ? t("。第 1～{n} 条已归档到数据目录的 audit-archive/，从那里接着校验", { n: verify.archived.toLocaleString() }) : ""}</span>
            ) : (
              <span><b>{t("哈希链在第 {n} 条断开", { n: verify.brokenAt ?? "" })}</b>{t("：{message}。这之后的记录不可信，请对照外部副本。", { message: ts(verify.message) })}</span>
            )}
          </div>
          <div className="card">
            <div className="card-head wrap">
              <h2>{t("记录")}</h2>
              <span className="muted num">{total.toLocaleString()}</span>
              <div className="row wrap" role="group" aria-label={t("显示范围")} style={{ gap: 4 }}>
                {scopes.map((x) => <button key={x.key} className={"tag" + (scope === x.key ? " on" : "")} aria-pressed={scope === x.key} title={x.hint} onClick={() => { setScope(x.key); setPage(0); }}>{x.label}</button>)}
              </div>
              <span className="grow" />
              <div className="search" style={{ width: 280, maxWidth: "100%" }}>
                <Search />
                <input className="input" placeholder={t("搜索人、主机、命令、事件")} value={input} onChange={(e) => setInput(e.target.value)} aria-label={t("搜索审计记录")} />
              </div>
            </div>
            {records.length === 0 ? (busy ? <div className="empty">{t("加载中…")}</div> : <Empty icon={query ? <Search /> : <ScrollText />} title={query ? t("没有匹配的记录") : t("还没有审计记录")}>{scope === "key" && t("当前只显示关键操作。")}</Empty>) : (
              <>
                <div className="table-wrap">
                  <table className="table audit-table">
                    <thead><tr><th>#</th><th>{t("时间")}</th><th>{t("事件")}</th><th>{t("等级")}</th><th>{t("谁")}</th><th>{t("主机")}</th><th>{t("内容")}</th></tr></thead>
                    <tbody>
                      {records.map((r) => {
                        const b = r.body;
                        return (
                          <tr key={r.seq} className="click" onClick={() => setOpen(r)}>
                            <td className="mono muted">{r.seq}</td>
                            <td className="mono dim" style={{ whiteSpace: "nowrap" }}>{stamp(r.ts)}</td>
                            <td><span className={"badge " + typeTone(b.type)}>{t(typeNames[b.type] ?? b.type)}</span></td>
                            <td>{b.level ? <LevelBadge level={b.level} /> : <span className="muted">—</span>}</td>
                            <td><div className="ellipsis">{b.approver ? t("{name}（批准）", { name: ts(b.approver) }) : ts(b.actor) || <span className="muted">{t("系统")}</span>}</div></td>
                            <td className="mono dim"><div className="ellipsis">{b.host ?? (Array.isArray(b.detail?.hosts) ? hostsText(b.detail!.hosts as string[]) : "")}</div></td>
                            <td>
                              {/* 有命令时命令在上、说明在下；没有命令时说明就是主要内容，不再灰着显示。 */}
                              {b.command
                                ? <><div className="ellipsis mono" title={ts(b.command)}>{ts(b.command)}</div>{(b.message || b.intent) && <div className="ellipsis muted" style={{ fontSize: 12 }}>{ts(b.message || b.intent)}</div>}</>
                                : <div className="ellipsis" title={ts(b.message || b.intent)}>{ts(b.message || b.intent) || (b.requestId ? <span className="mono muted">{t("请求 {id}", { id: b.requestId })}</span> : <span className="muted">—</span>)}</div>}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
                <Pager p={{ page, pages: Math.max(1, Math.ceil(total / SIZE)), setPage, total, size: SIZE }} />
              </>
            )}
          </div>
        </div>
      </div>
      {open && <Drawer r={open} onClose={() => setOpen(null)} />}
    </>
  );
}

const hostsText = (hs: string[]) => (hs.length <= 2 ? hs.join(", ") : t("{host} 等 {n} 台", { host: hs[0], n: hs.length }));

function Drawer({ r, onClose }: { r: AuditRecord; onClose: () => void }) {
  const t = useT();
  const b = r.body;
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);
  return (
    <>
      <div className="overlay" style={{ zIndex: 50 }} onMouseDown={onClose} />
      <aside className="drawer" role="dialog" aria-label={t("审计记录 {seq}", { seq: r.seq })}>
        <div className="drawer-head">
          <b>{t("审计记录 #{seq}", { seq: r.seq })}</b>
          <span className={"badge " + typeTone(b.type)}>{t(typeNames[b.type] ?? b.type)}</span>
          <span className="grow" />
          <button className="btn ghost icon sm" onClick={onClose} aria-label={t("关闭")}><X /></button>
        </div>
        <div className="drawer-body">
          <dl className="kv">
            <dt>{t("时间")}</dt><dd className="mono">{stamp(r.ts)}</dd>
            {b.actor && <><dt>{t("发起人")}</dt><dd>{ts(b.actor)}{b.actorKind ? <span className="muted"> · {b.actorKind}</span> : null}</dd></>}
            {b.approver && <><dt>{t("批准人")}</dt><dd>{b.approver}</dd></>}
            {b.entry && <><dt>{t("入口")}</dt><dd>{b.entry}</dd></>}
            {b.host && <><dt>{t("主机")}</dt><dd className="mono">{b.host}</dd></>}
            {b.level && <><dt>{t("等级")}</dt><dd><LevelBadge level={b.level} label /></dd></>}
            {b.intent && <><dt>{t("意图")}</dt><dd>{ts(b.intent)}</dd></>}
            {b.snippet && <><dt>Snippet</dt><dd className="mono">{b.snippet}</dd></>}
            {b.exitCode !== undefined && <><dt>{t("退出码")}</dt><dd className="mono">{b.exitCode}</dd></>}
            {b.message && <><dt>{t("说明")}</dt><dd>{ts(b.message)}</dd></>}
            {b.backup && <><dt>{t("旧文件备份")}</dt><dd className="mono">{b.backup}</dd></>}
            {b.requestId && <><dt>{t("请求")}</dt><dd><Link to={`/requests/${b.requestId}`} className="mono" style={{ color: "var(--accent)" }}>{b.requestId}</Link></dd></>}
            {b.parent ? <><dt>{t("父记录")}</dt><dd className="mono">#{b.parent}</dd></> : null}
          </dl>
          {b.command && <div className="col"><div className="section-title">{t("实际命令")}</div><pre className="code">{ts(b.command)}</pre></div>}
          {b.reasons && b.reasons.length > 0 && (
            <div className="col"><div className="section-title">{t("判级过程")}</div><div className="col" style={{ gap: 4 }}>{b.reasons.map((x, i) => <div key={i} className="dim">{ts(x)}</div>)}</div></div>
          )}
          {b.output && <div className="col"><div className="section-title">{t("输出摘要（已脱敏）")}</div><pre className="term" data-empty={t("（没有输出）")} style={{ maxHeight: 260 }}>{b.output}</pre></div>}
          {b.detail && <div className="col"><div className="section-title">{t("附加信息")}</div><pre className="code">{JSON.stringify(b.detail, null, 2)}</pre></div>}
          <div className="col">
            <div className="section-title">{t("哈希链")}</div>
            <dl className="kv">
              <dt>{t("上一条")}</dt><dd className="mono muted">{r.prevHash}</dd>
              <dt>{t("本条")}</dt><dd className="mono">{r.hash}</dd>
              <dt>{t("格式版本")}</dt><dd className="mono">{r.version}</dd>
            </dl>
          </div>
        </div>
      </aside>
    </>
  );
}
