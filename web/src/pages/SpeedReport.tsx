// 测速报告：一次测速在一台主机上的明细——延迟各段、下载和上传、网络信息、到每个追踪目标的路由和推测的线路。
// 每一跳的地区和运营商是打开报告时由面板去查的（所有者可以关掉）。IP、节点、路由、地区都是外来的内容，只按纯文本显示。
import { Fragment, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, Check, Plus, RefreshCw, Route as RouteIcon, Save, Trash2, TriangleAlert } from "lucide-react";
import { api, type SpeedDetail, type SpeedGeo, type SpeedGeoView, type SpeedHop, type SpeedOokla, type SpeedPreset, type SpeedResult, type SpeedRoute, type SpeedTarget, type SpeedTransfer } from "../api";
import { fail, toast, useStore } from "../store";
import { Modal, bytes, stamp } from "../ui";
import { lang, ts, useT } from "../i18n";
import { mbps, optsLabel } from "./Speedtest";

/** 骨干网标签的显示名。CHINANET 是只按自治系统号（AS4134）认出的电信地址，可能是省里的城域网，不一定是 163 骨干。 */
const netNames: Record<string, string> = {
  CN2: "电信 CN2", "163": "电信 163", CHINANET: "电信", CTGNet: "电信 CTGNet", "9929": "联通 9929", "4837": "联通 4837", CMIN2: "移动 CMIN2", CMI: "移动 CMI", CMNET: "移动 CMNET", LAN: "内网",
};

const ms = (v?: number) => (v || v === 0 ? (v >= 100 ? v.toFixed(0) : v.toFixed(1)) : "—");

function Stat({ label, value, unit, sub }: { label: string; value: string; unit?: string; sub?: string }) {
  return (
    <div className="sr-stat">
      <span className="muted">{label}</span>
      <span className="num"><b>{value}</b>{unit && <small> {unit}</small>}</span>
      {sub && <span className="muted num sr-sub">{sub}</span>}
    </div>
  );
}

