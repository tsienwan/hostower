// 月流量：主机页的流量卡片（本周期用量、按天 / 按月的柱状图、配额设置），总览上的小进度条和流量排行。
// 数据都来自采样时已经记下的网卡计数器，这里不会去主机上执行任何东西。
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Activity, Settings2, TriangleAlert } from "lucide-react";
import { api, type DayTraffic, type HostTraffic, type Quota, type QuotaMode, type QuotaStatus, type TrafficView } from "../api";
import { fail, toast, useStore } from "../store";
import { useCached } from "../cache";
import { Modal, SortTh, bytes, lastOnly, pctText, poll, useSort } from "../ui";
import { Select } from "../controls";
import { askConfirm } from "../dialogs";
import { useT } from "../i18n";

export const modeNames: Record<QuotaMode, string> = { out: "仅出站", in: "仅入站", sum: "入站 + 出站", max: "入站、出站取较大值" };

/** 按配额的单位写字节数：按 1024 换算时写 GiB / TiB。 */
export function quotaBytes(v: number, q: { unit: "GB" | "TB"; base: number }): string {
  const per = q.unit === "TB" ? q.base ** 4 : q.base ** 3;
  const unit = q.base === 1024 ? q.unit.replace("B", "iB") : q.unit;
  const x = v / per;
  return `${Math.abs(x) >= 100 ? x.toFixed(0) : Math.abs(x) >= 10 ? x.toFixed(1) : x.toFixed(2)} ${unit}`;
}

/** 已用百分比的色调：超出额度是严重，过了第一档告警是警告。 */
export function quotaTone(q: QuotaStatus): string {
  if (q.pct >= 100) return "crit";
  const first = q.thresholds?.length ? q.thresholds[0] : 80;
  return q.pct >= first ? "warn" : "";
}

const lastDay = (end: string) => {
  const d = new Date(end + "T00:00:00");
  d.setDate(d.getDate() - 1);
  return isoDay(d);
};
const isoDay = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

/** 总览表格里的小进度条：百分比数字 + 细条，不只靠颜色。 */
export function QuotaMini({ q }: { q: QuotaStatus }) {
  const t = useT();
  const tone = quotaTone(q);
  const v = Math.max(0, Math.min(100, q.pct));
  return (
    <div className="usage quota-mini" title={t("本周期流量 {used} / {limit}，还剩 {days} 天", { used: quotaBytes(q.used, q), limit: quotaBytes(q.limit, q), days: q.daysLeft.toFixed(1) })}>
      <div className="usage-top"><b className={tone}>{pctText(q.pct)}</b><span>{quotaBytes(q.limit, q)}</span></div>
      <div className="meter-track"><div className={`meter-fill ${tone}`} style={{ width: `${v}%` }} /></div>
    </div>
  );
}

