import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { askConfirm } from "../dialogs";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ChevronDown, ChevronRight, CornerLeftUp, Cpu, Gauge, Download, File, Folder, FolderSearch, HardDrive, MemoryStick, Pencil, RefreshCw, Search, Server, ShieldAlert, SquareTerminal, Trash2, TriangleAlert, Upload } from "lucide-react";
import { api, type ContainerStat, type FileEntry, type Hardware, type ProcStat, type Req, type Retention, type Sample, type SpeedConfig } from "../api";
import { cached, remember } from "../cache";
import { fail, refreshHosts, toast, useStore } from "../store";
import { HostForm } from "../HostForm";
import { SpeedtestDialog } from "./Speedtest";
import { ContainerDetailDialog } from "./Containers";
import { NetworkPanel } from "./Network";
import { TrafficCard } from "./Traffic";
import { BinaryPreview, TextPreview, previewKind, textKinds, type PreviewKind } from "../preview";
import { Empty, LevelBadge, Pager, poll, SortTh, Sparkline, StateBadge, TrendChart, Usage, ago, bytes, stamp, rate, uptime, usePaged, useSort, useSteadyOrder, type Series } from "../ui";
import { lang, ts, useT } from "../i18n";

// days 是这个范围覆盖多少天：超过 15 分钟级数据的保留时长的范围不显示。
const ranges = [
  { key: "live", label: "实时", days: 0 }, { key: "1h", label: "1 小时", days: 0 }, { key: "6h", label: "6 小时", days: 0 },
  { key: "24h", label: "24 小时", days: 1 }, { key: "7d", label: "7 天", days: 7 }, { key: "30d", label: "30 天", days: 30 },
  { key: "90d", label: "90 天", days: 90 }, { key: "1y", label: "1 年", days: 365 },
];

const pct = (n: number) => n.toFixed(1) + "%";
const num2 = (n: number) => n.toFixed(2);
const deg = (n: number) => n.toFixed(0) + " °C";
const sCPU: Series[] = [{ pick: (s) => s.cpu }];
const sMem: Series[] = [{ pick: (s) => s.mem, label: "已用" }, { pick: (s) => s.cache ?? 0, label: "缓存" }];
const sLoad: Series[] = [{ pick: (s) => s.load, label: "1 分钟" }, { pick: (s) => s.load15 ?? 0, label: "15 分钟" }];
const sDisk: Series[] = [{ pick: (s) => s.dr ?? 0, label: "读" }, { pick: (s) => s.dw ?? 0, label: "写" }];
const sNet: Series[] = [{ pick: (s) => s.rx, label: "入站" }, { pick: (s) => s.tx, label: "出站" }];
// 卡片右上角的大数字是两条线的合计；下面的最低、平均、最高也按合计算，三处才对得上。
const netTotal = (s: Sample) => s.rx + s.tx;
const diskTotal = (s: Sample) => (s.dr ?? 0) + (s.dw ?? 0);
const sTemp: Series[] = [{ pick: (s) => s.temp ?? 0 }];
const sRoot: Series[] = [{ pick: (s) => s.disk }];

