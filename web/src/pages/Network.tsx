// 主机的网络入口：防火墙放行端口、Caddy 反向代理站点、对外监听的端口。
// 这一页没有自己的执行通道：现状是只读检测，改动都变成请求，走判级和审批（改防火墙是 L2，改站点是 L1）。
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ExternalLink, Globe, Lock, Plus, RefreshCw, Shield, ShieldOff, Trash2, TriangleAlert } from "lucide-react";
import { api, type NetPort, type NetworkView, type Preview, type AppPlanResult } from "../api";
import { fail } from "../store";
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
      if (reqs[0]) nav(`/requests/${reqs[0].id}`);
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
  const [data, setData] = useState<NetworkView | null>(() => cached<NetworkView>("net:" + host) ?? null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [dialog, setDialog] = useState<null | "setup" | "allow" | "site" | "caddy" | { remove: NetPort } | { unsite: string }>(null);
  // 从应用页跳过来时带着端口：直接打开“添加站点”。
  const presetPort = Number(sp.get("port")) || 0;

  const load = useCallback(() => {
    setLoading(true); setError("");
    api.hostNetwork(host).then((r) => setData(remember("net:" + host, r))).catch((e) => setError(e.message)).finally(() => setLoading(false));
  }, [host]);
  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    if (presetPort && data) { setDialog("site"); const n = new URLSearchParams(sp); n.delete("port"); setSp(n, { replace: true }); }
  }, [presetPort, data, sp, setSp]);

  if (error) return <div className="note crit"><TriangleAlert /><span>{ts(error)}</span></div>;
  if (!data) return <div className="card"><div className="empty">{t("加载中…")}</div></div>;
  const st = data.status;
  const fw = st.firewall;
  const keep = new Set(data.keep.map(portText));
  const canRoot = st.root || (st.sudo && !st.noNewPrivs);
  const toolName = fw.tool === "ufw" ? "ufw" : fw.tool === "firewalld" ? "firewalld" : "";
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
          {!fw.tool && !fw.unknown && <button className="btn primary sm" onClick={() => setDialog("setup")} disabled={!canRoot || (!fw.recommended && !fw.ufwInstalled && !fw.firewalldInstalled)}>
            <Shield />{fw.ufwInstalled || fw.firewalldInstalled ? t("启用防火墙") : t("安装并启用防火墙")}</button>}
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
          <h2><Globe size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("站点（反向代理）")}</h2>
          {st.caddy.installed ? <span className={"badge " + (st.caddy.active ? "ok" : "warn")}>Caddy · {st.caddy.active ? t("运行中") : t("没有运行")}</span> : <span className="badge neutral">{t("没有 Caddy")}</span>}
          <span className="grow" />
          {st.caddy.installed
            ? <button className="btn primary sm" onClick={() => setDialog("site")} disabled={!canRoot}><Plus />{t("添加站点")}</button>
            : st.caddy.installable && <button className="btn primary sm" onClick={() => setDialog("caddy")} disabled={!canRoot}><Plus />{t("安装 Caddy")}</button>}
        </div>
        <div className="card-body">
          <p className="dim" style={{ marginTop: 0 }}>{t("用域名访问装在这台主机上的服务：Caddy 自动申请 HTTPS 证书，再转发到本机的端口。服务本身只监听 127.0.0.1，不用对外开放。")}</p>
          {!st.caddy.installed && !st.caddy.installable && <div className="muted">{t("这台主机的系统不支持一键安装 Caddy，请按 Caddy 官网的说明安装。")}</div>}
          {st.caddy.installed && !webOpen && <div className="note warn"><TriangleAlert /><span>{t("防火墙没有放行 80 和 443：Caddy 申请证书和对外提供 HTTPS 都要用到它们。")}</span></div>}
          {st.sites.length === 0 ? (st.caddy.installed && <Empty icon={<Globe />} title={t("还没有 Hostower 添加的站点")} />) : (
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("地址")}</th><th>{t("转发到")}</th><th className="act">{t("操作")}</th></tr></thead>
                <tbody>
                  {st.sites.map((s) => (
                    <tr key={s.address}>
                      <td className="mono"><a href={`https://${s.address}`} target="_blank" rel="noreferrer noopener" style={{ color: "var(--accent)" }}>{s.address} <ExternalLink size={12} /></a></td>
                      <td className="mono">{s.upstream}</td>
                      <td className="act"><button className="btn ghost icon sm" title={t("撤下站点")} aria-label={t("撤下站点")} onClick={() => setDialog({ unsite: s.address })}><Trash2 /></button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="hint" style={{ marginBottom: 0 }}>{t("这里只列出 Hostower 添加的站点（/etc/caddy/sites.d）；Caddyfile 里原有的站点不受影响。")}</p>
        </div>
      </div>

      <div className="card">
        <div className="card-head"><h2>{t("对外监听的端口")}</h2></div>
        <div className="card-body">
          {st.listen.length === 0 ? <div className="muted">{t("没有读到（可能没有 ss 命令）。")}</div> : (
            <div className="row wrap" style={{ gap: 6 }}>{st.listen.map((l) => <span key={l.port} className="tag mono" title={l.addr}>{l.port}{keep.has(`${l.port}/tcp`) ? " · SSH/" + t("面板") : ""}</span>)}</div>
          )}
          {st.docker.length > 0 && (
            <div className="note warn" style={{ marginTop: 12 }}><TriangleAlert /><span>
              {t("这些容器端口直接发布在所有地址上，会绕过 ufw 和 firewalld，防火墙挡不住：")}
              <span className="mono">{st.docker.map((d) => `${d.container} ${d.port}/${d.proto}`).join("、")}</span>
              {t("。只想本机或通过反向代理访问的，改成绑定 127.0.0.1。")}
            </span></div>
          )}
        </div>
      </div>

      {dialog === "setup" && <SetupDialog host={host} data={data} onClose={() => setDialog(null)} />}
      {dialog === "allow" && <AllowDialog host={host} tool={toolName} onClose={() => setDialog(null)} />}
      {dialog === "site" && <SiteDialog host={host} port={presetPort} onClose={() => setDialog(null)} />}
      {dialog === "caddy" && <PlanDialog title={t("在 {host} 上安装 Caddy", { host })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action: "install-caddy", preview })}>
        <p className="dim">{t("从系统软件源安装 Caddy 并开机自启。装好后它默认在 80 端口提供一个欢迎页。")}</p></PlanDialog>}
      {dialog && typeof dialog === "object" && "remove" in dialog && (
        <PlanDialog title={t("关闭端口 {port}", { port: portText(dialog.remove) })} onClose={() => setDialog(null)} send={(preview) => api.hostFirewall(host, { action: "remove", ports: [dialog.remove], preview })}>
          <p className="dim">{t("删掉这条放行规则后，外面就连不上这个端口了。改防火墙是高危操作，批准时要重新验证身份。")}</p></PlanDialog>
      )}
      {dialog && typeof dialog === "object" && "unsite" in dialog && (
        <PlanDialog title={t("撤下站点 {addr}", { addr: dialog.unsite })} onClose={() => setDialog(null)} send={(preview) => api.hostSites(host, { action: "remove", address: dialog.unsite, preview })}>
          <p className="dim">{t("站点文件改名为 .removed 留在主机上（想恢复改回名字即可），然后重载 Caddy。")}</p></PlanDialog>
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

function SiteDialog({ host, port: preset, onClose }: { host: string; port: number; onClose: () => void }) {
  const t = useT();
  const [addr, setAddr] = useState("");
  const [port, setPort] = useState(preset || 0);
  const ip = /^\d{1,3}(\.\d{1,3}){3}:\d+$/.test(addr.trim());
  return (
    <PlanDialog title={t("在 {host} 上添加站点", { host })} onClose={onClose} ready={!!addr.trim() && port > 0} send={(preview) => api.hostSites(host, { action: "add", address: addr.trim(), port, preview })}>
      <div className="form-grid">
        <div className="field"><label htmlFor="site-addr">{t("访问地址")}</label>
          <input id="site-addr" className="input mono" value={addr} onChange={(e) => setAddr(e.target.value)} placeholder="app.example.com" spellCheck={false} />
          <span className="hint">{ip ? t("IP:端口 用 Caddy 自签的证书，浏览器会提示不受信任。") : t("域名要先解析到这台主机，Caddy 才能自动申请证书。也可以写 IP:端口。")}</span>
        </div>
        <div className="field"><label htmlFor="site-port">{t("转发到本机端口")}</label>
          <input id="site-port" className="input mono" inputMode="numeric" value={port || ""} onChange={(e) => setPort(Number(e.target.value.replace(/\D/g, "")) || 0)} placeholder="3001" />
          <span className="hint">{t("转发到 127.0.0.1 的这个端口，比如应用市场里装的应用的端口。")}</span>
        </div>
      </div>
      <p className="hint">{t("写入一个单独的站点文件，先用 caddy validate 校验，通过才重载；校验不过会自动撤下，不影响已有的站点。")}</p>
    </PlanDialog>
  );
}
