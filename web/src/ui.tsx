// 通用界面组件。颜色只表达状态；状态从不只靠颜色——总有文字或图标同时出现。
import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ArrowDown, ArrowUp, ArrowUpDown, Ban, Check, ChevronLeft, ChevronRight, CircleAlert, CircleCheck, CircleDashed, CircleX, Clock, Loader, MinusCircle, ShieldAlert, X } from "lucide-react";
import type { Level, ReqState, Sample, Target } from "./api";
import { formatStamp, t, useT } from "./i18n";

export const levelMeta: Record<Level, { cls: string; name: string; desc: string }> = {
  L0: { cls: "ok", name: "只读", desc: "自动执行" },
  L1: { cls: "warn", name: "变更", desc: "需要一人批准" },
  L2: { cls: "serious", name: "高危", desc: "需要二次验证身份并输入确认内容" },
  L3: { cls: "crit", name: "禁止", desc: "任何角色、任何入口都不能放行" },
};

export function LevelBadge({ level, label }: { level: Level; label?: boolean }) {
  const t = useT();
  const m = levelMeta[level];
  return (
    <span className={`badge ${m.cls}`} title={t("{level} {name}：{desc}", { level, name: t(m.name), desc: t(m.desc) })}>
      <span className="level">{level}</span>
      {label && <span>{t(m.name)}</span>}
    </span>
  );
}

const stateMeta: Record<ReqState, { cls: string; text: string; icon: ReactNode }> = {
  pending: { cls: "warn", text: "等待批准", icon: <Clock /> },
  running: { cls: "accent", text: "执行中", icon: <Loader className="spin" /> },
  done: { cls: "ok", text: "完成", icon: <Check /> },
  failed: { cls: "crit", text: "失败", icon: <X /> },
  denied: { cls: "neutral", text: "已拒绝", icon: <Ban /> },
  rejected: { cls: "crit", text: "内核拒绝", icon: <ShieldAlert /> },
  expired: { cls: "neutral", text: "已过期", icon: <Clock /> },
  cancelled: { cls: "neutral", text: "已取消", icon: <MinusCircle /> },
  interrupted: { cls: "neutral", text: "已中断", icon: <MinusCircle /> },
};

export function StateBadge({ state }: { state: ReqState }) {
  const t = useT();
  const m = stateMeta[state];
  return <span className={`badge ${m.cls}`}>{m.icon}{t(m.text)}</span>;
}

export function TargetIcon({ state }: { state: Target["state"] }) {
  const t = useT();
  const s = { width: 15, height: 15 };
  switch (state) {
    case "ok": return <CircleCheck style={{ ...s, color: "var(--ok)" }} aria-label={t("成功")} />;
    case "failed": return <CircleX style={{ ...s, color: "var(--crit)" }} aria-label={t("失败")} />;
    case "error": return <CircleAlert style={{ ...s, color: "var(--crit)" }} aria-label={t("出错")} />;
    case "running": return <Loader className="spin" style={{ ...s, color: "var(--accent)" }} aria-label={t("执行中")} />;
    case "skipped": return <MinusCircle style={{ ...s, color: "var(--text-3)" }} aria-label={t("已跳过")} />;
    default: return <CircleDashed style={{ ...s, color: "var(--text-3)" }} aria-label={t("等待")} />;
  }
}

export const targetText: Record<Target["state"], string> = {
  waiting: "等待", running: "执行中", ok: "成功", failed: "失败", error: "出错", skipped: "已跳过",
};

const tone = (v: number) => (v >= 90 ? "crit" : v >= 75 ? "warn" : "");

/** 百分比的文字：10% 以下保留一位小数（0.2% 不会显示成 0%），以上取整；整数不带“.0”。 */
export function pctText(v: number): string {
  const s = v > 0 && v < 10 ? v.toFixed(1).replace(/\.0$/, "") : v.toFixed(0);
  return s + "%";
}

