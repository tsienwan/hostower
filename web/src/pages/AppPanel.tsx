// 应用的“打开面板”：地址按这个实例当前的参数（监听地址、端口）和清单里的路径现拼，改过设置以后自然跟着变；
// 拼不对的时候（端口或路径是在应用自己的页面里改的、套了反向代理、只绑在 127.0.0.1 上），所有者可以给实例设一个面板地址。
// 地址里不允许带账号、密码和查询参数，服务端也会再校验一遍。
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Copy, ExternalLink, Globe, RotateCcw, Save, TriangleAlert } from "lucide-react";
import { api, type AppGuide, type AppInstalled, type AppPanel, type Host, type NetSite } from "../api";
import { fail, toast, useStore } from "../store";
import { Modal } from "../ui";
import { ts, useT } from "../i18n";
import { browserHost } from "./AppGuide";

type Entry = AppGuide["open"][number];

export function panelEntry(r: Pick<AppInstalled, "guide">): Entry | undefined {
  return r.guide?.open.find((o) => o.panel);
}

/** 主机清单里的地址拆成 SSH 的目标：账号@主机，以及不是 22 时的端口。 */
export function sshTarget(h?: Host): string {
  if (!h) return "";
  let a = h.addr.trim();
  let user = "";
  const at = a.lastIndexOf("@");
  if (at >= 0) { user = a.slice(0, at + 1); a = a.slice(at + 1); }
  let port = "";
  const v6 = a.match(/^\[(.+)\](?::(\d+))?$/);
  if (v6) { a = v6[1]; port = v6[2] ?? ""; }
  else if ((a.match(/:/g) ?? []).length === 1) { const i = a.indexOf(":"); port = a.slice(i + 1); a = a.slice(0, i); }
  if (a === "" || h.executor === "local") a = "localhost";
  return (port && port !== "22" ? `-p ${port} ` : "") + user + a;
}

export interface PanelLink {
  /** 按钮打开的地址；只在本机监听、又没设面板地址时为空。 */
  url: string;
  /** 经由 SSH 隧道时，先在自己电脑上运行的命令。 */
  ssh?: string;
}

/** 按清单的面板入口、这个实例当前的参数和所有者设的面板地址，拼出按钮要打开的地址。 */
export function panelLink(h: Host | undefined, o: Entry, p?: AppPanel): PanelLink {
  const port = p?.port || o.port, path = p?.path || o.path || "/";
  if (p?.mode === "url" && p.url) return { url: p.url };
  if (p?.mode === "ssh") {
    const local = p.localPort || port;
    return { url: `http://localhost:${local}${path}`, ssh: `ssh -N -L ${local}:127.0.0.1:${port} ${sshTarget(h)}` };
  }
  if (o.local) return { url: "" };
  return { url: `${o.scheme}://${browserHost(h)}:${port}${path}` };
}

