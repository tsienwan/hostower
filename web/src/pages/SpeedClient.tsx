// “这台设备”测速：把正在看面板的这台设备（比如家里连着 Wi-Fi 的手机）当成一个节点。
// 两种测法：
// - 直连主机：在选中的主机上临时开一个测速服务（照常走请求、要批准），浏览器另开一页直接连它量，数字由那个服务写进命令的输出里；
// - 面板：延迟、下载、上传是浏览器和面板之间量的；回程路由是从选中的主机追踪到面板看到的来源地址（照常走请求）。
// 去程路由浏览器追踪不了，在报告里可以把 tracert / traceroute 的输出贴进来。
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Check, ExternalLink, FileText, Gauge, RefreshCw, Smartphone, TriangleAlert } from "lucide-react";
import { api, type SpeedClientInfo, type SpeedClientMeasure, type SpeedTransfer } from "../api";
import { fail, upsertRequest, useStore } from "../store";
import { LevelBadge, Modal, StateBadge } from "../ui";
import { Select } from "../controls";
import { ApprovalCard } from "./Requests";
import { useCached } from "../cache";
import { ts, useT } from "../i18n";
import { SpeedReport } from "./SpeedReport";
import { mbps } from "./Speedtest";

const BASE = "/api/v1/speedtest/client";
const SIZE_KEY = "hostower.speedtest.client.size";
const HOST_KEY = "hostower.speedtest.client.host";
const MODE_KEY = "hostower.speedtest.client.mode";
const PORT_KEY = "hostower.speedtest.client.port";
const ADDR_KEY = "hostower.speedtest.client.addr.";
const CONNS = 4;

/** 从主机的连接地址里取出浏览器能连的那部分（去掉用户名和端口）；本机回环地址不算。 */
function hostAddr(addr: string): string {
  let a = addr.replace(/^[^@]*@/, "");
  const m = /^\[([^\]]+)\](?::\d+)?$/.exec(a);
  if (m) return m[1];
  if ((a.match(/:/g) ?? []).length === 1) a = a.split(":")[0];
  return a === "localhost" || a.startsWith("127.") ? "" : a;
}
const ADDR_RE = /^[A-Za-z0-9][A-Za-z0-9.:-]{0,252}$/;

function remembered(key: string): string {
  try { return localStorage.getItem(key) ?? ""; } catch { return ""; }
}
function rememberChoice(key: string, v: string) {
  try { localStorage.setItem(key, v); } catch { /* 存不下就算了 */ }
}

export function median(v: number[]): number {
  if (!v.length) return 0;
  const s = [...v].sort((a, b) => a - b);
  return s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2;
}
/** 相邻两次之差的平均：抖动。 */
export function jitterOf(v: number[]): number {
  return v.length > 1 ? v.slice(1).reduce((a, x, i) => a + Math.abs(x - v[i]), 0) / (v.length - 1) : 0;
}

type Phase = "ping" | "down" | "up" | "save" | "done";
type Live = { phase: Phase; pings: number[]; lost: number; downBps: number; upBps: number };

/** 连发 n 次小请求量往返时间。前两次用来把连接建起来，不算。 */
async function measurePings(n: number, signal: AbortSignal, tick: (pings: number[], lost: number) => void) {
  const pings: number[] = [];
  let lost = 0;
  for (let i = 0; i < n + 2; i++) {
    if (signal.aborted) break;
    const t0 = performance.now();
    try {
      const r = await fetch(`${BASE}/ping?i=${i}`, { cache: "no-store", credentials: "same-origin", signal });
      if (!r.ok) throw new Error();
      if (i >= 2) pings.push(performance.now() - t0);
    } catch {
      if (i >= 2 && !signal.aborted) lost++;
    }
    tick(pings, lost);
  }
  return { pings, lost };
}