/** 用量条：细轨道、数值永远以文字出现；超过阈值时变色并加粗。 */
export function Meter({ value, label }: { value?: number; label?: string }) {
  const t = useT();
  if (value === undefined || Number.isNaN(value)) return <span className="muted">—</span>;
  const v = Math.max(0, Math.min(100, value));
  const tn = tone(v);
  return (
    <div className="meter" title={label ? `${label} ${v.toFixed(1)}%${tn === "crit" ? t("（过高）") : tn === "warn" ? t("（偏高）") : ""}` : undefined}>
      <div className="meter-track"><div className={`meter-fill ${tn}`} style={{ width: `${v}%` }} /></div>
      <span className={`meter-val ${tn}`}>{pctText(v)}</span>
    </div>
  );
}

/** 迷你趋势线：单序列、单色、无坐标轴；精确数值在旁边的文字里。 */
export function Sparkline({ data, width = 72, height = 22, max = 100 }: { data: number[]; width?: number; height?: number; max?: number }) {
  if (data.length < 2) return <svg className="spark" width={width} height={height} aria-hidden />;
  const top = Math.max(max, ...data) || 1;
  const pts = data.map((v, i) => [(i / (data.length - 1)) * (width - 2) + 1, height - 2 - (v / top) * (height - 4)] as const);
  const line = pts.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join("");
  return (
    <svg className="spark" width={width} height={height} aria-hidden>
      <path d={`${line}L${width - 1},${height}L1,${height}Z`} fill="var(--accent-soft)" />
      <path d={line} fill="none" stroke="var(--accent)" strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={pts[pts.length - 1][0]} cy={pts[pts.length - 1][1]} r="2" fill="var(--accent-3)" />
    </svg>
  );
}

export interface Series { pick: (s: Sample) => number; label?: string }

/** 趋势图：一个纵轴、最多两条线（第二条用虚线和另一种颜色，图例里有名字，不只靠颜色区分）。
 *  带纵轴刻度和时间轴；悬停显示十字线和各条线的数值。 */