export function SpeedReport({ requestId, host, onClose }: { requestId: string; host: string; onClose: () => void }) {
  const t = useT();
  const [r, setR] = useState<SpeedResult | null>(null);
  const [err, setErr] = useState("");
  const [geo, setGeo] = useState<SpeedGeoView | "loading" | null>(null);
  useEffect(() => { api.speedtestReport(requestId, host).then(setR).catch((e) => setErr(e instanceof Error ? e.message : String(e))); }, [requestId, host]);
  const d = r?.detail;
  const o = d?.ookla;
  const hasHops = !!d?.routes?.some((x) => x.hops.some((h) => h.ip));
  // 有路由时再去查各跳的地区（面板去查，只查这条记录里的公网地址）；查不到也不影响报告。
  useEffect(() => {
    if (!hasHops) return;
    setGeo("loading");
    api.speedtestGeo(requestId, host, lang()).then(setGeo).catch((e) => setGeo({ enabled: true, service: "", error: e instanceof Error ? e.message : String(e) }));
  }, [hasHops, requestId, host]);
  const tcp = d?.pings.map((p) => p.tcp) ?? [];
  const avg = tcp.length ? tcp.reduce((a, b) => a + b, 0) / tcp.length : 0;
  const jitter = tcp.length > 1 ? tcp.slice(1).reduce((a, v, i) => a + Math.abs(v - tcp[i]), 0) / (tcp.length - 1) : 0;
  const g = geo && geo !== "loading" ? geo : null;
  const routes = g?.routes ?? d?.routes ?? [];

  return (
    <Modal wide="xl" title={t("测速报告：{host}", { host })} onClose={onClose}
      footer={<>
        <Link to={`/requests/${requestId}`} className="btn ghost" onClick={onClose}>{t("原始输出")}</Link>
        <span className="grow" />
        <button className="btn primary" onClick={onClose}>{t("关闭")}</button>
      </>}>
      {err ? <div className="note crit"><span>{ts(err)}</span></div> : !r ? <div className="muted">{t("加载中…")}</div> : (
        <>
          <div className="row wrap muted" style={{ gap: 12, fontSize: 12.5 }}>
            <span>{stamp(r.ts)}</span>
            {r.method === "ookla"
              ? <span>Speedtest by Ookla{o ? " · " + serverLabel(o) : ""}{r.sizeMb ? " · " + t("共传了 {n} MB", { n: r.sizeMb }) : ""}</span>
              : <span>Cloudflare · {t("下载 {d} MB、上传 {u} MB", { d: r.sizeMb, u: r.sizeMb / 4 })}</span>}
            {optsLabel(t, r.threads, r.ip) && <span>{optsLabel(t, r.threads, r.ip)}</span>}
          </div>
          {r.error && <div className="note crit"><span>{ts(r.error)}</span></div>}
          <div className="sr-stats">
            <Stat label={t("延迟")} value={r.pingMs ? ms(r.pingMs) : "—"} unit="ms"
              sub={o ? t("抖动 {j} ms", { j: ms(o.jitter) }) : tcp.length ? t("平均 {avg} ms", { avg: ms(avg) }) : undefined} />
            <Stat label={t("下载")} value={mbps(r.downBps)} sub={transfer(t, d?.down)} />
            <Stat label={t("上传")} value={mbps(r.upBps)} sub={transfer(t, d?.up)} />
            {o ? <Stat label={t("丢包")} value={o.packetLoss !== undefined ? String(o.packetLoss) : "—"} unit={o.packetLoss !== undefined ? "%" : undefined}
              sub={o.packetLoss === undefined ? t("这台服务器不测丢包") : undefined} />
              : d && tcp.length > 1 && <Stat label={t("抖动")} value={ms(jitter)} unit="ms" sub={t("{n} 次握手之间", { n: tcp.length })} />}
          </div>
          {!d ? <div className="note"><span>{t("这条记录没有明细：明细是这个版本才开始记录的，之前的测速只有汇总。")}</span></div> : (
            <>
              {o ? <OoklaView o={o} d={d} /> : <div className="sr-grid">
                <section className="sr-sec sr-card">
                  <h3>{t("延迟")}</h3>
                  <p className="hint">{t("连 {n} 次测速服务器，每次分段计时；延迟取 TCP 握手最快的一次。", { n: d.pings.length })}
                    {tcp.length > 0 && " " + t("TCP 握手：最小 {min}、平均 {avg}、最大 {max} ms。", { min: ms(Math.min(...tcp)), avg: ms(avg), max: ms(Math.max(...tcp)) })}</p>
                  <div className="table-wrap">
                    <table className="table sr-table">
                      <thead><tr><th>#</th><th>{t("域名解析")}</th><th>{t("TCP 握手")}</th><th>{t("TLS 握手")}</th><th>{t("等待首字节")}</th><th>{t("合计")}</th></tr></thead>
                      <tbody>
                        {d.pings.map((p, i) => (
                          <tr key={i}>
                            <td className="muted num">{i + 1}</td>
                            <td className="num">{ms(p.dns)}</td>
                            <td className="num"><b>{ms(p.tcp)}</b></td>
                            <td className="num">{ms(p.tls)}</td>
                            <td className="num">{ms(p.wait)}</td>
                            <td className="num muted">{ms(p.dns + p.tcp + p.tls + p.wait)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <span className="hint">{t("单位：毫秒")}</span>
                </section>
                <section className="sr-sec sr-card">
                    <h3>{t("网络")}</h3>
                    <dl className="kv sm">
                      <dt>{t("出口 IP")}</dt><dd className="mono">{d.net.ip || "—"}</dd>
                      <dt>{t("国家或地区")}</dt><dd>{d.net.loc || "—"}</dd>
                      <dt>{t("测速节点")}</dt><dd className="mono">{d.net.colo || "—"}</dd>
                      <dt>{t("服务器地址")}</dt><dd className="mono">{d.net.serverIp || "—"}</dd>
                      <dt>{t("协议")}</dt><dd className="mono">{[d.net.http && "HTTP/" + d.net.http.replace(/^HTTP\//i, ""), d.net.tls].filter(Boolean).join(" · ") || "—"}</dd>
                    </dl>
                </section>
              </div>}
              <section className="sr-sec">
                <h3>{t("线路")}</h3>
                {!routes.length ? <p className="hint">{t("这次没有追踪路由。可以在测速页的“路由追踪目标”里添加目标，发起测速时勾选“顺便追踪路由”。")}</p> : (
                  <>
                    <p className="hint">{t("线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。看回程线路要追踪到你那边的地址。")}</p>
                    {geo === "loading" && <div className="note"><RefreshCw className="spin" /><span>{t("正在查询各跳的地区和运营商…")}</span></div>}
                    {g && !g.enabled && <div className="note"><span>{t("节点地区查询已关闭，只显示地址。所有者可以在测速页的“路由追踪目标”里打开。")}</span></div>}
                    {g?.error && <div className="note warn"><TriangleAlert /><span>{ts(g.error)}</span></div>}
                    {routes.map((rt, i) => <RouteView key={rt.target} r={rt} geo={g?.hops} loading={geo === "loading"} open={i === 0 || routes.length <= 2} />)}
                    {g?.enabled && g.service && <span className="hint">{t("地区和运营商来自 {service}，按 IP 库推测，骨干网节点的位置常常只到国家或城市。", { service: g.service })}</span>}
                  </>
                )}
              </section>
            </>
          )}
        </>
      )}
    </Modal>
  );
}

/** Ookla 的服务器：“Example Telecom · Shanghai”。 */
function serverLabel(o: SpeedOokla): string {
  return [o.server.name, o.server.location].filter(Boolean).join(" · ") || "#" + o.server.id;
}

/** Ookla 测速的服务器和网络信息。 */
function OoklaView({ o, d }: { o: SpeedOokla; d: SpeedDetail }) {
  const t = useT();
  return (
    <div className="sr-grid">
      <section className="sr-sec sr-card">
        <h3>{t("测速服务器")}</h3>
        <dl className="kv sm">
          <dt>{t("服务器")}</dt><dd>{o.server.name || "—"}</dd>
          <dt>{t("位置")}</dt><dd>{[o.server.location, o.server.country].filter(Boolean).join(", ") || "—"}</dd>
          <dt>{t("编号")}</dt><dd className="mono">{o.server.id || "—"}</dd>
          <dt>{t("主机名")}</dt><dd className="mono">{o.server.host || "—"}</dd>
          <dt>{t("服务器地址")}</dt><dd className="mono">{o.server.ip || "—"}</dd>
          <dt>{t("程序")}</dt><dd>{o.binary === "system" ? t("主机上装的官方 speedtest {v}", { v: o.version ?? "" })
            : o.binary === "pinned" ? t("Hostower 下载的锁定版本 {v}", { v: o.version ?? "" }) : "—"}</dd>
        </dl>
        {o.url && <a className="sr-link" href={o.url} target="_blank" rel="noreferrer noopener">{t("在 speedtest.net 上查看这次结果")}</a>}
      </section>
      <section className="sr-sec sr-card">
        <h3>{t("网络")}</h3>
        <dl className="kv sm">
          <dt>{t("出口 IP")}</dt><dd className="mono">{d.net.ip || "—"}</dd>
          <dt>{t("运营商")}</dt><dd>{o.isp || "—"}</dd>
          <dt>{t("空闲时延迟")}</dt><dd className="num">{o.low || o.high ? `${ms(o.low)} – ${ms(o.high)} ms` : "—"}</dd>
          <dt>{t("下载时延迟")}</dt><dd className="num">{o.downLatency ? ms(o.downLatency) + " ms" : "—"}</dd>
          <dt>{t("上传时延迟")}</dt><dd className="num">{o.upLatency ? ms(o.upLatency) + " ms" : "—"}</dd>
        </dl>
        <span className="hint">{t("下载、上传时的延迟比空闲时高得越多，说明线路在满载时排队越严重。")}</span>
      </section>
    </div>
  );
}

/** 下载或上传的明细：“23.8 MB · 0.33 s · 首字节 107 ms”。 */
function transfer(t: (k: string, p?: Record<string, string | number>) => string, x?: SpeedTransfer): string | undefined {
  if (!x || x.bytes <= 0) return undefined;
  const parts = [bytes(x.bytes)];
  if (x.ms) parts.push((x.ms / 1000).toFixed(2) + " s");
  if (x.ttfb) parts.push(t("首字节 {ms} ms", { ms: ms(x.ttfb) }));
  if (x.conns && x.conns > 1) parts.push(t("{n} 条连接", { n: x.conns }));
  return parts.join(" · ");
}

/** “中国 北京市 北京”：去掉重复的层级。 */
/** 国家名按界面语言显示（查询服务返回的中文国家名有简有繁，比如“中国”和“中華人民共和國”）。 */
function countryName(g: SpeedGeo): string | undefined {
  if (!g.cc || typeof Intl.DisplayNames !== "function") return g.country;
  try { return new Intl.DisplayNames([lang()], { type: "region", style: "short" }).of(g.cc) ?? g.country; } catch { return g.country; }
}

function place(g?: SpeedGeo): string {
  if (!g) return "";
  const out: string[] = [];
  const bare = (v: string) => v.replace(/[市省]$/, "");
  for (const p of [countryName(g), g.region, g.city]) {
    const v = (p ?? "").trim();
    if (v && !out.some((x) => bare(x) === bare(v))) out.push(v);
  }
  return out.join(" ");
}

type HopRow = { kind: "hop"; h: SpeedHop } | { kind: "gap"; from: number; to: number };

/** 连续两跳以上不回应的折成一行。 */
function hopRows(hops: SpeedHop[]): HopRow[] {
  const rows: HopRow[] = [];
  for (let i = 0; i < hops.length; i++) {
    if (hops[i].ip) { rows.push({ kind: "hop", h: hops[i] }); continue; }
    let j = i;
    while (j + 1 < hops.length && !hops[j + 1].ip) j++;
    rows.push(j > i ? { kind: "gap", from: hops[i].n, to: hops[j].n } : { kind: "hop", h: hops[i] });
    i = j;
  }
  return rows;
}

/**
 * 找跨境的长途段：相邻两个有回应的节点之间延迟跳得最多、并且至少 50 ms 的那一处。
 * 返回跳过去之前那一跳的跳数，以及跳前跳后的延迟；没有这样的地方返回 null。
 * 长途段（多半是海缆）两侧的延迟差一大截，比位置库可靠：位置库常按地址的登记地标注，比如运营商国际网络的地址都标在总部。
 */
function longHaul(hops: SpeedHop[]): { after: number; from: number; to: number } | null {
  type Seen = SpeedHop & { ms: number };
  // 先去掉明显不对的单跳。先去“凹点”（比前后两跳都低 50 ms 以上）：多半是探测包被负载均衡分到别的路径、
  // 在半路就到期了，回应的是离主机很近的路由器；往远处走延迟不会突然变小，所以凹点最不可信。
  // 再去“尖峰”（比前后两跳都高 50 ms 以上）：多半是那台路由器回应 ICMP 慢。
  const drop = (list: Seen[], bad: (p: number, c: number, n: number) => boolean) =>
    list.filter((h, i) => i === 0 || i === list.length - 1 || !bad(list[i - 1].ms, h.ms, list[i + 1].ms));
  const seen = hops.filter((h): h is Seen => !!h.ip && !!h.ms);
  const clean = drop(drop(seen, (p, c, n) => c < p - 50 && c < n - 50), (p, c, n) => c > p + 50 && c > n + 50);
  let best: { after: number; from: number; to: number } | null = null;
  for (let i = 1; i < clean.length; i++) {
    const prev = clean[i - 1], h = clean[i];
    if (h.ms - prev.ms >= 50 && (!best || h.ms - prev.ms > best.to - best.from)) best = { after: prev.n, from: prev.ms, to: h.ms };
  }
  return best;
}

function RouteView({ r, geo, loading, open }: { r: SpeedRoute; geo?: Record<string, SpeedGeo>; loading: boolean; open: boolean }) {
  const t = useT();
  // traceroute 到了目标就停；最后一跳不回应说明没走到（目标是域名时打印的是 IP，所以不拿目标比）。
  const unreached = r.hops.length > 0 && !r.hops[r.hops.length - 1].ip;
  // 到公网目标却只有一跳：真实网络里至少要经过网关，这种结果是网络环境造成的假象。
  const direct = r.hops.length === 1 && !!r.hops[0].ip && r.hops[0].net !== "LAN";
  const end = [...r.hops].reverse().find((h) => h.ip && h.ms);
  const haul = longHaul(r.hops);
  // 长途段之前的节点，位置库却说它和目标那一侧（最后一个查得到地区的节点）在同一个国家：
  // 按延迟看它离主机很近，位置多半是地址的登记地。
  const farCC = haul ? [...r.hops].reverse().find((h) => h.n > haul.after && h.ip && geo?.[h.ip]?.cc)?.ip : undefined;
  const far = farCC ? geo?.[farCC]?.cc : undefined;
  const doubtful = (h: SpeedHop) => !!haul && !!far && h.n <= haul.after && !!h.ip && geo?.[h.ip]?.cc === far;
  return (
    <details className="sr-route" open={open}>
      <summary>
        <RouteIcon className="muted" />
        <b>{r.name ? ts(r.name) : r.target}</b>
        {r.name && <span className="mono muted">{r.target}</span>}
        {r.error ? <span className="badge crit">{ts(r.error)}</span>
          : r.line ? r.line.split(" + ").map((l) => <span key={l} className="badge accent">{ts(l)}</span>)
          : <span className="badge neutral">{t("没有经过认得出的骨干网")}</span>}
        <span className="grow" />
        {r.hops.length > 0 && <span className="muted num">{t("{n} 跳", { n: r.hops.length })}{end ? " · " + ms(end.ms) + " ms" : ""}</span>}
      </summary>
      {r.hops.length > 0 && (
        <div className="table-wrap">
          <table className="table sr-table sr-hops">
            <thead><tr><th>{t("跳")}</th><th>{t("IP 地址")}</th><th>{t("位置")}</th><th>{t("运营商")}</th><th>{t("延迟")}</th></tr></thead>
            <tbody>
              {hopRows(r.hops).map((row) => {
                if (row.kind === "gap") return (
                  <tr key={row.from} className="muted">
                    <td className="num">{row.from}–{row.to}</td>
                    <td className="mono">*</td>
                    <td colSpan={3}>{t("这 {n} 跳都没有回应", { n: row.to - row.from + 1 })}</td>
                  </tr>
                );
                const h = row.h;
                const gi = h.ip ? geo?.[h.ip] : undefined;
                const lan = h.net === "LAN";
                const asn = gi?.as?.split(" ")[0];
                const hop = (
                  <tr key={h.n} className={h.ip ? "" : "muted"}>
                    <td className="num muted">{h.n}</td>
                    <td className="mono">{h.ip || "*"}</td>
                    <td>{lan ? <span className="muted">{t("内网")}</span> : gi ? <span className="sr-isp">{place(gi) || "—"}{doubtful(h) && <span className="badge warn" title={t("位置库多按地址的登记地标注（运营商国际网络的地址常标在总部）。按延迟看，这一跳还在长途段之前、离主机很近。")}>{t("位置可能不准")}</span>}</span> : <span className="muted">{h.ip && loading ? "…" : "—"}</span>}</td>
                    <td>
                      <span className="sr-isp">
                        {h.net && !lan && <span className="badge accent">{t(netNames[h.net] ?? h.net)}</span>}
                        {gi?.isp && <span>{gi.isp}</span>}
                        {asn && <span className="mono muted" title={gi?.as}>{asn}</span>}
                        {!(h.net && !lan) && !gi?.isp && !asn && <span className="muted">—</span>}
                      </span>
                    </td>
                    <td className="num">{h.ip && h.ms ? ms(h.ms) + " ms" : "—"}</td>
                  </tr>
                );
                if (!haul || h.n !== haul.after) return hop;
                return [hop, (
                  <tr key={h.n + "-haul"} className="sr-haul">
                    <td colSpan={5}><ArrowDown /><span>{t("延迟在这里从 {a} ms 跳到 {b} ms：多半是长途段（跨境或海缆），这里之前的节点离主机近，之后的离目标近。", { a: ms(haul.from), b: ms(haul.to) })}</span></td>
                  </tr>
                )];
              })}
            </tbody>
          </table>
        </div>
      )}
      {unreached && !r.error && <span className="hint">{t("最后一跳没有到达目标：中间有节点不回应，或者目标不回应探测。")}</span>}
      {direct && <span className="hint">{t("只有 1 跳就到了目标，中间一个节点都没有：多半是主机所在的网络不按 TTL 逐跳转发探测包，由网络自己代答（比如 Docker Desktop 和一些容器、虚拟化的网络）。这种环境看不到真实路由，在真实的服务器上测才准。")}</span>}
    </details>
  );
}

/** 路由追踪目标：所有者可以改。名字只给人看；目标是 IP 地址或者域名。 */
export function SpeedTargets({ targets, max, presets, geo, geoService, reload }: { targets: SpeedTarget[]; max: number; presets: SpeedPreset[]; geo: boolean; geoService: string; reload: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [rows, setRows] = useState<SpeedTarget[]>(targets);
  const [busy, setBusy] = useState(false);
  useEffect(() => setRows(targets), [targets]);
  const dirty = JSON.stringify(rows) !== JSON.stringify(targets);
  const save = () => {
    setBusy(true);
    api.speedtestTargets(rows.filter((r) => r.target.trim() || r.name.trim())).then(() => { toast(t("已保存")); reload(); }).catch(fail).finally(() => setBusy(false));
  };
  const set = (i: number, k: keyof SpeedTarget, v: string) => setRows(rows.map((r, j) => (j === i ? { ...r, [k]: v } : r)));
  // 常用目标：点一下加入，再点一下移出（改完要保存）。名字按当前界面语言写进去，之后就是所有者自己的数据。
  const cities = [...new Set(presets.map((p) => p.city))];
  const isps = [...new Set(presets.map((p) => p.isp))];
  const togglePreset = (p: SpeedPreset) => {
    if (rows.some((r) => r.target === p.target)) setRows(rows.filter((r) => r.target !== p.target));
    else setRows([...rows.filter((r) => r.target.trim() || r.name.trim()), { name: t("{city}{isp}", { city: t(p.city), isp: t(p.isp) }), target: p.target }]);
  };
  const setGeo = (on: boolean) => {
    setBusy(true);
    api.speedtestGeoSetting(on).then(() => { toast(t("已保存")); reload(); }).catch(fail).finally(() => setBusy(false));
  };
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("路由追踪目标")}</h2><span className="muted num">{rows.length}/{max}</span>
        <span className="grow" />
        {owner && <button className="btn ghost sm" disabled={busy || rows.length >= max} onClick={() => setRows([...rows, { name: "", target: "" }])}><Plus />{t("添加")}</button>}
        {owner && <button className="btn primary sm" disabled={busy || !dirty} onClick={save}><Save />{t("保存")}</button>}
      </div>
      <div className="card-body sr-targets-body">
        <div className="col" style={{ gap: 10, minWidth: 0 }}>
        <span className="hint">{t("测速时可以顺便从主机追踪到这些目标的路由，看经过哪些节点、走的哪条线路。主机在境外时，追踪到国内的地址看的是回程（主机到你这边）：CN2 GIA、联通 9929、移动 CMI 这类线路就在这里看出来。最准的目标是你自己家里或公司的公网 IP，其次是你所在城市各运营商的地址。去程（你到主机）要在你自己的电脑上用 tracert 或 mtr 追踪主机的 IP。")}</span>
        {rows.length === 0 && <span className="muted">{t("还没有目标")}</span>}
        {rows.map((r, i) => (
          <div key={i} className="row sr-target">
            <input className="input" placeholder={t("名字（可选）")} value={r.name} disabled={!owner || busy} maxLength={40} onChange={(e) => set(i, "name", e.target.value)} />
            <input className="input mono" placeholder={t("IP 地址或域名")} value={r.target} disabled={!owner || busy} onChange={(e) => set(i, "target", e.target.value)} />
            {owner && <button className="btn ghost icon sm" disabled={busy} title={t("删除")} aria-label={t("删除")} onClick={() => setRows(rows.filter((_, j) => j !== i))}><Trash2 /></button>}
          </div>
        ))}
        </div>
        <div className="col" style={{ gap: 14, minWidth: 0 }}>
        {owner && presets.length > 0 && (
          <div className="sr-presets">
            <div className="row" style={{ gap: 8 }}><b>{t("常用目标")}</b><span className="hint">{t("点一下加入或移出，改完记得保存。")}</span></div>
            <div className="sr-preset-grid" style={{ gridTemplateColumns: `auto repeat(${isps.length}, minmax(0, 1fr))` }}>
              {cities.map((city) => (
                <Fragment key={city}>
                  <span className="muted">{t(city)}</span>
                  {isps.map((isp) => {
                    const p = presets.find((x) => x.city === city && x.isp === isp);
                    if (!p) return <span key={isp} />;
                    const on = rows.some((r) => r.target === p.target);
                    return (
                      <button key={isp} type="button" className={"sr-preset" + (on ? " on" : "")} aria-pressed={on} title={p.target}
                        disabled={busy || (!on && rows.filter((r) => r.target.trim()).length >= max)} onClick={() => togglePreset(p)}>
                        {on && <Check />}{t(isp)}
                      </button>
                    );
                  })}
                </Fragment>
              ))}
            </div>
            <span className="hint">{t("这些是国内三家运营商在各地的公共 DNS 服务器地址，和常见的回程测试工具用的一样。有的不回应最后一跳，但沿路经过的骨干网照样能看到。")}</span>
          </div>
        )}
        <div className="sr-geo">
          <label className="check">
            <input type="checkbox" checked={geo} disabled={!owner || busy} onChange={(e) => setGeo(e.target.checked)} />
            {t("在报告里显示每一跳的地区和运营商")}
          </label>
          <span className="hint">{t("打开报告时，面板把路由里各跳的公网 IP 发给 {service} 查询（不含内网地址，结果只在内存里缓存一天）。关掉后报告里只有 IP。", { service: geoService })}</span>
        </div>
        </div>
      </div>
    </div>
  );
}