/** 同时开几条连接下载，到数据量或者到时间为止；从收到第一块数据开始计时。 */
async function measureDown(mb: number, seconds: number, signal: AbortSignal, tick: (bps: number) => void): Promise<SpeedTransfer | undefined> {
  const ctl = new AbortController();
  const stop = () => ctl.abort();
  signal.addEventListener("abort", stop);
  const per = Math.max(1, Math.floor(mb / CONNS));
  let bytes = 0, start = 0, end = 0;
  const timer = setTimeout(stop, seconds * 1000);
  const meter = setInterval(() => { if (start && end > start) tick((bytes * 8) / ((end - start) / 1000)); }, 200);
  await Promise.all(Array.from({ length: CONNS }, async () => {
    try {
      const r = await fetch(`${BASE}/down?mb=${per}`, { cache: "no-store", credentials: "same-origin", signal: ctl.signal });
      if (!r.ok || !r.body) return;
      const rd = r.body.getReader();
      for (;;) {
        const { done, value } = await rd.read();
        if (done) break;
        const now = performance.now();
        if (!start) start = now; else { bytes += value.byteLength; end = now; }
      }
    } catch { /* 到时间断开，或者网络断了：有多少算多少 */ }
  }));
  clearTimeout(timer); clearInterval(meter);
  signal.removeEventListener("abort", stop);
  const ms = end - start;
  return bytes > 0 && ms > 0 ? { bytes, ms, bps: (bytes * 8) / (ms / 1000), conns: CONNS } : undefined;
}

/** 分块上传：几条连接轮着传，块从 256 KB 起、传得快就加倍（最多 4 MB），到数据量或者到时间为止；只算传完的块。 */
async function measureUp(mb: number, seconds: number, signal: AbortSignal, tick: (bps: number) => void): Promise<SpeedTransfer | undefined> {
  const ctl = new AbortController();
  const stop = () => ctl.abort();
  signal.addEventListener("abort", stop);
  const buf = new Uint8Array(4 << 20);
  for (let i = 0; i < buf.length; i += 65536) crypto.getRandomValues(buf.subarray(i, i + 65536));
  const cap = mb * 1e6;
  let claimed = 0, sent = 0, size = 256 << 10;
  const start = performance.now();
  let end = start;
  const timer = setTimeout(stop, seconds * 1000);
  const conns = 2;
  await Promise.all(Array.from({ length: conns }, async () => {
    while (claimed < cap && !ctl.signal.aborted) {
      const n = Math.min(size, buf.length);
      claimed += n;
      const t0 = performance.now();
      try {
        const r = await fetch(`${BASE}/up`, {
          method: "POST", credentials: "same-origin", signal: ctl.signal,
          headers: { "X-Hostower": "1", "Content-Type": "application/octet-stream" }, body: buf.subarray(0, n),
        });
        if (!r.ok) return;
      } catch { return; }
      const now = performance.now();
      sent += n; end = now;
      if (now - t0 < 400) size = Math.min(size * 2, buf.length);
      tick((sent * 8) / ((end - start) / 1000));
    }
  }));
  clearTimeout(timer);
  signal.removeEventListener("abort", stop);
  const ms = end - start;
  return sent > 0 && ms > 0 ? { bytes: sent, ms, bps: (sent * 8) / (ms / 1000), conns } : undefined;
}

/** 浏览器知道的协议（h2、h3…）和连接类型（wifi、cellular…）；很多浏览器不报，那就是空的。 */
function browserNet(): { proto: string; conn: string } {
  let proto = "", conn = "";
  try {
    const e = (performance.getEntriesByType("resource") as PerformanceResourceTiming[]).filter((x) => x.name.includes(BASE + "/ping")).pop();
    proto = e?.nextHopProtocol ?? "";
    // effectiveType（4g、3g…）只是按速度估的档位，不是连接类型，不用它。
    const c = (navigator as unknown as { connection?: { type?: string } }).connection;
    conn = c?.type && c.type !== "unknown" && c.type !== "none" ? c.type : "";
  } catch { /* 不支持就算了 */ }
  return { proto, conn };
}

/** 页面上的那张卡片：说明这是什么，点一下开始。 */
export function ClientCard({ onDone }: { onDone: () => void }) {
  const t = useT();
  const [info, setInfo] = useCached<SpeedClientInfo>("speedtest.client");
  const [open, setOpen] = useState(false);
  useEffect(() => { api.speedClient().then(setInfo).catch(() => {}); }, [setInfo]);
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("这台设备 ↔ 主机")}</h2>
        {info && <span className="mono muted">{info.ip}</span>}
        <span className="grow" />
        <button className="btn primary sm" disabled={!info} onClick={() => setOpen(true)}><Smartphone />{t("测这台设备")}</button>
      </div>
      <div className="card-body">
        <span className="hint">{t("把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。")}</span>
      </div>
      {open && info && <ClientDialog info={info} onClose={() => { setOpen(false); onDone(); }} />}
    </div>
  );
}