export function HostDetail() {
  const t = useT();
  const { name = "" } = useParams();
  const host = useStore((s) => s.hosts.find((h) => h.name === name));
  const loaded = useStore((s) => s.hostsLoaded);
  const requests = useStore((s) => s.requests);
  // 分页记在地址里（?tab=network）：应用页可以直接跳到“网络”。
  const [sp, setSp] = useSearchParams();
  const tab = (["files", "network"].includes(sp.get("tab") ?? "") ? sp.get("tab") : "overview") as "overview" | "files" | "network";
  const setTab = (v: "overview" | "files" | "network") => setSp(v === "overview" ? {} : { tab: v }, { replace: true });
  const [editing, setEditing] = useState(false);
  const [speed, setSpeed] = useState<SpeedConfig | null>(null);
  const nav = useNavigate();
  // 时间范围：实时用内存里的最近采样；其余从落盘的历史里取，每分钟刷新一次。
  const [range, setRange] = useState(() => localStorage.getItem("hostower.range") ?? "1h");
  const [past, setPast] = useState<Sample[] | null>(null);
  const [pastErr, setPastErr] = useState("");
  // 保留时长决定能选哪些范围、每个点是几分钟的平均。
  const [keep, setKeep] = useState<Retention>(() => cached<Retention>("retention") ?? { fineDays: 1, coarseDays: 35 });
  useEffect(() => { api.retention().then((r) => setKeep(remember("retention", r.retention))).catch(() => {}); }, []);
  useEffect(() => {
    localStorage.setItem("hostower.range", range);
    if (range === "live") return;
    let on = true;
    setPast(null);
    setPastErr("");
    // 刷新失败（网络断了、SSH 隧道掉了、电脑休眠过）时保留已经画出来的数据，只提示加载失败，
    // 不要把图表清空成“还没有足够的历史”。
    const load = () => api.hostMetrics(name, range)
      .then((r) => { if (on) { setPast(r.samples); setPastErr(""); } })
      .catch((e) => { if (on) { setPast((p) => p ?? []); setPastErr(e instanceof Error ? e.message : String(e)); } });
    load();
    const stop = poll(load, 60_000);
    return () => { on = false; stop(); };
  }, [name, range]);

  const remove = async () => {
    if (!(await askConfirm(t("从清单里删除主机 {name}？这只是让塔台不再管理它，不会动主机本身。", { name })))) return;
    try {
      await api.deleteHost(name);
      await refreshHosts();
      toast(t("主机已删除"));
      nav("/");
    } catch (e) { fail(e); }
  };

  if (!host) {
    return (
      <>
        <div className="topbar"><Link to="/" className="crumb">{t("总览")}</Link><ChevronRight size={14} className="crumb-sep" /><h1>{name}</h1></div>
        <div className="content">{loaded ? <Empty icon={<TriangleAlert />} title={t("清单里没有这台主机")} /> : null}</div>
      </>
    );
  }
  const st = host.status, l = st.latest;
  const hist = range === "live" ? st.history ?? [] : past ?? [];
  const recent = requests.filter((r) => r.targets.some((tg) => tg.host === name)).slice(0, 8);

  return (
    <>
      <div className="topbar">
        <Link to="/" className="crumb">{t("总览")}</Link>
        <ChevronRight size={14} className="crumb-sep" />
        <h1>{host.name}</h1>
        <span className={"badge " + (!st.known ? "neutral" : st.online ? "ok" : "crit")}>
          <span className={"dot " + (!st.known ? "" : st.online ? "ok" : "crit")} style={{ boxShadow: "none" }} />
          {!st.known ? t("尚未采样") : st.online ? t("在线") : t("离线")}
        </span>
        {host.sensitivity === "high" && <span className="badge serious"><ShieldAlert />{t("高敏感")}</span>}
        <span className="grow" />
        {host.source !== "file" && <button className="btn" onClick={() => setEditing(true)}><Pencil />{t("编辑")}</button>}
        {host.source === "managed" && <button className="btn danger icon" onClick={remove} title={t("删除主机")} aria-label={t("删除主机")}><Trash2 /></button>}
        <button className="btn hide-sm" disabled={!host.status.online} onClick={() => api.speedtest().then(setSpeed).catch(fail)}><Gauge />{t("测速")}</button>
        <Link className="btn hide-sm" to={`/containers?host=${host.name}`}>{t("容器")}</Link>
        <Link className="btn primary" to={`/hosts/${host.name}/terminal`}><SquareTerminal />{t("终端")}</Link>
      </div>
      {editing && <HostForm host={host} onClose={() => setEditing(false)} />}
      {speed && <SpeedtestDialog hosts={[host.name]} config={speed} onClose={() => setSpeed(null)} />}
      <div className="content">
        <div className="page">
          <div className="tabs" role="tablist">
            <button role="tab" aria-selected={tab === "overview"} className={"tab" + (tab === "overview" ? " active" : "")} onClick={() => setTab("overview")}>{t("概览")}</button>
            <button role="tab" aria-selected={tab === "files"} className={"tab" + (tab === "files" ? " active" : "")} onClick={() => setTab("files")}>{t("文件")}</button>
            <button role="tab" aria-selected={tab === "network"} className={"tab" + (tab === "network" ? " active" : "")} onClick={() => setTab("network")}>{t("网络")}</button>
          </div>

          {tab === "overview" && (
            <>
              {st.known && !st.online && (
                <div className="note crit" role="alert"><TriangleAlert /><span>{t("无法连接这台主机：{error}", { error: ts(st.error) })}</span></div>
              )}

              <div className="card">
                <div className="facts">
                  <Fact label={t("地址")}><span className="mono">{host.addr}</span></Fact>
                  <Fact label={t("系统@@操作系统")}>{/* 读不到发行版名字时（比如 Docker Desktop 的虚拟机），和总览一样退到内核版本 */}<span title={st.os ?? st.kernel}>{st.os ?? st.kernel ?? "—"}</span></Fact>
                  <Fact label={t("内核")}><span className="mono">{st.kernel?.replace(/^Linux /, "") ?? "—"}</span></Fact>
                  <Fact label={t("已运行")}>{uptime(st.uptimeSec)}</Fact>
                  <Fact label={t("规格")}>{st.ncpu ? t("{n} 核 · {mem} 内存", { n: st.ncpu, mem: bytes((st.memKB ?? 0) * 1024) }) : "—"}</Fact>
                  <Fact label={t("连接方式")}>{host.executor === "sim" ? t("模拟主机") : host.executor === "local" ? t("本机直连") : host.executor.toUpperCase()}{host.credential ? <span className="mono muted">　{host.credential}</span> : null}</Fact>
                  <Fact label={t("敏感度")}>{t({ low: "低", normal: "普通", high: "高（写操作升一级）" }[host.sensitivity])}</Fact>
                  <Fact label={t("来源")}>{host.source === "managed" ? t("在页面上管理") : host.source === "builtin" ? t("内置（Hostower 所在的机器）") : t("配置文件（只读）")}</Fact>
                  <Fact label={t("标签")} wide>
                    <div className="row wrap" style={{ gap: 4 }}>{host.tags.length ? host.tags.map((tag) => <span key={tag} className="tag">{tag}</span>) : <span className="muted">{t("没有标签")}</span>}</div>
                  </Fact>
                  <Fact label={t("进程数")}>{st.procs ?? "—"}</Fact>
                  <Fact label={t("最近采样")}>{ago(st.lastSeen)}</Fact>
                </div>
              </div>

              {st.hardware && <HardwareCard hw={st.hardware} memKB={st.memKB} swapKB={st.swapKB} />}

              <div className="row wrap">
                <div className="row wrap" role="group" aria-label={t("时间范围")} style={{ gap: 4 }}>
                  {ranges.filter((r) => r.days <= keep.coarseDays).map((r) => <button key={r.key} className={"tag" + (range === r.key ? " on" : "")} onClick={() => setRange(r.key)}>{t(r.label)}</button>)}
                </div>
                <span className="muted" style={{ fontSize: 12.5 }}>
                  {range === "live" ? t("内存里最近的采样，重启后清空") : past === null ? t("加载中…") : pastErr ? t("历史数据刷新失败，稍后自动重试：{error}", { error: pastErr }) : past.length < 2 ? t("这个范围里还没有足够的历史：塔台每分钟存一个点，运行一会儿就有了") : (ranges.find((r) => r.key === range)?.days ?? 0) > keep.fineDays ? t("每个点是 15 分钟或更长时间的平均值") : t("每个点是 1 分钟或更长时间的平均值")}
                </span>
              </div>

              <div className="metric-grid">
                <MetricCard title="CPU" desc={st.ncpu ? t("{n} 核的平均利用率", { n: st.ncpu }) : t("平均利用率")} value={l ? pct(l.cpu) : "—"} samples={hist} series={sCPU} format={pct} max={100} />
                <MetricCard title={t("内存")} desc={l && st.memKB ? t("已用 {used} / {total}", { used: bytes(st.memKB * 1024 * l.mem / 100), total: bytes(st.memKB * 1024) }) + (st.swapKB ? t("，交换分区 {n}%", { n: (l.swap ?? 0).toFixed(0) }) : "") : t("已用与缓存")}
                  value={l ? pct(l.mem) : "—"} now={l} samples={hist} series={sMem} format={pct} max={100} />
                <MetricCard title={t("负载")} desc={st.ncpu ? t("超过 {n}（核数）说明在排队", { n: st.ncpu }) : t("运行和等待中的进程数")} value={l ? l.load.toFixed(2) : "—"} now={l} samples={hist} series={sLoad} format={num2} />
                <MetricCard title={t("磁盘读写")} desc={t("所有磁盘合计的吞吐")} value={l ? rate(diskTotal(l)) : "—"} now={l} samples={hist} series={sDisk} stat={diskTotal} format={rate} />
                <MetricCard title={t("网络")} desc={t("出口网卡合计（隧道、网桥不重复计算）")} value={l ? rate(netTotal(l)) : "—"} now={l} samples={hist} series={sNet} stat={netTotal} format={rate} />
                {hist.some((x) => x.temp > 0)
                  ? <MetricCard title={t("温度")} desc={t("各传感器里最高的一个")} value={l ? `${l.temp.toFixed(0)} °C` : "—"} samples={hist} series={sTemp} format={deg} />
                  : <MetricCard title={t("根分区")} desc={l && st.diskKB ? t("已用 {used} / {total}", { used: bytes(st.diskKB * 1024 * l.disk / 100), total: bytes(st.diskKB * 1024) }) : t("占用")} value={l ? pct(l.disk) : "—"} samples={hist} series={sRoot} format={pct} max={100} />}
              </div>

              <TrafficCard host={host.name} status={st.quota} />

              {/* 分区通常只有几行，排成多列；容器占用是一张多列的表，单独占一整行，容器名不用截断。 */}
              <div className="card">
                <div className="card-head"><h2>{t("分区")}</h2><span className="muted num">{st.disks?.length || ""}</span></div>
                {!st.disks?.length ? <div className="empty sm"><span>{t("还没有分区数据")}</span></div> : (
                  <div className="card-body disk-grid">
                    {st.disks.map((d) => (
                      <div key={d.device + d.mount} className="disk-row">
                        <div className="cell-2"><span className="mono ellipsis" style={{ fontWeight: 600 }} title={d.mount}>{d.mount}</span><small className="mono ellipsis" title={d.device}>{d.device}</small></div>
                        <Usage value={d.used} totalKB={d.totalKB} />
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <ContainerUsage host={host.name} list={st.containers ?? []} />

              <ProcessCard host={host.name} executor={host.executor} agent={st.agent} procs={st.topProcs ?? []} online={!!st.online} />

              <div className="card">
                <div className="card-head"><h2>{t("最近操作")}</h2><span className="muted num">{recent.length || ""}</span></div>
                {recent.length === 0 ? (
                  <div className="empty sm"><SquareTerminal /><span>{t("这台主机还没有操作记录")}</span></div>
                ) : (
                  <div className="table-wrap">
                    <table className="table recent-table">
                      <thead><tr><th>{t("等级")}</th><th>{t("操作")}</th><th className="hide-sm">{t("发起人")}</th><th>{t("状态")}</th><th className="hide-sm">{t("时间")}</th></tr></thead>
                      <tbody>
                        {recent.map((r) => (
                          <RecentRow key={r.id} id={r.id} level={<LevelBadge level={r.level} />} state={<StateBadge state={r.state} />} actor={r.actor.name} ai={r.actor.kind === "ai"}
                            command={r.kind === "command" ? r.targets.find((tg) => tg.host === name)?.command ?? "" : ts(r.targets.find((tg) => tg.host === name)?.command)} intent={r.intent} when={ago(r.createdAt)} at={stamp(r.createdAt)} />
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </>
          )}

          {tab === "files" && <Files host={host.name} />}
          {tab === "network" && <NetworkPanel host={host.name} />}
        </div>
      </div>
    </>
  );
}

/** 最近操作的一行：先写“做什么”（意图），下面一行是实际命令；没有意图时命令作为主文字。 */
function RecentRow({ id, level, state, command, intent, when, at, actor, ai }: { id: string; level: React.ReactNode; state: React.ReactNode; command: string; intent: string; when: string; at: string; actor: string; ai: boolean }) {
  useT();
  const nav = useNavigate();
  const title = intent ? ts(intent) : command;
  return (
    <tr className="click" onClick={() => nav(`/requests/${id}`)}>
      <td>{level}</td>
      <td>
        <div className="ellipsis recent-title" title={title}>{title}</div>
        {intent && command && <div className="mono muted ellipsis recent-cmd" title={command}>{command}</div>}
      </td>
      <td className="hide-sm"><div className="ellipsis">{actor}{ai && <span className="muted">{" · AI"}</span>}</div></td>
      <td>{state}</td>
      <td className="hide-sm muted num" title={at}>{when}</td>
    </tr>
  );
}

function Files({ host }: { host: string }) {
  const t = useT();
  const [path, setPath] = useState("/");
  const [entries, setEntries] = useState<FileEntry[] | null>(null);
  const [error, setError] = useState("");
  const [file, setFile] = useState<{ path: string; content?: string; binary?: boolean; size: number; truncated: boolean } | null>(null);
  // 能预览的文件：kind 是预览方式；source 为真时看的是源码（只对 Markdown、CSV 这类文本有意义）。
  const [kind, setKind] = useState<PreviewKind | null>(null);
  const [source, setSource] = useState(false);
  const [draft, setDraft] = useState<string | null>(null);
  const [intent, setIntent] = useState("");
  const [busy, setBusy] = useState(false);
  const nav = useNavigate();
  const picker = useRef<HTMLInputElement>(null);
  // 筛选当前目录（边输入边筛）；按回车在子目录里按名字搜索。
  const [word, setWord] = useState("");
  const [found, setFound] = useState<{ word: string; paths: string[]; truncated: boolean } | null>(null);
  const [searching, setSearching] = useState(false);
  const search = async () => {
    const w = word.trim();
    if (!w) { setFound(null); return; }
    setSearching(true);
    try { const r = await api.searchFiles(host, path, w); setFound({ word: w, ...r }); } catch (e) { fail(e); } finally { setSearching(false); }
  };
  // 搜索结果不知道是文件还是目录：先当目录打开，不行再当文件打开。
  const goto = (p: string) => {
    api.listDir(host, p).then((r) => { setFound(null); setWord(""); setEntries(r.entries); setPath(r.path); })
      .catch(() => { const dir = p.slice(0, p.lastIndexOf("/")) || "/"; setFound(null); setWord(""); load(dir); open(p); });
  };
  const [dragging, setDragging] = useState(false);
  const [sending, setSending] = useState<{ name: string; fraction: number; index: number; total: number } | null>(null);

  // 上传到当前目录：文件先暂存到 Hostower，再各生成一个“写入主机”的请求，批准后才真的写过去。
  const upload = async (list: FileList | File[]) => {
    const files = [...list].filter((f) => f.size > 0);
    if (!files.length || sending) return;
    const made: string[] = [];
    try {
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        setSending({ name: f.name, fraction: 0, index: i + 1, total: files.length });
        const info = await api.upload(f, (fraction) => setSending({ name: f.name, fraction, index: i + 1, total: files.length }));
        made.push((await api.uploadFile(host, path, f.name, info.id)).id);
      }
    } catch (e) { fail(e); } finally { setSending(null); }
    if (made.length === 1) nav(`/requests/${made[0]}`);
    else if (made.length > 1) { toast(t("已生成 {n} 个上传请求，批准后写入主机", { n: made.length })); nav("/approvals"); }
  };
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files.length) upload(e.dataTransfer.files);
  };

  const load = useCallback((p: string) => {
    setError("");
    setEntries(null);
    setFound(null);
    setWord("");
    api.listDir(host, p).then((r) => { setEntries(r.entries); setPath(r.path); }).catch((e) => { setError(e.message); setEntries([]); });
  }, [host]);
  useEffect(() => { load("/"); }, [load]);

  const open = (p: string) => {
    setDraft(null);
    setIntent("");
    setSource(false);
    const k = previewKind(p);
    setKind(k);
    // 二进制格式（图片、PDF、Office）不走“读文本”的接口：预览组件自己去取，这里只记下路径和大小。
    if (k && !textKinds.includes(k)) {
      const size = entries?.find((e) => join(e.name) === p)?.size ?? 0;
      setFile({ path: p, size, truncated: false, binary: true });
      return;
    }
    api.readFile(host, p).then(setFile).catch(fail);
  };
  const join = (n: string) => (path === "/" ? "/" + n : path + "/" + n);
  const parts = path.split("/").filter(Boolean);

  const shownEntries = useMemo(() => {
    const w = word.trim().toLowerCase();
    return !entries ? [] : !w ? entries : entries.filter((e) => e.name.toLowerCase().includes(w));
  }, [entries, word]);

  const save = async () => {
    if (!file || draft === null) return;
    setBusy(true);
    try {
      const r = await api.writeFile(host, file.path, draft, intent);
      nav(`/requests/${r.id}`);
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <div className="split even files">
      <div className={"card drop-zone" + (dragging ? " dragging" : "")}
        onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragging(true); } }}
        onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setDragging(false); }}
        onDrop={onDrop}>
        {dragging && <div className="drop-hint"><Upload /><strong>{t("松开鼠标，上传到 {path}", { path })}</strong><span>{t("会生成一个请求，批准后才写入主机")}</span></div>}
        <div className="card-head">
          <div className="row grow mono" style={{ gap: 2, overflow: "hidden" }}>
            <button className="btn ghost sm" onClick={() => load("/")}>/</button>
            {parts.map((p, i) => (
              <span key={i} className="row" style={{ gap: 2 }}>
                <button className="btn ghost sm" onClick={() => load("/" + parts.slice(0, i + 1).join("/"))}>{p}</button>
                {i < parts.length - 1 && <span className="muted">/</span>}
              </span>
            ))}
          </div>
          <input ref={picker} type="file" multiple hidden onChange={(e) => { if (e.target.files) upload(e.target.files); e.target.value = ""; }} />
          <button className="btn sm" disabled={!!sending} onClick={() => picker.current?.click()} title={t("上传文件到当前目录；也可以把文件拖到这里")}><Upload /><span className="hide-sm">{t("上传")}</span></button>
          <button className="btn ghost icon sm" onClick={() => load(path)} title={t("刷新")} aria-label={t("刷新")}><RefreshCw /></button>
        </div>
        {sending && (
          <div className="upload-bar">
            <span className="ellipsis">{t("正在上传 {name}", { name: sending.name })}{sending.total > 1 ? `（${sending.index}/${sending.total}）` : ""}</span>
            <span className="num">{Math.round(sending.fraction * 100)}%</span>
            <div className="upload-track"><div style={{ width: `${Math.round(sending.fraction * 100)}%` }} /></div>
          </div>
        )}
        <div className="file-tools">
          <div className="search grow">
            <Search />
            <input className="input" value={word} placeholder={t("筛选这个目录；回车在子目录里搜索")} aria-label={t("筛选或搜索文件")}
              onChange={(e) => { setWord(e.target.value); if (found) setFound(null); }} onKeyDown={(e) => e.key === "Enter" && search()} />
          </div>
          <button className="btn sm" disabled={!word.trim() || searching} onClick={search} title={t("在 {path} 和它下面六层以内按名字搜索", { path })}>{searching ? <RefreshCw className="spin" /> : <FolderSearch />}<span className="hide-sm">{t("搜索子目录")}</span></button>
          {entries && !found && <span className="muted num" style={{ fontSize: 12 }}>{word.trim() ? `${shownEntries.length} / ${entries.length}` : entries.length}</span>}
        </div>
        {error && <div className="card-body"><div className="note crit"><TriangleAlert /><span>{error}</span></div></div>}
        {found ? (
          <div className="file-list">
            <div className="file-found">{t("在 {path} 下找到 {n} 项名字里有“{word}”的", { path, n: found.paths.length, word: found.word })}{found.truncated ? t("（只显示前 200 项）") : ""}<span className="grow" /><button className="btn ghost sm" onClick={() => { setFound(null); setWord(""); }}>{t("返回目录")}</button></div>
            {found.paths.map((p) => (
              <div key={p} className={"host-row" + (file?.path === p ? " active" : "")} onClick={() => goto(p)}>
                <Search size={15} className="muted" />
                <span className="cell-2"><span className="ellipsis mono">{p.slice(p.lastIndexOf("/") + 1)}</span><small className="ellipsis mono">{p.slice(0, p.lastIndexOf("/")) || "/"}</small></span>
                <span />
              </div>
            ))}
            {found.paths.length === 0 && <div className="empty sm"><span>{t("没有找到")}</span></div>}
          </div>
        ) : entries === null ? <div className="empty">{t("加载中…")}</div> : (
          <div className="file-list">
            {path !== "/" && (
              <div className="host-row" onClick={() => load("/" + parts.slice(0, -1).join("/"))}>
                <CornerLeftUp size={15} className="muted" /><span className="muted">{t("上一级")}</span><span />
              </div>
            )}
            {shownEntries.map((e) => (
              <div key={e.name} className={"host-row" + (file?.path === join(e.name) ? " active" : "")} onClick={() => (e.dir ? load(join(e.name)) : open(join(e.name)))}>
                {e.dir ? <Folder size={15} style={{ color: "var(--accent)" }} /> : <File size={15} className="muted" />}
                <span className="ellipsis mono">{e.name}</span>
                <span className="row" style={{ gap: 6 }}>
                  <span className="muted num" style={{ fontSize: 12 }}>{e.dir ? "" : bytes(e.size)}</span>
                  {!e.dir && <a className="btn ghost icon sm row-dl" href={api.downloadURL(host, join(e.name))} download={e.name} onClick={(ev) => ev.stopPropagation()} title={t("下载 {name}", { name: e.name })} aria-label={t("下载 {name}", { name: e.name })}><Download /></a>}
                </span>
              </div>
            ))}
            {entries.length > 0 && shownEntries.length === 0 && <div className="empty sm"><span>{t("这个目录里没有名字匹配的；按回车可以搜索子目录")}</span></div>}
            {entries.length === 0 && !error && <div className="empty"><Upload /><strong>{t("空目录")}</strong><span>{t("把文件拖到这里，或点右上角的“上传”")}</span></div>}
          </div>
        )}
      </div>

      <div className="card">
        {!file ? (
          <Empty icon={<File />} title={t("选择一个文件查看")}>{t("读文件是只读操作，会自动执行并留痕；私钥和凭据文件是 L3，任何人都读不了。")}</Empty>
        ) : (
          <>
            <div className="card-head">
              <span className="mono ellipsis grow">{file.path}</span>
              <span className="muted num">{bytes(file.size)}</span>
              {kind && textKinds.includes(kind) && !file.binary && draft === null && (
                <div className="seg" role="tablist" aria-label={t("显示方式")}>
                  <button role="tab" aria-selected={!source} className={!source ? "on" : ""} onClick={() => setSource(false)}>{t("预览")}</button>
                  <button role="tab" aria-selected={source} className={source ? "on" : ""} onClick={() => setSource(true)}>{t("源码")}</button>
                </div>
              )}
              <a className="btn ghost icon sm" href={api.downloadURL(host, file.path)} download={file.path.slice(file.path.lastIndexOf("/") + 1)} title={t("下载")} aria-label={t("下载")}><Download /></a>
              {!file.binary && !file.truncated && draft === null && <button className="btn sm" onClick={() => setDraft(file.content ?? "")}><Pencil />{t("编辑")}</button>}
            </div>
            <div className="card-body col" style={{ gap: 12 }}>
              {kind && file.binary && !textKinds.includes(kind) ? (
                <BinaryPreview host={host} path={file.path} kind={kind} size={file.size} />
              ) : kind && textKinds.includes(kind) && !file.binary && !source && draft === null ? (
                <>
                  {file.truncated && <div className="note warn"><TriangleAlert /><span>{t("文件超过 1 MB，只预览开头部分。")}</span></div>}
                  <TextPreview kind={kind} text={file.content ?? ""} />
                </>
              ) : file.binary ? (
                <div className="note"><TriangleAlert /><span>{t("这是二进制文件，不在页面上显示。")}</span></div>
              ) : draft === null ? (
                <>
                  {file.truncated && <div className="note warn"><TriangleAlert /><span>{t("文件超过 1 MB，只显示开头部分，不能在页面上编辑。")}</span></div>}
                  <pre className="term" data-empty={t("（没有输出）")} style={{ maxHeight: 560 }}>{file.content}</pre>
                </>
              ) : (
                <>
                  <textarea className="textarea mono" rows={18} value={draft} onChange={(e) => setDraft(e.target.value)} spellCheck={false} aria-label={t("文件内容")} />
                  <div className="field">
                    <label htmlFor="intent">{t("为什么改（会写进审计）")}</label>
                    <input id="intent" className="input" value={intent} onChange={(e) => setIntent(e.target.value)} placeholder={t("例如：把监听端口改成 8080")} />
                  </div>
                  <div className="row between">
                    <span className="muted" style={{ fontSize: 12 }}>{t("保存会生成一个待批准的请求，批准前可以看到 diff；旧内容会先备份。")}</span>
                    <div className="row">
                      <button className="btn" onClick={() => setDraft(null)}>{t("取消")}</button>
                      <button className="btn primary" disabled={busy || draft === file.content} onClick={save}>{t("提交改动")}</button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

/** CPU 型号去掉商标符号和“CPU @ 2.00GHz”这类尾巴；基础频率单独返回。 */
function cpuName(model: string): { name: string; base?: string } {
  const [head, freq] = model.split(/\s+@\s+/);
  const name = head.replace(/\((R|TM|tm|r)\)/g, "").replace(/\s+(CPU|Processor)$/i, "").replace(/\s+/g, " ").trim();
  return { name: name || model, base: freq?.replace(/GHz/i, " GHz") };
}

// 每一段内部不折行（“48 MiB”不会被拆成两行），只在段与段之间换行。
const keep = (s: string) => s.replace(/ /g, String.fromCharCode(160)); // 不间断空格

const ghz = (mhz: number) => (mhz / 1000).toFixed(mhz >= 10000 ? 0 : 2).replace(/\.?0+$/, "") + " GHz";

/** 硬件：CPU、内存、整机三块并排，下面是磁盘。只显示读得到的，读不到的那一行不出现。 */
function HardwareCard({ hw, memKB, swapKB }: { hw: Hardware; memKB?: number; swapKB?: number }) {
  const t = useT();
  const cpu = hw.cpuModel ? cpuName(hw.cpuModel) : null;
  const topo = [
    hw.sockets && hw.sockets > 1 ? t("{n} 颗", { n: hw.sockets }) : "",
    hw.cores ? t("{n} 核", { n: hw.cores }) : "",
    hw.threads ? t("{n} 线程", { n: hw.threads }) : "",
  ].filter(Boolean).map(keep).join(" · ");
  const clock = [
    cpu?.base ? t("基础 {f}", { f: cpu.base }) : "",
    hw.maxMHz ? (cpu?.base ? t("最高 {f}", { f: ghz(hw.maxMHz) }) : ghz(hw.maxMHz)) : "",
    hw.l3 ? t("三级缓存 {n}", { n: hw.l3 }) : "",
  ].filter(Boolean).map(keep).join(" · ");
  const platform = hw.hypervisor ? t("虚拟机 · {p}", { p: hw.hypervisor }) : hw.virtual ? t("虚拟机") : t("物理机");
  // 磁盘清单默认收起：处理器、内存、整机是常看的，磁盘多的时候会把卡片拉得很长。
  const [showDisks, setShowDisks] = useState(false);
  return (
    <div className="card">
      <div className="card-head">
        <h2>{t("硬件")}</h2>
        <span className={"badge " + (hw.virtual ? "neutral" : "accent")}>{platform}</span>
        <span className="grow" />
        <span className="muted hide-sm" style={{ fontSize: 12.5 }}>{t("{ago}读取，每 6 小时更新", { ago: ago(new Date(hw.at).toISOString()) })}</span>
      </div>
      <div className="hw-grid">
        <div className="hw-tile">
          <span className="hw-icon"><Cpu /></span>
          <div className="hw-text">
            <div className="hw-label">{t("处理器")}{hw.cpuVendor ? <span className="hw-vendor">{hw.cpuVendor}</span> : null}</div>
            <div className="hw-title" title={hw.cpuModel}>{cpu?.name ?? "—"}</div>
            {topo && <div className="hw-sub">{topo}</div>}
            {clock && <div className="hw-sub">{clock}</div>}
            {hw.arch && <div className="hw-sub mono">{hw.arch}</div>}
          </div>
        </div>
        <div className="hw-tile">
          <span className="hw-icon"><MemoryStick /></span>
          <div className="hw-text">
            <div className="hw-label">{t("内存")}</div>
            <div className="hw-title num">{memKB ? bytes(memKB * 1024) : "—"}</div>
            {swapKB ? <div className="hw-sub">{t("交换分区 {n}", { n: bytes(swapKB * 1024) })}</div> : <div className="hw-sub">{t("没有交换分区")}</div>}
            <div className="hw-sub">{t("内存条型号和频率需要 root 才能读取，不显示")}</div>
          </div>
        </div>
        <div className="hw-tile">
          <span className="hw-icon"><Server /></span>
          <div className="hw-text">
            <div className="hw-label">{t("整机")}</div>
            <div className="hw-title" title={hw.product}>{hw.product || (hw.virtual ? t("虚拟机") : "—")}</div>
            {hw.vendor && <div className="hw-sub">{hw.vendor}</div>}
            {hw.board && <div className="hw-sub">{t("主板 {b}", { b: hw.board })}</div>}
            {!hw.vendor && !hw.product && <div className="hw-sub">{t("读不到厂家信息（容器里或部分虚拟机上没有）")}</div>}
          </div>
        </div>
      </div>
      {!!hw.disks?.length && (
        <button className={"hw-more" + (showDisks ? " open" : "")} aria-expanded={showDisks} onClick={() => setShowDisks(!showDisks)}>
          <HardDrive />
          <span>{t("磁盘")}</span>
          <span className="num">{t("{n} 块 · 共 {size}", { n: hw.disks.length, size: bytes(hw.disks.reduce((a, d) => a + d.bytes, 0)) })}</span>
          <span className="grow" />
          <span>{showDisks ? t("收起") : t("展开")}</span>
          <ChevronDown className="chev" />
        </button>
      )}
      {showDisks && !!hw.disks?.length && (
        <div className="hw-disks">
          {hw.disks.map((d) => (
            <div key={d.name} className="hw-disk">
              <HardDrive />
              <span className="mono hw-disk-name">{d.name}</span>
              <span className="hw-disk-model ellipsis" title={d.model}>{d.model || <span className="muted">{t("未知型号")}</span>}</span>
              {d.kind ? <span className="badge neutral">{d.kind === "ssd" ? "SSD" : "HDD"}</span> : <span />}
              <span className="num hw-disk-size">{bytes(d.bytes)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Fact({ label, children, wide }: { label: string; children: React.ReactNode; wide?: boolean }) {
  return (
    <div className={"fact" + (wide ? " wide" : "")}>
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

/** 进程排行。它来自可选的“增强采集”程序：没装时这里说明它是什么、怎么装；装了就显示排行和版本。 */
function ProcessCard({ host, executor, agent, procs, online }: { host: string; executor: string; agent?: string; procs: ProcStat[]; online: boolean }) {
  const t = useT();
  const nav = useNavigate();
  const version = useStore((s) => s.boot?.version);
  const [archs, setArchs] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => { api.system().then((r) => setArchs(r.agentArchs ?? [])).catch(() => setArchs([])); }, []);
  const sorter = useSort(procs, { name: (p) => p.name.toLowerCase(), pid: (p) => p.pid, cpu: (p) => p.cpu, mem: (p) => p.memBytes });
  const steady = useSteadyOrder(sorter.sorted, (p) => p.pid, sorter.sort ? sorter.sort.key + sorter.sort.dir : "");
  const act = async (action: "install" | "remove") => {
    setBusy(true);
    try { nav(`/requests/${(await api.hostAgent(host, action)).id}`); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const bundled = !!archs?.length;
  const outdated = !!agent && !!version && agent !== version;
  // 这台主机上最近一次安装或卸载采集程序的请求：按它的进度给按钮一个状态。
  const reqs = useStore((s) => s.requests);
  const job = agentJob(reqs, host, agent, version);
  const jobLabel = job && {
    approval: t("等待批准"),
    running: job.action === "install" ? t("正在安装…") : t("正在卸载…"),
    switching: t("正在切换采集方式…"),
  }[job.phase];
  const jobBtn = job && (
    <Link className="btn sm" to={`/requests/${job.id}`} title={job.phase === "approval" ? t("请求还在等批准，点开去批准") : t("点开看这次请求的详情")}>
      {job.phase === "approval" ? <ShieldAlert /> : <RefreshCw className="spin" />}{jobLabel}
    </Link>
  );
  // 条的长度按列表里最大的那个算，只用来比较进程之间的高低；具体数值看旁边的数字。
  const maxCpu = Math.max(1, ...procs.map((p) => p.cpu));
  const maxMem = Math.max(1, ...procs.map((p) => p.memBytes));
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("进程")}</h2>
        {agent
          ? <span className={"badge " + (outdated ? "warn" : "ok")} title={t("这台主机上装了增强采集程序：CPU、内存、磁盘、网络、温度和容器占用也都由它采集，仍通过同一条 SSH 连接")}>{t("增强采集 {v}", { v: agent })}</span>
          : <span className="badge neutral" title={t("用的是内置的只读脚本，主机上没有安装任何东西")}>{t("内置脚本")}</span>}
        {agent && <span className="muted hide-sm" style={{ fontSize: 12.5 }} title={t("数值每 5 秒更新；顺序最多每 15 秒调整一次，鼠标停在表上时不调整")}>{t("按 CPU 排前 {n} 的进程，每 5 秒更新", { n: procs.length })}</span>}
        <span className="grow" />
        {agent && jobBtn}
        {agent && !job && outdated && bundled && <button className="btn sm" disabled={busy} onClick={() => act("install")}><Download />{t("更新到 {v}", { v: version ?? "" })}</button>}
        {agent && !job && <button className="btn ghost sm" disabled={busy} onClick={() => act("remove")} title={t("删掉主机上的采集程序，回到内置脚本")}>{t("卸载")}</button>}
      </div>
      {agent ? (
        procs.length === 0 ? <div className="empty sm"><span>{t("正在收集数据…")}</span></div> : (
          <div className="table-wrap">
            <table className="table proc-table" {...steady.hover}>
              <thead><tr><SortTh s={sorter} k="name">{t("进程")}</SortTh><SortTh s={sorter} k="cpu" desc>CPU</SortTh><SortTh s={sorter} k="mem" desc>{t("内存")}</SortTh><SortTh s={sorter} k="pid" className="pid">PID</SortTh></tr></thead>
              <tbody>
                {steady.rows.map((p) => (
                  <tr key={p.pid}>
                    <td className="mono"><div className="ellipsis" title={p.name}>{p.name}</div></td>
                    <td><ProcBar share={p.cpu / maxCpu} label={p.cpu.toFixed(1) + "%"} /></td>
                    <td><ProcBar share={p.memBytes / maxMem} label={bytes(p.memBytes)} /></td>
                    <td className="mono dim num pid">{p.pid}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : (
        <div className="card-body">
          <div className="agent-intro">
            <div>
              <div className="setting-name">{t("想看每个进程的占用？装上增强采集")}</div>
              <div className="setting-desc">{t("现在的指标来自一段内置的只读脚本，主机上什么都不用装。进程排行这类更细的数据脚本做不了，需要在主机上放一个很小的采集程序（约 2 MB）。")}</div>
              <div className="setting-desc" style={{ marginTop: 6 }}>{t("它不开端口、不常驻：只在 Hostower 连着的时候运行，只读系统数据。安装会生成一个请求，你批准后才写到主机上；不想要了随时卸载，会自动回到内置脚本。")}</div>
            </div>
            {jobBtn ?? <button className="btn primary" disabled={busy || !bundled || !online || executor === "sim"} onClick={() => act("install")}><Download />{t("安装增强采集")}</button>}
          </div>
          {archs !== null && !bundled && <div className="note" style={{ marginTop: 12 }}><TriangleAlert /><span>{t("这个构建里没有带采集程序，装不了。")}</span></div>}
          {executor === "sim" && <div className="hint" style={{ marginTop: 8 }}>{t("模拟主机上装不了。")}</div>}
        </div>
      )}
    </div>
  );
}

/**
 * 找这台主机上最近一次安装或卸载采集程序、还没“落定”的请求：
 * 等批准、执行中，或者刚执行成功但采样还没切换过来（最多等两分钟）。都不是就返回 null。
 * 两条命令都是服务端固定的写法：都操作 ~/.hostower/bin/hostower-agent，安装的那条带着上传的文件。
 */
function agentJob(reqs: Req[], host: string, agent: string | undefined, version: string | undefined):
  { id: string; action: "install" | "remove"; phase: "approval" | "running" | "switching" } | null {
  let latest: { r: Req; action: "install" | "remove" } | null = null;
  for (const r of reqs) {
    if (r.kind !== "command" || r.targets.length !== 1 || r.targets[0].host !== host || !r.targets[0].command.includes("~/.hostower/bin/hostower-agent")) continue;
    if (!latest || r.createdAt > latest.r.createdAt) latest = { r, action: r.input?.kind === "upload" ? "install" : "remove" };
  }
  if (!latest) return null;
  const { r, action } = latest;
  if (r.state === "pending") return { id: r.id, action, phase: "approval" };
  if (r.state === "running") return { id: r.id, action, phase: "running" };
  if (r.state !== "done" || r.targets[0].state !== "ok" || !r.finishedAt || Date.now() - Date.parse(r.finishedAt) > 120_000) return null;
  // 执行成功了：装的是这一版就等采样报上这个版本，卸载就等它消失。
  const settled = action === "install" ? !!agent && (!version || agent === version) : !agent;
  return settled ? null : { id: r.id, action, phase: "switching" };
}

/** 进程表里的一格：数值加一根相对长度条。 */
function ProcBar({ share, label }: { share: number; label: string }) {
  return (
    <div className="proc-bar">
      <span className="proc-val num">{label}</span>
      <div className="meter-track" aria-hidden><div className="meter-fill" style={{ width: Math.min(100, Math.max(1, share * 100)) + "%" }} /></div>
    </div>
  );
}

/** 各容器的占用，按 CPU 从高到低；一页 8 个。 */
function ContainerUsage({ host, list }: { host: string; list: ContainerStat[] }) {
  const t = useT();
  const sorter = useSort(list, { name: (c) => c.name.toLowerCase(), cpu: (c) => c.cpu, mem: (c) => c.memBytes, net: (c) => c.netRx + c.netTx });
  const steady = useSteadyOrder(sorter.sorted, (c) => c.name, sorter.sort ? sorter.sort.key + sorter.sort.dir : "");
  const paged = usePaged(steady.rows, 8);
  const [open, setOpen] = useState("");
  return (
    <div className="card">
      {open && <ContainerDetailDialog host={host} name={open} stat={list.find((c) => c.name === open)} onClose={() => setOpen("")} />}
      <div className="card-head"><h2>{t("容器占用")}</h2><span className="muted num">{list.length || ""}</span><span className="grow" />
        <Link to={`/containers?host=${host}`} className="muted" style={{ fontSize: 12.5 }}>{t("管理容器")}</Link></div>
      {!list.length ? <div className="empty sm"><span>{t("没有运行中的容器，或这台主机上用不了 Docker")}</span></div> : (
        <>
          <div className="table-wrap">
            <table className="table usage-table" {...steady.hover}>
              <thead><tr><SortTh s={sorter} k="name">{t("容器")}</SortTh><SortTh s={sorter} k="cpu" desc>CPU</SortTh><SortTh s={sorter} k="mem" desc>{t("内存")}</SortTh><SortTh s={sorter} k="net" desc className="hide-sm">{t("网络累计")}</SortTh></tr></thead>
              <tbody>
                {paged.rows.map((c) => (
                  <tr key={c.name}>
                    <td className="mono"><button className="link-btn ellipsis" title={t("查看 {name} 的配置", { name: c.name })} onClick={() => setOpen(c.name)}>{c.name}</button></td>
                    <td><div className="cpu-cell"><Sparkline data={c.history ?? []} width={96} height={22} max={Math.max(10, ...(c.history ?? []))} /><b>{c.cpu.toFixed(1)}%</b></div></td>
                    <td className="num"><div className="cell-2"><span>{bytes(c.memBytes)}</span><small>{c.mem.toFixed(1)}%</small></div></td>
                    <td className="hide-sm num dim"><div className="cell-2" style={{ fontSize: 12.5 }}><span><span className="muted">↓ </span>{bytes(c.netRx)}</span><span><span className="muted">↑ </span>{bytes(c.netTx)}</span></div></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager p={paged} />
        </>
      )}
    </div>
  );
}

/** 当前值：数字用大字，单位用小字紧跟其后，各种单位（%、KB/s、°C）间距一致。 */
function MetricValue({ text }: { text: string }) {
  const m = /^(-?[\d.,]+)\s*(.*)$/.exec(text);
  if (!m) return <div className="metric-val">{text}</div>;
  return <div className="metric-val">{m[1]}{m[2] && <small>{m[2]}</small>}</div>;
}

/** 一张指标卡：标题和一行说明、当前值、带刻度的趋势图，以及这段时间里的最低、平均、最高。 */
function MetricCard({ title, desc, value, now, samples, series, stat, format, max }: {
  title: string; desc: string; value: string;
  /** 最新一次采样：大数字和图例都显示它，两处才对得上（图上的点是一段时间的平均，会比它滞后）。 */
  now?: Sample;
  samples: Sample[]; series: Series[];
  /** 最低、平均、最高按什么算；默认是第一条线。 */
  stat?: (s: Sample) => number;
  format: (n: number) => string; max?: number;
}) {
  const t = useT();
  const language = lang();
  // 序列名在模块级的表里是中文原文，这里换成当前语言再交给图表。
  const shown = useMemo(() => series.map((sr) => (sr.label ? { ...sr, label: t(sr.label) } : sr)), [series, language, t]);
  const vals = samples.map(stat ?? series[0].pick);
  const last = samples[samples.length - 1];
  const cur = now ?? last;
  const mins = samples.length > 1 ? Math.max(1, Math.round((last.ts - samples[0].ts) / 60000)) : 0;
  const span = mins >= 2880 ? t("{n} 天", { n: Math.round(mins / 1440) }) : mins >= 120 ? t("{n} 小时", { n: Math.round(mins / 60) }) : t("{n} 分钟", { n: mins });
  return (
    <div className="card metric">
      <div className="metric-head">
        <div style={{ minWidth: 0 }}>
          <div className="metric-title">{title}</div>
          <div className="metric-sub ellipsis" title={desc}>{desc}</div>
        </div>
        <MetricValue text={value} />
      </div>
      {(
        <div className="legend">
          {shown.length > 1 && shown.map((sr, i) => (
            <span key={i}>
              <svg width="16" height="6" aria-hidden><line x1="0" x2="16" y1="3" y2="3" stroke={i ? "var(--accent-2)" : "var(--accent)"} strokeWidth="2" strokeDasharray={i ? "5 4" : undefined} /></svg>
              {sr.label}{cur && <b>{format(sr.pick(cur))}</b>}
            </span>
          ))}
        </div>
      )}
      <TrendChart samples={samples} series={shown} format={format} max={max} />
      <div className="metric-stats">
        {vals.length > 1 ? (
          <>
            <span>{t("最低")} <b>{format(Math.min(...vals))}</b></span>
            <span>{t("平均")} <b>{format(vals.reduce((x, y) => x + y, 0) / vals.length)}</b></span>
            <span>{t("最高")} <b>{format(Math.max(...vals))}</b></span>
            <span className="grow" />
            <span>{t("近 {span}", { span })}</span>
          </>
        ) : <span>{t("正在收集数据")}</span>}
      </div>
    </div>
  );
}