/** 主机页的流量卡片。 */
export function TrafficCard({ host, status }: { host: string; status?: QuotaStatus }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [view, setView] = useState<TrafficView | null>(null);
  const [err, setErr] = useState("");
  const [mode, setMode] = useState<"days" | "periods">(() => (localStorage.getItem("hostower.traffic.view") === "periods" ? "periods" : "days"));
  const [editing, setEditing] = useState(false);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let on = true;
    const next = lastOnly();
    const load = () => {
      const fresh = next();
      api.hostTraffic(host).then((v) => { if (on && fresh()) { setView(v); setErr(""); } }).catch((e) => { if (on && fresh()) setErr(String(e?.message ?? e)); });
    };
    load();
    const stop = poll(load, 60_000);
    return () => { on = false; stop(); };
  }, [host, tick]);
  useEffect(() => { try { localStorage.setItem("hostower.traffic.view", mode); } catch { /* 存不了就算了 */ } }, [mode]);

  const q = status ?? view?.status ?? undefined;
  const fmt = (v: number) => (q ? quotaBytes(v, q) : bytes(v));
  const totals = useMemo(() => (view?.days ?? []).reduce((a, d) => ({ in: a.in + d.in, out: a.out + d.out }), { in: 0, out: 0 }), [view]);
  const tone = q ? quotaTone(q) : "";
  const over = q && q.projected > q.limit && q.pct < 100;

  return (
    <div className="card">
      {editing && view && <QuotaDialog host={host} view={view} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); setTick((n) => n + 1); }} />}
      <div className="card-head wrap">
        <Activity size={16} className="muted" />
        <h2>{t("流量")}</h2>
        {view && <span className="muted num" style={{ fontSize: 12.5 }}>{t("{start} ~ {end}", { start: view.start, end: lastDay(view.end) })}</span>}
        <span className="grow" />
        <div className="row" role="group" aria-label={t("流量图")} style={{ gap: 4 }}>
          <button className={"tag" + (mode === "days" ? " on" : "")} onClick={() => setMode("days")}>{t("按天")}</button>
          <button className={"tag" + (mode === "periods" ? " on" : "")} onClick={() => setMode("periods")}>{t("按月")}</button>
        </div>
        {owner && <button className="btn sm" disabled={!view} onClick={() => setEditing(true)}><Settings2 />{view?.quota ? t("修改配额") : t("设置配额")}</button>}
      </div>
      <div className="card-body col" style={{ gap: 12 }}>
        {err && <div className="note crit"><TriangleAlert /><span>{t("读取流量失败：{error}", { error: err })}</span></div>}
        {q ? (
          <>
            <div className="quota-head">
              <div className="metric-val">{quotaBytes(q.used, q)}<small> / {quotaBytes(q.limit, q)}</small></div>
              <b className={"quota-pct " + tone}>{pctText(q.pct)}</b>
              {q.adjusted && <span className="badge neutral" title={t("按服务商面板上的已用量校准过")}>{t("已校准")}</span>}
              <span className="grow" />
              <span className="muted">{t(modeNames[q.mode])}</span>
            </div>
            <div className="meter-track"><div className={`meter-fill ${tone}`} style={{ width: `${Math.max(0, Math.min(100, q.pct))}%` }} /></div>
            <div className="quota-facts">
              <span>{t("入站")} <b>{quotaBytes(q.in, q)}</b></span>
              <span>{t("出站")} <b>{quotaBytes(q.out, q)}</b></span>
              <span>{t("剩余")} <b>{t("{n} 天", { n: q.daysLeft.toFixed(1) })}</b></span>
              <span>{t("预计周期末")} <b className={over ? "warn" : ""}>{q.projected > 0 ? quotaBytes(q.projected, q) : "—"}</b></span>
              <span>{t("剩余额度")} <b>{quotaBytes(Math.max(0, q.limit - q.used), q)}</b></span>
            </div>
            {q.pct >= 100 ? (
              <div className="note crit"><TriangleAlert /><span>{t("本周期的流量已经超出额度。")}</span></div>
            ) : over ? (
              <div className="note warn"><TriangleAlert /><span>{t("按本周期的平均速度，到周期末约用 {v}，会超出额度。", { v: quotaBytes(q.projected, q) })}</span></div>
            ) : null}
          </>
        ) : view && (
          <div className="quota-facts">
            <span className="muted">{t("没有设流量配额，按自然月统计。")}</span>
            <span>{t("入站")} <b>{bytes(totals.in)}</b></span>
            <span>{t("出站")} <b>{bytes(totals.out)}</b></span>
            <span>{t("合计")} <b>{bytes(totals.in + totals.out)}</b></span>
          </div>
        )}
        {view && (mode === "days"
          ? <TrafficBars bars={fillDays(view.days, view.start, view.end)} fmt={fmt} empty={t("这个周期还没有流量数据")} />
          : <TrafficBars bars={view.periods.map((p) => ({ key: p.start, label: p.start.slice(2, 7), title: `${p.start} ~ ${lastDay(p.end)}`, in: p.in, out: p.out }))} fmt={fmt} empty={t("还没有流量数据")} />)}
        <span className="hint">{t("只算出口网卡：隧道（WireGuard 等）、网桥、VLAN 子接口、绑定的成员网卡上是同一份流量，不重复算；lo、docker、veth 这类虚拟网卡也不算。按面板所在机器的时区划分日期。主机重启前最后几秒的流量可能漏算，以服务商的账单为准。")}</span>
      </div>
    </div>
  );
}

type Bar = { key: string; label: string; title: string; in: number; out: number; future?: boolean };