function ClientDialog({ info, onClose }: { info: SpeedClientInfo; onClose: () => void }) {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const online = hosts.filter((h) => h.status.online).map((h) => h.name);
  const [size, setSize] = useState(() => { const v = +remembered(SIZE_KEY); return info.sizes.includes(v) ? v : info.sizes.includes(100) ? 100 : info.sizes[0]; });
  const [host, setHost] = useState(() => { const v = remembered(HOST_KEY); return online.includes(v) ? v : online.includes("local") ? "local" : online[0] ?? ""; });
  const [trace, setTrace] = useState<boolean | null>(null);
  const [install, setInstall] = useState(true);
  const [busy, setBusy] = useState(false);
  const [reqId, setReqId] = useState("");
  const [live, setLive] = useState<Live | null>(null);
  const [err, setErr] = useState("");
  const [report, setReport] = useState(false);
  const req = useStore((st) => st.requests.find((r) => r.id === reqId));
  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);
  // 直连：速度测到选中的主机；面板所在的那台（本机）只能按面板测。
  const isLocal = hosts.find((h) => h.name === host)?.executor === "local";
  const [mode, setMode] = useState<"direct" | "panel">(() => remembered(MODE_KEY) === "panel" ? "panel" : "direct");
  const direct = mode === "direct" && !isLocal;
  const [addr, setAddr] = useState("");
  const [port, setPort] = useState(() => { const v = +remembered(PORT_KEY); return v >= 1024 && v <= 65535 ? String(v) : String(info.directPort); });
  const [link, setLink] = useState<{ port: number; token: string } | null>(null);
  const [out, setOut] = useState("");
  useEffect(() => {
    const h = hosts.find((x) => x.name === host);
    setAddr(remembered(ADDR_KEY + host) || (h ? hostAddr(h.addr) : ""));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host]);
  // 直连时从请求的输出里看进度：测速服务起来了没有、设备连上了没有、测完了没有。
  const reqState = req?.state;
  useEffect(() => {
    if (!link || !reqId || !reqState || reqState === "pending") return;
    let on = true;
    const pull = () => api.request(reqId).then((r) => on && setOut(r.targets[0]?.output ?? "")).catch(() => {});
    pull();
    if (reqState !== "running") return () => { on = false; };
    const timer = setInterval(pull, 1000);
    return () => { on = false; clearInterval(timer); };
  }, [link, reqId, reqState]);

  // 选中的主机上有没有 traceroute 或 tracepath（只读查询）。
  useEffect(() => {
    if (!host) return;
    let on = true;
    setTrace(null);
    api.speedtestCheck([host]).then((r) => on && setTrace(!!r.hosts[0]?.trace || !!r.hosts[0]?.error)).catch(() => on && setTrace(true));
    return () => { on = false; };
  }, [host]);

  const start = async () => {
    setBusy(true); setErr("");
    rememberChoice(SIZE_KEY, String(size)); rememberChoice(HOST_KEY, host);
    if (!isLocal) rememberChoice(MODE_KEY, mode);
    if (direct) {
      // 直连：只发起请求；等主机上的测速服务起来后，在另一个页面里测。
      rememberChoice(PORT_KEY, port); rememberChoice(ADDR_KEY + host, addr);
      try {
        const r = await api.speedClientRun(host, trace === false && install, { port: +port, sizeMb: size });
        upsertRequest(r.request);
        setLink(r.direct ?? null);
        setReqId(r.request.id);
      } catch (e) { fail(e); }
      setBusy(false);
      return;
    }
    let id = "";
    try {
      const r = await api.speedClientRun(host, trace === false && install);
      upsertRequest(r.request);
      id = r.request.id;
      setReqId(id);
    } catch (e) { fail(e); setBusy(false); return; }
    // 回程路由在主机上跑（可能还要等批准）；浏览器这边同时开始量。
    const ctl = new AbortController();
    abort.current = ctl;
    const now: Live = { phase: "ping", pings: [], lost: 0, downBps: 0, upBps: 0 };
    const show = (p: Partial<Live>) => { Object.assign(now, p); setLive({ ...now }); };
    show({});
    try {
      const p = await measurePings(info.pings, ctl.signal, (pings, lost) => show({ pings: [...pings], lost }));
      show({ phase: "down" });
      const down = await measureDown(size, info.seconds, ctl.signal, (bps) => show({ downBps: bps }));
      show({ phase: "up", downBps: down?.bps ?? 0 });
      const up = await measureUp(size / 4, info.seconds, ctl.signal, (bps) => show({ upBps: bps }));
      show({ phase: "save", upBps: up?.bps ?? 0 });
      if (ctl.signal.aborted) return;
      const measure: SpeedClientMeasure = { pings: p.pings, lost: p.lost, down, up, ...browserNet() };
      await api.speedClientResult(id, measure);
      show({ phase: "done" });
    } catch (e) {
      if (!ctl.signal.aborted) setErr(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const finished = !!req && !["pending", "running"].includes(req.state);
  // 请求执行过（成功或失败）才有记录可看。
  const recorded = !!req && (req.state === "done" || req.state === "failed");
  const total = size + size / 4;
  const title = t("测这台设备");

  if (!reqId) {
    return (
      <Modal wide title={title} onClose={onClose}
        footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !host || trace === null || (direct ? !ADDR_RE.test(addr) || !(+port >= 1024 && +port <= 65535) : !info.traceable)} onClick={start}><Gauge />{t("开始")}</button></>}>
        <div className="field">
          <label>{t("测哪台主机")}</label>
          {online.length === 0 ? <span className="crit-text">{t("没有在线的主机")}</span>
            : <Select value={host} onChange={setHost} mono options={online.map((h) => ({ value: h, label: h }))} />}
        </div>
        {!isLocal && (
          <div className="field">
            <label>{t("速度测到哪里")}</label>
            <div className="seg" role="radiogroup" aria-label={t("速度测到哪里")}>
              <button role="radio" aria-checked={direct} className={direct ? "on" : ""} onClick={() => setMode("direct")}>{t("这台主机（直连）")}</button>
              <button role="radio" aria-checked={!direct} className={!direct ? "on" : ""} onClick={() => setMode("panel")}>{t("面板")}</button>
            </div>
            <span className="hint">{direct
              ? t("在 {host} 上临时开一个测速服务，这台设备的浏览器直接连它量延迟、下载、上传，再从 {host} 追踪回到这台设备。服务只认带一次性口令的地址，最多等 {w} 秒，测完自己退出。开端口是一次变更（L1），需要批准。", { host, w: info.directWait })
              : t("延迟和速度量的是这台设备和面板之间；{host} 只用来追踪回到这台设备的路由。", { host })}</span>
          </div>
        )}
        {direct && (
          <>
            <div className="field">
              <label>{t("主机的地址和端口")}</label>
              <div className="row" style={{ gap: 8 }}>
                <input className="input mono grow" value={addr} maxLength={253} placeholder="203.0.113.10" aria-label={t("主机的地址")} onChange={(e) => setAddr(e.target.value.trim())} />
                <input className="input mono" style={{ width: 96 }} inputMode="numeric" value={port} aria-label={t("端口")} onChange={(e) => setPort(e.target.value.replace(/\D/g, "").slice(0, 5))} />
              </div>
              <span className="hint">{t("填这台设备能直接连到的地址（公网 IP 或域名）。主机的防火墙和云厂商的安全组要放行这个 TCP 端口，端口在 1024 到 65535 之间。")}</span>
            </div>
            <div className="note"><span>{t("测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。")}</span></div>
          </>
        )}
        {!direct && (
          <>
            <dl className="kv sm">
              <dt>{t("这台设备的地址")}</dt><dd className="mono">{info.ip}</dd>
            </dl>
            <span className="hint">{t("这是面板看到的来源地址，回程路由就追踪到它。用了代理或 VPN 访问面板时，它是代理的出口地址。")}</span>
            {info.proxy && <div className="note"><span>{t("面板前面有反向代理：延迟和速度量的是这台设备到代理（或 CDN）那一段，加上代理到面板。")}</span></div>}
            {!info.traceable && <div className="note crit"><TriangleAlert /><span>{t("读不出可以追踪的来源地址（比如从本机回环地址访问），测不了。")}</span></div>}
          </>
        )}
        <div className="field">
          <label>{t("数据量上限")}</label>
          <div className="seg" role="radiogroup" aria-label={t("数据量上限")}>
            {info.sizes.map((x) => <button key={x} role="radio" aria-checked={size === x} className={size === x ? "on" : ""} onClick={() => setSize(x)}>{x} MB</button>)}
          </div>
          <span className="hint">{t("下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，最多约 {total} MB。", { d: size, u: size / 4, s: info.seconds, total })}</span>
        </div>
        {trace === null && host ? <div className="note"><RefreshCw className="spin" /><span>{t("正在检查主机上有没有 traceroute…")}</span></div>
          : trace === false && (
            <div className="note warn">
              <TriangleAlert />
              <div className="col" style={{ gap: 8 }}>
                <span>{t("{host} 上没有 traceroute 或 tracepath，追踪不了路由。", { host })}</span>
                <label className="check"><input type="checkbox" checked={install} onChange={(e) => setInstall(e.target.checked)} />{t("先用系统的包管理器安装 traceroute")}</label>
                <span className="hint">{t("安装是一次变更（L1），需要批准；不装的话只有速度，没有回程路由。")}</span>
              </div>
            </div>
          )}
      </Modal>
    );
  }

  if (link) {
    const ready = out.includes("speedtest listen port=");
    const connected = out.includes("speedtest client ip=");
    const measured = out.includes("speedtest direct up ");
    const failed = /^speedtest error=/m.test(out);
    const labels = {
      title: t("这台设备 ↔ {host}", { host }), ping: t("延迟"), down: t("下载"), up: t("上传"),
      run: t("正在测，请不要关闭这个页面…"), done: t("测完了，可以关掉这个页面回到面板。"), fail: t("结果没有交回去，请回到面板重试。"),
    };
    const url = `http://${addr.includes(":") ? `[${addr}]` : addr}:${link.port}/${link.token}/#${encodeURIComponent(JSON.stringify(labels))}`;
    const running = req?.state === "running";
    return (
      <Modal wide title={title} onClose={onClose}
        footer={<>
          {recorded && <button className="btn" onClick={() => setReport(true)}><FileText />{t("查看报告")}</button>}
          <button className={"btn" + (finished ? " primary" : "")} onClick={onClose}>{finished ? t("完成") : t("关闭")}</button>
        </>}>
        {!req ? <div className="note"><RefreshCw className="spin" /><span>{t("加载中…")}</span></div> : (
          <>
            <div className="row wrap" style={{ gap: 10 }}>
              <LevelBadge level={req.level} />
              <span className="dim">{ts(req.intent)}</span>
              <span className="grow" />
              <StateBadge state={req.state} />
              <Link to={`/requests/${req.id}`} className="muted" style={{ fontSize: 12.5 }} onClick={onClose}>{t("请求详情")}</Link>
            </div>
            {req.state === "pending" && <ApprovalCard r={req} title={t("批准后才会在主机上开测速服务")}
              lead={t("这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。", { host, port: link.port })} />}
            {running && !ready && <div className="note"><RefreshCw className="spin" /><span>{t("正在 {host} 上启动测速服务…", { host })}</span></div>}
            {running && ready && !connected && (
              <div className="note ok">
                <Check />
                <div className="col" style={{ gap: 10 }}>
                  <span>{t("测速服务已经在 {host} 上等着了。在这台设备上打开测速页，它会自己开始测。", { host })}</span>
                  <a className="btn primary" href={url} target="_blank" rel="noopener noreferrer"><ExternalLink />{t("打开测速页")}</a>
                  <span className="hint">{t("打不开或一直转圈：主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者地址 {addr} 这台设备连不上。服务最多等 {w} 秒。", { port: link.port, addr, w: info.directWait })}</span>
                </div>
              </div>
            )}
            {running && connected && !measured && <div className="note"><RefreshCw className="spin" /><span>{t("这台设备已经连上，正在另一个页面里测…")}</span></div>}
            {running && measured && <div className="note"><RefreshCw className="spin" /><span>{t("正在从 {host} 追踪到这台设备，最多要半分钟…", { host })}</span></div>}
            {finished && recorded && !failed && measured && <div className="note ok"><Check /><span>{t("测完了，在报告里看速度、线路和每一跳。")}</span></div>}
            {finished && (!recorded || failed || !measured) && <div className="note crit"><TriangleAlert /><span>{t("没有测成，原因在报告或请求详情里。")}</span></div>}
          </>
        )}
        {report && <SpeedReport requestId={reqId} host={host} onClose={() => setReport(false)} />}
      </Modal>
    );
  }

  const phase = live?.phase ?? "ping";
  const state = (p: Phase, order: Phase[]) => order.indexOf(phase) > order.indexOf(p) ? "ok" : phase === p ? "warn pulse" : "";
  const order: Phase[] = ["ping", "down", "up", "save", "done"];
  const pings = live?.pings ?? [];
  return (
    <Modal wide title={title} onClose={onClose}
      footer={<>
        {recorded && phase === "done" && <button className="btn" onClick={() => setReport(true)}><FileText />{t("查看报告")}</button>}
        <button className={"btn" + (finished && phase === "done" ? " primary" : "")} onClick={onClose}>{finished && phase === "done" ? t("完成") : t("关闭")}</button>
      </>}>
      <div className="sp-progress">
        <div className="sp-row">
          <span className={"dot " + state("ping", order)} />
          <span style={{ fontWeight: 600 }}>{t("延迟")}</span>
          <span className="sp-nums num">
            {pings.length ? <><span><b>{median(pings).toFixed(0)}</b> ms</span><span>{t("抖动 {j} ms", { j: jitterOf(pings).toFixed(1) })}</span></> : <span className="muted">{phase === "ping" ? t("正在测…") : "—"}</span>}
            {(live?.lost ?? 0) > 0 && <span className="crit-text">{t("失败 {n} 次", { n: live?.lost ?? 0 })}</span>}
          </span>
        </div>
        <div className="sp-row">
          <span className={"dot " + state("down", order)} />
          <span style={{ fontWeight: 600 }}>{t("下载")}</span>
          <span className="sp-nums num">{live?.downBps ? <span>↓ <b>{mbps(live.downBps)}</b></span> : <span className="muted">{phase === "down" ? t("正在测…") : order.indexOf(phase) > 1 ? t("没有测出来") : t("排队中")}</span>}</span>
        </div>
        <div className="sp-row">
          <span className={"dot " + state("up", order)} />
          <span style={{ fontWeight: 600 }}>{t("上传")}</span>
          <span className="sp-nums num">{live?.upBps ? <span>↑ <b>{mbps(live.upBps)}</b></span> : <span className="muted">{phase === "up" ? t("正在测…") : order.indexOf(phase) > 2 ? t("没有测出来") : t("排队中")}</span>}</span>
        </div>
      </div>
      {err && <div className="note crit"><TriangleAlert /><span>{ts(err)}</span></div>}
      {phase === "done" && <div className="note ok"><Check /><span>{t("速度已经记下。")}</span></div>}
      {!req ? <div className="note"><RefreshCw className="spin" /><span>{t("加载中…")}</span></div> : (
        <>
          <div className="row wrap" style={{ gap: 10 }}>
            <LevelBadge level={req.level} />
            <span className="dim">{ts(req.intent)}</span>
            <span className="grow" />
            <StateBadge state={req.state} />
            <Link to={`/requests/${req.id}`} className="muted" style={{ fontSize: 12.5 }} onClick={onClose}>{t("请求详情")}</Link>
          </div>
          {req.state === "pending" && <ApprovalCard r={req} title={t("批准后才会追踪回程路由")} lead={t("这次要先安装 traceroute，是一次变更；速度不受影响，已经在测。")} />}
          {req.state === "running" && <div className="note"><RefreshCw className="spin" /><span>{t("正在从 {host} 追踪到这台设备，最多要半分钟…", { host })}</span></div>}
          {finished && req.state === "done" && <div className="note ok"><Check /><span>{t("回程路由追踪完了，在报告里看线路和每一跳。")}</span></div>}
          {finished && req.state !== "done" && <div className="note crit"><TriangleAlert /><span>{t("回程路由没有追踪成功，报告里只有速度。")}</span></div>}
        </>
      )}
      {report && <SpeedReport requestId={reqId} host={host} onClose={() => setReport(false)} />}
    </Modal>
  );
}
