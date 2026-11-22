// 文件同步：把文件分发到若干台主机，或者把若干台主机上匹配规则的文件收集下来。
// 这一页没有自己的执行通道：分发的每个文件是一个请求（判级、审批、审计照常）；
// 收集先用只读查询列出文件，下载时每个文件走“下载文件”的读路径，打成一个 zip。
import { useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Download, FileUp, FolderSearch, Search, TriangleAlert, Upload, X } from "lucide-react";
import { api, type CollectHost, type Host } from "../api";
import { fail, toast, useStore } from "../store";
import { Empty, bytes } from "../ui";
import { ts, useT } from "../i18n";

/** 选主机：全部、按标签筛、逐台勾。 */
function HostPicker({ picked, onChange }: { picked: string[]; onChange: (v: string[]) => void }) {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const [query, setQuery] = useState("");
  const shown = hosts.filter((h) => !query.trim() || [h.name, h.addr, ...h.tags].some((x) => x.toLowerCase().includes(query.trim().toLowerCase())));
  const toggle = (n: string) => onChange(picked.includes(n) ? picked.filter((x) => x !== n) : [...picked, n]);
  const online = (h: Host) => h.status.online || !h.status.known;
  return (
    <div className="field">
      <div className="row wrap" style={{ gap: 6 }}>
        <label>{t("主机")}</label>
        <span className="muted" style={{ fontSize: 12.5 }}>{t("已选 {n} 台", { n: picked.length })}</span>
        <span className="grow" />
        <div className="search" style={{ width: 200, maxWidth: "100%" }}>
          <Search />
          <input className="input" placeholder={t("按名称、地址、标签筛选")} aria-label={t("按名称、地址、标签筛选")} value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
        <button className="btn sm" onClick={() => onChange([...new Set([...picked, ...shown.filter(online).map((h) => h.name)])])}>{query.trim() ? t("选中筛出来的") : t("全部在线主机")}</button>
        <button className="btn sm" onClick={() => onChange([])} disabled={picked.length === 0}>{t("清空")}</button>
      </div>
      <div className="app-hosts">
        {shown.map((h) => (
          <label key={h.name} className={"app-host" + (picked.includes(h.name) ? " on" : "")}>
            <input type="checkbox" checked={picked.includes(h.name)} onChange={() => toggle(h.name)} />
            <span className="mono ellipsis">{h.name}</span>
            {!online(h) && <span className="badge neutral">{t("离线")}</span>}
          </label>
        ))}
      </div>
    </div>
  );
}

interface Picked { file: File; progress: number; error?: string }