function fillDays(days: DayTraffic[], start: string, end: string): Bar[] {
  const by = new Map(days.map((d) => [d.day, d]));
  const out: Bar[] = [];
  const today = isoDay(new Date());
  for (const d = new Date(start + "T00:00:00"); isoDay(d) < end && out.length < 31; d.setDate(d.getDate() + 1)) {
    const k = isoDay(d), v = by.get(k);
    out.push({ key: k, label: String(d.getDate()), title: k, in: v?.in ?? 0, out: v?.out ?? 0, future: k > today });
  }
  return out;
}

/** 入站、出站并排的柱状图。每根柱子有提示框，图例写明颜色对应哪个方向。 */
function TrafficBars({ bars, fmt, empty }: { bars: Bar[]; fmt: (v: number) => string; empty: string }) {
  const t = useT();
  const [hover, setHover] = useState(-1);
  // 按容器的实际宽度画（不拉伸）：文字和柱子的圆角才不会变形。
  const box = useRef<HTMLDivElement>(null);
  const [W, setW] = useState(720);
  const max = Math.max(0, ...bars.map((b) => Math.max(b.in, b.out)));
  const has = max > 0;
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const measure = () => setW(Math.max(160, Math.round(el.clientWidth)));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [has]); // 从“没有数据”变成有图时换了一个元素，要重新挂上
  const H = 140, pad = 18, slot = W / Math.max(1, bars.length), bw = Math.max(1, Math.min(14, slot * 0.36));
  // 日期标签每个至少留 28 像素，放不下就隔几个标一个。
  const every = Math.max(1, Math.ceil(bars.length / Math.max(1, Math.floor(W / 28))));
  const y = (v: number) => H - pad - (v / max) * (H - pad - 8);
  const h = hover >= 0 ? bars[hover] : null;
  if (max <= 0) return <div ref={box} className="empty sm"><span>{empty}</span></div>;
  return (
    <div className="traffic-chart" ref={box}>
      <div className="legend">
        <span><svg width="10" height="10" aria-hidden><rect width="10" height="10" rx="2" fill="var(--accent)" /></svg>{t("入站")}</span>
        <span><svg width="10" height="10" aria-hidden><rect width="10" height="10" rx="2" fill="var(--accent-2)" /></svg>{t("出站")}</span>
        <span className="grow" />
        {h ? <span><span className="muted">{h.title}</span><b>↓ {fmt(h.in)}</b><b>↑ {fmt(h.out)}</b></span> : <span className="muted">{t("最高 {v}", { v: fmt(max) })}</span>}
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} width={W} height={H} role="img" aria-label={t("流量图")} onMouseLeave={() => setHover(-1)}>
        <line x1="0" x2={W} y1={H - pad} y2={H - pad} stroke="var(--border)" />
        {bars.map((b, i) => {
          const cx = slot * i + slot / 2;
          return (
            <g key={b.key} onMouseEnter={() => setHover(i)} opacity={hover >= 0 && hover !== i ? 0.55 : 1}>
              <title>{`${b.title}  ↓ ${fmt(b.in)}  ↑ ${fmt(b.out)}`}</title>
              <rect x={slot * i} y={0} width={slot} height={H} fill="transparent" />
              {!b.future && <>
                <rect x={cx - bw - 1} y={y(b.in)} width={bw} height={Math.max(0, H - pad - y(b.in))} rx="2" fill="var(--accent)" />
                <rect x={cx + 1} y={y(b.out)} width={bw} height={Math.max(0, H - pad - y(b.out))} rx="2" fill="var(--accent-2)" />
              </>}
              {i % every === 0 && (
                <text x={cx} y={H - 4} textAnchor="middle" fontSize="11" fill="var(--text-3)" style={{ fontVariantNumeric: "tabular-nums" }}>{b.label}</text>
              )}
            </g>
          );
        })}
      </svg>
    </div>
  );
}

