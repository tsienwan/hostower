// 确认框和输入框：替代浏览器自带的 window.confirm / window.prompt（样式和项目一致、能翻译、深色模式下也协调）。
// 用法和原生的差不多，只是返回 Promise：先 await askConfirm(提示文字) 得到是否确认，
// await askText(提示文字, { initial }) 得到输入的内容（取消时是 null）。
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { TriangleAlert } from "lucide-react";
import { Modal } from "./ui";
import { useT } from "./i18n";

interface Ask {
  kind: "confirm" | "text";
  message: string;
  title?: string;
  confirmLabel?: string;
  danger?: boolean;
  initial?: string;
  placeholder?: string;
  maxLength?: number;
  resolve: (v: boolean | string | null) => void;
}

let current: Ask | null = null;
const listeners = new Set<() => void>();
const show = (a: Ask | null) => { current = a; listeners.forEach((l) => l()); };

/** 请人确认一件事。默认按“会丢东西”的操作处理（红色的确认按钮）；不是的话传 danger: false。 */
export function askConfirm(message: string, opts: { title?: string; confirmLabel?: string; danger?: boolean } = {}): Promise<boolean> {
  return new Promise((resolve) => show({ kind: "confirm", message, danger: true, ...opts, resolve: (v) => resolve(v === true) }));
}

/** 请人输入一段文字；取消时返回 null。 */
export function askText(message: string, opts: { title?: string; confirmLabel?: string; initial?: string; placeholder?: string; maxLength?: number } = {}): Promise<string | null> {
  return new Promise((resolve) => show({ kind: "text", message, ...opts, resolve: (v) => resolve(typeof v === "string" ? v : null) }));
}

/** 挂在应用最外层（和提示条放在一起），同一时间只显示一个。 */
export function DialogHost() {
  const ask = useSyncExternalStore((cb) => { listeners.add(cb); return () => listeners.delete(cb); }, () => current);
  return ask ? <AskDialog key={ask.message + ask.kind} ask={ask} /> : null;
}

function AskDialog({ ask }: { ask: Ask }) {
  const t = useT();
  const [value, setValue] = useState(ask.initial ?? "");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { input.current?.select(); }, []);
  const done = (v: boolean | string | null) => { show(null); ask.resolve(v); };
  const cancel = () => done(ask.kind === "confirm" ? false : null);
  const ok = () => done(ask.kind === "confirm" ? true : value.trim());
  const canOK = ask.kind === "confirm" || value.trim() !== "";
  return (
    <Modal title={ask.title ?? (ask.kind === "confirm" ? t("请确认") : t("请输入"))} onClose={cancel}
      footer={<>
        <button className="btn" onClick={cancel}>{t("取消")}</button>
        <button className={"btn " + (ask.kind === "confirm" && ask.danger ? "danger-solid" : "primary")} disabled={!canOK} onClick={ok} autoFocus={ask.kind === "confirm"}>{ask.confirmLabel ?? t("确定")}</button>
      </>}>
      {ask.kind === "confirm"
        ? <div className="row" style={{ gap: 10, alignItems: "flex-start" }}>{ask.danger && <TriangleAlert size={18} style={{ color: "var(--crit)", flex: "none", marginTop: 1 }} />}<p style={{ margin: 0 }}>{ask.message}</p></div>
        : (
          <form className="field" onSubmit={(e) => { e.preventDefault(); if (canOK) ok(); }}>
            <label htmlFor="ask-text">{ask.message}</label>
            <input id="ask-text" ref={input} className="input" value={value} placeholder={ask.placeholder} maxLength={ask.maxLength ?? 64}
              onChange={(e) => setValue(e.target.value)} autoComplete="off" spellCheck={false} />
          </form>
        )}
    </Modal>
  );
}