function Distribute() {
  const t = useT();
  const nav = useNavigate();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [hosts, setHosts] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [files, setFiles] = useState<Picked[]>([]);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  const add = (list: FileList | null) => {
    if (!list) return;
    setFiles((cur) => {
      const names = new Set(cur.map((x) => x.file.name));
      const next = [...cur];
      for (const f of Array.from(list)) if (!names.has(f.name)) { names.add(f.name); next.push({ file: f, progress: 0 }); }
      return next.slice(0, 20);
    });
  };
  const dirOK = dir.trim().startsWith("/");
  const ready = hosts.length > 0 && dirOK && files.length > 0 && !busy;

  const submit = async () => {
    setBusy(true);
    try {
      const staged: { name: string; upload: string }[] = [];
      for (let i = 0; i < files.length; i++) {
        const info = await api.upload(files[i].file, (p) => setFiles((cur) => cur.map((x, j) => (j === i ? { ...x, progress: p } : x))));
        staged.push({ name: files[i].file.name, upload: info.id });
      }
      const r = await api.fileDistribute({ hosts, dir: dir.trim(), files: staged });
      if (r.error) toast(ts(r.error), "err");
      const reqs = r.requests ?? [];
      if (reqs.length === 1) nav(`/requests/${reqs[0].id}`);
      else if (reqs.length > 1) { toast(t("已提交 {n} 个请求", { n: reqs.length })); nav("/approvals"); }
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  if (!owner) return <div className="card"><Empty icon={<FileUp />} title={t("只有所有者能上传文件")} /></div>;
  return (
    <div className="card">
      <div className="card-head"><h2>{t("分发到主机")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("每个文件是一个请求，批准后写到所选主机的同一个目录下")}</span></div>
      <div className="card-body col" style={{ gap: 16 }}>
        <div className={"dropzone" + (dragging ? " over" : "")}
          onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragging(true); } }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => { e.preventDefault(); setDragging(false); add(e.dataTransfer.files); }}
          onClick={() => input.current?.click()} role="button" tabIndex={0}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); input.current?.click(); } }}>
          <Upload />
          <b>{t("把文件拖到这里，或者点一下选择")}</b>
          <span className="muted">{t("一次最多 20 个文件；同名文件会被覆盖")}</span>
          <input ref={input} type="file" multiple hidden onChange={(e) => { add(e.target.files); e.target.value = ""; }} />
        </div>
        {files.length > 0 && (
          <div className="sync-files">
            {files.map((f, i) => (
              <div key={f.file.name} className="sync-file">
                <span className="mono ellipsis grow" title={f.file.name}>{f.file.name}</span>
                <span className="muted num">{bytes(f.file.size)}</span>
                {busy && <span className="muted num" style={{ width: 44, textAlign: "right" }}>{Math.round(f.progress * 100)}%</span>}
                <button className="btn ghost icon sm" disabled={busy} title={t("移除")} aria-label={t("移除")} onClick={() => setFiles((cur) => cur.filter((_, j) => j !== i))}><X /></button>
              </div>
            ))}
          </div>
        )}
        <div className="field">
          <label htmlFor="sync-dir">{t("目标目录")}</label>
          <input id="sync-dir" className="input mono" placeholder="/opt/app/config" value={dir} onChange={(e) => setDir(e.target.value)} />
          <span className="hint">{t("绝对路径；目录不存在会自动创建。写系统目录需要登录账号有权限，敏感路径会被判成高危。")}</span>
        </div>
        <HostPicker picked={hosts} onChange={setHosts} />
        <div className="row">
          <span className="muted" style={{ fontSize: 12.5 }}>{ready ? t("{f} 个文件 → {h} 台主机的 {dir}", { f: files.length, h: hosts.length, dir: dir.trim() }) : t("选好文件、目标目录和主机后提交")}</span>
          <span className="grow" />
          <button className="btn primary" disabled={!ready} onClick={submit}><FileUp />{busy ? t("上传中…") : t("提交分发")}</button>
        </div>
      </div>
    </div>
  );
}