/** 网络页里反向代理到这个端口的站点：可以直接拿来当面板地址。 */
function siteUrls(sites: NetSite[], port: string, path: string): string[] {
  const out: string[] = [];
  for (const s of sites) {
    if (s.kind !== "proxy" || s.target !== `127.0.0.1:${port}`) continue;
    let a = s.address.trim();
    if (!a || a.startsWith(":")) continue;
    if (!/^https?:\/\//.test(a)) a = (s.tls === "http" ? "http://" : "https://") + a;
    out.push(a.replace(/\/+$/, "") + (path || "/"));
  }
  return [...new Set(out)];
}

function copy(text: string, done: string) {
  navigator.clipboard?.writeText(text).then(() => toast(done)).catch(() => {});
}

export function AppPanelDialog({ row, title, onClose, onSaved }: { row: AppInstalled & { host: string }; title: string; onClose: () => void; onSaved: () => void }) {
  const t = useT();
  const h = useStore((s) => s.hosts.find((x) => x.name === row.host));
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const o = panelEntry(row)!;
  const cur = row.panel;
  const [mode, setMode] = useState<AppPanel["mode"]>(cur?.mode ?? (o.local ? "url" : "direct"));
  const [url, setUrl] = useState(cur?.url ?? "");
  const [port, setPort] = useState(cur?.port ?? "");
  const [path, setPath] = useState(cur?.path ?? "");
  const [localPort, setLocalPort] = useState(cur?.localPort ?? "");
  const [sites, setSites] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);

  // 网络页里已经有反向代理到这个端口的站点：列出来，点一下就填进去。查不到不影响别的。
  useEffect(() => {
    let live = true;
    api.hostNetwork(row.host).then((v) => live && setSites(siteUrls(v.status?.sites ?? [], port || o.port, path || o.path))).catch(() => {});
    return () => { live = false; };
  }, [row.host]); // eslint-disable-line react-hooks/exhaustive-deps

  const draft: AppPanel = mode === "url" ? { mode, url: url.trim() } : mode === "ssh" ? { mode, port: port.trim(), path: path.trim(), localPort: localPort.trim() } : { mode, port: port.trim(), path: path.trim() };
  const preview = useMemo(() => panelLink(h, o, { ...draft, path: draft.path && !draft.path.startsWith("/") ? "/" + draft.path : draft.path }), [h, o, mode, url, port, path, localPort]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = (p: AppPanel) => {
    setBusy(true);
    api.setAppPanel(row.host, row.instance, p).then(() => { toast(t("已保存")); onSaved(); onClose(); }).catch(fail).finally(() => setBusy(false));
  };

  return (
    <Modal wide title={t("{app} 的面板地址", { app: title })} onClose={onClose}
      footer={<>
        {cur && owner && <button className="btn" disabled={busy} onClick={() => save({ mode: "direct" })}><RotateCcw />{t("恢复默认")}</button>}
        <span className="grow" />
        <button className="btn" onClick={onClose}>{t("取消")}</button>
        {owner && <button className="btn primary" disabled={busy} onClick={() => save(draft)}><Save />{t("保存")}</button>}
      </>}>
      <div className="col" style={{ gap: 14 }}>
        <div className="muted" style={{ fontSize: 12.5 }}>{t("实例 {instance} · 主机 {host}", { instance: row.instance, host: row.host })}</div>
        <div className="hint">{t("默认按这个实例当前的监听地址和端口拼出地址，在“修改设置”里改过以后会自动跟着变。下面的设置只存在塔台里，不改主机上的任何东西；改动会写进审计。")}</div>
        {o.inApp && <div className="note"><TriangleAlert /><span>{ts(o.inApp)}</span></div>}

        <div className="field">
          <label>{t("怎么访问")}</label>
          <div className="seg" role="radiogroup" aria-label={t("怎么访问")}>
            <button role="radio" aria-checked={mode === "direct"} className={mode === "direct" ? "on" : ""} onClick={() => setMode("direct")}>{t("直接访问主机")}</button>
            <button role="radio" aria-checked={mode === "url"} className={mode === "url" ? "on" : ""} onClick={() => setMode("url")}>{t("填写完整地址")}</button>
            <button role="radio" aria-checked={mode === "ssh"} className={mode === "ssh" ? "on" : ""} onClick={() => setMode("ssh")}>{t("经由 SSH 隧道")}</button>
          </div>
          <span className="hint">{mode === "direct" ? t("打开 主机地址:端口/路径。端口或路径在应用自己的页面里改过时，在下面填上新的；留空就照应用的设置。")
            : mode === "url" ? t("应用前面套了反向代理、用域名访问，或者只绑在 127.0.0.1 上时，直接填浏览器里要打开的地址。不能带账号、密码或 ? 后面的参数。")
            : t("不对外开放端口：在自己电脑上运行下面这条命令建立隧道（用的是这台主机的 SSH 登录方式），按钮打开本机的端口。")}</span>
        </div>

        {mode === "url" && (
          <div className="field">
            <label htmlFor="panel-url">{t("面板地址")}</label>
            <input id="panel-url" className="input mono" placeholder="https://panel.example.com/" value={url} onChange={(e) => setUrl(e.target.value)} />
            {sites.length > 0 && (
              <div className="row wrap" style={{ gap: 6 }}>
                <span className="hint">{t("网络页里反向代理到这个端口的站点：")}</span>
                {sites.map((s) => <button key={s} className="btn sm mono" onClick={() => setUrl(s)}>{s}</button>)}
              </div>
            )}
            {sites.length === 0 && <span className="hint"><Link to={`/hosts/${encodeURIComponent(row.host)}?tab=network&port=${port || o.port}`}><Globe style={{ width: 12, height: 12, verticalAlign: -1 }} /> {t("给它配一个域名（反向代理）")}</Link></span>}
          </div>
        )}
        {mode !== "url" && (
          <div className="row wrap" style={{ gap: 12 }}>
            <div className="field"><label htmlFor="panel-port">{t("应用端口")}</label>
              <input id="panel-port" className="input mono" style={{ width: 120 }} inputMode="numeric" placeholder={o.port} value={port} onChange={(e) => setPort(e.target.value)} /></div>
            <div className="field grow"><label htmlFor="panel-path">{t("路径")}</label>
              <input id="panel-path" className="input mono" placeholder={o.path || "/"} value={path} onChange={(e) => setPath(e.target.value)} /></div>
            {mode === "ssh" && <div className="field"><label htmlFor="panel-local">{t("本地端口")}</label>
              <input id="panel-local" className="input mono" style={{ width: 120 }} inputMode="numeric" placeholder={port || o.port} value={localPort} onChange={(e) => setLocalPort(e.target.value)} /></div>}
          </div>
        )}

        {preview.ssh && (
          <div className="guide-link">
            <span className="muted">{t("先运行")}</span>
            <span className="mono grow" style={{ overflowWrap: "anywhere" }}>{preview.ssh}</span>
            <button className="btn ghost icon sm" title={t("复制命令")} aria-label={t("复制命令")} onClick={() => copy(preview.ssh!, t("已复制"))}><Copy /></button>
          </div>
        )}
        <div className="guide-link">
          <span className="muted">{t("按钮打开")}</span>
          {preview.url
            ? <a className="mono grow" href={preview.url} target="_blank" rel="noopener noreferrer" style={{ overflowWrap: "anywhere" }}>{preview.url}<ExternalLink /></a>
            : <span className="grow muted">{t("这个应用只在主机本机（127.0.0.1）监听，从外面打不开：填写完整地址或者经由 SSH 隧道。")}</span>}
        </div>
        {!owner && <div className="hint">{t("只有所有者可以修改面板地址。")}</div>}
      </div>
    </Modal>
  );
}
