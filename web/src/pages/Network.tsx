// 主机的网络入口：防火墙放行端口；Caddy / Nginx 站点和证书；Fail2ban；对外监听的端口。
// 这一页没有自己的执行通道：现状是只读检测，改动都变成请求，走判级和审批
// （改防火墙和 Fail2ban 是 L2，改站点、申请和续期证书是 L1）。远端的内容（日志、证书信息）一律按纯文本显示。
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Ban, ExternalLink, FileText, Globe, KeyRound, Lock, Play, Plus, RefreshCw, Settings2, Shield, ShieldAlert, ShieldCheck, ShieldOff, Trash2, TriangleAlert } from "lucide-react";
import { api, type F2BConfig, type NetPort, type NetSite, type NetworkView, type Preview, type Req, type AppPlanResult, type SiteForm, type SiteKind, type SiteServer, type SiteTLS } from "../api";
import { backAfterApproval, fail, onHost, useRequestFinished, useStore } from "../store";
import { Empty, Modal } from "../ui";
import { Select } from "../controls";
import { PreviewPanel } from "../shared";
import { cached, remember } from "../cache";
import { ts, useT } from "../i18n";

const portText = (p: NetPort) => `${p.port}/${p.proto}`;

/** 预览 + 提交的通用弹窗：先看判级和实际命令，再提交，提交后跳到请求页。 */
function PlanDialog({ title, send, onClose, children, ready = true }: {
  title: string; send: (preview: boolean) => Promise<AppPlanResult>; onClose: () => void; children: React.ReactNode; ready?: boolean;
}) {
  const t = useT();
  const nav = useNavigate();
  const [preview, setPreview] = useState<AppPlanResult | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const pv: Preview | null = preview?.targets ? { level: preview.level!, targets: preview.targets } : null;
  const doPreview = async () => {
    setBusy(true); setErr("");
    try { setPreview(await send(true)); } catch (e) { setErr(e instanceof Error ? e.message : String(e)); setPreview(null); } finally { setBusy(false); }
  };
  const submit = async () => {
    setBusy(true);
    try {
      const r = await send(false);
      const reqs = Array.isArray(r.requests) ? r.requests : [];
      onClose();
      if (reqs[0]) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <Modal wide title={title} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button>
        <button className="btn" onClick={doPreview} disabled={busy || !ready}>{t("预览")}</button>
        <button className="btn primary" onClick={submit} disabled={busy || !preview?.targets}>{t("提交")}</button></>}>
      {/* 和弹窗正文一样的竖向间距：包了一层之后，正文的 gap 管不到里面的各个字段。 */}
      <div className="col" style={{ gap: 16 }} onChange={() => setPreview(null)}>{children}</div>
      <div className="section-title">{t("判级预览")}</div>
      {!preview && !err ? <div className="muted" style={{ fontSize: 12.5 }}>{t("点“预览”，看每台主机上实际要执行的命令和风险等级。预览没有任何副作用。")}</div> : <PreviewPanel preview={pv} error={err} />}
      {preview?.targets && <details className="app-details"><summary>{t("实际要执行的命令")}</summary><pre className="code" style={{ marginTop: 8 }}>{preview.targets[0].command}</pre></details>}
    </Modal>
  );
}

/**
 * 这台主机上还在等批准或执行中的请求里，意图符合 match 的最新一条。
 * 意图是服务端的固定写法（internal/gateway/network.go）。有它时不再给“安装”按钮，免得重复提交。
 */
function activeReq(reqs: Req[], host: string, match: (intent: string) => boolean): Req | null {
  let latest: Req | null = null;
  for (const r of reqs) {
    if (r.state !== "pending" && r.state !== "running") continue;
    if (r.targets.length !== 1 || r.targets[0].host !== host || !match(r.intent)) continue;
    if (!latest || r.createdAt > latest.createdAt) latest = r;
  }
  return latest;
}

/** 代替“安装”按钮：显示请求走到哪一步，点了去看这条请求。 */
function ReqLink({ req, running, name }: { req: Req; running: string; name?: string }) {
  const t = useT();
  return (
    <Link className="btn sm" to={`/requests/${req.id}`} state={backAfterApproval}>
      {req.state === "pending" ? <ShieldAlert /> : <RefreshCw className="spin" />}
      {name ? name + " · " : ""}{req.state === "pending" ? t("等待批准") : running}
    </Link>
  );
}

function PortInput({ value, onChange }: { value: NetPort; onChange: (p: NetPort) => void }) {
  const t = useT();
  return (
    <div className="row" style={{ gap: 6 }}>
      <input className="input mono" style={{ width: 120 }} inputMode="numeric" aria-label={t("端口")} value={value.port || ""} placeholder="3001"
        onChange={(e) => onChange({ ...value, port: Number(e.target.value.replace(/\D/g, "")) || 0 })} />
      <div style={{ width: 100 }}><Select value={value.proto} onChange={(v) => onChange({ ...value, proto: v as "tcp" | "udp" })} options={[{ value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }]} /></div>
    </div>
  );
}