function Collect() {
  const t = useT();
  const [hosts, setHosts] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [pattern, setPattern] = useState("*.log");
  const [depth, setDepth] = useState(1);
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<{ hosts: CollectHost[]; limit: number; query: string } | null>(null);

  const query = useMemo(() => new URLSearchParams({ hosts: hosts.join(","), dir: dir.trim(), pattern: pattern.trim(), depth: String(depth) }).toString(), [hosts, dir, pattern, depth]);
  const ready = hosts.length > 0 && dir.trim().startsWith("/") && pattern.trim() !== "" && !busy;
  const find = () => {
    setBusy(true);
    api.fileCollect(query).then((r) => setRes({ ...r, query })).catch(fail).finally(() => setBusy(false));
  };
  const all = res?.hosts.flatMap((h) => h.files) ?? [];
  const total = all.reduce((n, f) => n + Math.max(0, f.size), 0);
  const stale = res && res.query !== query;

  return (
    <div className="card">
      <div className="card-head"><h2>{t("从主机收集")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("按文件名规则在各台主机上找文件，打成一个压缩包下载到本地")}</span></div>
      <div className="card-body col" style={{ gap: 16 }}>
        <div className="form-grid">
          <div className="field">
            <label htmlFor="col-dir">{t("目录")}</label>
            <input id="col-dir" className="input mono" placeholder="/var/log" value={dir} onChange={(e) => setDir(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="col-pat">{t("文件名规则")}</label>
            <input id="col-pat" className="input mono" placeholder="*.log" value={pattern} onChange={(e) => setPattern(e.target.value)} />
            <span className="hint">{t("* 代表任意一串字符，? 代表一个字符，例如 *.log、app-2026-??.json")}</span>
          </div>
          <div className="field">
            <label htmlFor="col-depth">{t("往下找几层")}</label>
            <select id="col-depth" className="input" value={depth} onChange={(e) => setDepth(Number(e.target.value))}>
              <option value={1}>{t("只找这个目录")}</option>
              {[2, 3, 4, 6, 8].map((n) => <option key={n} value={n}>{t("往下 {n} 层", { n })}</option>)}
            </select>
          </div>
        </div>
        <HostPicker picked={hosts} onChange={setHosts} />
        <div className="row">
          <span className="muted" style={{ fontSize: 12.5 }}>{t("查找是只读查询；下载时每个文件都会记一条审计，私钥和凭据文件读不了。")}</span>
          <span className="grow" />
          <button className="btn" disabled={!ready} onClick={find}><FolderSearch />{busy ? t("查找中…") : t("查找")}</button>
        </div>
        {res && (
          <>
            {stale && <div className="note warn"><TriangleAlert /><span>{t("条件改过了，下面是上一次查找的结果；重新查找后再下载。")}</span></div>}
            <div className="row wrap">
              <b>{t("找到 {n} 个文件", { n: all.length })}</b>
              {total > 0 && <span className="muted num">{bytes(total)}</span>}
              <span className="grow" />
              {all.length > 0 && !stale
                ? <a className="btn primary" href={`/api/v1/files/collect/download?${res.query}`} download><Download />{t("下载压缩包")}</a>
                : <button className="btn primary" disabled><Download />{t("下载压缩包")}</button>}
            </div>
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("主机")}</th><th>{t("文件")}</th><th style={{ textAlign: "right" }}>{t("大小")}</th></tr></thead>
                <tbody>
                  {res.hosts.flatMap((h) => h.error
                    ? [<tr key={h.host}><td className="mono">{h.host}</td><td colSpan={2} className="crit">{ts(h.error)}</td></tr>]
                    : h.files.length === 0
                      ? [<tr key={h.host}><td className="mono">{h.host}</td><td colSpan={2} className="muted">{t("没有匹配的文件")}</td></tr>]
                      : [...h.files.map((f, i) => (
                          <tr key={h.host + f.path}>
                            <td className="mono">{i === 0 ? h.host : ""}</td>
                            <td className="mono" style={{ overflowWrap: "anywhere" }}>{f.path}</td>
                            <td className="num muted" style={{ textAlign: "right" }}>{f.size >= 0 ? bytes(f.size) : "—"}</td>
                          </tr>
                        )),
                        ...(h.truncated ? [<tr key={h.host + "-more"}><td /><td colSpan={2} className="muted">{t("匹配的文件超过 {n} 个，只列出和收集前面这些；把规则写细一点。", { n: res.limit })}</td></tr>] : [])])}
                </tbody>
              </table>
            </div>
            <div className="hint">{t("压缩包里按“主机名/文件路径”存放；读不了或超过 100 MB 的文件会记在包里的“_跳过的文件.txt”里。")}</div>
          </>
        )}
      </div>
    </div>
  );
}

export function FileSyncPage() {
  const t = useT();
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") === "collect" ? "collect" : "distribute";
  return (
    <>
      <div className="topbar">
        <h1>{t("文件同步")}</h1>
        <div className="tabs" role="tablist">
          <button role="tab" aria-selected={tab === "distribute"} className={"tab" + (tab === "distribute" ? " active" : "")} onClick={() => setSp({})}>{t("分发到主机")}</button>
          <button role="tab" aria-selected={tab === "collect"} className={"tab" + (tab === "collect" ? " active" : "")} onClick={() => setSp({ tab: "collect" })}>{t("从主机收集")}</button>
        </div>
      </div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("把同一批文件发到多台主机，或者把多台主机上的文件收到本地。分发会变成请求：先看到每台主机上要写到哪里和风险等级，批准后才写。")}</p>
          {tab === "distribute" ? <Distribute /> : <Collect />}
        </div>
      </div>
    </>
  );
}