/** 设置、修改、取消配额。 */
function QuotaDialog({ host, view, onClose, onSaved }: { host: string; view: TrafficView; onClose: () => void; onSaved: () => void }) {
  const t = useT();
  const q0 = view.quota;
  const [limit, setLimit] = useState(q0 ? String(q0.limit) : "1");
  const [unit, setUnit] = useState<"GB" | "TB">(q0?.unit ?? "TB");
  const [base, setBase] = useState<1000 | 1024>(q0?.base ?? 1000);
  const [mode, setMode] = useState<QuotaMode>(q0?.mode ?? "out");
  const [resetDay, setResetDay] = useState(String(q0?.resetDay ?? 1));
  const [iface, setIface] = useState(q0?.iface ?? "");
  const [thresholds, setThresholds] = useState((q0?.thresholds ?? view.defaultThresholds).join(", "));
  const [used, setUsed] = useState("");
  const [clear, setClear] = useState(false);
  const [busy, setBusy] = useState(false);

  const nLimit = Number(limit), nDay = Number(resetDay), nUsed = used.trim() === "" ? undefined : Number(used);
  const th = thresholds.split(/[,，、\s]+/).filter(Boolean).map(Number);
  const valid = nLimit > 0 && Number.isInteger(nDay) && nDay >= 1 && nDay <= 31 && th.every((v) => Number.isInteger(v) && v >= 1 && v <= 200) && th.length <= 5 &&
    (nUsed === undefined || (Number.isFinite(nUsed) && nUsed >= 0));
  const calibrated = !!q0?.adjust && q0.adjustStart === view.start;
  const unitLabel = base === 1024 ? unit.replace("B", "iB") : unit;

  const save = async () => {
    setBusy(true);
    try {
      const quota: Quota = { limit: nLimit, unit, base, mode, resetDay: nDay, iface, thresholds: th };
      await api.setHostTraffic(host, { quota, used: nUsed, clearAdjust: clear });
      toast(t("流量配额已保存"));
      onSaved();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const remove = async () => {
    if (!(await askConfirm(t("取消 {host} 的流量配额？已经记下的流量不会删除。", { host }), { danger: true, confirmLabel: t("取消配额") }))) return;
    setBusy(true);
    try {
      await api.deleteHostTraffic(host);
      toast(t("流量配额已取消"));
      onSaved();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <Modal title={t("{host} 的流量配额", { host })} onClose={onClose}
      footer={<>
        {q0 && <button className="btn danger" disabled={busy} onClick={remove}>{t("取消配额")}</button>}
        <span className="grow" />
        <button className="btn" onClick={onClose}>{t("取消")}</button>
        <button className="btn primary" disabled={busy || !valid} onClick={save}>{t("保存")}</button>
      </>}>
      <div className="form-grid">
        <div className="field">
          <label htmlFor="q-limit">{t("每月额度")}</label>
          <div className="row" style={{ gap: 6 }}>
            <input id="q-limit" className="input num" inputMode="decimal" value={limit} onChange={(e) => setLimit(e.target.value)} style={{ flex: 1, minWidth: 0 }} autoFocus />
            <div style={{ width: 96 }}><Select value={unit} onChange={(v) => setUnit(v as "GB" | "TB")} options={[{ value: "GB", label: "GB" }, { value: "TB", label: "TB" }]} /></div>
          </div>
        </div>
        <div className="field">
          <label htmlFor="q-base">{t("换算")}</label>
          <Select id="q-base" value={String(base)} onChange={(v) => setBase(Number(v) as 1000 | 1024)}
            options={[{ value: "1000", label: t("1 GB = 1000³ 字节"), hint: t("多数服务商") }, { value: "1024", label: t("1 GiB = 1024³ 字节") }]} />
        </div>
        <div className="field">
          <label htmlFor="q-mode">{t("计费方式")}</label>
          <Select id="q-mode" value={mode} onChange={(v) => setMode(v as QuotaMode)}
            options={view.modes.map((m) => ({ value: m, label: t(modeNames[m]) }))} />
        </div>
        <div className="field">
          <label htmlFor="q-day">{t("每月重置日")}</label>
          <input id="q-day" className="input num" inputMode="numeric" value={resetDay} onChange={(e) => setResetDay(e.target.value)} />
          <span className="hint">{t("1–31；没有这一天的月份按当月最后一天算")}</span>
        </div>
        <div className="field">
          <label htmlFor="q-iface">{t("统计的网卡")}</label>
          <Select id="q-iface" mono value={iface} onChange={setIface}
            options={[{ value: "", label: t("出口网卡（默认）"), hint: t("隧道、网桥等不重复算") }, ...[...new Set([...view.ifaces, ...(iface ? [iface] : [])])].map((n) => ({ value: n, label: n }))]} />
        </div>
        <div className="field">
          <label htmlFor="q-th">{t("告警百分比")}</label>
          <input id="q-th" className="input num" value={thresholds} onChange={(e) => setThresholds(e.target.value)} placeholder="80, 95, 100" />
          <span className="hint">{t("到达时通过通知渠道发一次，每个周期每档只发一次；留空不告警")}</span>
        </div>
      </div>
      <div className="field">
        <label htmlFor="q-used">{t("校准：服务商面板上显示的本周期已用量（{unit}）", { unit: unitLabel })}</label>
        <input id="q-used" className="input num" inputMode="decimal" value={used} onChange={(e) => { setUsed(e.target.value); if (e.target.value) setClear(false); }}
          placeholder={t("不填就不校准")} />
        <span className="hint">{t("面板接入之前已经用掉的部分这里测不到；填上服务商显示的数字，本周期就按它对齐。校准只对当前周期有效。")}</span>
        {calibrated && !used && (
          <label className="check"><input type="checkbox" checked={clear} onChange={(e) => setClear(e.target.checked)} />
            {t("清除本周期的校准（当前差额 {v}）", { v: quotaBytes(q0!.adjust ?? 0, q0!) })}</label>
        )}
      </div>
    </Modal>
  );
}

/** 总览上的流量排行：各主机本周期的用量。设了配额的按配额的周期和网卡，没设的按自然月。 */
export function TrafficRanking() {
  const t = useT();
  // 再回到总览时先显示上次的排行，不用等接口返回才出现；新数据到了再换上。
  const [list, setList] = useCached<HostTraffic[]>("traffic.ranking");
  useEffect(() => {
    let on = true;
    const next = lastOnly();
    const load = () => { const fresh = next(); api.trafficRanking().then((r) => on && fresh() && setList(r.hosts)).catch(() => {}); };
    load();
    const stop = poll(load, 60_000);
    return () => { on = false; stop(); };
  }, []);
  const rows = useMemo(() => (list ?? []).filter((h) => h.in + h.out > 0 || h.quota), [list]);
  const sorter = useSort(rows, { name: (h) => h.host.toLowerCase(), in: (h) => h.in, out: (h) => h.out, total: (h) => h.in + h.out, pct: (h) => h.quota?.pct ?? -1 });
  const sorted = sorter.sort ? sorter.sorted : [...rows].sort((a, b) => (b.quota?.pct ?? -1) - (a.quota?.pct ?? -1) || b.in + b.out - (a.in + a.out));
  if (!rows.length) return null;
  const fmt = (v: number, h: HostTraffic) => (h.quota ? quotaBytes(v, h.quota) : bytes(v));
  return (
    <div className="card">
      <div className="card-head wrap">
        <Activity size={16} className="muted" />
        <h2>{t("流量排行")}</h2>
        <span className="muted hide-sm" style={{ fontSize: 12.5 }}>{t("本周期；没设配额的主机按自然月")}</span>
      </div>
      <div className="table-wrap">
        <table className="table">
          <thead><tr>
            <SortTh s={sorter} k="name">{t("主机")}</SortTh>
            <th className="hide-sm">{t("周期")}</th>
            <SortTh s={sorter} k="in" desc className="hide-sm">{t("入站")}</SortTh>
            <SortTh s={sorter} k="out" desc className="hide-sm">{t("出站")}</SortTh>
            <SortTh s={sorter} k="total" desc>{t("合计")}</SortTh>
            <SortTh s={sorter} k="pct" desc>{t("配额")}</SortTh>
          </tr></thead>
          <tbody>
            {sorted.slice(0, 20).map((h) => (
              <tr key={h.host}>
                <td><Link to={`/hosts/${h.host}`} style={{ fontWeight: 600 }}>{h.host}</Link></td>
                <td className="hide-sm num dim" style={{ fontSize: 12.5 }}>{h.start} ~ {lastDay(h.end)}</td>
                <td className="hide-sm num">{fmt(h.in, h)}</td>
                <td className="hide-sm num">{fmt(h.out, h)}</td>
                <td className="num">{fmt(h.in + h.out, h)}</td>
                <td style={{ minWidth: 120 }}>{h.quota ? <QuotaMini q={h.quota} /> : <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
