// 自绘的表单控件：下拉框、提示框、标签输入。
// 浮层都渲染到 body 上并用视口坐标定位，这样不会被弹窗或面板裁掉。
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown, X } from "lucide-react";
import { useT } from "./i18n";

export interface Option { value: string; label: string; hint?: string }

interface Pos { left: number; top?: number; bottom?: number; width: number; maxHeight: number }

function place(el: HTMLElement, wantHeight: number): Pos {
  const r = el.getBoundingClientRect();
  const below = window.innerHeight - r.bottom - 12, above = r.top - 12;
  const up = below < Math.min(wantHeight, 200) && above > below;
  const width = Math.max(r.width, 160);
  const left = Math.max(8, Math.min(r.left, window.innerWidth - width - 8));
  return up
    ? { left, bottom: window.innerHeight - r.top + 6, width, maxHeight: Math.min(320, above) }
    : { left, top: r.bottom + 6, width, maxHeight: Math.min(320, below) };
}

/** 下拉框：样式和其余界面一致，支持键盘上下选择。 */
export function Select({ value, onChange, options, placeholder, id, mono, disabled }: {
  value: string; onChange: (v: string) => void; options: Option[]; placeholder?: string; id?: string; mono?: boolean; disabled?: boolean;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<Pos | null>(null);
  const [active, setActive] = useState(0);
  const btn = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const current = options.find((o) => o.value === value);

  const close = useCallback(() => setOpen(false), []);
  useLayoutEffect(() => {
    if (!open || !btn.current) return;
    setPos(place(btn.current, options.length * 36 + 8));
    setActive(Math.max(0, options.findIndex((o) => o.value === value)));
  }, [open, options, value]);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => { if (!btn.current?.contains(e.target as Node) && !list.current?.contains(e.target as Node)) close(); };
    const onScroll = (e: Event) => { if (!list.current?.contains(e.target as Node)) close(); };
    window.addEventListener("pointerdown", onDown);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", close);
    return () => { window.removeEventListener("pointerdown", onDown); window.removeEventListener("scroll", onScroll, true); window.removeEventListener("resize", close); };
  }, [open, close]);

  const pick = (v: string) => { onChange(v); close(); btn.current?.focus(); };
  const onKey = (e: KeyboardEvent) => {
    if (!open) {
      if (e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") { e.preventDefault(); setOpen(true); }
      return;
    }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(options.length - 1, a + 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(0, a - 1)); }
    else if (e.key === "Enter" || e.key === " ") { e.preventDefault(); if (options[active]) pick(options[active].value); }
    else if (e.key === "Tab") close();
  };

  return (
    <>
      <button ref={btn} id={id} type="button" className={"select-btn" + (open ? " open" : "") + (mono ? " mono" : "")} disabled={disabled}
        role="combobox" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen(!open)} onKeyDown={onKey}>
        <span className={"ellipsis grow" + (current ? "" : " muted")}>{current?.label ?? placeholder ?? t("请选择")}</span>
        <ChevronDown className="chev" />
      </button>
      {open && pos && createPortal(
        <div ref={list} className="listbox" role="listbox" style={pos}>
          {options.length === 0 && <div className="listbox-empty">{t("没有可选项")}</div>}
          {options.map((o, i) => (
            <div key={o.value} role="option" aria-selected={o.value === value} className={"listbox-item" + (i === active ? " active" : "") + (o.value === value ? " on" : "") + (mono ? " mono" : "")}
              onPointerEnter={() => setActive(i)} onClick={() => pick(o.value)}>
              <span className="grow ellipsis">{o.label}{o.hint && <span className="muted">　{o.hint}</span>}</span>
              {o.value === value && <Check className="tick" />}
            </div>
          ))}
        </div>,
        document.body,
      )}
    </>
  );
}

/** 提示框：悬停或聚焦时在元素上方显示一块说明。 */
export function Tip({ content, children }: { content: ReactNode; children: ReactNode }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<{ x: number; y: number; below: boolean } | null>(null);
  const show = () => {
    const r = ref.current?.getBoundingClientRect();
    if (r) setPos({ x: Math.max(120, Math.min(window.innerWidth - 120, r.left + r.width / 2)), y: r.top < 90 ? r.bottom + 8 : r.top - 8, below: r.top < 90 });
  };
  const hide = () => setPos(null);
  return (
    <span ref={ref} className="tip-anchor" onPointerEnter={show} onPointerLeave={hide} onFocus={show} onBlur={hide}>
      {children}
      {pos && createPortal(<div className={"tip" + (pos.below ? " below" : "")} role="tooltip" style={{ left: pos.x, top: pos.y }}>{content}</div>, document.body)}
    </span>
  );
}

/** 标签输入：回车、空格或逗号生成一个标签；也可以直接点已有的标签。 */
export function TagInput({ value, onChange, suggestions, id, placeholder, pattern, invalid, hint, keepCase }: {
  value: string[]; onChange: (v: string[]) => void; suggestions: string[]; id?: string; placeholder?: string;
  /** 自定义校验（默认是标签的规则）；invalid 是不通过时的提示，hint 是建议列表前面的文字。 */
  pattern?: RegExp; invalid?: string; hint?: string; keepCase?: boolean;
}) {
  const t = useT();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  const add = (raw: string) => {
    const tag = keepCase ? raw.trim() : raw.trim().toLowerCase();
    if (!tag) return;
    if (!(pattern ?? /^[a-z0-9][a-z0-9._-]{0,31}$/).test(tag)) { setError(invalid ?? t("标签只能用字母、数字、点、下划线、连字符")); return; }
    setError("");
    if (!value.includes(tag)) onChange([...value, tag]);
    setDraft("");
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === " " || e.key === "," || e.key === "，") { e.preventDefault(); add(draft); }
    else if (e.key === "Backspace" && !draft && value.length) onChange(value.slice(0, -1));
  };
  const rest = suggestions.filter((s) => !value.includes(s) && s.includes(draft.trim().toLowerCase()));
  return (
    <div className="col" style={{ gap: 8 }}>
      <div className="tag-input" onClick={(e) => (e.currentTarget.querySelector("input") as HTMLInputElement | null)?.focus()}>
        {value.map((tag) => (
          <span key={tag} className="chip">{tag}<button type="button" aria-label={t("移除标签 {tag}", { tag })} onClick={(e) => { e.stopPropagation(); onChange(value.filter((x) => x !== tag)); }}><X /></button></span>
        ))}
        <input id={id} value={draft} onChange={(e) => { setDraft(e.target.value); setError(""); }} onKeyDown={onKey} onBlur={() => add(draft)}
          placeholder={value.length ? "" : placeholder} spellCheck={false} autoComplete="off" />
      </div>
      {error && <span className="hint" style={{ color: "var(--crit)" }}>{error}</span>}
      {rest.length > 0 && (
        <div className="row wrap" style={{ gap: 4 }}>
          <span className="hint">{hint ?? t("已有标签：")}</span>
          {rest.slice(0, 24).map((s) => <button key={s} type="button" className="tag" onClick={() => add(s)}>+ {s}</button>)}
        </div>
      )}
    </div>
  );
}
