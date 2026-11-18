// 小部件：背景层、主题切换、语言切换、标志、数字滚动。
import { useEffect, useRef, useState } from "react";
import { Check, Languages, Monitor, Moon, Sun } from "lucide-react";
import type { Host } from "./api";
import { setThemeMode, useThemeMode, type ThemeMode } from "./theme";
import { langSetting, languages, setLang, useT, type LangMode } from "./i18n";

const reduced = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** 全屏背景层：一层安静的纯色。内容本身才是主角。 */
export function Backdrop() {
  return <div className="backdrop" aria-hidden />;
}

const modes: { mode: ThemeMode; label: string; icon: typeof Sun }[] = [
  { mode: "system", label: "跟随系统", icon: Monitor },
  { mode: "dark", label: "深色", icon: Moon },
  { mode: "light", label: "浅色", icon: Sun },
];

/** 主题切换：一个图标按钮，点开后三选一（跟随系统 / 深色 / 浅色）。 */
export function ThemeSwitch({ placement = "top" }: { placement?: "top" | "bottom" }) {
  const t = useT();
  const mode = useThemeMode();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const Current = modes.find((m) => m.mode === mode)!.icon;

  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("pointerdown", onDown);
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("pointerdown", onDown); window.removeEventListener("keydown", onKey); };
  }, [open]);

  return (
    <div className="pop-anchor" ref={ref}>
      <button className="btn ghost icon sm" aria-haspopup="menu" aria-expanded={open} aria-label={t("主题")} title={t("主题")} onClick={() => setOpen(!open)}>
        <Current />
      </button>
      {open && (
        <div className={"menu " + placement} role="menu" aria-label={t("主题")}>
          {modes.map((m) => (
            <button key={m.mode} role="menuitemradio" aria-checked={mode === m.mode} className={"menu-item" + (mode === m.mode ? " on" : "")}
              onClick={() => { setThemeMode(m.mode); setOpen(false); }}>
              <m.icon /><span>{t(m.label)}</span>{mode === m.mode && <Check className="tick" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export type HostTone = "ok" | "warn" | "crit" | "idle";

export function hostTone(h: Host): HostTone {
  const st = h.status, l = st.latest;
  if (!st.known) return "idle";
  if (!st.online) return "crit";
  if (l && (l.cpu >= 90 || l.mem >= 90 || l.disk >= 90)) return "crit";
  if (l && (l.cpu >= 75 || l.mem >= 75 || l.disk >= 75)) return "warn";
  return "ok";
}

// 数字从 0 滚上来只在打开页面后的第一次出现；之后切回总览直接显示当前值，只有数值变化时才滚动。
// 否则每次切页签都要等 0.7 秒的动画，看起来像在加载、像卡顿。
let introDone = false;

/** 数字滚动到目标值。 */
export function CountUp({ value }: { value: number }) {
  const skip = reduced() || introDone;
  const [shown, setShown] = useState(skip ? value : 0);
  const from = useRef(skip ? value : 0);
  useEffect(() => {
    if (reduced()) { setShown(value); return; }
    if (from.current === value) { setShown(value); return; }
    introDone = true; // 同一次渲染里的其他数字已经按“第一次”渲染过了，这里改标记不影响它们
    const start = performance.now(), a = from.current, d = 700;
    let raf = 0;
    const tick = (t: number) => {
      const p = Math.min(1, (t - start) / d);
      const v = Math.round(a + (value - a) * (1 - Math.pow(1 - p, 3)));
      from.current = v;
      setShown(v);
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value]);
  return <>{shown.toLocaleString()}</>;
}

/** 项目标志：海上的灯塔。页面里用的是没有底板的版本——直接画在页面背景上，
 *  塔身用文字颜色（随明暗主题变化），灯和海浪用主题里的强调色。
 *  浏览器标签页图标（public/favicon.svg）是同一个图形加上深色方块底。 */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg className="logo" width={size} height={size} viewBox="0 0 64 64" role="img" aria-label="Hostower" style={{ flex: "none", display: "block" }}>
      <path d="M39 21 62 11v20z" fill="var(--warn)" opacity=".45" />
      <path d="M9 47.5c5-4 9-4 14 0s9 4 14 0 9-4 14 0" fill="none" stroke="var(--accent)" strokeWidth="4" strokeLinecap="round" opacity=".55" />
      <g fill="currentColor">
        <path d="M24 17 32 9l8 8z" />
        <rect x="22.5" y="25" width="19" height="3.6" rx="1.8" />
        <path d="M26 28.6h12L41.4 50H22.6z" />
      </g>
      <rect x="25.5" y="17" width="13" height="8" fill="var(--warn)" />
      <path d="M5 54.5c6-5 11-5 17 0s11 5 17 0 11-5 18 0" fill="none" stroke="var(--primary)" strokeWidth="5" strokeLinecap="round" />
    </svg>
  );
}

/** 语言切换：一个图标按钮，点开后选择。 */
export function LangSwitch({ placement = "top" }: { placement?: "top" | "bottom" }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const current = langSetting();
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("pointerdown", onDown);
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("pointerdown", onDown); window.removeEventListener("keydown", onKey); };
  }, [open]);
  const options: { mode: LangMode; label: string }[] = [{ mode: "system", label: t("跟随系统") }, ...languages.map((l) => ({ mode: l.code as LangMode, label: l.label }))];
  return (
    <div className="pop-anchor" ref={ref}>
      <button className="btn ghost icon sm" aria-haspopup="menu" aria-expanded={open} aria-label={t("语言")} title={t("语言")} onClick={() => setOpen(!open)}><Languages /></button>
      {open && (
        <div className={"menu " + placement} role="menu" aria-label={t("语言")}>
          {options.map((o) => (
            <button key={o.mode} role="menuitemradio" aria-checked={current === o.mode} className={"menu-item" + (current === o.mode ? " on" : "")}
              onClick={() => { setLang(o.mode); setOpen(false); }}>
              <span>{o.label}</span>{current === o.mode && <Check className="tick" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
