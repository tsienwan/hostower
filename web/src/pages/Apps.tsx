// 应用市场。这一页没有自己的执行通道：目录是内置的清单，“已安装”是只读查询，
// 安装、更新、启停、卸载都生成请求，走和手敲命令相同的判级与审批。批量操作是一个请求、一次审批。
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import {
  Activity, ArrowUpCircle, Bot, Box, Check, Database, Dices, Download, Eye, Network, Package, Play, Plus, RefreshCw, RotateCw, Search,
  ExternalLink, Globe, KeyRound, Link2, Rocket, Settings2, Shield, ShieldAlert, Sparkles, Square, SquareTerminal, Trash2, TriangleAlert,
} from "lucide-react";
import { api, type AIConfig, type AppAutoStatus, type AppActionName, type AppInstallTarget, type AppInstalled, type AppItem, type AppParam, type AppPlanResult, type HostApps, type Preview } from "../api";
import { backAfterApproval, fail, toast, useRequestFinished, useStore } from "../store";
import { Empty, LevelBadge, Modal, ago } from "../ui";
import { Select } from "../controls";
import { PreviewPanel } from "../shared";
import { DockerSetup } from "../DockerSetup";
import { cached, remember } from "../cache";
import { ts, useT } from "../i18n";
import { AppGuideDialog, guideUrl } from "./AppGuide";
import { AppPanelDialog, panelEntry, panelLink, siteUrls, usePanelSites } from "./AppPanel";
import { EnvMatrix } from "./HostEnv";

const icons: Record<string, typeof Bot> = { bot: Bot, sparkles: Sparkles, activity: Activity, database: Database, network: Network, box: Box, shield: Shield, terminal: SquareTerminal };

/** 官方图标放在白底的方块里（深色模式下也能看清深色的品牌色）；没有就用通用图标。 */
function AppIcon({ app, size = 40 }: { app?: AppItem; size?: number }) {
  const Ico = icons[app?.icon ?? ""] ?? Package;
  return (
    <span className={"app-icon" + (app?.logo ? " logo" : "")} style={{ width: size, height: size }} aria-hidden="true">
      {app?.logo ? <img src={app.logo} alt="" /> : <Ico />}
    </span>
  );
}

/** 清单里的说明可以引用参数：{{ .port }}。 */
function fill(text: string, values: Record<string, string>) {
  return text.replace(/\{\{\s*\.([a-z0-9_]+)\s*\}\}/g, (m, k) => values[k] ?? m);
}

const typeLabel = { compose: "容器应用", npm: "命令行工具" } as const;

export function AppsPage() {
  const t = useT();
  const [sp, setSp] = useSearchParams();
  // “已安装”最常用，排在最前、默认打开；目录和环境用 ?tab= 指定。
  const tab = sp.get("tab") === "catalog" ? "catalog" : sp.get("tab") === "env" ? "env" : "installed";
  const [catalog, setCatalog] = useState<AppItem[] | null>(() => cached<AppItem[]>("apps:catalog") ?? null);
  const [categories, setCategories] = useState<string[]>([]);
  const [installing, setInstalling] = useState<AppItem | null>(null);
  const [installHost, setInstallHost] = useState("");

  useEffect(() => {
    api.apps().then((r) => { setCatalog(remember("apps:catalog", r.apps)); setCategories(r.categories); }).catch(fail);
  }, []);
  const byName = useMemo(() => new Map((catalog ?? []).map((a) => [a.name, a])), [catalog]);
  // 从主机的“环境”页点“安装”过来（?install=应用&host=主机）：直接打开安装框并选好那台主机。
  const wantInstall = sp.get("install");
  useEffect(() => {
    const a = wantInstall ? byName.get(wantInstall) : undefined;
    if (!a) return;
    setInstallHost(sp.get("host") ?? "");
    setInstalling(a);
    setSp({ tab: "catalog" }, { replace: true });
  }, [wantInstall, byName]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <>
      <div className="topbar">
        <h1>{t("应用")}</h1>
        <div className="tabs" role="tablist">
          <button role="tab" aria-selected={tab === "installed"} className={"tab" + (tab === "installed" ? " active" : "")} onClick={() => setSp({})}>{t("已安装")}</button>
          <button role="tab" aria-selected={tab === "catalog"} className={"tab" + (tab === "catalog" ? " active" : "")} onClick={() => setSp({ tab: "catalog" })}>{t("应用目录")}</button>
          <button role="tab" aria-selected={tab === "env"} className={"tab" + (tab === "env" ? " active" : "")} onClick={() => setSp({ tab: "env" })}>{t("环境")}</button>
        </div>
      </div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("安装、更新、启停和卸载都会变成请求：先看到每台主机上实际要执行的命令、要写进主机的 compose 文件和风险等级，批准后才执行。应用要的权限由判级引擎看实际内容决定，不由清单自己说了算。")}</p>
          {tab === "catalog"
            ? <Catalog apps={catalog} categories={categories} onInstall={(a) => { setInstallHost(""); setInstalling(a); }} />
            : tab === "env" ? <EnvMatrix />
            : <Installed byName={byName} />}
        </div>
      </div>
      {installing && <InstallDialog app={installing} initialHost={installHost} onClose={() => setInstalling(null)} />}
    </>
  );
}

// ---------- 目录 ----------

function Catalog({ apps, categories, onInstall }: { apps: AppItem[] | null; categories: string[]; onInstall: (a: AppItem) => void }) {
  const t = useT();
  const [cat, setCat] = useState("");
  const [query, setQuery] = useState("");
  const shown = (apps ?? []).filter((a) => (!cat || a.category === cat) &&
    (!query.trim() || [a.name, a.title, ts(a.summary)].some((x) => x.toLowerCase().includes(query.trim().toLowerCase()))));
  return (
    <>
      <div className="row wrap" style={{ gap: 8 }}>
        <div className="row wrap" role="group" aria-label={t("按分类筛选")} style={{ gap: 4 }}>
          <button className={"tag" + (cat === "" ? " on" : "")} onClick={() => setCat("")}>{t("全部")}</button>
          {categories.map((c) => <button key={c} className={"tag" + (cat === c ? " on" : "")} onClick={() => setCat(c)}>{ts(c)}</button>)}
        </div>
        <span className="grow" />
        <div className="search" style={{ width: 240, maxWidth: "100%" }}>
          <Search />
          <input className="input" placeholder={t("搜索应用")} aria-label={t("搜索应用")} value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
      </div>
      {!apps ? <div className="card"><div className="empty">{t("加载中…")}</div></div>
        : shown.length === 0 ? <div className="card"><Empty icon={<Search />} title={t("没有匹配的应用")} /></div>
        : (
          <div className="app-grid">
            {shown.map((a) => (
              <div key={a.name} className="card app-card">
                <div className="row" style={{ gap: 12, alignItems: "flex-start" }}>
                  <AppIcon app={a} />
                  <div className="grow" style={{ minWidth: 0 }}>
                    <div className="app-title">{a.title}</div>
                    <div className="row wrap" style={{ gap: 4, marginTop: 4 }}>
                      <span className="badge neutral">{ts(a.category)}</span>
                      <span className="badge neutral">{t(typeLabel[a.type])}</span>
                      {a.preview && <span className="badge warn">{t("预览版")}</span>}
                    </div>
                  </div>
                </div>
                <div className="app-summary">{ts(a.summary)}</div>
                {a.permissions.length > 0 && (
                  <div className="app-perm"><ShieldAlert /><span>{t("需要宿主机级权限，安装是高危操作（L2）：")}{[...new Set(a.permissions.map((p) => ts(p.reason)))].join(t("；"))}</span></div>
                )}
                <div className="row app-foot">
                  <span className="muted mono" style={{ fontSize: 12 }}>{a.npm ? a.npm.package : ""} {a.npm?.version ?? a.version}</span>
                  <span className="grow" />
                  {a.homepage && <a className="btn ghost icon sm" href={a.homepage} target="_blank" rel="noopener noreferrer" title={t("官方文档")} aria-label={t("官方文档")}><ExternalLink /></a>}
                  <button className="btn primary sm" onClick={() => onInstall(a)}><Download />{t("安装")}</button>
                </div>
              </div>
            ))}
          </div>
        )}
    </>
  );
}

