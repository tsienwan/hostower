// 容器管理。这一页没有自己的执行通道：列表是只读查询，其余每个动作都生成一个请求，
// 走和手敲命令相同的判级与审批。
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { DockerSetup } from "../DockerSetup";
import { askConfirm } from "../dialogs";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { ArrowUpCircle, Box, Copy, Download, FileText, Info, Pencil, Search, KeyRound, Layers, LogIn, Play, Plus, RefreshCw, RotateCw, Square, Trash2, TriangleAlert, Upload } from "lucide-react";
import { api, type AppInstalled, type Container, type ImageNews, type ContainerAction, type ContainerDetail, type ContainerStat, type Image, type Registry } from "../api";
import { backAfterApproval, fail, onHost, toast, useRequestFinished, useStore } from "../store";
import { Empty, Modal, Pager, SortTh, Sparkline, bytes, usePaged, useSort } from "../ui";
import { Select } from "../controls";
import { cached, remember } from "../cache";
import { formatStamp, ts, useT } from "../i18n";
import { AppBadge, AppConfigureDialog, AppUninstallDialog, appInstanceOf } from "./Apps";

type Dialog = null | "pull" | "deploy" | "upload" | "registries" | { logs: string } | { app: string; uninstall?: boolean };
type ContainersData = { available: boolean; message?: string; fix?: "docker-group"; user?: string; local?: boolean; containers: Container[]; images: Image[] };

/** docker 输出的大小（如 76.6MB）换成字节数，用来排序。 */
function sizeOf(text: string): number | undefined {
  const m = /^([\d.]+)\s*([kMGT]?)i?B$/i.exec(text.trim());
  if (!m) return undefined;
  return Number(m[1]) * 1000 ** ["", "k", "m", "g", "t"].indexOf(m[2].toLowerCase());
}

const lines = (s: string) => s.split("\n").map((x) => x.trim()).filter(Boolean);