export function TrendChart({ samples, series, format, max, height = 120 }: {
  samples: Sample[]; series: Series[]; format: (n: number) => string; max?: number; height?: number;
}) {
  const t = useT();
  const ref = useRef<HTMLDivElement>(null);
  const [w, setW] = useState(300);
  const [hover, setHover] = useState<number | null>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setW(el.clientWidth));
    ro.observe(el);
    setW(el.clientWidth);
    return () => ro.disconnect();
  }, []);
  const lines = useMemo(() => series.map((sr) => samples.map(sr.pick)), [samples, series]);
  const n = samples.length;
  if (n < 2) return <div ref={ref} className="chart muted" style={{ height, display: "grid", placeItems: "center", fontSize: 12.5 }}>{t("正在收集数据…")}</div>;
  const axis = 18, plot = height - axis;
  const peak = Math.max(...lines.flat(), 0);
  const top = max ?? (peak > 0 ? peak * 1.25 : 1);
  const x = (i: number) => (i / (n - 1)) * (w - 2) + 1;
  const y = (v: number) => plot - 1 - (Math.min(v, top) / top) * (plot - 12);
  const path = (vals: number[]) => vals.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join("");
  const long = samples[n - 1].ts - samples[0].ts > 20 * 3600_000;
  const time = (ts: number) => (long ? stamp(ts).slice(5, 16) : stamp(ts).slice(11, 16));
  const hi = hover ?? n - 1;
  const colors = ["var(--accent)", "var(--accent-2)"];
  return (
    <div
      ref={ref} className="chart" style={{ height }}
      onMouseMove={(e) => {
        const r = e.currentTarget.getBoundingClientRect();
        setHover(Math.max(0, Math.min(n - 1, Math.round(((e.clientX - r.left) / r.width) * (n - 1)))));
      }}
      onMouseLeave={() => setHover(null)}
    >
      <svg width={w} height={height} role="img" aria-label={t("趋势图，最新值 {values}", { values: series.map((sr, i) => (sr.label ? sr.label + " " : "") + format(lines[i][n - 1])).join(t("，")) })}>
        {[1, 0.5].map((f) => (
          <g key={f}>
            <line x1="0" x2={w} y1={y(top * f)} y2={y(top * f)} stroke="var(--border)" strokeDasharray="2 4" />
            <text x="2" y={y(top * f) - 4} className="axis-label">{format(top * f)}</text>
          </g>
        ))}
        <line x1="0" x2={w} y1={plot} y2={plot} stroke="var(--border)" />
        <path d={`${path(lines[0])}L${x(n - 1)},${plot}L${x(0)},${plot}Z`} fill="var(--accent-soft)" />
        {lines.map((vals, i) => (
          <path key={i} d={path(vals)} fill="none" stroke={colors[i]} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" strokeDasharray={i ? "5 4" : undefined} />
        ))}
        {hover !== null && <line x1={x(hi)} x2={x(hi)} y1="0" y2={plot} stroke="var(--border-strong)" />}
        {lines.map((vals, i) => <circle key={i} cx={x(hi)} cy={y(vals[hi])} r="3.5" fill={colors[i]} stroke="var(--surface-solid)" strokeWidth="2" />)}
        <text x="2" y={height - 4} className="axis-label">{time(samples[0].ts)}</text>
        <text x={w / 2} y={height - 4} className="axis-label" textAnchor="middle">{time(samples[Math.floor(n / 2)].ts)}</text>
        <text x={w - 2} y={height - 4} className="axis-label" textAnchor="end">{time(samples[n - 1].ts)}</text>
      </svg>
      {hover !== null && (
        <div className="chart-tip" style={{ left: Math.max(70, Math.min(w - 70, x(hi))) }}>
          <div className="muted">{long ? stamp(samples[hi].ts).slice(0, 16) : stamp(samples[hi].ts).slice(11)}</div>
          {series.map((sr, i) => (
            <div key={i} className="row" style={{ gap: 6 }}>
              <svg width="14" height="6" aria-hidden><line x1="0" x2="14" y1="3" y2="3" stroke={colors[i]} strokeWidth="2" strokeDasharray={i ? "5 4" : undefined} /></svg>
              {sr.label && <span className="dim">{sr.label}</span>}<b className="num">{format(lines[i][hi])}</b>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function Modal({ title, children, footer, onClose, wide }: { title: string; children: ReactNode; footer: ReactNode; onClose: () => void; wide?: boolean | "xl" }) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);
  return createPortal(
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={"modal" + (wide ? " wide" : "") + (wide === "xl" ? " xl" : "")} role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-head">{title}</div>
        <div className="modal-body">{children}</div>
        <div className="modal-foot">{footer}</div>
      </div>
    </div>,
    document.body,
  );
}

export function Empty({ icon, title, children }: { icon: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      {icon}
      <strong>{title}</strong>
      {children && <div style={{ maxWidth: 420 }}>{children}</div>}
    </div>
  );
}

// ---- 格式化 ----
export function ago(ts: string | number | undefined): string {
  if (!ts) return "—";
  const d = typeof ts === "number" ? ts : Date.parse(ts);
  const s = Math.max(0, (Date.now() - d) / 1000);
  if (s < 5) return t("刚刚");
  if (s < 60) return t("{n} 秒前", { n: Math.floor(s) });
  if (s < 3600) return t("{n} 分钟前", { n: Math.floor(s / 60) });
  if (s < 86400) return t("{n} 小时前", { n: Math.floor(s / 3600) });
  return t("{n} 天前", { n: Math.floor(s / 86400) });
}

/** 短时间：月-日 时:分:秒。 */
export function clock(ts: string): string {
  return formatStamp(ts).slice(5);
}

/** 运行时间，精确到分钟。 */
export function uptime(sec?: number): string {
  if (!sec) return "—";
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  if (d > 0) return t("{d} 天 {h} 小时 {m} 分", { d, h, m });
  if (h > 0) return t("{h} 小时 {m} 分", { h, m });
  return t("{m} 分", { m });
}

/** 完整时间：年-月-日 时:分:秒，按“设置”里选的时区显示。 */
export function stamp(ts: string | number): string {
  return formatStamp(ts);
}

/** 速率：字节每秒。 */
export function rate(n?: number): string {
  if (n === undefined || Number.isNaN(n)) return "—";
  return bytes(Math.round(n)) + "/s";
}

/** 用量：百分比和“已用 / 总量”在上，细条在下。 */
export function Usage({ value, totalKB }: { value?: number; totalKB?: number }) {
  if (value === undefined || Number.isNaN(value)) return <span className="muted">—</span>;
  const v = Math.max(0, Math.min(100, value));
  const tn = v >= 90 ? "crit" : v >= 75 ? "warn" : "";
  return (
    <div className="usage">
      <div className="usage-top">
        <b className={tn}>{pctText(v)}</b>
        {totalKB ? <span>{bytes(totalKB * 1024 * v / 100)} / {bytes(totalKB * 1024)}</span> : null}
      </div>
      <div className="meter-track"><div className={`meter-fill ${tn}`} style={{ width: `${v}%` }} /></div>
    </div>
  );
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const u = ["KB", "MB", "GB", "TB"];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`;
}

export function duration(a?: string, b?: string): string {
  if (!a || !b) return "";
  const ms = Date.parse(b) - Date.parse(a);
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}

export function b64text(b64?: string): string {
  if (!b64) return "";
  const bin = atob(b64);
  return new TextDecoder().decode(Uint8Array.from(bin, (c) => c.charCodeAt(0)));
}

export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setV(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return v;
}

/** 把一个列表切成页。列表变短时自动退回到最后一页。 */
export function usePaged<T>(items: T[], size: number) {
  const [page, setPage] = useState(0);
  const pages = Math.max(1, Math.ceil(items.length / size));
  const cur = Math.min(page, pages - 1);
  return { rows: items.slice(cur * size, (cur + 1) * size), page: cur, pages, setPage, total: items.length, size };
}

/** 表格底部的分页条；只有一页时不显示。 */
export function Pager({ p }: { p: { page: number; pages: number; setPage: (n: number) => void; total: number; size: number } }) {
  const t = useT();
  if (p.pages <= 1) return null;
  const from = p.page * p.size + 1, to = Math.min(p.total, (p.page + 1) * p.size);
  // 页码：首页、末页、当前页前后各一页，中间用省略号。
  const nums: (number | "…")[] = [];
  for (let i = 0; i < p.pages; i++) {
    if (i === 0 || i === p.pages - 1 || Math.abs(i - p.page) <= 1) nums.push(i);
    else if (nums[nums.length - 1] !== "…") nums.push("…");
  }
  return (
    <div className="pager">
      <span className="num">{t("第 {a}–{b} 项，共 {n} 项", { a: from, b: to, n: p.total })}</span>
      <span className="grow" />
      <button className="btn ghost icon sm" disabled={p.page === 0} onClick={() => p.setPage(p.page - 1)} aria-label={t("上一页")} title={t("上一页")}><ChevronLeft /></button>
      {nums.map((n, i) => n === "…" ? <span key={"g" + i} className="muted">…</span>
        : <button key={n} className={"pager-num" + (n === p.page ? " on" : "")} aria-current={n === p.page ? "page" : undefined} onClick={() => p.setPage(n)}>{n + 1}</button>)}
      <button className="btn ghost icon sm" disabled={p.page >= p.pages - 1} onClick={() => p.setPage(p.page + 1)} aria-label={t("下一页")} title={t("下一页")}><ChevronRight /></button>
    </div>
  );
}

export interface Sorter<T> { sorted: T[]; sort: { key: string; dir: 1 | -1 } | null; toggle: (key: string, first?: 1 | -1) => void }

/** 点表头排序。cols 给出每一列用来比较的值；没有值的行排在最后。第三次点击回到默认顺序。 */
export function useSort<T>(rows: T[], cols: Record<string, (r: T) => string | number | undefined>): Sorter<T> {
  const [sort, setSort] = useState<{ key: string; dir: 1 | -1 } | null>(null);
  const pick = sort ? cols[sort.key] : undefined;
  const sorted = useMemo(() => {
    if (!sort || !pick) return rows;
    return [...rows].sort((a, b) => {
      const x = pick(a), y = pick(b);
      if (x === undefined || y === undefined) return x === y ? 0 : x === undefined ? 1 : -1;
      const d = typeof x === "number" && typeof y === "number" ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true });
      return d * sort.dir;
    });
    // cols 每次渲染都是新对象，这里只看它引用的数据有没有变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, sort]);
  const toggle = (key: string, first: 1 | -1 = 1) =>
    setSort((cur) => (!cur || cur.key !== key ? { key, dir: first } : cur.dir === first ? { key, dir: (-first) as 1 | -1 } : null));
  return { sorted, sort, toggle };
}

/**
 * 让实时刷新的排行不要每次都跳：数值照常更新，但行的先后最多每 everyMs 重排一次；
 * 鼠标停在表上时不重排（新进来的行排在后面，掉出去的行直接去掉），方便看清和点击。
 * rows 本身应该已经按想要的顺序排好（比如服务端按 CPU 排）。
 */
export function useSteadyOrder<T>(rows: T[], keyOf: (r: T) => string | number, reset?: unknown, everyMs = 15000) {
  const order = useRef<(string | number)[]>([]);
  const at = useRef(0);
  const hold = useRef(false);
  const last = useRef<unknown>(reset);
  const out = useMemo(() => {
    const now = Date.now();
    // reset 变了（比如点了表头换排序）就马上按新顺序排。
    const changed = last.current !== reset;
    last.current = reset;
    if (changed || (!hold.current && now - at.current >= everyMs)) {
      order.current = rows.map(keyOf);
      at.current = now;
      return rows;
    }
    // 还在的行留在原位；掉出去的行空出来的位置由新进来的行顶上，这样下面的行不会跟着上移。
    const by = new Map(rows.map((r) => [keyOf(r), r] as const));
    const before = new Set(order.current);
    const fresh = rows.filter((r) => !before.has(keyOf(r)));
    const list: T[] = [];
    for (const k of order.current) {
      const r = by.get(k) ?? fresh.shift();
      if (r) list.push(r);
    }
    list.push(...fresh);
    order.current = list.map(keyOf);
    return list;
    // keyOf 每次渲染都是新函数，这里只看数据有没有变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, reset, everyMs]);
  return { rows: out, hover: { onMouseEnter: () => { hold.current = true; }, onMouseLeave: () => { hold.current = false; } } };
}

/** 可以点击排序的表头。desc 表示第一次点击按从大到小排（数值列）。 */
export function SortTh<T>({ s, k, desc, children, className, style }: { s: Sorter<T>; k: string; desc?: boolean; children: ReactNode; className?: string; style?: CSSProperties }) {
  const t = useT();
  const on = s.sort?.key === k;
  return (
    <th className={className} style={style} aria-sort={on ? (s.sort!.dir === 1 ? "ascending" : "descending") : undefined}>
      <button className={"th-sort" + (on ? " on" : "")} onClick={() => s.toggle(k, desc ? -1 : 1)} title={t("点击排序")}>
        {children}{on ? (s.sort!.dir === 1 ? <ArrowUp /> : <ArrowDown />) : <ArrowUpDown />}
      </button>
    </th>
  );
}

/**
 * 定时刷新：每 ms 毫秒调一次 load，另外在切回页面、网络恢复、实时连接重新连上时立即补一次——
 * 页面在后台时浏览器会推迟定时器，电脑睡眠唤醒后要等一个周期才会刷新，期间出错的提示会一直挂着。
 * 返回清理函数，直接交给 useEffect。
 */
/**
 * 同一张卡片的刷新可能同时有几次在路上（切回页面、实时连接恢复、定时器各触发一次）。
 * 只认最后发出的那次：先发出、后失败的旧请求不能把新结果盖成报错。
 * 用法：const fresh = next(); api.x().then((v) => fresh() && …)。
 */
export function lastOnly(): () => () => boolean {
  let n = 0;
  return () => { const id = ++n; return () => id === n; };
}

export function poll(load: () => void, ms: number): () => void {
  const timer = setInterval(load, ms);
  const onShow = () => { if (document.visibilityState === "visible") load(); };
  document.addEventListener("visibilitychange", onShow);
  window.addEventListener("online", load);
  window.addEventListener("hostower:reconnected", load);
  return () => {
    clearInterval(timer);
    document.removeEventListener("visibilitychange", onShow);
    window.removeEventListener("online", load);
    window.removeEventListener("hostower:reconnected", load);
  };
}