export function NetworkPanel({ host }: { host: string }) {
  const t = useT();
  const [sp, setSp] = useSearchParams();
  const reqs = useStore((s) => s.requests);
  const [data, setData] = useState<NetworkView | null>(() => cached<NetworkView>("net:" + host) ?? null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [dialog, setDialog] = useState<null | "setup" | "allow" | "site" | "caddy" | "nginx" | "start-caddy" | "start-nginx" | "f2b" | { remove: NetPort } | { unsite: string }
    | { log: string } | { cert: "dry-run" | "renew" } | { force: string } | { unban: { jail: string; ip: string } }>(null);
  // 从应用页跳过来时带着端口：直接打开“添加站点”。
  const presetPort = Number(sp.get("port")) || 0;

  const load = useCallback(() => {
    setLoading(true); setError("");
    api.hostNetwork(host).then((r) => setData(remember("net:" + host, r))).catch((e) => setError(e.message)).finally(() => setLoading(false));
  }, [host]);
  useEffect(() => { load(); }, [load]);
  // 批准后回到这一页：放行端口、改站点等执行完就重新查一遍。
  useRequestFinished(load, onHost(host));
  useEffect(() => {
    if (presetPort && data) { setDialog("site"); const n = new URLSearchParams(sp); n.delete("port"); setSp(n, { replace: true }); }
  }, [presetPort, data, sp, setSp]);

  if (error) return <div className="note crit"><TriangleAlert /><span>{ts(error)}</span></div>;
  if (!data) return <div className="card"><div className="empty">{t("加载中…")}</div></div>;
  const st = data.status;
  const fw = st.firewall;
  const f2b = st.fail2ban;
  const keep = new Set(data.keep.map(portText));
  const canRoot = st.root || (st.sudo && !st.noNewPrivs);
  const toolName = fw.tool === "ufw" ? "ufw" : fw.tool === "firewalld" ? "firewalld" : "";
  // 请求刚结束、现状还在重新检测时（loading），这几个“安装”按钮先禁用：这时页面上还是装之前的状态。
  const fwReq = activeReq(reqs, host, (i) => i.startsWith(`在 ${host} 上启用防火墙 `));
  const caddyReq = activeReq(reqs, host, (i) => i === `在 ${host} 上安装 Caddy`);
  const nginxReq = activeReq(reqs, host, (i) => i === `在 ${host} 上安装 Nginx`);
  const startReq = activeReq(reqs, host, (i) => i === `在 ${host} 上启动 Caddy` || i === `在 ${host} 上启动 Nginx`);
  // 装好了但没在运行的那个（两个都装了时先看 Caddy）。
  const stopped = st.caddy.installed && !st.caddy.active ? "caddy" as const : st.nginx.installed && !st.nginx.active ? "nginx" as const : null;
  const f2bReq = activeReq(reqs, host, (i) => i === `在 ${host} 上安装 Fail2ban`);
  const webOpen = !fw.tool || ["80/tcp", "443/tcp", "80", "443", "http", "https"].some((r) => fw.rules.includes(r));

  return (
    <>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <span className="muted" style={{ fontSize: 12.5 }}>{st.os}</span>
        <button className="btn ghost icon sm" onClick={load} disabled={loading} title={t("刷新")} aria-label={t("刷新")}><RefreshCw className={loading ? "spin" : ""} /></button>
      </div>
      {!canRoot && (st.noNewPrivs
        ? <div className="note warn"><TriangleAlert /><span>
            {t("这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。")}
            <br />{t("推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。")}
          </span></div>
        : <div className="note warn"><TriangleAlert /><span>{t("登录账号不是 root，也没有 sudo：改不了防火墙和 Caddy。给它配置免密 sudo，或者用 root 连接这台主机。")}</span></div>)}

      <div className="card">
        <div className="card-head wrap">
          <h2><Shield size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("防火墙")}</h2>
          {fw.tool === "conflict" ? <span className="badge crit">{t("ufw 和 firewalld 同时启用")}</span>
            : toolName ? <span className="badge ok">{toolName} · {t("已启用")}</span>
            : fw.unknown ? <span className="badge warn">{t("读不出状态")}</span>
            : <span className="badge neutral"><ShieldOff />{t("没有启用")}</span>}
          <span className="grow" />
          {toolName && <button className="btn sm" onClick={() => setDialog("allow")} disabled={!canRoot}><Plus />{t("开放端口")}</button>}
          {!fw.tool && !fw.unknown && (fwReq ? <ReqLink req={fwReq} running={t("执行中")} /> : <button className="btn primary sm" onClick={() => setDialog("setup")} disabled={!canRoot || loading || (!fw.recommended && !fw.ufwInstalled && !fw.firewalldInstalled)}>
            <Shield />{fw.ufwInstalled || fw.firewalldInstalled ? t("启用防火墙") : t("安装并启用防火墙")}</button>)}
        </div>
        <div className="card-body">
          {fw.tool === "conflict" && <div className="note crit"><TriangleAlert /><span>{t("两个防火墙同时管规则会互相覆盖。请先在主机上停掉其中一个（比如 systemctl disable --now firewalld），Hostower 不会同时管两个。")}</span></div>}
          {fw.unknown && <div className="note warn"><TriangleAlert /><span>{t("装了 ufw，但登录账号读不出它的状态（ufw 要 root 才能查看）。给登录账号配置免密 sudo 后再刷新。")}</span></div>}
          {!fw.tool && !fw.unknown && (
            <p className="dim" style={{ margin: 0 }}>{fw.recommended
              ? t("这台主机推荐用 {tool}。安装并启用时会先放行 SSH 端口（{ssh}），再启用，不会把你锁在外面。", { tool: fw.recommended, ssh: st.sshPort })
              : fw.ufwInstalled || fw.firewalldInstalled ? t("已经装了防火墙，但没有启用。")
              : t("这台主机的系统不支持一键安装防火墙，请在主机上自己安装 ufw 或 firewalld。")}</p>
          )}
          {toolName && (fw.rules.length === 0 ? <div className="muted">{t("还没有放行任何端口。")}</div> : (
            <div className="row wrap" style={{ gap: 6 }}>
              {fw.rules.map((r) => {
                const m = /^(\d+)\/(tcp|udp)$/.exec(r);
                const locked = keep.has(r);
                return (
                  <span key={r} className="tag mono" style={{ gap: 6 }}>
                    {r}{locked && <Lock size={12} aria-label={t("SSH 或面板在用，不能关")} />}
                    {m && !locked && <button className="link-btn" title={t("关闭这个端口")} aria-label={t("关闭这个端口")} onClick={() => setDialog({ remove: { port: Number(m[1]), proto: m[2] as "tcp" | "udp" } })}><Trash2 size={12} /></button>}
                  </span>
                );
              })}
            </div>
          ))}
          <p className="hint" style={{ marginBottom: 0 }}>{t("云服务商的安全组（控制台里的防火墙）在主机之外，Hostower 管不到；要对外开放的端口在那里也要放行。")}</p>
        </div>
      </div>

      <div className="card">
        <div className="card-head wrap">
          <h2><Globe size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("站点")}</h2>
          {st.caddy.installed && <span className={"badge " + (st.caddy.active ? "ok" : "warn")}>Caddy · {st.caddy.active ? t("运行中") : t("没有运行")}</span>}
          {st.nginx.installed && <span className={"badge " + (st.nginx.active ? "ok" : "warn")}>Nginx · {st.nginx.active ? t("运行中") : t("没有运行")}</span>}
          {!st.caddy.installed && !st.nginx.installed && <span className="badge neutral">{t("没有 Caddy 或 Nginx")}</span>}
          <span className="grow" />
          {startReq ? <ReqLink req={startReq} running={t("执行中")} />
            : stopped && <button className="btn sm" onClick={() => setDialog(`start-${stopped}`)} disabled={!canRoot || loading}><Play />{t("启动 {name}", { name: stopped === "caddy" ? "Caddy" : "Nginx" })}</button>}
          {(st.caddy.installed || st.nginx.installed)
            ? <button className="btn primary sm" onClick={() => setDialog("site")} disabled={!canRoot}><Plus />{t("添加站点")}</button>
            : caddyReq ? <ReqLink req={caddyReq} running={t("正在安装…")} name="Caddy" />
            : nginxReq ? <ReqLink req={nginxReq} running={t("正在安装…")} name="Nginx" />
            : <>
                {st.caddy.installable && <button className="btn primary sm" onClick={() => setDialog("caddy")} disabled={!canRoot || loading}><Plus />{t("安装 Caddy")}</button>}
                {st.nginx.installable && <button className="btn sm" onClick={() => setDialog("nginx")} disabled={!canRoot || loading}><Plus />{t("安装 Nginx")}</button>}
              </>}
        </div>
        <div className="card-body">
          <p className="dim" style={{ marginTop: 0 }}>{t("用域名访问这台主机上的服务：反向代理到本机端口、发布一个静态目录，或者跳转到别的网址。服务本身只监听 127.0.0.1，不用对外开放。")}</p>
          {!st.caddy.installed && !st.nginx.installed && (st.caddy.installable
            ? <p className="hint">{t("Caddy 自动申请和续期 HTTPS 证书，配置最省事；Nginx 用 Let's Encrypt（certbot）申请证书并自动续期，也可以用已有的证书。两个都要用 80 和 443 端口，每台主机只装一个。")}</p>
            : <div className="muted">{t("这台主机的系统不支持一键安装 Caddy 或 Nginx，请在主机上自己安装。")}</div>)}
          {(st.caddy.installed || st.nginx.installed) && !webOpen && <div className="note warn"><TriangleAlert /><span>{t("防火墙没有放行 80 和 443：申请证书和对外提供 HTTPS 都要用到它们。")}</span></div>}
          {st.sites.length === 0 ? ((st.caddy.installed || st.nginx.installed) && <Empty icon={<Globe />} title={t("还没有 Hostower 添加的站点")} />) : (
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("地址")}</th><th>{t("类型")}</th><th>{t("目标")}</th><th>HTTPS</th><th className="act">{t("操作")}</th></tr></thead>
                <tbody>
                  {st.sites.map((s) => (
                    <tr key={s.address}>
                      <td className="mono"><a href={`${s.tls === "http" ? "http" : "https"}://${s.address}`} target="_blank" rel="noreferrer noopener" style={{ color: "var(--accent)" }}>{s.address} <ExternalLink size={12} /></a>
                        {st.caddy.installed && st.nginx.installed && <span className="muted" style={{ marginLeft: 6, fontSize: 12 }}>{s.server === "nginx" ? "Nginx" : "Caddy"}</span>}</td>
                      <td>{kindLabel(t, s.kind)}</td>
                      <td className="mono">{s.target}</td>
                      <td><TLSBadge site={s} /></td>
                      <td className="act">
                        <button className="btn ghost icon sm" title={t("看日志")} aria-label={t("看日志")} onClick={() => setDialog({ log: s.address })}><FileText /></button>
                        <button className="btn ghost icon sm" title={t("撤下站点")} aria-label={t("撤下站点")} onClick={() => setDialog({ unsite: s.address })}><Trash2 /></button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="hint" style={{ marginBottom: 0 }}>{t("这里只列出 Hostower 添加的站点（/etc/caddy/sites.d、/etc/nginx/conf.d/hostower-*.conf）；主机上原有的站点不受影响。")}</p>
        </div>
      </div>

      {(st.certbot.installed || st.nginx.installed) && (
        <div className="card">
          <div className="card-head wrap">
            <h2><KeyRound size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("证书")}</h2>
            {st.certbot.installed
              ? (st.certbot.autoRenew ? <span className="badge ok">{t("自动续期已开启")}</span> : <span className="badge warn">{t("没有自动续期")}</span>)
              : <span className="badge neutral">{t("没有 certbot")}</span>}
            <span className="grow" />
            {st.certbot.installed && <>
              <button className="btn sm" onClick={() => setDialog({ cert: "dry-run" })} disabled={!canRoot}>{t("演练续期")}</button>
              <button className="btn sm" onClick={() => setDialog({ cert: "renew" })} disabled={!canRoot}><RefreshCw />{t("续期快到期的")}</button>
            </>}
          </div>
          <div className="card-body">
            {!st.certbot.installed ? <p className="dim" style={{ margin: 0 }}>{t("给 Nginx 站点选“Let's Encrypt 自动申请”时，会顺带装上 certbot。")}</p>
              : st.certs.length === 0 ? <div className="muted">{t("还没有 certbot 管理的证书。")}</div> : (
              <div className="table-wrap">
                <table className="table">
                  <thead><tr><th>{t("证书")}</th><th>{t("域名")}</th><th>{t("到期时间")}</th><th>{t("剩余")}</th><th className="act">{t("操作")}</th></tr></thead>
                  <tbody>
                    {st.certs.map((c) => (
                      <tr key={c.name}>
                        <td className="mono">{c.name}</td>
                        <td className="mono">{c.domains.join(" ")}</td>
                        <td className="mono">{c.expiry}</td>
                        <td>{!c.valid || c.days < 0 ? <span className="badge crit">{t("已过期")}</span>
                          : <span className={"badge " + (c.days < 15 ? "warn" : "ok")}>{t("{n} 天", { n: c.days })}</span>}</td>
                        <td className="act"><button className="btn ghost sm" onClick={() => setDialog({ force: c.name })} disabled={!canRoot}>{t("强制续期")}</button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {st.certbot.installed && !st.certbot.autoRenew && <div className="note warn" style={{ marginTop: 12 }}><TriangleAlert /><span>{t("没找到 certbot 的定时续期（certbot.timer 或 /etc/cron.d/certbot）：证书到期前不会自动续期。")}</span></div>}
            <p className="hint" style={{ marginBottom: 0 }}>{t("certbot 在到期前 30 天内自动续期，续期后重载 Nginx。Caddy 站点的证书由 Caddy 自己申请和续期，不在这里列出。")}</p>
          </div>
        </div>
      )}

      <div className="card">
        <div className="card-head wrap">
          <h2><ShieldCheck size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("Fail2ban（防暴力破解）")}</h2>
          {!f2b.installed ? <span className="badge neutral">{t("没有安装")}</span>
            : f2b.unknown ? <span className="badge warn">{t("读不出状态")}</span>
            : f2b.active ? <span className="badge ok">{t("运行中")}</span> : <span className="badge warn">{t("没有运行")}</span>}
          <span className="grow" />
          {f2b.installed
            ? <>
                <button className="btn ghost icon sm" title={t("看日志")} aria-label={t("看日志")} onClick={() => setDialog({ log: "" })}><FileText /></button>
                <button className="btn sm" onClick={() => setDialog("f2b")} disabled={!canRoot}><Settings2 />{t("设置")}</button>
              </>
            : f2bReq ? <ReqLink req={f2bReq} running={t("正在安装…")} />
            : f2b.installable && <button className="btn primary sm" onClick={() => setDialog("f2b")} disabled={!canRoot || loading}><ShieldCheck />{t("安装并启用")}</button>}
        </div>
        <div className="card-body">
          <p className="dim" style={{ marginTop: 0 }}>{t("按日志里的失败次数自动封禁来源 IP（它改的是防火墙规则）。安装、改设置、解封都是高危操作，批准时要重新验证身份。")}</p>
          {!f2b.installed && !f2b.installable && <div className="muted">{t("这台主机的系统不支持一键安装 Fail2ban，请在主机上自己安装。")}</div>}
          {f2b.unknown && <div className="note warn"><TriangleAlert /><span>{t("Fail2ban 在运行，但登录账号读不出它的状态（要 root）。给登录账号配置免密 sudo 后再刷新。")}</span></div>}
          {f2b.installed && (f2b.config
            ? <p style={{ margin: "0 0 12px" }}>{t("{find}内失败 {n} 次，封禁 {ban}。", { find: span(t, f2b.config.findtime), n: f2b.config.maxretry, ban: span(t, f2b.config.bantime) })}
                {f2b.config.ignore.length > 0 && <span className="muted"> {t("白名单：")}<span className="mono">{f2b.config.ignore.join(" ")}</span></span>}</p>
            : <p className="hint" style={{ marginTop: 0 }}>{t("现在用的是主机上原有的设置。点“设置”后由 Hostower 接管：只写 /etc/fail2ban/jail.d/hostower.local，原有的文件不动。")}</p>)}
          {f2b.jails.length > 0 && (
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("防护项")}</th><th>{t("当前失败")}</th><th>{t("累计封禁")}</th><th>{t("正在封禁的 IP")}</th></tr></thead>
                <tbody>
                  {f2b.jails.map((j) => (
                    <tr key={j.name}>
                      <td className="mono">{j.name}</td>
                      <td>{j.failed}</td>
                      <td>{j.totalBanned}</td>
                      <td>{j.ips.length === 0 ? <span className="muted">{t("没有")}</span> : (
                        <div className="row wrap" style={{ gap: 6 }}>
                          {j.ips.map((ip) => (
                            <span key={ip} className="tag mono" style={{ gap: 6 }}>{ip}
                              <button className="link-btn" title={t("解封")} aria-label={t("解封 {ip}", { ip })} disabled={!canRoot} onClick={() => setDialog({ unban: { jail: j.name, ip } })}><Ban size={12} /></button>
                            </span>
                          ))}
                        </div>
                      )}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {st.clientIp && <p className="hint" style={{ marginBottom: 0 }}>{t("Hostower 连这台主机用的地址 {ip} 固定在白名单里，不会被封。", { ip: st.clientIp })}</p>}
        </div>
      </div>

      <ListenCard data={data} />

      {dialog === "setup" && <SetupDialog host={host} data={data} onClose={() => setDialog(null)} />}
      {dialog === "allow" && <AllowDialog host={host} tool={toolName} onClose={() => setDialog(null)} />}
      {dialog === "site" && <SiteDialog host={host} data={data} port={presetPort} onClose={() => setDialog(null)} />}
      {dialog === "caddy" && <PlanDialog title={t("在 {host} 上安装 Caddy", { host })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action: "install-caddy", preview })}>
        <p className="dim">{t("从系统软件源安装 Caddy 并开机自启。装好后它默认在 80 端口提供一个欢迎页。")}</p></PlanDialog>}
      {dialog && typeof dialog === "object" && "remove" in dialog && (
        <PlanDialog title={t("关闭端口 {port}", { port: portText(dialog.remove) })} onClose={() => setDialog(null)} send={(preview) => api.hostFirewall(host, { action: "remove", ports: [dialog.remove], preview })}>
          <p className="dim">{t("删掉这条放行规则后，外面就连不上这个端口了。改防火墙是高危操作，批准时要重新验证身份。")}</p></PlanDialog>
      )}
      {(dialog === "start-caddy" || dialog === "start-nginx") && (() => {
        const action = dialog, name = dialog === "start-caddy" ? "Caddy" : "Nginx";
        return <PlanDialog title={t("在 {host} 上启动 {name}", { host, name })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action, preview })}>
          <p className="dim">{t("{name} 没在运行时，这台主机上的站点都打不开，证书也不会申请。起不来的话，输出里会带上它最近的日志，原因多半是 80 或 443 端口被别的程序占了。", { name })}</p></PlanDialog>;
      })()}
      {dialog === "nginx" && <PlanDialog title={t("在 {host} 上安装 Nginx", { host })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action: "install-nginx", preview })}>
        <p className="dim">{t("从系统软件源安装 Nginx 并开机自启。RHEL 系还会打开 SELinux 的 httpd_can_network_connect，否则反向代理会被挡住。")}</p>
        {st.listen.some((l) => l.port === 80) && <div className="note warn"><TriangleAlert /><span>{t("80 端口已经有程序在监听：Nginx 可能启动不了。")}</span></div>}</PlanDialog>}
      {dialog === "f2b" && <F2BDialog host={host} data={data} onClose={() => setDialog(null)} />}
      {dialog && typeof dialog === "object" && "unsite" in dialog && (
        <PlanDialog title={t("撤下站点 {addr}", { addr: dialog.unsite })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action: "remove", address: dialog.unsite, preview })}>
          <p className="dim">{t("站点文件改名为 .removed 留在主机上（想恢复改回名字即可），然后重载服务。申请到的证书留着不动。")}</p></PlanDialog>
      )}
      {dialog && typeof dialog === "object" && "log" in dialog && <LogDialog host={host} site={st.sites.find((x) => x.address === dialog.log)} onClose={() => setDialog(null)} />}
      {dialog && typeof dialog === "object" && "cert" in dialog && (
        <PlanDialog title={dialog.cert === "dry-run" ? t("演练证书续期") : t("续期快到期的证书")} onClose={() => setDialog(null)} send={(preview) => api.hostCerts(host, { action: dialog.cert, preview })}>
          <p className="dim">{dialog.cert === "dry-run" ? t("用 Let's Encrypt 的测试环境走一遍续期流程，不换证书，用来确认自动续期能成功。") : t("只续期 30 天内到期的证书，其余的不动。")}</p></PlanDialog>
      )}
      {dialog && typeof dialog === "object" && "force" in dialog && (
        <PlanDialog title={t("强制续期 {name}", { name: dialog.force })} onClose={() => setDialog(null)} send={(preview) => api.hostCerts(host, { action: "force", name: dialog.force, preview })}>
          <p className="dim">{t("不管还剩几天都重新申请一张。Let's Encrypt 对同一组域名每周最多签发 5 次，别频繁使用。")}</p></PlanDialog>
      )}
      {dialog && typeof dialog === "object" && "unban" in dialog && (
        <PlanDialog title={t("解封 {ip}", { ip: dialog.unban.ip })} onClose={() => setDialog(null)} send={(preview) => api.hostFail2ban(host, { action: "unban", jail: dialog.unban.jail, ip: dialog.unban.ip, preview })}>
          <p className="dim">{t("从 {jail} 里解封这个地址。它再失败够次数还会被封；要一直放行就加进白名单。", { jail: dialog.unban.jail })}</p></PlanDialog>
      )}

    </>
  );
}

/** 安装并启用防火墙：列出正在对外监听的端口让你勾选一起放行；SSH（和面板）固定放行。 */
function SetupDialog({ host, data, onClose }: { host: string; data: NetworkView; onClose: () => void }) {
  const t = useT();
  const fw = data.status.firewall;
  const keep = new Set(data.keep.map(portText));
  const options = [...(fw.ufwInstalled ? ["ufw"] : []), ...(fw.firewalldInstalled ? ["firewalld"] : [])];
  if (fw.recommended && !options.includes(fw.recommended)) options.unshift(fw.recommended);
  const [tool, setTool] = useState(options.includes(fw.recommended) ? fw.recommended : options[0] ?? "");
  const listen = data.status.listen.map((l) => ({ port: l.port, proto: "tcp" as const })).filter((p) => !keep.has(portText(p)));
  const [picked, setPicked] = useState<string[]>(listen.map(portText));
  const [extra, setExtra] = useState<NetPort>({ port: 0, proto: "tcp" });
  const ports = [...listen.filter((p) => picked.includes(portText(p))), ...(extra.port ? [extra] : [])];
  return (
    <PlanDialog title={t("在 {host} 上启用防火墙", { host })} onClose={onClose} send={(preview) => api.hostFirewall(host, { action: "setup", tool, ports, preview })} ready={!!tool}>
      <div className="field"><label>{t("用哪个防火墙")}</label>
        <Select value={tool} onChange={setTool} options={options.map((o) => ({ value: o, label: o, hint: o === fw.recommended ? t("推荐") : undefined }))} />
        <span className="hint">{!fw.ufwInstalled && tool === "ufw" ? t("会先用 apt 安装 ufw。") : !fw.firewalldInstalled && tool === "firewalld" ? t("会先用 dnf 安装 firewalld。") : t("已经装好，直接启用。")}</span>
      </div>
      <div className="field"><label>{t("一起放行的端口")}</label>
        <div className="row wrap" style={{ gap: 6 }}>
          {data.keep.map((p) => <span key={portText(p)} className="tag on mono"><Lock size={12} />{portText(p)}</span>)}
          {listen.map((p) => (
            <label key={portText(p)} className={"tag mono" + (picked.includes(portText(p)) ? " on" : "")} style={{ cursor: "pointer" }}>
              <input type="checkbox" style={{ marginRight: 4 }} checked={picked.includes(portText(p))} onChange={(e) => setPicked(e.target.checked ? [...picked, portText(p)] : picked.filter((x) => x !== portText(p)))} />{portText(p)}
            </label>
          ))}
        </div>
        <span className="hint">{t("带锁的是 SSH（和面板）的端口，固定放行。其余是现在正在对外监听的端口：不勾的启用后会被挡住。")}</span>
      </div>
      <div className="field"><label>{t("再加一个端口（可选）")}</label><PortInput value={extra} onChange={setExtra} /></div>
      <div className="note"><TriangleAlert /><span>{t("顺序是：安装 → 放行上面这些端口 → 最后才启用。任何一步失败都不会启用。改防火墙是高危操作，批准时要重新验证身份。")}</span></div>
    </PlanDialog>
  );
}

function AllowDialog({ host, tool, onClose }: { host: string; tool: string; onClose: () => void }) {
  const t = useT();
  const [p, setP] = useState<NetPort>({ port: 0, proto: "tcp" });
  return (
    <PlanDialog title={t("在 {host} 上开放端口", { host })} onClose={onClose} ready={p.port > 0} send={(preview) => api.hostFirewall(host, { action: "allow", ports: [p], preview })}>
      <div className="field"><label>{t("端口")}</label><PortInput value={p} onChange={setP} />
        <span className="hint">{t("用 {tool} 放行。Docker 直接发布到所有地址的端口本来就不受它控制。", { tool })}</span></div>
    </PlanDialog>
  );
}

type T = ReturnType<typeof useT>;

function kindLabel(t: T, k: SiteKind) {
  return k === "static" ? t("静态目录") : k === "redirect" ? t("跳转") : t("反向代理");
}

function TLSBadge({ site }: { site: NetSite }) {
  const t = useT();
  switch (site.tls) {
    case "auto": return <span className="badge ok">{t("自动证书")}</span>;
    case "le": return <span className="badge ok">Let&apos;s Encrypt</span>;
    case "internal": return <span className="badge warn">{t("自签证书")}</span>;
    case "custom": return <span className="badge accent">{t("已有证书")}</span>;
    default: return <span className="badge neutral">{t("只用 HTTP")}</span>;
  }
}

/** 把秒数写成“1 小时”“10 分钟”这样的说法。 */
function span(t: T, sec: number) {
  if (sec % 86400 === 0) return t("{n} 天", { n: sec / 86400 });
  if (sec % 3600 === 0) return t("{n} 小时", { n: sec / 3600 });
  if (sec % 60 === 0) return t("{n} 分钟", { n: sec / 60 });
  return t("{n} 秒", { n: sec });
}

function Check({ checked, onChange, children }: { checked: boolean; onChange: (v: boolean) => void; children: React.ReactNode }) {
  return <label className="check"><input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />{children}</label>;
}

const numIn = (v: string) => Number(v.replace(/\D/g, "")) || 0;

function SiteDialog({ host, data, port: preset, onClose }: { host: string; data: NetworkView; port: number; onClose: () => void }) {
  const t = useT();
  const st = data.status;
  const servers: SiteServer[] = [...(st.caddy.installed ? ["caddy" as const] : []), ...(st.nginx.installed ? ["nginx" as const] : [])];
  const [f, setF] = useState<SiteForm>({
    server: servers[0] ?? "caddy", address: "", kind: "proxy", port: preset || 0, dir: "", url: "", tls: servers[0] === "nginx" ? "le" : "auto",
    email: "", agree: false, cert: "", key: "", maxBody: 100, hsts: false, keepHttp: false, perm: false,
  });
  const set = (p: Partial<SiteForm>) => setF((o) => ({ ...o, ...p }));
  const addr = f.address.trim().toLowerCase();
  const ip = /^\d{1,3}(\.\d{1,3}){3}:\d+$/.test(addr);
  const hasPort = /:\d+$/.test(addr);
  const tlsOptions: { value: SiteTLS; label: string; hint?: string }[] = f.server === "caddy"
    ? [...(ip ? [] : [{ value: "auto" as const, label: t("自动申请（Let's Encrypt / ZeroSSL）"), hint: t("推荐") }]),
       { value: "internal", label: t("Caddy 自签证书") }, { value: "custom", label: t("用已有的证书") }, { value: "http", label: t("只用 HTTP") }]
    : [...(ip || hasPort ? [] : [{ value: "le" as const, label: t("Let's Encrypt 自动申请"), hint: t("推荐") }]),
       { value: "custom", label: t("用已有的证书") }, { value: "http", label: t("只用 HTTP") }];
  // 地址或服务器变了，原来的方式不能用时换成第一个能用的。
  const tls = tlsOptions.some((o) => o.value === f.tls) ? f.tls : tlsOptions[0].value;
  const target = f.kind === "proxy" ? f.port > 0 : f.kind === "static" ? !!f.dir.trim() : !!f.url.trim();
  const ready = !!addr && target && (tls !== "le" || f.agree) && (tls !== "custom" || (!!f.cert.trim() && !!f.key.trim()));
  const busy443 = tls !== "http" && st.listen.some((l) => l.port === 443) && !st.sites.some((x) => x.server === f.server && x.tls !== "http") && !(f.server === "caddy" && st.caddy.active);
  const send = (preview: boolean) => api.hostSites(host, { action: "add", ...f, address: addr, tls, dir: f.dir.trim(), url: f.url.trim(), cert: f.cert.trim(), key: f.key.trim(), email: f.email.trim(), preview });
  return (
    <PlanDialog title={t("在 {host} 上添加站点", { host })} onClose={onClose} ready={ready} send={send}>
      <div className="form-grid">
        {servers.length > 1 && <div className="field"><label>{t("用哪个服务器")}</label>
          <Select value={f.server} onChange={(v) => set({ server: v as SiteServer })} options={servers.map((s) => ({ value: s, label: s === "nginx" ? "Nginx" : "Caddy" }))} /></div>}
        <div className="field"><label>{t("站点类型")}</label>
          <Select value={f.kind} onChange={(v) => set({ kind: v as SiteKind })} options={[
            { value: "proxy", label: t("反向代理"), hint: t("转发到本机端口") }, { value: "static", label: t("静态目录") }, { value: "redirect", label: t("跳转"), hint: t("跳到别的网址") }]} /></div>
        <div className="field"><label htmlFor="site-addr">{t("访问地址")}</label>
          <input id="site-addr" className="input mono" value={f.address} onChange={(e) => set({ address: e.target.value })} placeholder="app.example.com" spellCheck={false} />
          <span className="hint">{t("域名要先解析到这台主机，才能申请证书。也可以写 IP:端口，或者 域名:端口。")}</span>
        </div>
        {f.kind === "proxy" && <div className="field"><label htmlFor="site-port">{t("转发到本机端口")}</label>
          <input id="site-port" className="input mono" inputMode="numeric" value={f.port || ""} onChange={(e) => set({ port: numIn(e.target.value) })} placeholder="3001" />
          <span className="hint">{t("转发到 127.0.0.1 的这个端口，比如应用市场里装的应用的端口。支持 WebSocket。")}</span></div>}
        {f.kind === "static" && <div className="field"><label htmlFor="site-dir">{t("网站目录")}</label>
          <input id="site-dir" className="input mono" value={f.dir} onChange={(e) => set({ dir: e.target.value })} placeholder="/var/www/site" spellCheck={false} />
          <span className="hint">{t("要在 /var/www、/srv、/opt 或 /home 下面，不能是隐藏目录。目录里的文件会公开给所有人。")}</span></div>}
        {f.kind === "redirect" && <div className="field"><label htmlFor="site-url">{t("跳转到")}</label>
          <input id="site-url" className="input mono" value={f.url} onChange={(e) => set({ url: e.target.value })} placeholder="https://www.example.com" spellCheck={false} /></div>}
        <div className="field"><label>HTTPS</label>
          <Select value={tls} onChange={(v) => set({ tls: v as SiteTLS })} options={tlsOptions} />
          <span className="hint">{tls === "auto" ? t("Caddy 自动申请证书并在到期前续期。")
            : tls === "le" ? t("先以 HTTP 开站，用 certbot 从 80 端口验证域名、申请证书，再换成 HTTPS。certbot 会在到期前自动续期。")
            : tls === "internal" ? t("浏览器会提示证书不受信任，适合内网或测试。")
            : tls === "custom" ? t("证书由你自己负责续期，换了证书后要重载服务。")
            : t("不加密：密码等内容会明文传输。")}</span>
        </div>
      </div>
      {tls === "le" && <div className="form-grid">
        <div className="field"><label htmlFor="site-mail">{t("联系邮箱（可选）")}</label>
          <input id="site-mail" className="input mono" value={f.email} onChange={(e) => set({ email: e.target.value })} placeholder="ops@example.com" spellCheck={false} />
          <span className="hint">{t("Let's Encrypt 用它发到期和安全通知；不填也能申请。")}</span></div>
        <div className="field"><label>&nbsp;</label>
          <Check checked={f.agree} onChange={(v) => set({ agree: v })}>{t("我同意 Let's Encrypt 的订户协议")}</Check>
          <a className="hint" href="https://letsencrypt.org/repository/" target="_blank" rel="noreferrer noopener">letsencrypt.org/repository <ExternalLink size={11} /></a></div>
      </div>}
      {tls === "custom" && <div className="form-grid">
        <div className="field"><label htmlFor="site-cert">{t("证书文件")}</label>
          <input id="site-cert" className="input mono" value={f.cert} onChange={(e) => set({ cert: e.target.value })} placeholder="/etc/ssl/certs/app.pem" spellCheck={false} /></div>
        <div className="field"><label htmlFor="site-key">{t("私钥文件")}</label>
          <input id="site-key" className="input mono" value={f.key} onChange={(e) => set({ key: e.target.value })} placeholder="/etc/ssl/private/app.key" spellCheck={false} />
          <span className="hint">{t("主机上的路径（/etc/ssl、/etc/letsencrypt、/etc/pki 等下面）。只写进配置，Hostower 不读私钥。")}</span></div>
      </div>}
      <div className="field"><label>{t("选项")}</label>
        <div className="col" style={{ gap: 6 }}>
          {f.kind !== "redirect" && <div className="row" style={{ gap: 8 }}><span className="dim">{t("请求体上限")}</span>
            <input className="input mono" style={{ width: 90 }} inputMode="numeric" aria-label={t("请求体上限")} value={f.maxBody || ""} onChange={(e) => set({ maxBody: numIn(e.target.value) })} placeholder="0" /><span className="dim">MB</span>
            <span className="hint">{t("上传大文件时调大；留空用服务器的默认值。")}</span></div>}
          {tls !== "http" && <Check checked={f.keepHttp} onChange={(v) => set({ keepHttp: v })}>{t("同时允许 HTTP 访问（不自动跳到 HTTPS）")}</Check>}
          {tls !== "http" && <Check checked={f.hsts} onChange={(v) => set({ hsts: v })}>{t("开启 HSTS（浏览器之后只用 HTTPS 访问，开了不容易撤回）")}</Check>}
          {f.kind === "redirect" && <Check checked={f.perm} onChange={(v) => set({ perm: v })}>{t("永久跳转（301，浏览器会记住）")}</Check>}
        </div>
      </div>
      {busy443 && <div className="note warn"><TriangleAlert /><span>{t("443 端口已经有程序在监听（比如 3x-ui 的 Xray）：HTTPS 站点可能起不来。")}</span></div>}
      <p className="hint">{t("写入一个单独的站点文件，先校验配置（caddy validate / nginx -t），通过才重载；校验不过会自动撤下，不影响已有的站点。")}</p>
    </PlanDialog>
  );
}

/** 安装 Fail2ban 或改它的设置：时长按分钟填。 */
function F2BDialog({ host, data, onClose }: { host: string; data: NetworkView; onClose: () => void }) {
  const t = useT();
  const st = data.status;
  const f2b = st.fail2ban;
  const init: F2BConfig = f2b.config ?? { bantime: 3600, findtime: 600, maxretry: 5, ignore: [], jails: ["sshd"] };
  const [ban, setBan] = useState(Math.round(init.bantime / 60));
  const [find, setFind] = useState(Math.round(init.findtime / 60));
  const [retry, setRetry] = useState(init.maxretry);
  const [ignore, setIgnore] = useState(init.ignore.join("\n"));
  const [jails, setJails] = useState<string[]>(init.jails);
  const toggle = (j: string, on: boolean) => setJails(on ? [...jails, j] : jails.filter((x) => x !== j));
  const config: F2BConfig = { bantime: ban * 60, findtime: find * 60, maxretry: retry, ignore: ignore.split(/[\s,]+/).filter(Boolean), jails };
  const install = !f2b.installed;
  return (
    <PlanDialog title={install ? t("在 {host} 上安装 Fail2ban", { host }) : t("{host} 上的 Fail2ban 设置", { host })} onClose={onClose} ready={ban > 0 && find > 0 && retry > 0}
      send={(preview) => api.hostFail2ban(host, { action: install ? "install" : "config", config, preview })}>
      <div className="form-grid">
        <div className="field"><label htmlFor="f2b-retry">{t("失败几次就封")}</label>
          <input id="f2b-retry" className="input mono" inputMode="numeric" value={retry || ""} onChange={(e) => setRetry(numIn(e.target.value))} /></div>
        <div className="field"><label htmlFor="f2b-find">{t("统计时间窗口（分钟）")}</label>
          <input id="f2b-find" className="input mono" inputMode="numeric" value={find || ""} onChange={(e) => setFind(numIn(e.target.value))} /></div>
        <div className="field"><label htmlFor="f2b-ban">{t("封禁时长（分钟）")}</label>
          <input id="f2b-ban" className="input mono" inputMode="numeric" value={ban || ""} onChange={(e) => setBan(numIn(e.target.value))} /></div>
      </div>
      <div className="field"><label>{t("防护项")}</label>
        <div className="col" style={{ gap: 6 }}>
          <Check checked={jails.includes("sshd")} onChange={(v) => toggle("sshd", v)}>{t("SSH 登录（sshd，端口 {port}）", { port: st.sshPort })}</Check>
          {st.nginx.installed && <Check checked={jails.includes("nginx-http-auth")} onChange={(v) => toggle("nginx-http-auth", v)}>{t("Nginx 密码验证失败（nginx-http-auth）")}</Check>}
          {st.nginx.installed && <Check checked={jails.includes("nginx-botsearch")} onChange={(v) => toggle("nginx-botsearch", v)}>{t("扫描漏洞路径的机器人（nginx-botsearch）")}</Check>}
        </div>
      </div>
      <div className="field"><label htmlFor="f2b-ignore">{t("白名单（每行一个 IP 或网段）")}</label>
        <textarea id="f2b-ignore" className="textarea lines" rows={3} value={ignore} onChange={(e) => setIgnore(e.target.value)} spellCheck={false} placeholder={"192.0.2.10\n198.51.100.0/24"} />
        <span className="hint">{st.clientIp ? t("本机回环地址和 Hostower 的来源地址 {ip} 会自动加进去，不用填。", { ip: st.clientIp }) : t("本机回环地址会自动加进去，不用填。")}</span>
      </div>
      {install && !f2b.authLog && st.family === "debian" && <p className="hint">{t("这台主机没有 /var/log/auth.log，会让 Fail2ban 从 systemd 的日志里读 SSH 登录记录（顺带装 python3-systemd）。")}</p>}
      <div className="note"><TriangleAlert /><span>{install
        ? t("顺序是：先写好配置 → 安装 → 检查配置 → 启动。Fail2ban 改的是防火墙规则，这是高危操作，批准时要重新验证身份。")
        : t("改之前备份原来的设置，检查不通过就换回去。Fail2ban 改的是防火墙规则，这是高危操作，批准时要重新验证身份。")}</span></div>
    </PlanDialog>
  );
}

/** 看站点或 Fail2ban 的日志（最后 200 行，按纯文本显示）。site 为空时看 Fail2ban 的。 */
function LogDialog({ host, site, onClose }: { host: string; site?: NetSite; onClose: () => void }) {
  const t = useT();
  const [kind, setKind] = useState<"access" | "error">("access");
  const [text, setText] = useState<string | null>(null);
  const [err, setErr] = useState("");
  // 切换日志种类或刷新时，先发出的请求可能后回来：只认最后一次的结果。
  const seq = useRef(0);
  const load = useCallback(() => {
    const n = ++seq.current;
    setText(null); setErr("");
    api.hostNetLog(host, site ? { kind, server: site.server, address: site.address } : { kind: "fail2ban" })
      .then((r) => { if (n === seq.current) setText(r.text); })
      .catch((e) => { if (n === seq.current) setErr(e instanceof Error ? e.message : String(e)); });
  }, [host, site, kind]);
  useEffect(() => { load(); }, [load]);
  return (
    <Modal wide="xl" title={site ? t("{addr} 的日志", { addr: site.address }) : t("Fail2ban 日志")} onClose={onClose}
      footer={<><button className="btn" onClick={load}><RefreshCw />{t("刷新")}</button><button className="btn primary" onClick={onClose}>{t("关闭")}</button></>}>
      {site && <div className="seg" role="tablist">
        <button role="tab" aria-selected={kind === "access"} className={kind === "access" ? "on" : ""} onClick={() => setKind("access")}>{t("访问日志")}</button>
        <button role="tab" aria-selected={kind === "error"} className={kind === "error" ? "on" : ""} onClick={() => setKind("error")}>{t("错误日志")}</button>
      </div>}
      {err ? <div className="note crit"><TriangleAlert /><span>{ts(err)}</span></div>
        : text === null ? <div className="muted">{t("加载中…")}</div>
        : text.trim() === "" ? <div className="muted">{t("没有内容（这个站点可能还没有访问，或者是旧版本添加的站点，没有单独的日志）。")}</div>
        : <pre className="code" style={{ maxHeight: "60vh", overflow: "auto" }}>{text}</pre>}
    </Modal>
  );
}

/**
 * 对外监听的端口：按“来源”分组的一张表。第一行是主机进程（SSH / 面板的端口带锁），其后每个直接发布到所有地址的容器一行。
 * 每条信息只出现一次：卡片头给出总数和“绕过防火墙”的数量，警告只有一句，表里的端口一律是中性的小标签；
 * 有风险的容器行用图标加文字标出来（不只靠颜色）。
 */
function ListenCard({ data }: { data: NetworkView }) {
  const t = useT();
  const st = data.status;
  const keep = new Set(data.keep.map(portText));
  // 容器 → 端口 → 协议（同一个端口的 TCP 和 UDP、IPv4 和 IPv6 合成一个标签）。
  const ctrs = new Map<string, Map<number, Set<string>>>();
  const ctrTCP = new Set<number>();
  for (const d of st.docker) {
    const m = ctrs.get(d.container) ?? new Map<number, Set<string>>();
    m.set(d.port, (m.get(d.port) ?? new Set<string>()).add(d.proto));
    ctrs.set(d.container, m);
    if (d.proto === "tcp") ctrTCP.add(d.port);
  }
  const exposed = new Set(st.docker.map((d) => `${d.port}/${d.proto}`)).size;
  const hostPorts = st.listen.filter((l) => !ctrTCP.has(l.port)).sort((a, b) => a.port - b.port);
  const rows = [...ctrs.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  const protoText = (s: Set<string>) => (s.size === 1 && s.has("tcp") ? "" : "/" + [...s].sort().join("+"));
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("对外监听的端口")}</h2>
        {st.listen.length > 0 && <span className="badge neutral">{t("{n} 个端口", { n: st.listen.length })}</span>}
        {exposed > 0 && <span className="badge warn"><ShieldOff />{t("{n} 个绕过防火墙", { n: exposed })}</span>}
      </div>
      <div className="card-body">
        {exposed > 0 && <div className="note warn" style={{ marginTop: 0, marginBottom: 12 }}><TriangleAlert /><span>
          {t("{n} 个容器端口直接发布在所有地址上，会绕过 ufw 和 firewalld，防火墙挡不住。只想本机或通过反向代理访问的，改成绑定 127.0.0.1。", { n: exposed })}
        </span></div>}
        {st.listen.length === 0 && rows.length === 0 ? <div className="muted">{t("没有读到（可能没有 ss 命令）。")}</div> : (
          <div className="port-table" role="table" aria-label={t("对外监听的端口")}>
            <div className="port-row head" role="row">
              <span role="columnheader">{t("来源")}</span><span role="columnheader">{t("端口")}</span><span role="columnheader">{t("状态")}</span>
            </div>
            {hostPorts.length > 0 && (
              <div className="port-row" role="row">
                <span className="src" role="cell">{t("主机进程")}</span>
                <span className="ports" role="cell">{hostPorts.map((l) => keep.has(`${l.port}/tcp`)
                  ? <span key={l.port} className="tag mono" title={l.addr}><Lock size={11} style={{ marginRight: 4 }} />{l.port} SSH/{t("面板")}</span>
                  : <span key={l.port} className="tag mono" title={l.addr}>{l.port}</span>)}</span>
                <span className="state" role="cell" />
              </div>
            )}
            {rows.map(([name, ports]) => (
              <div key={name} className="port-row" role="row">
                <span className="src mono" role="cell">{name}</span>
                <span className="ports" role="cell">{[...ports.entries()].sort((a, b) => a[0] - b[0]).map(([p, s]) => <span key={p} className="tag mono">{p}{protoText(s)}</span>)}</span>
                <span className="state" role="cell"><span className="risk"><ShieldOff size={13} />{t("绕过防火墙")}</span></span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