export function Containers() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const [sp, setSp] = useSearchParams();
  const nav = useNavigate();
  const host = sp.get("host") ?? "";
  const [data, setData] = useState<ContainersData | null>(() => (host ? cached<ContainersData>("containers:" + host) ?? null : null));
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [preset, setPreset] = useState<DeployPreset>({});
  const [updates, setUpdates] = useState<Record<string, ImageNews>>({});
  const [appInfo, setAppInfo] = useState<Record<string, AppInstalled>>({});

  // 默认选第一台带 docker 标签的在线主机。
  useEffect(() => {
    if (host || hosts.length === 0) return;
    const pick = hosts.find((h) => h.tags.includes("docker") && h.status.online) ?? hosts.find((h) => h.status.online) ?? hosts[0];
    setSp({ host: pick.name }, { replace: true });
  }, [host, hosts, setSp]);

  const load = useCallback(() => {
    if (!host) return;
    setLoading(true);
    setError("");
    api.containers(host).then((r) => setData(remember("containers:" + host, r))).catch((e) => { setData(null); setError(e.message); }).finally(() => setLoading(false));
    // 更新信息：容器列表先显示，这两项（要去镜像仓库查）随后补上；查不到就不显示。
    setUpdates({}); setAppInfo({});
    api.containerUpdates(host).then((r) => setUpdates(r.containers)).catch(() => {});
    api.appsInstalled([host]).then((r) => setAppInfo(Object.fromEntries((r.hosts[0]?.installed ?? []).map((x) => [x.instance, x])))).catch(() => {});
  }, [host]);
  // 换主机时先显示这台主机上次的结果（没有就是空），后台再取最新的：容器列表要经 SSH 现查，远端主机上要等一会儿。
  useEffect(() => { setData(host ? cached<ContainersData>("containers:" + host) ?? null : null); load(); }, [load, host]);
  // 批准后回到这一页：这台主机上的操作执行完就重新查一遍。
  useRequestFinished(load, onHost(host));

  /** 提交一个动作：生成请求后跳到请求页，在那里看判级、批准和输出。 */
  const act = async (a: Omit<ContainerAction, "host">) => {
    try {
      const r = await api.containerAction({ host, ...a });
      nav(`/requests/${r.id}`, { state: backAfterApproval });
    } catch (e) { fail(e); }
  };

  const [imgQuery, setImgQuery] = useState("");
  const shownImages = useMemo(() => {
    const q = imgQuery.trim().toLowerCase();
    return (data?.images ?? []).filter((im) => !q || `${im.Repository}:${im.Tag}`.toLowerCase().includes(q) || im.ID.replace("sha256:", "").startsWith(q));
  }, [data, imgQuery]);
  const imageSort = useSort(shownImages, {
    name: (im) => (im.Repository + ":" + im.Tag).toLowerCase(),
    size: (im) => sizeOf(im.Size),
  });
  const images = usePaged(imageSort.sorted, 10);

  const hostOptions = hosts.map((h) => ({ value: h.name, label: h.name, hint: !h.status.online && h.status.known ? t("离线") : h.tags.includes("docker") ? "docker" : undefined }));

  return (
    <>
      <div className="topbar">
        <h1>{t("容器")}</h1>
        <div style={{ width: 200, maxWidth: "50vw" }}><Select mono value={host} onChange={(v) => setSp({ host: v })} options={hostOptions} placeholder={t("选择主机")} /></div>
        <button className="btn ghost icon" onClick={load} disabled={loading || !host} title={t("刷新")} aria-label={t("刷新")}><RefreshCw className={loading ? "spin" : ""} /></button>
        <span className="grow" />
        <button className="btn" onClick={() => setDialog("registries")}><KeyRound /><span className="hide-sm">{t("镜像仓库")}</span></button>
        <button className="btn" onClick={() => setDialog("upload")} disabled={!data?.available}><Upload /><span className="hide-sm">{t("上传镜像")}</span></button>
        <button className="btn" onClick={() => setDialog("pull")} disabled={!data?.available}><Download /><span className="hide-sm">{t("拉取镜像")}</span></button>
        <button className="btn primary" onClick={() => { setPreset({}); setDialog("deploy"); }} disabled={!data?.available}><Plus />{t("部署容器")}</button>
      </div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("查看是只读操作，自动执行。启动、停止、拉取、部署这些都会变成一个请求：你先看到实际要跑的 docker 命令和它的风险等级，批准后才执行。")}</p>
          {error && <div className="note crit"><TriangleAlert /><span>{error}</span></div>}
          {data && !data.available && (
            <div className="card"><Empty icon={<Box />} title={t("这台主机上用不了 Docker")}>
              {data.message ? <span className="mono">{ts(data.message)}</span> : t("没有找到 docker 命令，或者当前账号没有权限访问它。")}
              {data.fix === "docker-group" && data.user && <DockerFix host={host} user={data.user} local={!!data.local} onReconnected={load} />}
            </Empty>
            {data.fix !== "docker-group" && <div className="docker-setup-wrap"><DockerSetup host={host} /></div>}
            </div>
          )}
          {!data && !error && <div className="card"><div className="empty">{host ? t("加载中…") : t("先选择一台主机")}</div></div>}

          {data?.available && (
            <>
              <ContainerTable host={host} containers={data.containers} stats={hosts.find((h) => h.name === host)?.status.containers ?? []} onLogs={(name) => setDialog({ logs: name })} onAct={act}
                onEdit={(p) => { if (p.app) setDialog({ app: p.app }); else { setPreset(p); setDialog("deploy"); } }}
                onUninstallApp={(inst) => setDialog({ app: inst, uninstall: true })}
                updates={updates} appInfo={appInfo}
                onUpdateApp={async (inst) => { try { const r = await api.appAction({ action: "update", targets: [{ host, instance: inst }] }); const q = Array.isArray(r.requests) ? r.requests : []; if (q[0]) nav(`/requests/${q[0].id}`, { state: backAfterApproval }); } catch (e) { fail(e); } }} />

              <div className="card">
                <div className="card-head wrap">
                  <h2>{t("镜像")}</h2>
                  <span className="muted num">{imgQuery.trim() ? `${shownImages.length} / ${data.images.length}` : data.images.length}</span>
                  <span className="grow" />
                  <div className="search" style={{ width: 240, maxWidth: "100%" }}>
                    <Search />
                    <input className="input" placeholder={t("搜索镜像名、标签或编号")} value={imgQuery} onChange={(e) => { setImgQuery(e.target.value); images.setPage(0); }} aria-label={t("搜索镜像")} />
                  </div>
                </div>
                {data.images.length === 0 ? <Empty icon={<Layers />} title={t("还没有镜像")} /> : shownImages.length === 0 ? <Empty icon={<Search />} title={t("没有匹配的镜像")} /> : (
                  <div className="table-wrap">
                    <table className="table img-table">
                      <thead><tr><SortTh s={imageSort} k="name">{t("镜像")}</SortTh><th className="hide-sm">{t("编号")}</th><SortTh s={imageSort} k="size" desc>{t("大小")}</SortTh><th className="hide-sm">{t("创建@@时间")}</th><th className="act">{t("操作")}</th></tr></thead>
                      <tbody>
                        {images.rows.map((im, i) => {
                          const ref = im.Tag && im.Tag !== "<none>" ? `${im.Repository}:${im.Tag}` : im.Repository;
                          return (
                            <tr key={im.ID + i}>
                              <td className="mono"><span style={{ fontWeight: 600 }}>{im.Repository}</span><span className="muted">:{im.Tag}</span></td>
                              <td className="hide-sm mono dim">{im.ID.replace("sha256:", "").slice(0, 12)}</td>
                              <td className="num">{im.Size}</td>
                              <td className="hide-sm dim">{im.CreatedSince}</td>
                              <td className="act">
                                <div className="row act-row">
                                  <button className="btn ghost icon sm" title={t("用这个镜像部署容器")} aria-label={t("部署")} onClick={() => { setPreset({ image: ref }); setDialog("deploy"); }}><Play /></button>
                                  <button className="btn ghost icon sm" title={t("重新拉取（更新）")} aria-label={t("重新拉取")} onClick={() => act({ action: "pull", image: ref })}><Download /></button>
                                  <button className="btn ghost icon sm" title={t("删除镜像")} aria-label={t("删除镜像")} onClick={() => act({ action: "rmi", image: ref })}><Trash2 /></button>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                    <Pager p={images} />
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {dialog && typeof dialog === "object" && "app" in dialog && (dialog.uninstall
        ? <AppUninstallDialog host={host} instance={dialog.app} onClose={() => setDialog(null)} />
        : <AppConfigureDialog host={host} instance={dialog.app} onClose={() => setDialog(null)} />)}
      {dialog === "pull" && <PullDialog onClose={() => setDialog(null)} onSubmit={(image) => act({ action: "pull", image })} />}
      {dialog === "deploy" && <DeployDialog preset={preset} onClose={() => setDialog(null)} onSubmit={(deploy) => act({ action: "deploy", deploy })} />}
      {dialog === "upload" && <UploadDialog host={host} onClose={() => setDialog(null)} onSubmit={(upload) => act({ action: "load", upload })} />}
      {dialog === "registries" && <RegistriesDialog host={host} canLogin={!!data?.available} onClose={() => setDialog(null)} onLogin={(registry) => act({ action: "login", registry })} />}
      {dialog && typeof dialog === "object" && "logs" in dialog && <LogsDialog host={host} name={dialog.logs} onClose={() => setDialog(null)} />}
    </>
  );
}

/** 把 docker 的端口串整理成几项：同一条映射的 IPv4 / IPv6 两份合并，只在容器内的端口单独标出。 */
function parsePorts(raw: string): { text: string; published: boolean }[] {
  const seen = new Set<string>();
  const out: { text: string; published: boolean }[] = [];
  for (const part of raw.split(",").map((x) => x.trim()).filter(Boolean)) {
    const m = /^(?:(.*):)?([\d-]+)->([\d-]+)\/(\w+)$/.exec(part);
    let text: string, published = true;
    if (m) {
      const local = m[1] === "127.0.0.1" || m[1] === "[::1]" || m[1] === "::1";
      text = (local ? "127.0.0.1:" : "") + (m[2] === m[3] ? m[2] : `${m[2]}→${m[3]}`) + (m[4] === "tcp" ? "" : "/" + m[4]);
    } else {
      text = part.replace(/\/tcp$/, "");
      published = false;
    }
    if (seen.has(text)) continue;
    seen.add(text);
    out.push({ text, published });
  }
  // 对外映射的排前面。
  return out.sort((a, b) => Number(b.published) - Number(a.published));
}

type Filter = "running" | "stopped" | "all";

const HEALTH = /\s*\((healthy|unhealthy|health: starting)\)/;

function StateBadges({ state, health }: { state: string; health?: string }) {
  const t = useT();
  const running = state === "running";
  return (
    <span className="row" style={{ gap: 6 }}>
      <span className={"badge " + (health === "unhealthy" ? "crit" : running ? "ok" : state === "restarting" || state === "paused" ? "warn" : "neutral")}>
        {running ? t("运行中") : state === "exited" ? t("已停止") : state === "restarting" ? t("重启中") : state === "paused" ? t("已暂停") : state === "created" ? t("已创建") : state}
      </span>
      {health === "unhealthy" && <span className="badge crit">{t("不健康")}</span>}
      {health === "healthy" && <span className="muted" style={{ fontSize: 12 }}>{t("健康")}</span>}
      {health === "health: starting" || health === "starting" ? <span className="muted" style={{ fontSize: 12 }}>{t("检查中")}</span> : null}
    </span>
  );
}

/** 容器列表：每行是运行状态和资源占用，点名称看完整配置。默认只看运行中的，可筛选、搜索、分页。 */
function ContainerTable({ host, containers, stats, onLogs, onAct, onEdit, onUninstallApp, updates, appInfo, onUpdateApp }: {
  host: string; containers: Container[]; stats: ContainerStat[];
  onLogs: (name: string) => void; onAct: (a: Omit<ContainerAction, "host">) => void; onEdit: (p: DeployPreset) => void;
  onUninstallApp: (instance: string) => void;
  updates: Record<string, ImageNews>; appInfo: Record<string, AppInstalled>; onUpdateApp: (instance: string) => void;
}) {
  const t = useT();
  const nRunning = containers.filter((c) => c.State === "running").length;
  const [filter, setFilter] = useState<Filter>(nRunning > 0 ? "running" : "all");
  const [query, setQuery] = useState("");
  const [detail, setDetail] = useState<Container | null>(null);
  const byName = useMemo(() => new Map(stats.map((x) => [x.name, x])), [stats]);
  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return containers
      .filter((c) => filter === "all" || (filter === "running") === (c.State === "running"))
      .filter((c) => !q || c.Names.toLowerCase().includes(q) || c.Image.toLowerCase().includes(q) || c.ID.startsWith(q))
      .sort((a, b) => Number(b.State === "running") - Number(a.State === "running") || a.Names.localeCompare(b.Names));
  }, [containers, filter, query]);
  const sorter = useSort(shown, {
    name: (c) => c.Names.toLowerCase(),
    state: (c) => (c.State === "running" ? 0 : 1) + c.Status,
    cpu: (c) => (c.State === "running" ? byName.get(c.Names)?.cpu : undefined),
    mem: (c) => (c.State === "running" ? byName.get(c.Names)?.memBytes : undefined),
  });
  const paged = usePaged(sorter.sorted, 10);
  // 修改设置：先读出现在的完整配置，再把它填进部署表单。
  // updating：为了更新而重建；newImage：换成这个镜像（固定版本标签升级到新版本）。
  const edit = (name: string, updating = false, newImage?: string) => api.containerInspect(host, name).then((d) => {
    setDetail(null);
    const p = presetFrom(d);
    onEdit({ ...p, updating, image: newImage ?? p.image });
  }).catch(fail);
  const tabs: { key: Filter; label: string; n: number }[] = [
    { key: "running", label: t("运行中"), n: nRunning },
    { key: "stopped", label: t("已停止"), n: containers.length - nRunning },
    { key: "all", label: t("全部"), n: containers.length },
  ];
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("容器")}</h2>
        <div className="row wrap" role="group" aria-label={t("按状态筛选")} style={{ gap: 4 }}>
          {tabs.map((x) => <button key={x.key} className={"tag" + (filter === x.key ? " on" : "")} aria-pressed={filter === x.key} onClick={() => { setFilter(x.key); paged.setPage(0); }}>{x.label}<span className="num" style={{ marginLeft: 6, opacity: 0.7 }}>{x.n}</span></button>)}
        </div>
        <span className="grow" />
        <div className="search" style={{ width: 240, maxWidth: "100%" }}>
          <Search />
          <input className="input" placeholder={t("搜索名称或镜像")} value={query} onChange={(e) => { setQuery(e.target.value); paged.setPage(0); }} aria-label={t("搜索容器")} />
        </div>
      </div>
      {containers.length === 0 ? <Empty icon={<Box />} title={t("还没有容器")}>{t("点右上角的“部署容器”，给一个镜像地址就能拉取并运行。")}</Empty>
        : shown.length === 0 ? <Empty icon={<Search />} title={t("没有匹配的容器")} /> : (
          <>
            <div className="table-wrap">
              <table className="table ctr-table">
                <thead><tr><SortTh s={sorter} k="name">{t("容器")}</SortTh><SortTh s={sorter} k="state">{t("状态")}</SortTh><SortTh s={sorter} k="cpu" desc>CPU</SortTh><SortTh s={sorter} k="mem" desc>{t("内存")}</SortTh><th className="hide-sm">{t("端口")}</th><th className="act">{t("操作")}</th></tr></thead>
                <tbody>
                  {paged.rows.map((c) => {
                    const running = c.State === "running";
                    const health = HEALTH.exec(c.Status)?.[1];
                    const ports = parsePorts(c.Ports ?? "");
                    const st = running ? byName.get(c.Names) : undefined;
                    const app = appInstanceOf(c.Labels);
                    // 更新：应用市场的容器看应用的版本信息；其他容器看镜像仓库里同一标签有没有新构建。
                    const info = app ? appInfo[app] : undefined;
                    const news = updates[c.Names];
                    const composeProject = !app && /(?:^|,)com\.docker\.compose\.project=/.test(c.Labels ?? "");
                    const canUpdate = app ? !!(info?.update || info?.rebuilt) : !!(news?.rebuilt || news?.newer);
                    return (
                      <tr key={c.ID}>
                        <td>
                          <div className="cell-2">
                            <span className="row ctr-name-row">
                              <button className="link-btn ellipsis" title={t("查看 {name} 的配置", { name: c.Names })} onClick={() => setDetail(c)}>{c.Names}</button>
                              {app && <AppBadge instance={app} />}
                              {app && info?.update && <span className="badge accent" title={t("由应用市场更新")}><ArrowUpCircle />{t("可更新到 {v}", { v: info.update })}</span>}
                              {!app && news?.newer && <span className="badge accent" title={t("仓库里有同一大版本内更新的版本，更新会换成新标签重新部署，原来的设置不变。")}><ArrowUpCircle />{t("有新版本 {v}", { v: news.newer })}</span>}
                              {(app ? info?.rebuilt && !info?.update : news?.rebuilt) && <span className="badge accent" title={t("同一个镜像标签在仓库里有了新的构建（通常是小版本或安全修复），更新会重新拉取并重建容器。")}><ArrowUpCircle />{t("镜像有新构建")}</span>}
                              {(app ? info?.upstream : news?.upstream) && <span className="badge neutral" title={t("镜像仓库里已经有这个大版本，但目录还没收录：要先确认能不能原地升级，Hostower 不会自动换过去。")}>{t("上游有新大版本 {v}", { v: (app ? info?.upstream : news?.upstream) ?? "" })}</span>}
                            </span>
                            <small className="mono ellipsis" title={c.Image}>{c.Image}</small>
                          </div>
                        </td>
                        <td><div className="cell-2"><StateBadges state={c.State} health={health} /><small className="ctr-since">{c.Status.replace(HEALTH, "")}</small></div></td>
                        <td>{st ? <div className="cpu-cell"><Sparkline data={st.history ?? []} width={56} height={22} max={Math.max(10, ...(st.history ?? []))} /><b>{st.cpu.toFixed(1)}%</b></div> : <span className="muted">—</span>}</td>
                        <td className="num">{st ? <div className="cell-2"><span>{bytes(st.memBytes)}</span><small>{st.mem.toFixed(1)}%</small></div> : <span className="muted">—</span>}</td>
                        <td className="ctr-ports hide-sm">
                          {ports.length === 0 ? <span className="muted">—</span> : (
                            <div className="row" style={{ gap: 4, flexWrap: "nowrap" }}>
                              {ports.slice(0, 2).map((x) => <span key={x.text} className={"tag mono" + (x.published ? "" : " dim")} title={x.published ? t("映射到主机的端口") : t("只在容器网络里开放")}>{x.text}</span>)}
                              {ports.length > 2 && <button className="link-btn muted" style={{ fontSize: 12, fontWeight: 500 }} title={t("查看全部端口")} onClick={() => setDetail(c)}>+{ports.length - 2}</button>}
                            </div>
                          )}
                        </td>
                        <td className="act">
                          <div className="row act-row">
                            <button className="btn ghost icon sm" title={t("查看配置")} aria-label={t("查看配置")} onClick={() => setDetail(c)}><Info /></button>
                            <button className="btn ghost icon sm" title={t("看日志（只读）")} aria-label={t("看日志")} onClick={() => onLogs(c.Names)}><FileText /></button>
                            {running
                              ? <button className="btn ghost icon sm" title={t("停止")} aria-label={t("停止")} onClick={() => onAct({ action: "stop", name: c.Names })}><Square /></button>
                              : <button className="btn ghost icon sm" title={t("启动")} aria-label={t("启动")} onClick={() => onAct({ action: "start", name: c.Names })}><Play /></button>}
                            <button className="btn ghost icon sm" title={t("重启")} aria-label={t("重启")} onClick={() => onAct({ action: "restart", name: c.Names })}><RotateCw /></button>
                            {canUpdate && (app
                              ? <button className="btn ghost icon sm" title={info?.update ? t("更新到 {v}", { v: info.update }) : t("拉取新构建并重建容器")} aria-label={t("更新")} onClick={() => onUpdateApp(app)}><ArrowUpCircle /></button>
                              : composeProject
                                ? <button className="btn ghost icon sm" disabled title={t("这个容器由你自己的 Compose 项目管理：在它的 compose 目录里执行 docker compose pull && docker compose up -d 更新，在这里重建会让它脱离 Compose。")} aria-label={t("更新")}><ArrowUpCircle /></button>
                                : <button className="btn ghost icon sm" title={news?.newer ? t("更新到 {v}：换成新标签，按原来的设置重建容器", { v: news.newer }) : t("更新：拉取新镜像，按原来的设置重建容器")} aria-label={t("更新")}
                                    onClick={() => edit(c.Names, true, news?.newerImage)}><ArrowUpCircle /></button>)}
                            <button className="btn ghost icon sm" title={app ? t("修改应用 {name} 的设置", { name: app }) : t("修改设置（端口、挂载、环境变量、镜像）")} aria-label={t("修改设置")} onClick={() => (app ? onEdit({ app }) : edit(c.Names))}><Pencil /></button>
                            {app
                              ? <button className="btn ghost icon sm" title={t("卸载应用 {name}", { name: app })} aria-label={t("卸载应用 {name}", { name: app })} onClick={() => onUninstallApp(app)}><Trash2 /></button>
                              : <button className="btn ghost icon sm" title={t("删除容器")} aria-label={t("删除容器")} onClick={() => onAct({ action: "remove", name: c.Names })}><Trash2 /></button>}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <Pager p={paged} />
          </>
        )}
      {detail && <ContainerDetailDialog host={host} name={detail.Names} stat={byName.get(detail.Names)} onClose={() => setDetail(null)} onLogs={() => { onLogs(detail.Names); setDetail(null); }} onEdit={(d) => { setDetail(null); onEdit(presetFrom(d)); }} />}
    </div>
  );
}

const stamp = (v?: string) => (v && !v.startsWith("0001-") ? formatStamp(v) : "");

/** 一个容器的完整配置：状态、资源、端口、挂载、环境变量、网络。全部只读。 */
export function ContainerDetailDialog({ host, name, stat, onClose, onLogs, onEdit }: { host: string; name: string; stat?: ContainerStat; onClose: () => void; onLogs?: () => void; onEdit?: (d: ContainerDetail) => void }) {
  const t = useT();
  const [d, setD] = useState<ContainerDetail | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.containerInspect(host, name).then(setD).catch((e) => setError(ts(e.message)));
  }, [host, name]);
  const running = d?.status === "running";
  return (
    <Modal wide title={name} onClose={onClose}
      footer={<>
        {onEdit && <button className="btn" disabled={!d} onClick={() => d && onEdit(d)}><Pencil />{t("修改设置")}</button>}
        {onLogs && <button className="btn" onClick={onLogs}><FileText />{t("看日志")}</button>}
        {!onEdit && !onLogs && <Link className="btn" to={`/containers?host=${host}`}>{t("到容器管理里操作")}</Link>}
        <button className="btn primary" onClick={onClose}>{t("关闭")}</button>
      </>}>
      {error && <div className="note crit"><TriangleAlert /><span>{error}</span></div>}
      {!d && !error && <div className="empty sm">{t("加载中…")}</div>}
      {d && (
        <>
          <section className="dsec">
            <h3>{t("概况")}</h3>
            <dl className="kv">
              <dt>{t("状态")}</dt>
              <dd className="row wrap" style={{ gap: 8 }}>
                <StateBadges state={d.status} health={d.health} />
                {d.privileged && <span className="badge serious">{t("特权容器")}</span>}
                {d.oomKilled && <span className="badge crit">{t("曾因内存不足被结束")}</span>}
                {!running && d.finishedAt && stamp(d.finishedAt) && <span className="muted">{t("退出码 {code}", { code: d.exitCode })}</span>}
              </dd>
              <dt>{t("镜像")}</dt><dd className="mono">{d.image}</dd>
              <dt>{t("容器编号")}</dt><dd className="mono">{d.id.slice(0, 12)}</dd>
              <dt>{t("创建时间")}</dt><dd className="num">{stamp(d.created) || "—"}</dd>
              <dt>{running ? t("启动时间") : t("停止时间")}</dt><dd className="num">{stamp(running ? d.startedAt : d.finishedAt) || "—"}</dd>
              <dt>{t("重启策略")}</dt><dd>{d.restart === "unless-stopped" ? t("除非手动停止") : d.restart === "always" ? t("总是重启") : d.restart === "on-failure" ? t("出错时重启") : t("不重启")}<span className="muted">　{t("已重启 {n} 次", { n: d.restartCount })}</span></dd>
              {d.project && <><dt>Compose</dt><dd className="mono">{d.project}{d.service ? " / " + d.service : ""}</dd></>}
              <dt>{t("启动命令")}</dt><dd className="mono">{d.command.length ? d.command.join(" ") : "—"}</dd>
              {(d.user || d.workDir) && <><dt>{t("用户 / 工作目录")}</dt><dd className="mono">{d.user || "root"}　{d.workDir || "/"}</dd></>}
            </dl>
          </section>

          <section className="dsec">
            <h3>{t("资源")}</h3>
            <div className="res-grid">
              <div><small>CPU</small><b>{running && stat ? stat.cpu.toFixed(1) + "%" : "—"}</b><span>{d.cpuLimit > 0 ? t("上限 {n} 核", { n: d.cpuLimit }) : t("没有上限")}</span></div>
              <div><small>{t("内存")}</small><b>{running && stat ? bytes(stat.memBytes) : "—"}</b><span>{d.memoryLimit > 0 ? t("上限 {n}", { n: bytes(d.memoryLimit) }) : t("没有上限")}</span></div>
              <div><small>{t("网络累计")}</small><b>{running && stat ? "↓ " + bytes(stat.netRx) : "—"}</b><span>{running && stat ? "↑ " + bytes(stat.netTx) : t("没有运行")}</span></div>
            </div>
          </section>

          <section className="dsec">
            <h3>{t("端口")}<span className="muted num">{d.ports.length || ""}</span></h3>
            {d.ports.length === 0 ? <p className="muted">{t("没有开放端口")}</p> : (
              <div className="subtable"><table className="table"><thead><tr><th>{t("主机上的地址")}</th><th>{t("容器端口")}</th><th>{t("协议")}</th></tr></thead><tbody>
                {d.ports.map((x, i) => (
                  <tr key={i}><td className="mono">{x.hostPort ? `${(x.hostIp ?? "").includes(":") ? `[${x.hostIp}]` : x.hostIp || "0.0.0.0"}:${x.hostPort}` : <span className="muted">{t("没有映射，只在容器网络里开放")}</span>}</td><td className="mono">{x.container}</td><td className="mono dim">{x.proto}</td></tr>
                ))}
              </tbody></table></div>
            )}
          </section>

          <section className="dsec">
            <h3>{t("挂载")}<span className="muted num">{d.mounts.length || ""}</span></h3>
            {d.mounts.length === 0 ? <p className="muted">{t("没有挂载")}</p> : (
              <div className="subtable"><table className="table"><thead><tr><th>{t("来源")}</th><th>{t("容器内路径")}</th><th style={{ width: 90 }}>{t("读写")}</th></tr></thead><tbody>
                {d.mounts.map((m, i) => (
                  <tr key={i}><td className="mono wrap-any"><span className="tag" style={{ marginRight: 6 }}>{m.type === "bind" ? t("目录") : m.type === "volume" ? t("卷") : m.type}</span>{m.source}</td><td className="mono wrap-any">{m.destination}</td><td>{m.readOnly ? t("只读") : t("读写")}</td></tr>
                ))}
              </tbody></table></div>
            )}
          </section>

          <section className="dsec">
            <h3>{t("环境变量")}<span className="muted num">{d.env.length || ""}</span></h3>
            {d.env.length === 0 ? <p className="muted">{t("没有环境变量")}</p> : (
              <>
                <div className="subtable"><table className="table env-table"><tbody>
                  {d.env.map((e) => (
                    <tr key={e.key}><td className="mono">{e.key}</td><td className="mono wrap-any">{e.masked && !e.value ? <span className="badge neutral">{t("已隐藏")}</span> : e.value || <span className="muted">{t("（空）")}</span>}</td></tr>
                  ))}
                </tbody></table></div>
                {d.env.some((e) => e.masked) && <p className="hint">{t("名字像口令、密钥、令牌的变量，值不会显示，也不会离开服务端。")}</p>}
              </>
            )}
          </section>

          <section className="dsec">
            <h3>{t("网络")}<span className="muted num">{d.networks.length || ""}</span></h3>
            {d.networks.length === 0 ? <p className="muted mono">{d.networkMode || "—"}</p> : (
              <div className="subtable"><table className="table"><thead><tr><th>{t("网络名")}</th><th>{t("地址")}</th><th>{t("别名")}</th></tr></thead><tbody>
                {d.networks.map((n) => <tr key={n.name}><td className="mono">{n.name}</td><td className="mono">{n.ip || "—"}</td><td className="mono dim wrap-any">{(n.aliases ?? []).join(" ") || "—"}</td></tr>)}
              </tbody></table></div>
            )}
          </section>
        </>
      )}
    </Modal>
  );
}

function PullDialog({ onClose, onSubmit }: { onClose: () => void; onSubmit: (image: string) => void }) {
  const t = useT();
  const [image, setImage] = useState("");
  return (
    <Modal title={t("拉取镜像")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={!image.trim()} onClick={() => onSubmit(image.trim())}>{t("提交")}</button></>}>
      <div className="field">
        <label htmlFor="pull-image">{t("镜像地址")}</label>
        <input id="pull-image" className="input mono" value={image} onChange={(e) => setImage(e.target.value)} spellCheck={false} autoFocus placeholder={t("nginx:1.27 或 registry.example.com/team/app:2.4.1")} />
        <span className="hint">{t("公开镜像直接拉。私有仓库先在“镜像仓库”里保存凭据，并在这台主机上登录一次。")}</span>
      </div>
    </Modal>
  );
}

/** 部署弹窗的预填内容。editing 表示这是在改一个已有容器的设置（删除并重建）。 */
interface DeployPreset {
  name?: string; image?: string; ports?: string[]; env?: string[]; volumes?: string[]; restart?: string;
  editing?: boolean; project?: string; lost?: string[]; hidden?: string[];
  /** 是为了更新而打开的：标题说清楚“拉取新镜像并按原设置重建”。 */
  updating?: boolean;
  /** 应用市场的实例名：这种容器到应用页改设置。 */
  app?: string;
}

const PORT_OK = /^([0-9.]+:)?[0-9]{1,5}:[0-9]{1,5}(\/tcp|\/udp)?$/;
const VOLUME_OK = /^(\/[^:]*|[a-zA-Z0-9][a-zA-Z0-9_.-]*):\/[^:]*(:ro|:rw)?$/;

/** 把一个容器现在的配置整理成部署表单能填的内容；带不过来的东西列在 lost 里，如实告诉用户。 */
function presetFrom(d: ContainerDetail): DeployPreset {
  const lost: string[] = [];
  const ports: string[] = [];
  for (const x of d.ports) {
    if (!x.hostPort) continue; // 没有映射的端口是镜像声明的，不用写
    const ip = x.hostIp && x.hostIp !== "0.0.0.0" && x.hostIp !== "::" ? x.hostIp + ":" : "";
    const line = `${ip}${x.hostPort}:${x.container}${x.proto === "tcp" ? "" : "/" + x.proto}`;
    if (!PORT_OK.test(line)) { if (!ip.includes("[") && !ip.includes("::")) lost.push(line); continue; }
    if (!ports.includes(line)) ports.push(line);
  }
  const volumes: string[] = [];
  for (const m of d.mounts) {
    const line = `${m.source}:${m.destination}${m.readOnly ? ":ro" : ""}`;
    if ((m.type === "bind" || m.type === "volume") && VOLUME_OK.test(line) && !line.includes("..")) volumes.push(line);
    else lost.push(`${m.type} ${m.source || ""} → ${m.destination}`);
  }
  const env = d.env.filter((e) => !e.fromImage && !e.masked && !/[\n\r]/.test(e.value)).map((e) => `${e.key}=${e.value}`);
  const hidden = d.env.filter((e) => !e.fromImage && e.masked).map((e) => e.key);
  const custom = d.networks.map((n) => n.name).filter((n) => n !== "bridge");
  if (custom.length) lost.push("network: " + custom.join(", "));
  if (d.memoryLimit > 0 || d.cpuLimit > 0) lost.push("cpu / memory limit");
  if (d.privileged) lost.push("privileged");
  return { name: d.name, image: d.image, ports, env, volumes, restart: ["no", "always", "unless-stopped", "on-failure"].includes(d.restart) ? d.restart : "no",
    editing: true, project: d.project, lost, hidden, app: d.app };
}

function DeployDialog({ preset, onClose, onSubmit }: { preset: DeployPreset; onClose: () => void; onSubmit: (d: ContainerAction["deploy"] & {}) => void }) {
  const t = useT();
  const presetName = preset.name;
  const [name, setName] = useState(preset.name ?? "");
  const [image, setImage] = useState(preset.image ?? "");
  const [ports, setPorts] = useState((preset.ports ?? []).join("\n"));
  const [env, setEnv] = useState((preset.env ?? []).join("\n"));
  const [volumes, setVolumes] = useState((preset.volumes ?? []).join("\n"));
  const [restart, setRestart] = useState(preset.restart ?? "unless-stopped");
  const [replace, setReplace] = useState(!!presetName);
  const [secrets, setSecrets] = useState<{ env: string; secret: string }[]>([]);
  const [known, setKnown] = useState<string[]>([]);
  useEffect(() => { api.appSecrets().then((r) => setKnown(r.secrets.map((x) => x.name))).catch(() => {}); }, []);
  return (
    <Modal wide title={preset.updating ? t("更新 {name}：拉取新镜像并按原设置重建", { name: presetName ?? "" }) : preset.editing ? t("修改 {name} 的设置", { name: presetName ?? "" }) : presetName ? t("重新部署 {name}", { name: presetName }) : t("部署容器")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={!name.trim() || !image.trim()}
        onClick={() => onSubmit({ name: name.trim(), image: image.trim(), ports: lines(ports), env: lines(env), volumes: lines(volumes), restart, replace, secrets: secrets.filter((x) => x.env && x.secret) })}>{t("提交")}</button></>}>
      {preset.editing && (
        <div className="note warn"><TriangleAlert /><span>
          {t("Docker 不能直接改一个已有容器的端口、挂载这些设置：提交后会删除这个容器，再用下面的设置重新创建。数据卷和挂载的目录不受影响，容器自己文件系统里的改动会丢失。")}
          {preset.project ? " " + t("这个容器由 Compose 管理（项目 {p}）：在这里重建会让它脱离 Compose，建议改 Compose 文件后再更新。", { p: preset.project }) : ""}
        </span></div>
      )}
      {preset.editing && !!preset.hidden?.length && (
        <div className="note crit"><TriangleAlert /><span>{t("这些环境变量的值是隐藏的，带不过来：")}<b className="mono">{preset.hidden.join("、")}</b>{t("。请在“环境变量”里重新填写，或在下面用应用密钥注入，否则新容器里就没有它们了。")}</span></div>
      )}
      {preset.editing && !!preset.lost?.length && (
        <div className="note"><TriangleAlert /><span>{t("这些设置这里改不了，重建后不会保留：")}<span className="mono">{preset.lost.join("；")}</span></span></div>
      )}
      <div className="form-grid">
        <div className="field"><label htmlFor="d-name">{t("容器名")}</label><input id="d-name" className="input mono" value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} placeholder="web" /></div>
        <div className="field"><label htmlFor="d-restart">{t("重启策略")}</label>
          <Select id="d-restart" value={restart} onChange={setRestart} options={[
            { value: "unless-stopped", label: t("除非手动停止"), hint: t("推荐") }, { value: "always", label: t("总是重启") }, { value: "on-failure", label: t("出错时重启") }, { value: "no", label: t("不重启") }]} />
        </div>
      </div>
      <div className="field"><label htmlFor="d-image">{t("镜像地址")}</label><input id="d-image" className="input mono" value={image} onChange={(e) => setImage(e.target.value)} spellCheck={false} placeholder="nginx:1.27" /><span className="hint">{t("会先拉取这个镜像，再用它运行容器。")}</span></div>
      <div className="form-grid">
        <div className="field"><label htmlFor="d-ports">{t("端口映射（每行一条）")}</label><textarea id="d-ports" className="textarea lines" rows={3} value={ports} onChange={(e) => setPorts(e.target.value)} spellCheck={false} placeholder={"8080:80\n127.0.0.1:5432:5432"} /></div>
        <div className="field"><label htmlFor="d-vols">{t("挂载（每行一条）")}</label><textarea id="d-vols" className="textarea lines" rows={3} value={volumes} onChange={(e) => setVolumes(e.target.value)} spellCheck={false} placeholder={"/opt/app/data:/data\nappcache:/cache"} /></div>
      </div>
      <div className="field"><label htmlFor="d-env">{t("环境变量（每行一条）")}</label><textarea id="d-env" className="textarea lines" rows={3} value={env} onChange={(e) => setEnv(e.target.value)} spellCheck={false} placeholder={"APP_ENV=production\nLOG_LEVEL=info"} />
        <span className="hint">{t("这里填的值会出现在命令和审计里，不要放密码。需要保密的值用下面的“密钥”。")}</span></div>
      <div className="field">
        <label>{t("密钥（作为环境变量注入）")}</label>
        {secrets.map((row, i) => (
          <div key={i} className="row">
            <input className="input mono" style={{ flex: 1 }} value={row.env} placeholder={t("变量名，例如 DB_PASSWORD")} spellCheck={false} aria-label={t("变量名")}
              onChange={(e) => setSecrets(secrets.map((x, j) => (j === i ? { ...x, env: e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "_") } : x)))} />
            <div style={{ flex: 1, minWidth: 0 }}><Select mono value={row.secret} placeholder={t("选择密钥")} options={known.map((k) => ({ value: k, label: k }))}
              onChange={(v) => setSecrets(secrets.map((x, j) => (j === i ? { ...x, secret: v, env: x.env || v.toUpperCase().replace(/[^A-Z0-9_]/g, "_") } : x)))} /></div>
            <button className="btn ghost icon sm" aria-label={t("移除")} title={t("移除")} onClick={() => setSecrets(secrets.filter((_, j) => j !== i))}><Trash2 /></button>
          </div>
        ))}
        <div className="row">
          <button type="button" className="btn sm" disabled={known.length === 0} onClick={() => setSecrets([...secrets, { env: "", secret: "" }])}><Plus />{t("添加密钥")}</button>
          <span className="hint">{known.length === 0 ? <>{t("还没有应用密钥，先到 ")}<Link to="/credentials" style={{ color: "var(--accent)" }}>{t("凭据")}</Link>{t(" 页添加。")}</> : t("值不会出现在命令和审计里：部署时由塔台写成主机上只有登录账号可读的环境文件。")}</span>
        </div>
      </div>
      <label className="check"><input type="checkbox" checked={replace} onChange={(e) => setReplace(e.target.checked)} />{t("如果同名容器已存在，先删除它再部署（更新时勾选）")}</label>
    </Modal>
  );
}

function UploadDialog({ host, onClose, onSubmit }: { host: string; onClose: () => void; onSubmit: (uploadId: string) => void }) {
  const t = useT();
  const [file, setFile] = useState<File | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const start = async () => {
    if (!file) return;
    setProgress(0);
    try {
      const info = await api.upload(file, setProgress);
      onSubmit(info.id);
    } catch (e) { fail(e); setProgress(null); }
  };
  return (
    <Modal title={t("上传镜像包")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose} disabled={progress !== null}>{t("取消")}</button><button className="btn primary" disabled={!file || progress !== null} onClick={start}>{progress !== null ? t("上传中 {n}%", { n: (progress * 100).toFixed(0) }) : t("上传并提交")}</button></>}>
      <p className="dim">{t("选择 ")}<span className="mono">docker save</span>{t(" 导出的镜像包（")}<span className="mono">.tar</span>{t(" 或 ")}<span className="mono">.tar.gz</span>{t("，最大 4 GB）。文件先暂存到塔台，批准后再流式送到 ")}<b>{host}</b>{t(" 上执行 ")}<span className="mono">docker load</span>{t("。")}</p>
      <input ref={input} type="file" accept=".tar,.gz,.tgz,application/x-tar,application/gzip" hidden onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
      <button className="btn" onClick={() => input.current?.click()} disabled={progress !== null}><Upload />{file ? t("换一个文件") : t("选择文件")}</button>
      {file && <div className="row"><Layers size={15} className="muted" /><span className="mono ellipsis grow">{file.name}</span><span className="muted num">{bytes(file.size)}</span></div>}
      {progress !== null && <div className="progress" role="progressbar" aria-valuenow={Math.round(progress * 100)} aria-valuemin={0} aria-valuemax={100}><div style={{ width: `${progress * 100}%` }} /></div>}
    </Modal>
  );
}

function RegistriesDialog({ host, canLogin, onClose, onLogin }: { host: string; canLogin: boolean; onClose: () => void; onLogin: (name: string) => void }) {
  const t = useT();
  const [list, setList] = useState<Registry[]>([]);
  const [available, setAvailable] = useState(true);
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ name: "", server: "docker.io", username: "", password: "" });
  const [busy, setBusy] = useState(false);
  const load = useCallback(() => { api.registries().then((r) => { setList(r.registries); setAvailable(r.available); }).catch(fail); }, []);
  useEffect(load, [load]);
  const save = async () => {
    setBusy(true);
    try {
      await api.putRegistry(form);
      toast(t("仓库凭据已保存"));
      setAdding(false);
      setForm({ name: "", server: "docker.io", username: "", password: "" });
      load();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <Modal title={t("镜像仓库")} onClose={onClose} footer={<button className="btn" onClick={onClose}>{t("关闭")}</button>}>
      <p className="dim">{t("仓库的密码用信封加密保存在塔台，保存后任何界面都取不回。“在这台主机上登录”会生成一个请求：密码从标准输入送给 ")}<span className="mono">docker login</span>{t("，不出现在命令行和审计里。")}</p>
      {!available && <div className="note warn"><TriangleAlert /><span>{t("没有配置主密钥，凭据功能不可用（见“设置”页的说明）。")}</span></div>}
      {list.length === 0 && !adding && <span className="muted">{t("还没有保存任何仓库。拉取公开镜像不需要登录。")}</span>}
      {list.map((r) => (
        <div key={r.name} className="row">
          <KeyRound size={15} className="muted" />
          <div className="cell-2 grow"><span style={{ fontWeight: 600 }}>{r.name}</span><small className="mono ellipsis">{r.username}@{r.server}</small></div>
          <button className="btn sm" disabled={!canLogin} title={canLogin ? "" : t("当前主机上用不了 Docker")} onClick={() => onLogin(r.name)}><LogIn />{t("在 {host} 上登录", { host })}</button>
          <button className="btn ghost icon sm" aria-label={t("删除 {name}", { name: r.name })} title={t("删除")} onClick={async () => (await askConfirm(t("删除仓库凭据 {name}？", { name: r.name }))) && api.deleteRegistry(r.name).then(load).catch(fail)}><Trash2 /></button>
        </div>
      ))}
      {adding ? (
        <>
          <div className="form-grid">
            <div className="field"><label htmlFor="r-name">{t("名称")}</label><input id="r-name" className="input mono" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} spellCheck={false} placeholder="hub" /></div>
            <div className="field"><label htmlFor="r-server">{t("仓库地址")}</label><input id="r-server" className="input mono" value={form.server} onChange={(e) => setForm({ ...form, server: e.target.value })} spellCheck={false} placeholder="docker.io" /></div>
            <div className="field"><label htmlFor="r-user">{t("用户名")}</label><input id="r-user" className="input mono" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} spellCheck={false} autoComplete="off" /></div>
            <div className="field"><label htmlFor="r-pass">{t("密码或访问令牌")}</label><input id="r-pass" className="input mono" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} autoComplete="new-password" /></div>
          </div>
          <span className="hint">{t("Docker Hub 的地址填 ")}<span className="mono">docker.io</span>{t("；建议用只读的访问令牌，而不是账号密码。")}</span>
          <div className="row" style={{ justifyContent: "flex-end" }}><button className="btn" onClick={() => setAdding(false)}>{t("取消")}</button><button className="btn primary" disabled={busy || !form.name || !form.server || !form.username || !form.password} onClick={save}>{t("加密保存")}</button></div>
        </>
      ) : <button className="btn" disabled={!available} onClick={() => setAdding(true)} style={{ alignSelf: "flex-start" }}><Plus />{t("添加仓库")}</button>}
    </Modal>
  );
}