// ---------- 安装 ----------

/** 随机生成一个只含字母和数字的密码（在浏览器里生成，保存前你能看到它）。 */
function randomSecret(n = 24) {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789";
  const buf = new Uint32Array(n);
  crypto.getRandomValues(buf);
  return Array.from(buf, (x) => chars[x % chars.length]).join("");
}

interface Row { host: string; instance: string; params: Record<string, string>; secrets: Record<string, string> }

function InstallDialog({ app, initialHost, onClose }: { app: AppItem; initialHost?: string; onClose: () => void }) {
  const t = useT();
  const nav = useNavigate();
  const hosts = useStore((s) => s.hosts);
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const params = app.params ?? [];
  const defaults = Object.fromEntries(params.filter((p) => p.type !== "secret").map((p) => [p.name, p.default ?? ""]));
  const [picked, setPicked] = useState<string[]>(() => (initialHost && hosts.some((h) => h.name === initialHost) ? [initialHost] : []));
  const [hostQuery, setHostQuery] = useState("");
  const [common, setCommon] = useState<Row>({ host: "", instance: app.name, params: defaults, secrets: {} });
  const [each, setEach] = useState(false);
  const [rows, setRows] = useState<Record<string, Row>>({});
  const [version, setVersion] = useState(app.npm?.version ?? "");
  const [known, setKnown] = useState<string[]>([]);
  const [preview, setPreview] = useState<AppPlanResult | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [newSecret, setNewSecret] = useState<{ param: string; name: string; value: string } | null>(null);
  const [withAI, setWithAI] = useState(false);
  const [ai, setAI] = useState<AIConfig>({ provider: app.ai?.providers[0]?.id ?? "" });
  // 容器应用：每台选中的主机有没有 Docker 和 Compose（null 表示还在检查）。缺的时候不能提交。
  const [docker, setDocker] = useState<Record<string, boolean | null>>({});
  const noDocker = app.type === "compose" && picked.some((h) => docker[h] === false);

  const loadSecrets = useCallback(() => api.appSecrets().then((r) => setKnown(r.secrets.map((x) => x.name))).catch(() => {}), []);
  useEffect(() => { loadSecrets(); }, [loadSecrets]);

  const rowOf = (h: string): Row => (each && rows[h]) || { ...common, host: h };
  const targets: AppInstallTarget[] = picked.map((h) => {
    const r = rowOf(h);
    return { host: h, instance: app.type === "npm" ? app.name : r.instance.trim(), params: r.params, secrets: r.secrets, version: app.type === "npm" ? version.trim() : undefined, ai: withAI ? ai : undefined };
  });
  const body = { app: app.name, targets };
  const key = JSON.stringify(body);
  // 内容变了，之前的预览就作废。
  useEffect(() => { setPreview(null); setError(""); }, [key]);

  const toggle = (h: string) => setPicked((p) => (p.includes(h) ? p.filter((x) => x !== h) : [...p, h]));
  const setEachOn = (on: boolean) => {
    if (on) setRows(Object.fromEntries(picked.map((h) => [h, rows[h] ?? { ...common, host: h, params: { ...common.params }, secrets: { ...common.secrets } }])));
    setEach(on);
  };
  const updateRow = (h: string, patch: (r: Row) => Row) => setRows((rs) => ({ ...rs, [h]: patch(rs[h] ?? { ...common, host: h }) }));

  const doPreview = async () => {
    setBusy(true); setError("");
    try { setPreview(await api.appInstall({ ...body, preview: true })); } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.appInstall(body);
      const reqs = Array.isArray(r.requests) ? r.requests : [];
      if (r.error) toast(ts(r.error), "err");
      if (reqs.length === 1) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
      else { toast(t("已提交 {n} 个请求", { n: reqs.length })); nav("/approvals", { state: backAfterApproval }); }
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const saveSecret = async () => {
    if (!newSecret) return;
    try {
      await api.putAppSecret(newSecret.name, newSecret.value);
      await loadSecrets();
      setCommon((c) => ({ ...c, secrets: { ...c.secrets, [newSecret.param]: newSecret.name } }));
      setRows((rs) => Object.fromEntries(Object.entries(rs).map(([h, r]) => [h, { ...r, secrets: { ...r.secrets, [newSecret.param]: r.secrets[newSecret.param] || newSecret.name } }])));
      setNewSecret(null);
      toast(t("已保存应用密钥"));
    } catch (e) { fail(e); }
  };

  const shownHosts = hosts.filter((h) => !hostQuery.trim() || h.name.toLowerCase().includes(hostQuery.trim().toLowerCase()) || h.tags.some((x) => x.includes(hostQuery.trim())));
  const pv: Preview | null = preview?.targets ? { level: preview.level!, targets: preview.targets } : null;
  const groups = typeof preview?.requests === "number" ? preview.requests : 0;
  const plain = params.filter((p) => p.type !== "secret");
  const secretParams = params.filter((p) => p.type === "secret");

  const paramInput = (p: AppParam, value: string, onChange: (v: string) => void, id: string) =>
    p.type === "enum"
      ? <Select id={id} mono value={value} onChange={onChange} options={(p.options ?? []).map((o) => ({ value: o, label: o }))} />
      : <input id={id} className="input mono" value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} inputMode={p.type === "port" ? "numeric" : undefined} />;
  const secretInput = (_p: AppParam, value: string, onChange: (v: string) => void, id: string) =>
    <Select id={id} mono value={value} placeholder={t("选择密钥")} onChange={onChange} options={known.map((k) => ({ value: k, label: k }))} />;

  return (
    <Modal wide="xl" title={t("安装 {app}", { app: app.title })} onClose={onClose}
      footer={<>
        <button className="btn" onClick={onClose}>{t("取消")}</button>
        <button className="btn" onClick={doPreview} disabled={busy || picked.length === 0}><Eye />{t("预览")}</button>
        <button className="btn primary" onClick={submit} disabled={busy || !preview?.targets || noDocker} title={noDocker ? t("有主机还没装 Docker") : undefined}>{t("提交")}</button>
      </>}>
      <div className="row" style={{ gap: 12, alignItems: "flex-start" }}>
        <AppIcon app={app} size={44} />
        <div className="grow" style={{ minWidth: 0 }}>
          <div className="dim">{ts(app.description || app.summary)}</div>
          {app.notes && <div className="hint" style={{ marginTop: 4 }}>{ts(fill(app.notes, common.params))}</div>}
        </div>
      </div>
      {app.permissions.length > 0 && (
        <div className="note crit"><ShieldAlert /><span>{t("这个应用需要宿主机级权限：")}{[...new Set(app.permissions.map((p) => ts(p.reason)))].join(t("；"))}{t("。批准时要重新验证身份并手动输入确认内容。")}</span></div>
      )}

      <div className="field">
        <label>{t("安装到哪些主机")}<span className="muted num" style={{ marginLeft: 6 }}>{picked.length > 0 ? t("已选 {n} 台", { n: picked.length }) : ""}</span></label>
        <div className="row" style={{ gap: 6 }}>
          <div className="search grow"><Search /><input className="input" placeholder={t("按主机名或标签筛选")} aria-label={t("按主机名或标签筛选")} value={hostQuery} onChange={(e) => setHostQuery(e.target.value)} /></div>
          <button className="btn sm" onClick={() => setPicked([...new Set([...picked, ...shownHosts.map((h) => h.name)])])}>{t("全选")}</button>
          <button className="btn sm" onClick={() => setPicked([])} disabled={picked.length === 0}>{t("清空")}</button>
        </div>
        <div className="app-hosts">
          {shownHosts.map((h) => (
            <label key={h.name} className={"app-host" + (picked.includes(h.name) ? " on" : "")}>
              <input type="checkbox" checked={picked.includes(h.name)} onChange={() => toggle(h.name)} />
              <span className="mono ellipsis">{h.name}</span>
              {!h.status.online && h.status.known && <span className="badge neutral">{t("离线")}</span>}
              {app.type === "compose" && h.tags.includes("docker") && <span className="badge neutral">docker</span>}
            </label>
          ))}
        </div>
      </div>

      {app.type === "npm" ? (
        <div className="form-grid">
          <div className="field"><label htmlFor="app-ver">{t("版本")}</label>
            <input id="app-ver" className="input mono" value={version} onChange={(e) => setVersion(e.target.value)} spellCheck={false} />
            <span className="hint">{t("默认是目录里验证过的版本；填 latest 装 npm 上最新的版本。需要 Node.js {n} 以上，系统里没有时会自动装一份校验过的 Node.js。", { n: app.npm?.node ?? 18 })}</span>
          </div>
          <div className="field"><label>{t("装到哪里")}</label>
            <div className="dim" style={{ fontSize: 13 }}>{t("登录账号自己的目录，不用 sudo：")}<span className="mono">~/.hostower/npm</span>{t("，命令放在 ")}<span className="mono">~/.local/bin/{app.npm?.bins.join(", ")}</span></div>
          </div>
        </div>
      ) : null}
      {app.type === "npm" ? (app.ai && (
        <>
          <label className="check"><input type="checkbox" checked={withAI} onChange={(e) => setWithAI(e.target.checked)} />{t("装好后直接配置（API Key、模型供应商），开箱即用")}</label>
          {withAI && <AIConfigForm ai={app.ai} value={ai} onChange={setAI} known={known} onKnown={setKnown} />}
          {!withAI && app.ai.login && <span className="hint">{t("也可以装好后在“已安装”里配置，或者用订阅登录。")}</span>}
        </>)
      ) : (
        <>
          <div className="row between">
            <div className="section-title">{t("参数")}</div>
            {picked.length > 1 && <label className="check"><input type="checkbox" checked={each} onChange={(e) => setEachOn(e.target.checked)} />{t("每台主机分别设置")}</label>}
          </div>
          {!each || picked.length < 2 ? (
            <div className="form-grid">
              <div className="field"><label htmlFor="app-inst">{t("实例名")}</label>
                <input id="app-inst" className="input mono" value={common.instance} onChange={(e) => setCommon({ ...common, instance: e.target.value })} spellCheck={false} />
                <span className="hint">{t("也是主机上的目录名 ~/.hostower/apps/<实例名> 和 compose 项目名。同一台主机上装第二份时换个名字。")}</span>
              </div>
              {plain.map((p) => (
                <div key={p.name} className="field"><label htmlFor={"app-p-" + p.name}>{ts(p.title)}</label>
                  {paramInput(p, common.params[p.name] ?? "", (v) => setCommon({ ...common, params: { ...common.params, [p.name]: v } }), "app-p-" + p.name)}
                  {p.hint && <span className="hint">{ts(p.hint)}</span>}
                </div>
              ))}
              {secretParams.map((p) => (
                <div key={p.name} className="field"><label htmlFor={"app-s-" + p.name}>{ts(p.title)}</label>
                  <div className="row" style={{ gap: 6 }}>
                    <div className="grow" style={{ minWidth: 0 }}>{secretInput(p, common.secrets[p.name] ?? "", (v) => setCommon({ ...common, secrets: { ...common.secrets, [p.name]: v } }), "app-s-" + p.name)}</div>
                    {owner && <button className="btn sm" onClick={() => setNewSecret({ param: p.name, name: `${common.instance || app.name}-${p.name}`.replace(/[^a-z0-9._-]/g, "-"), value: randomSecret() })}><Plus />{t("新建")}</button>}
                  </div>
                  <span className="hint">{t("值不会出现在命令和审计里：执行时由塔台写进主机上只有登录账号可读的 .env 文件。")}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="table-wrap">
              <table className="table app-each">
                <thead><tr><th>{t("主机")}</th><th>{t("实例名")}</th>{plain.map((p) => <th key={p.name}>{ts(p.title)}</th>)}{secretParams.map((p) => <th key={p.name}>{ts(p.title)}</th>)}</tr></thead>
                <tbody>
                  {picked.map((h) => {
                    const r = rowOf(h);
                    return (
                      <tr key={h}>
                        <td className="mono">{h}</td>
                        <td><input className="input mono" aria-label={t("实例名")} value={r.instance} onChange={(e) => updateRow(h, (x) => ({ ...x, instance: e.target.value }))} spellCheck={false} /></td>
                        {plain.map((p) => <td key={p.name}>{paramInput(p, r.params[p.name] ?? "", (v) => updateRow(h, (x) => ({ ...x, params: { ...x.params, [p.name]: v } })), `app-${h}-${p.name}`)}</td>)}
                        {secretParams.map((p) => <td key={p.name}>{secretInput(p, r.secrets[p.name] ?? "", (v) => updateRow(h, (x) => ({ ...x, secrets: { ...x.secrets, [p.name]: v } })), `app-${h}-s-${p.name}`)}</td>)}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          {secretParams.length > 0 && known.length === 0 && !owner && (
            <div className="note"><TriangleAlert /><span>{t("还没有应用密钥，请所有者先到 ")}<Link to="/credentials" style={{ color: "var(--accent)" }}>{t("凭据")}</Link>{t(" 页添加。")}</span></div>
          )}
        </>
      )}

      {newSecret && (
        <div className="card app-secret">
          <div className="section-title">{t("新建应用密钥")}</div>
          <div className="form-grid">
            <div className="field"><label htmlFor="ns-name">{t("名称")}</label><input id="ns-name" className="input mono" value={newSecret.name} onChange={(e) => setNewSecret({ ...newSecret, name: e.target.value })} spellCheck={false} /></div>
            <div className="field"><label htmlFor="ns-val">{t("值")}</label>
              <div className="row" style={{ gap: 6 }}>
                <input id="ns-val" className="input mono grow" value={newSecret.value} onChange={(e) => setNewSecret({ ...newSecret, value: e.target.value })} spellCheck={false} autoComplete="off" />
                <button className="btn ghost icon sm" title={t("随机生成")} aria-label={t("随机生成")} onClick={() => setNewSecret({ ...newSecret, value: randomSecret() })}><Dices /></button>
              </div>
              <span className="hint">{t("保存后就再也读不出来了（只写不读）：需要的话现在把它记到密码管理器里。")}</span>
            </div>
          </div>
          <div className="row" style={{ justifyContent: "flex-end", gap: 6 }}>
            <button className="btn sm" onClick={() => setNewSecret(null)}>{t("取消")}</button>
            <button className="btn primary sm" onClick={saveSecret} disabled={!newSecret.name || !newSecret.value || newSecret.value.includes("'")}><Check />{t("保存")}</button>
          </div>
        </div>
      )}

      {app.type === "compose" && picked.map((h) => <DockerSetup key={h} host={h} onReady={(ok) => setDocker((d) => ({ ...d, [h]: ok }))} />)}
      <div className="section-title">{t("判级预览")}</div>
      {!preview && !error
        ? <div className="muted" style={{ fontSize: 12.5 }}>{picked.length === 0 ? t("先选择主机。") : t("点“预览”，看每台主机上实际要执行的命令和风险等级。预览没有任何副作用。")}</div>
        : <PreviewPanel preview={pv} error={error} />}
      {groups > 1 && <div className="note"><TriangleAlert /><span>{t("这几台主机选的密钥不一样，会拆成 {n} 个请求分别审批。", { n: groups })}</span></div>}
      {preview?.targets && (
        <details className="app-details">
          <summary>{t("每台主机上的命令和 compose 文件")}</summary>
          {preview.targets.map((x) => (
            <div key={x.host} style={{ marginTop: 10 }}>
              <div className="row" style={{ gap: 8, marginBottom: 4 }}><span className="mono">{x.host}</span><LevelBadge level={x.level} /></div>
              <pre className="code">{x.command}</pre>
            </div>
          ))}
        </details>
      )}
    </Modal>
  );
}

// ---------- 已安装 ----------

const actionLabel: Record<AppActionName, string> = { start: "启动", stop: "停止", restart: "重启", update: "更新", uninstall: "卸载", purge: "卸载并删除数据" };

function Installed({ byName }: { byName: Map<string, AppItem> }) {
  const t = useT();
  const nav = useNavigate();
  const hosts = useStore((s) => s.hosts);
  const [data, setData] = useState<HostApps[] | null>(() => cached<HostApps[]>("apps:installed") ?? null);
  const [loading, setLoading] = useState(false);
  const [sel, setSel] = useState<string[]>([]);
  const [confirm, setConfirm] = useState<{ action: AppActionName; keys: string[] } | null>(null);
  const [configuring, setConfiguring] = useState<{ host: string; instance: string } | null>(null);
  const [guiding, setGuiding] = useState<string | null>(null);
  const [paneling, setPaneling] = useState<string | null>(null);
  const [sp, setSp] = useSearchParams();
  const names = hosts.map((h) => h.name).join(",");

  const load = useCallback(() => {
    if (!names) return;
    setLoading(true);
    api.appsInstalledAll(names.split(",")).then((r) => setData(remember("apps:installed", r))).catch(fail).finally(() => setLoading(false));
  }, [names]);
  useEffect(() => { load(); }, [load]);
  // 批准后回到这一页：装好、更新完、卸载完就重新查一遍。
  useRequestFinished(load);

  const rows = (data ?? []).flatMap((h) => h.installed.map((x) => ({ ...x, host: h.host, key: h.host + "/" + x.instance })));
  const errors = (data ?? []).filter((h) => h.error);
  // 有面板、又没设过面板地址的实例：查它所在主机上的站点，有反向代理到应用端口的就让“打开面板”用站点地址。
  const sites = usePanelSites([...new Set(rows.filter((r) => panelEntry(r) && !r.panel).map((r) => r.host))]);
  const updates = rows.filter((r) => r.update || r.rebuilt);
  // 从请求详情页跳过来（?host=…&app=…）：数据到了以后直接打开那个实例的上手指南，只打开一次。
  const wantHost = sp.get("host"), wantApp = sp.get("app");
  useEffect(() => {
    if (!data || !wantHost || !wantApp) return;
    const r = rows.find((x) => x.host === wantHost && x.app === wantApp && x.guide);
    if (r) setGuiding(r.key);
    setSp({ tab: "installed" }, { replace: true });
  }, [data, wantHost, wantApp]); // eslint-disable-line react-hooks/exhaustive-deps
  const guided = rows.find((r) => r.key === guiding);
  const paneled = rows.find((r) => r.key === paneling && panelEntry(r));
  const toggle = (k: string) => setSel((s) => (s.includes(k) ? s.filter((x) => x !== k) : [...s, k]));

  const run = async (action: AppActionName, keys: string[]) => {
    const targets = keys.map((k) => { const [host, ...rest] = k.split("/"); return { host, instance: rest.join("/") }; });
    try {
      const r = await api.appAction({ action, targets });
      const reqs = Array.isArray(r.requests) ? r.requests : [];
      setSel([]);
      if (reqs.length === 1) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
      else { toast(t("已提交 {n} 个请求", { n: reqs.length })); nav("/approvals", { state: backAfterApproval }); }
    } catch (e) { fail(e); }
  };
  const ask = (action: AppActionName, keys: string[]) => (action === "uninstall" || action === "purge" ? setConfirm({ action, keys }) : run(action, keys));
  const selRows = rows.filter((r) => sel.includes(r.key));
  const selCompose = selRows.every((r) => r.type === "compose");

  return (
    <>
      <AutoUpdateCard onChecked={load} />
      {updates.length > 0 && (
        <div className="note"><ArrowUpCircle /><span>{t("{n} 个实例有新版本。", { n: updates.length })}
          {[...new Set(updates.filter((r) => r.update).map((r) => byName.get(r.app)?.upgrade?.note).filter(Boolean))].map((n) => <span key={n} className="hint" style={{ display: "block", marginTop: 4 }}>{ts(n!)}</span>)}</span>
          <button className="btn sm" style={{ marginLeft: "auto" }} onClick={() => run("update", updates.map((r) => r.key))}>{t("全部更新")}</button></div>
      )}
      {rows.filter((r) => r.blocked).map((r) => (
        <div key={r.key + "-blocked"} className="note warn"><TriangleAlert /><span>
          <b>{byName.get(r.app)?.title ?? r.app} · {r.instance} · {r.host}</b>{t("：有新大版本 {v}，但不能原地升级。", { v: r.blocked! })}{byName.get(r.app)?.upgrade?.note ? " " + ts(byName.get(r.app)!.upgrade!.note!) : ""}
        </span></div>
      ))}
      {errors.map((h) => <div key={h.host} className="note crit"><TriangleAlert /><span><span className="mono">{h.host}</span>{t("：")}{ts(h.error)}</span></div>)}
      <div className="card">
        <div className="card-head wrap">
          <h2>{t("已安装")}</h2>
          <span className="muted num">{rows.length}</span>
          <button className="btn ghost icon sm" onClick={load} disabled={loading} title={t("刷新")} aria-label={t("刷新")}><RefreshCw className={loading ? "spin" : ""} /></button>
          <span className="grow" />
          {sel.length > 0 && (
            <div className="row wrap" style={{ gap: 6 }}>
              <span className="muted">{t("已选 {n} 个", { n: sel.length })}</span>
              <button className="btn sm" onClick={() => run("update", sel)} disabled={!selRows.every((r) => r.known)}><ArrowUpCircle />{t("更新")}</button>
              {selCompose && <button className="btn sm" onClick={() => run("start", sel)}><Play />{t("启动")}</button>}
              {selCompose && <button className="btn sm" onClick={() => run("stop", sel)}><Square />{t("停止")}</button>}
              <button className="btn sm" onClick={() => ask("uninstall", sel)}><Trash2 />{t("卸载")}</button>
            </div>
          )}
        </div>
        {!data ? <div className="empty">{loading ? t("加载中…") : t("还没有主机")}</div>
          : rows.length === 0 ? <Empty icon={<Package />} title={t("还没有通过应用市场安装的应用")}><button className="btn primary sm" onClick={() => setSp({ tab: "catalog" })}><Search />{t("应用目录")}</button></Empty>
          : (
            <div className="table-wrap">
              <table className="table app-table">
                <thead><tr>
                  <th style={{ width: 32 }}><input type="checkbox" aria-label={t("全选")} checked={sel.length === rows.length} onChange={(e) => setSel(e.target.checked ? rows.map((r) => r.key) : [])} /></th>
                  <th>{t("应用")}</th><th>{t("主机")}</th><th>{t("版本")}</th><th>{t("状态")}</th><th className="act">{t("操作")}</th>
                </tr></thead>
                <tbody>
                  {rows.map((r) => {
                    const app = byName.get(r.app);
                    const running = r.containers.filter((c) => c.state === "running").length;
                    return (
                      <tr key={r.key}>
                        <td><input type="checkbox" aria-label={t("选择 {name}", { name: r.instance })} checked={sel.includes(r.key)} onChange={() => toggle(r.key)} /></td>
                        <td>
                          <div className="row" style={{ gap: 10 }}>
                            <AppIcon app={app} size={28} />
                            <div style={{ minWidth: 0 }}>
                              <div style={{ fontWeight: 560 }}>{app?.title ?? r.app}</div>
                              <div className="muted mono" style={{ fontSize: 12 }}>{r.instance}</div>
                            </div>
                          </div>
                        </td>
                        <td className="mono">{r.host}</td>
                        <td>
                          <span className="mono">{r.version}</span>
                          {r.update && <span className="badge accent" style={{ marginLeft: 6 }} title={app?.upgrade?.note ? ts(app.upgrade.note) : undefined}><ArrowUpCircle />{t("可更新到 {v}", { v: r.update })}</span>}
                          {r.rebuilt && <span className="badge accent" style={{ marginLeft: 6 }} title={t("同一个镜像标签在仓库里有了新的构建（通常是小版本或安全修复），更新会重新拉取并重建容器。")}><ArrowUpCircle />{t("镜像有新构建")}</span>}
                          {r.upstream && <span className="badge neutral" style={{ marginLeft: 6 }} title={t("镜像仓库里已经有这个大版本，但目录还没收录：要先确认能不能原地升级，Hostower 不会自动换过去。")}>{t("上游有新大版本 {v}", { v: r.upstream })}</span>}
                          {r.blocked && <span className="badge warn" style={{ marginLeft: 6 }} title={app?.upgrade?.note ? ts(app.upgrade.note) : undefined}><TriangleAlert />{t("新大版本 {v} 要手动迁移", { v: r.blocked })}</span>}
                        </td>
                        <td>
                          {r.type === "npm" ? <span className="badge neutral">{t("命令行工具")}</span>
                            : r.containers.length === 0 ? <span className="badge warn">{t("没有容器")}</span>
                            : <span className={"badge " + (running === r.containers.length ? "ok" : running === 0 ? "neutral" : "warn")}>
                                {running === r.containers.length ? t("运行中") : running === 0 ? t("已停止") : t("{a}/{b} 运行中", { a: running, b: r.containers.length })}</span>}
                          {r.containers[0]?.ports && <div className="muted mono" style={{ fontSize: 12, marginTop: 2 }}>{r.containers[0].ports}</div>}
                          {running > 0 && !panelEntry(r) && r.guide?.open.filter((o) => !o.local).slice(0, 1).map((o) => {
                            const url = guideUrl(hosts.find((h) => h.name === r.host), o);
                            return <a key={url} className="mono app-open" href={url} target="_blank" rel="noopener noreferrer"><ExternalLink />{t("打开")}</a>;
                          })}
                        </td>
                        <td className="act">
                          <div className="row act-row">
                            {(() => {
                              const pe = panelEntry(r);
                              if (!pe) return null;
                              const l = panelLink(hosts.find((h) => h.name === r.host), pe, r.panel, r.panel ? undefined : siteUrls(sites[r.host] ?? [], pe.port, pe.path)[0]);
                              if (!l.url) return <button className="btn sm" title={t("这个应用只在主机本机（127.0.0.1）监听：先设置面板地址（反向代理的域名或 SSH 隧道）")} onClick={() => setPaneling(r.key)}><ExternalLink />{t("打开面板")}</button>;
                              return <a className={"btn sm" + (running > 0 ? "" : " disabled")} href={l.url} target="_blank" rel="noopener noreferrer"
                                title={running === 0 ? t("应用已停止") : l.ssh ? t("经由 SSH 隧道：先在自己电脑上运行 {cmd}", { cmd: l.ssh }) : l.url}
                                onClick={(e) => { if (running === 0) e.preventDefault(); }} aria-disabled={running === 0}><ExternalLink />{t("打开面板")}</a>;
                            })()}
                            {panelEntry(r) && <button className="btn ghost icon sm" title={t("面板地址：反向代理、SSH 隧道、改过的端口和路径")} aria-label={t("面板地址")} onClick={() => setPaneling(r.key)}><Link2 /></button>}
                            {r.guide && <button className="btn ghost icon sm" title={t("上手指南：访问地址、初始账号、接下来几步")} aria-label={t("上手指南")} onClick={() => setGuiding(r.key)}><Rocket /></button>}
                            {(r.update || r.rebuilt) && <button className="btn ghost icon sm" title={r.update ? t("更新到 {v}", { v: r.update }) : t("拉取新构建并重建容器")} aria-label={t("更新")} onClick={() => run("update", [r.key])}><ArrowUpCircle /></button>}
                            {r.type === "compose" && (running > 0
                              ? <button className="btn ghost icon sm" title={t("停止")} aria-label={t("停止")} onClick={() => run("stop", [r.key])}><Square /></button>
                              : <button className="btn ghost icon sm" title={t("启动")} aria-label={t("启动")} onClick={() => run("start", [r.key])}><Play /></button>)}
                            {r.type === "compose" && <button className="btn ghost icon sm" title={t("重启")} aria-label={t("重启")} onClick={() => run("restart", [r.key])}><RotateCw /></button>}
                            {r.known && (r.type === "compose" || app?.ai) && <button className="btn ghost icon sm" title={r.type === "npm" ? t("配置 API Key 和模型") : t("修改设置")} aria-label={t("修改设置")} onClick={() => setConfiguring({ host: r.host, instance: r.instance })}><Settings2 /></button>}
                            {r.type === "npm" && app?.ai?.login && <Link className="btn ghost icon sm" title={t("订阅登录")} aria-label={t("订阅登录")} to={`/hosts/${encodeURIComponent(r.host)}/terminal?login=${encodeURIComponent(r.app)}`}><KeyRound /></Link>}
                            {r.type === "compose" && r.params?.port && <Link className="btn ghost icon sm" title={t("用域名访问（反向代理）")} aria-label={t("用域名访问（反向代理）")} to={`/hosts/${encodeURIComponent(r.host)}?tab=network&port=${r.params.port}`}><Globe /></Link>}
                            {r.type === "compose" && <Link className="btn ghost icon sm" title={t("在容器页查看")} aria-label={t("在容器页查看")} to={`/containers?host=${encodeURIComponent(r.host)}`}><Box /></Link>}
                            <button className="btn ghost icon sm" title={t("卸载")} aria-label={t("卸载")} onClick={() => ask("uninstall", [r.key])}><Trash2 /></button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
      </div>
      {configuring && <AppConfigureDialog host={configuring.host} instance={configuring.instance} onClose={() => setConfiguring(null)} />}
      {paneled && <AppPanelDialog row={paneled} title={byName.get(paneled.app)?.title ?? paneled.app} onClose={() => setPaneling(null)} onSaved={load} />}
      {guided?.guide && <AppGuideDialog host={guided.host} instance={guided.instance} app={byName.get(guided.app)} guide={guided.guide} onClose={() => setGuiding(null)} />}
      {confirm && (
        <UninstallDialog count={confirm.keys.length} compose={rows.filter((r) => confirm.keys.includes(r.key)).every((r) => r.type === "compose")}
          onClose={() => setConfirm(null)} onSubmit={(purge) => { setConfirm(null); run(purge ? "purge" : "uninstall", confirm.keys); }} />
      )}
    </>
  );
}

function UninstallDialog({ count, compose, onClose, onSubmit }: { count: number; compose: boolean; onClose: () => void; onSubmit: (purge: boolean) => void }) {
  const t = useT();
  const [purge, setPurge] = useState(false);
  return (
    <Modal title={t("卸载 {n} 个实例", { n: count })} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className={"btn " + (purge ? "danger" : "primary")} onClick={() => onSubmit(purge)}>{t(actionLabel[purge ? "purge" : "uninstall"])}</button></>}>
      <p className="dim">{compose
        ? t("默认停掉并移除容器，数据目录留在主机上（~/.hostower/apps/<实例名>），以后用同样的实例名重新安装就能接着用。")
        : t("移除命令和 npm 包，工具自己的配置（比如家目录里的登录信息）不动。")}</p>
      {compose && <label className="check"><input type="checkbox" checked={purge} onChange={(e) => setPurge(e.target.checked)} />{t("连数据一起删除（不可恢复，是高危操作）")}</label>}
    </Modal>
  );
}

/** 自动发起更新：发现新版本时以系统身份提交更新请求，照常等人批准。 */
function AutoUpdateCard({ onChecked }: { onChecked: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [st, setSt] = useState<AppAutoStatus | null>(null);
  const [busy, setBusy] = useState(false);
  // 点了“立即检查”之后等着：直到出现一次新的检查结果（最多一分钟）。
  const [waitSince, setWaitSince] = useState<number | null>(null);
  const load = useCallback(() => api.appAuto().then(setSt).catch(() => {}), []);
  useEffect(() => { load(); }, [load]);
  useRequestFinished(load);
  useEffect(() => {
    if (waitSince === null) return;
    const started = Date.now();
    const id = setInterval(() => api.appAuto().then((n) => {
      setSt(n);
      if ((!n.running && (n.lastRun ?? 0) > waitSince) || Date.now() - started > 60000) { setWaitSince(null); onChecked(); }
    }).catch(() => {}), 2000);
    return () => clearInterval(id);
  }, [waitSince, onChecked]);
  // 返回是否成功：失败时不要开始“正在检查”的轮询。
  const act = async (f: () => Promise<AppAutoStatus>) => {
    setBusy(true);
    try { setSt(await f()); return true; } catch (e) { fail(e); return false; } finally { setBusy(false); }
  };
  if (!st) return null;
  return (
    <div className="card">
      <div className="card-body">
        <div className="setting-row">
          <div>
            <div className="setting-name">{t("自动发起更新")}</div>
            <div className="setting-desc">{t("每 {n} 小时检查一次已安装的应用。发现新版本时以“应用自动更新”的身份提交更新请求，照常要你批准才会执行，不会自己更新；你拒绝过的版本不再提醒。", { n: st.intervalHours })}</div>
            {st.enabled && (
              <div className="hint" style={{ marginTop: 4 }}>
                {st.running || waitSince !== null ? t("正在检查…") : st.lastRun ? t("上次检查：{when}，提交了 {n} 个更新请求。", { when: ago(st.lastRun), n: st.submitted }) : t("还没有检查过：打开后两分钟内会查第一次。")}
                {st.error && <span className="crit-text"> {ts(st.error)}</span>}
              </div>
            )}
          </div>
          <div className="row" style={{ gap: 8 }}>
            {st.enabled && owner && <button className="btn sm" disabled={busy || st.running || waitSince !== null} onClick={() => { const since = st.lastRun ?? 0; act(api.checkAppAuto).then((ok) => ok && setWaitSince(since)); }}><RefreshCw className={st.running || waitSince !== null ? "spin" : ""} />{t("立即检查")}</button>}
            <button role="switch" aria-checked={st.enabled} aria-label={t("自动发起更新")} className={"switch" + (st.enabled ? " on" : "")}
              disabled={!owner || busy} title={owner ? undefined : t("只有所有者能修改")} onClick={() => { const on = !st.enabled, since = st.lastRun ?? 0; act(() => api.setAppAuto(on)).then((ok) => ok && on && setWaitSince(since)); }}><span /></button>
          </div>
        </div>
      </div>
    </div>
  );
}

/** 应用市场实例的卸载（容器页也用它）：提交后跳到请求页。 */
export function AppUninstallDialog({ host, instance, onClose }: { host: string; instance: string; onClose: () => void }) {
  const nav = useNavigate();
  return (
    <UninstallDialog count={1} compose onClose={onClose} onSubmit={async (purge) => {
      try {
        const r = await api.appAction({ action: purge ? "purge" : "uninstall", targets: [{ host, instance }] });
        onClose();
        const reqs = Array.isArray(r.requests) ? r.requests : [];
        if (reqs[0]) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
      } catch (e) { fail(e); }
    }} />
  );
}

/** 修改应用市场实例的设置（应用页和容器页共用）：按应用自己的参数改，重新生成 compose 文件后 up -d。 */
export function AppConfigureDialog({ host, instance, onClose }: { host: string; instance: string; onClose: () => void }) {
  const t = useT();
  const nav = useNavigate();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [app, setApp] = useState<AppItem | null>(null);
  const [rec, setRec] = useState<AppInstalled | null>(null);
  const [error, setError] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [changeSecrets, setChangeSecrets] = useState(false);
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [known, setKnown] = useState<string[]>([]);
  const [preview, setPreview] = useState<AppPlanResult | null>(null);
  const [perr, setPerr] = useState("");
  const [busy, setBusy] = useState(false);
  const [ai, setAI] = useState<AIConfig>({ provider: "" });

  useEffect(() => {
    Promise.all([api.apps(), api.appsInstalled([host]), api.appSecrets().catch(() => ({ secrets: [] as { name: string }[] }))]).then(([c, i, sec]) => {
      const h = i.hosts[0];
      if (h?.error) { setError(h.error); return; }
      const r = h?.installed.find((x) => x.instance === instance);
      const a = c.apps.find((x) => x.name === r?.app);
      if (!r || !a) { setError(t("没有找到这个实例，或者目录里没有这个应用。")); return; }
      setApp(a); setRec(r); setKnown(sec.secrets.map((x) => x.name));
      setValues(Object.fromEntries((a.params ?? []).filter((p) => p.type !== "secret").map((p) => [p.name, r.params?.[p.name] ?? p.default ?? ""])));
      if (a.ai) {
        const cfg = r.config ?? {};
        const prov = a.ai.providers.find((x) => x.id === cfg.provider) ?? a.ai.providers[0];
        setAI({ provider: prov.id, baseUrl: prov.custom || a.ai.kind === "dsh" ? (cfg.base_url ?? "") : "", model: cfg.model ?? "", smallModel: cfg.small_model ?? "",
          api: prov.apis ? (cfg.api ?? prov.apis[0]) : undefined });
      }
    }).catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [host, instance, t]);

  const isAI = app?.type === "npm" && !!app.ai;
  const body = isAI ? { host, instance, ai } : { host, instance, params: values, secrets: changeSecrets ? secrets : undefined };
  const key = JSON.stringify(body);
  useEffect(() => { setPreview(null); setPerr(""); }, [key]);
  const params = app?.params ?? [];
  const plain = params.filter((p) => p.type !== "secret");
  const secretParams = params.filter((p) => p.type === "secret");

  const doPreview = async () => {
    setBusy(true); setPerr("");
    try { setPreview(await api.appConfigure({ ...body, preview: true })); } catch (e) { setPerr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.appConfigure(body);
      const reqs = Array.isArray(r.requests) ? r.requests : [];
      onClose();
      if (reqs[0]) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const pv: Preview | null = preview?.targets ? { level: preview.level!, targets: preview.targets } : null;

  return (
    <Modal wide title={isAI ? t("配置 {name}", { name: app?.title ?? instance }) : t("修改 {name} 的设置", { name: instance })} onClose={onClose}
      footer={<>
        <button className="btn" onClick={onClose}>{t("取消")}</button>
        <button className="btn" onClick={doPreview} disabled={busy || !app}><Eye />{t("预览")}</button>
        <button className="btn primary" onClick={submit} disabled={busy || !preview?.targets}>{t("提交")}</button>
      </>}>
      {error && <div className="note crit"><TriangleAlert /><span>{ts(error)}</span></div>}
      {!app && !error && <div className="muted">{t("加载中…")}</div>}
      {app && rec && (
        <>
          <div className="row" style={{ gap: 12 }}>
            <AppIcon app={app} size={36} />
            <div className="grow" style={{ minWidth: 0 }}>
              <div style={{ fontWeight: 600 }}>{app.title} <span className="muted mono" style={{ fontWeight: 400 }}>{instance} · {host}</span></div>
              <div className="hint">{isAI
                ? t("配置写进主机上只有登录账号可读的文件，下次运行时生效；API Key 不会出现在命令和审计里。不碰你自己的配置目录。")
                : t("按新设置重新生成 compose 文件，只重建设置变了的容器；数据目录不动，仍由应用市场管理。")}</div>
            </div>
          </div>
          {isAI && app.ai && <AIConfigForm ai={app.ai} value={ai} onChange={setAI} known={known} current={rec.config} onKnown={setKnown}
            login={app.ai.login ? `/hosts/${encodeURIComponent(host)}/terminal?login=${encodeURIComponent(app.name)}` : undefined} />}
          {!isAI && rec.version !== app.version && <div className="note"><ArrowUpCircle /><span>{t("目录里的版本较新：保存设置会同时更新到 {v}（先拉取新镜像）。", { v: app.version })}</span></div>}
          {!isAI && plain.length === 0 && <div className="muted">{t("这个应用没有可以修改的参数。")}</div>}
          <div className="form-grid">
            {plain.map((p) => (
              <div key={p.name} className="field"><label htmlFor={"cfg-" + p.name}>{ts(p.title)}</label>
                <ParamInput p={p} id={"cfg-" + p.name} value={values[p.name] ?? ""} onChange={(v) => setValues({ ...values, [p.name]: v })} />
                {p.hint && <span className="hint">{ts(p.hint)}</span>}
              </div>
            ))}
          </div>
          {secretParams.length > 0 && (
            <>
              <label className="check"><input type="checkbox" checked={changeSecrets} onChange={(e) => setChangeSecrets(e.target.checked)} />{t("更换密钥（不勾选时保持不变）")}</label>
              {changeSecrets && (
                <div className="form-grid">
                  {secretParams.map((p) => (
                    <div key={p.name} className="field"><label htmlFor={"cfg-s-" + p.name}>{ts(p.title)}</label>
                      <Select id={"cfg-s-" + p.name} mono value={secrets[p.name] ?? ""} placeholder={t("选择密钥")} onChange={(v) => setSecrets({ ...secrets, [p.name]: v })} options={known.map((k) => ({ value: k, label: k }))} />
                    </div>
                  ))}
                  {known.length === 0 && <span className="hint">{owner ? <>{t("还没有应用密钥，先到 ")}<Link to="/credentials" style={{ color: "var(--accent)" }}>{t("凭据")}</Link>{t(" 页添加。")}</> : t("还没有应用密钥，请所有者先到 ")}</span>}
                </div>
              )}
            </>
          )}
          <div className="section-title">{t("判级预览")}</div>
          {!preview && !perr ? <div className="muted" style={{ fontSize: 12.5 }}>{t("点“预览”，看每台主机上实际要执行的命令和风险等级。预览没有任何副作用。")}</div> : <PreviewPanel preview={pv} error={perr} />}
          {preview?.targets && (
            <details className="app-details"><summary>{t("每台主机上的命令和 compose 文件")}</summary><pre className="code" style={{ marginTop: 8 }}>{preview.targets[0].command}</pre></details>
          )}
        </>
      )}
    </Modal>
  );
}

function ParamInput({ p, id, value, onChange }: { p: AppParam; id: string; value: string; onChange: (v: string) => void }) {
  return p.type === "enum"
    ? <Select id={id} mono value={value} onChange={onChange} options={(p.options ?? []).map((o) => ({ value: o, label: o }))} />
    : <input id={id} className="input mono" value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} inputMode={p.type === "port" ? "numeric" : undefined} />;
}

/** 容器带的应用标签（docker ps 的 Labels 是 “k=v,k=v” 的文本）：返回应用市场的实例名。 */
export function appInstanceOf(labels?: string): string | undefined {
  const m = /(?:^|,)com\.hostower\.app=([a-z0-9-]+)(?:,|$)/.exec(labels ?? "");
  return m?.[1];
}

/** 应用市场实例的小标识：容器页里用。 */
export function AppBadge({ instance }: { instance: string }) {
  const t = useT();
  return <Link to="/apps?tab=installed" className="badge accent" title={t("由应用市场安装，在应用页管理")}><Package />{instance}</Link>;
}

/** AI 命令行工具的配置表单：供应商、接口地址、模型、API Key（应用密钥）。安装和配置共用。 */
/** 接口协议的显示名（协议名本身是 DeepSeek Harness 配置里的写法）。 */
function apiLabel(api: string): string {
  return ({ "openai-completions": "OpenAI Chat Completions", "openai-responses": "OpenAI Responses", "anthropic-messages": "Anthropic Messages" } as Record<string, string>)[api] ?? api;
}

function AIConfigForm({ ai, value, onChange, known, onKnown, current, login }: {
  ai: NonNullable<AppItem["ai"]>; value: AIConfig; onChange: (v: AIConfig) => void; known: string[]; onKnown: (k: string[]) => void;
  current?: Record<string, string>; login?: string;
}) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [newKey, setNewKey] = useState<{ name: string; value: string } | null>(null);
  const prov = ai.providers.find((p) => p.id === value.provider) ?? ai.providers[0];
  const set = (patch: Partial<AIConfig>) => onChange({ ...value, ...patch });
  // 现在用的 Key 能不能沿用：同一个供应商（同一种 Key）且已经配过。
  const keepable = !!current?.secret && current.provider === prov?.id;
  const listId = "ai-models-" + (prov?.id ?? "");
  const saveKey = async () => {
    if (!newKey) return;
    try {
      await api.putAppSecret(newKey.name, newKey.value);
      const r = await api.appSecrets();
      onKnown(r.secrets.map((x) => x.name));
      set({ secret: newKey.name });
      setNewKey(null);
      toast(t("已保存应用密钥"));
    } catch (e) { fail(e); }
  };
  if (!prov) return null;
  // 必须填模型：Codex 的自定义供应商；DeepSeek Harness 的第三方提供商（模型就是默认模型）。
  const modelRequired = (prov.custom && ai.kind === "codex") || (ai.kind === "dsh" && prov.id !== "deepseek");
  return (
    <div className="col" style={{ gap: 12 }}>
      <div className="form-grid">
        <div className="field"><label htmlFor="ai-prov">{t("模型供应商")}</label>
          <Select id="ai-prov" value={prov.id} onChange={(id) => onChange({ provider: id, secret: value.secret, api: ai.providers.find((p) => p.id === id)?.apis?.[0] })}
            options={ai.providers.map((p) => ({ value: p.id, label: ts(p.title) }))} />
          {prov.note && <span className="hint">{ts(prov.note)}</span>}
        </div>
        {prov.apis && (
          <div className="field"><label htmlFor="ai-api">{t("接口协议")}</label>
            <Select id="ai-api" mono value={value.api ?? prov.apis[0]} onChange={(v) => set({ api: v })}
              options={prov.apis.map((a) => ({ value: a, label: apiLabel(a) }))} />
            <span className="hint">{t("大多数网关和自己部署的服务（vLLM、Ollama、LiteLLM 等）选 OpenAI Chat Completions。")}</span>
          </div>
        )}
        <div className="field"><label htmlFor="ai-base">{t("接口地址")}</label>
          {prov.custom || ai.kind === "dsh"
            ? <input id="ai-base" className="input mono" value={value.baseUrl ?? ""} onChange={(e) => set({ baseUrl: e.target.value.trim() })} spellCheck={false}
                placeholder={ai.kind === "dsh" && !prov.custom ? t("留空用官方地址") : "https://"} />
            : <input id="ai-base" className="input mono" value={prov.baseUrl ?? t("官方默认")} disabled />}
          {prov.baseUrl && <span className="hint">{t("预置的地址以供应商的文档为准。")}</span>}
        </div>
        <div className="field"><label htmlFor="ai-model">{t("模型")}</label>
          <input id="ai-model" className="input mono" list={listId} value={value.model ?? ""} onChange={(e) => set({ model: e.target.value.trim() })} spellCheck={false}
            placeholder={modelRequired ? t("必填") : t("留空用工具的默认模型")} />
          <datalist id={listId}>{(prov.models ?? []).map((m) => <option key={m} value={m} />)}</datalist>
          {ai.kind === "dsh" && prov.id !== "deepseek" && <span className="hint">{t("作为默认模型；运行时还可以在 dsh 里切换。")}</span>}
        </div>
        {ai.kind === "claude" && prov.id !== "anthropic" && (
          <div className="field"><label htmlFor="ai-small">{t("轻量模型（后台任务用）")}</label>
            <input id="ai-small" className="input mono" list={listId} value={value.smallModel ?? ""} onChange={(e) => set({ smallModel: e.target.value.trim() })} spellCheck={false} placeholder={t("留空和上面的模型相同")} />
          </div>
        )}
        <div className="field"><label htmlFor="ai-key">API Key</label>
          <div className="row" style={{ gap: 6 }}>
            <div className="grow" style={{ minWidth: 0 }}>
              <Select id="ai-key" mono value={value.secret ?? ""} onChange={(v) => set({ secret: v })}
                options={[{ value: "", label: keepable ? t("保持不变（{name}）", { name: current!.secret }) : prov.key === "optional" ? t("不填（用订阅登录）") : t("选择密钥") }, ...known.map((k) => ({ value: k, label: k }))]} />
            </div>
            {owner && <button className="btn sm" onClick={() => setNewKey({ name: (prov.id + "-key").replace(/[^a-z0-9._-]/g, "-"), value: "" })}><Plus />{t("新建")}</button>}
          </div>
          <span className="hint">{t("Key 存在塔台的凭据库里，执行时才写进主机上只有登录账号可读的文件；命令和审计里只有名字。")}</span>
        </div>
      </div>
      {newKey && (
        <div className="card app-secret">
          <div className="section-title">{t("新建应用密钥")}</div>
          <div className="form-grid">
            <div className="field"><label htmlFor="ak-name">{t("名称")}</label><input id="ak-name" className="input mono" value={newKey.name} onChange={(e) => setNewKey({ ...newKey, name: e.target.value })} spellCheck={false} /></div>
            <div className="field"><label htmlFor="ak-val">API Key</label><input id="ak-val" className="input mono" type="password" value={newKey.value} onChange={(e) => setNewKey({ ...newKey, value: e.target.value })} spellCheck={false} autoComplete="off" />
              <span className="hint">{t("保存后就再也读不出来了（只写不读）。")}</span></div>
          </div>
          <div className="row" style={{ justifyContent: "flex-end", gap: 6 }}>
            <button className="btn sm" onClick={() => setNewKey(null)}>{t("取消")}</button>
            <button className="btn primary sm" onClick={saveKey} disabled={!newKey.name || !newKey.value || newKey.value.includes("'")}><Check />{t("保存")}</button>
          </div>
        </div>
      )}
      {login && ai.login && (
        <div className="note"><KeyRound /><span>{t("用订阅（ChatGPT、Claude 订阅）而不是 API Key：")}
          <Link to={login} style={{ color: "var(--accent)" }}>{t("打开终端登录")}</Link>{t("。终端里会预先填好登录命令，回车由你来按；开终端要按高危操作审批，全程录制。")}
          {ai.loginHint && <span className="hint" style={{ display: "block", marginTop: 4 }}>{ts(ai.loginHint)}</span>}
          {prov.key === "optional" && <span className="hint" style={{ display: "block", marginTop: 4 }}>{t("用订阅时，上面的 API Key 选“不填”。")}</span>}
        </span></div>
      )}
    </div>
  );
}