function LogsDialog({ host, name, onClose }: { host: string; name: string; onClose: () => void }) {
  const t = useT();
  const [logs, setLogs] = useState<string | null>(null);
  const load = useCallback(() => { setLogs(null); api.containerLogs(host, name).then((r) => setLogs(r.logs)).catch((e) => { fail(e); setLogs(""); }); }, [host, name]);
  useEffect(load, [load]);
  return (
    <Modal wide title={t("{name} 的日志（最后 300 行）", { name })} onClose={onClose} footer={<><button className="btn" onClick={load}><RefreshCw />{t("刷新")}</button><button className="btn" onClick={onClose}>{t("关闭")}</button></>}>
      {logs === null ? <div className="muted">{t("加载中…")}</div> : <pre className="term" data-empty={t("（没有输出）")} style={{ maxHeight: "60vh" }}>{logs}</pre>}
      <span className="hint">{t("看起来像凭据的内容已经遮掉。")}</span>
    </Modal>
  );
}

/** 账号没有权限访问 Docker 时的修复办法：把它加进 docker 组。面板不替你做（那是给自己提权），只给出命令。 */
function DockerFix({ host, user, local, onReconnected }: { host: string; user: string; local: boolean; onReconnected: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [busy, setBusy] = useState(false);
  // 远程主机：重新登录一次就能拿到新的组，不用重启 Hostower。
  const reconnect = async () => {
    setBusy(true);
    try { await api.hostReconnect(host); toast(t("已重新连接 {host}", { host })); onReconnected(); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const cmd = `sudo usermod -aG docker ${user}` + (local ? " && sudo systemctl restart hostower" : "");
  return (
    <div className="docker-fix">
      <span>{t("Hostower 在这台主机上用的账号是 {user}，它不在 docker 组里。用 root 在这台主机上执行：", { user })}</span>
      <div className="row docker-fix-cmd">
        <code className="mono">{cmd}</code>
        <button className="btn ghost icon sm" title={t("复制")} aria-label={t("复制")} onClick={() => navigator.clipboard?.writeText(cmd).then(() => toast(t("已复制"))).catch(fail)}><Copy /></button>
      </div>
      <span className="hint">{local
        ? t("加进组之后要重启 Hostower 才生效（上面的命令已经带上）。")
        : t("加进组之后，点下面的“重新连接”让 Hostower 重新登录这台主机，新的组就生效了。")}
        {" "}{t("注意：docker 组等同 root 权限。面板里启停、部署容器照样要你审批。")}</span>
      {!local && owner && <button className="btn sm" style={{ alignSelf: "flex-start" }} disabled={busy} onClick={reconnect}><RefreshCw className={busy ? "spin" : undefined} />{t("重新连接")}</button>}
    </div>
  );
}
