// 身份验证框：改登录方式、密码、动态码、Passkey 之前，以及批准高危操作（L2）之前弹出。
// 账号有 Passkey 或动态码时只能用它们；两样都没有时才用密码；批 L2 时密码不算。
import { useEffect, useState, useSyncExternalStore } from "react";
import { Fingerprint, KeyRound, ShieldCheck, Smartphone, TriangleAlert } from "lucide-react";
import { api, ApiError, type AuthFactors, type VerifyMethod } from "./api";
import { Modal } from "./ui";
import { useT } from "./i18n";

/** 用户关掉了验证框。fail() 不会为它弹提示。 */
export class VerifyCancelled extends Error {
  constructor() {
    super("已取消验证");
    this.name = "VerifyCancelled";
  }
}

type Pending = { l2: boolean; resolve: () => void; reject: (e: Error) => void };
let pending: Pending | null = null;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((f) => f());

/** 弹出验证框，验证通过后返回；关掉时抛出 VerifyCancelled。l2 为真时只给 Passkey 和动态码。 */
export function verify(opts: { l2?: boolean } = {}): Promise<void> {
  pending?.reject(new VerifyCancelled());
  return new Promise<void>((resolve, reject) => {
    pending = { l2: !!opts.l2, resolve, reject };
    emit();
  });
}

const NEED = "需要先验证身份";

/** 执行 fn；服务端说“需要先验证身份”时弹出验证框，通过后再执行一次。 */
export async function withVerify<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    if (e instanceof ApiError && e.status === 403 && e.raw === NEED) {
      await verify();
      return fn();
    }
    throw e;
  }
}

const icons: Record<VerifyMethod, typeof Fingerprint> = { passkey: Fingerprint, totp: Smartphone, password: KeyRound };
const labels: Record<VerifyMethod, string> = { passkey: "Passkey", totp: "动态码", password: "密码" };

/** 挂在应用根上，负责显示验证框。 */
export function VerifyHost() {
  const t = useT();
  const cur = useSyncExternalStore((f) => { listeners.add(f); return () => listeners.delete(f); }, () => pending);
  const [info, setInfo] = useState<AuthFactors | null>(null);
  const [method, setMethod] = useState<VerifyMethod | "">("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!cur) return;
    setInfo(null); setValue(""); setError(""); setMethod("");
    api.factors().then((f) => {
      setInfo(f);
      const list = cur.l2 ? f.strong : f.verify;
      setMethod(list.includes(f.defaultVerify as VerifyMethod) ? (f.defaultVerify as VerifyMethod) : list[0] ?? "");
    }).catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [cur]);

  if (!cur) return null;
  const list = info ? (cur.l2 ? info.strong : info.verify) : [];
  const close = () => { pending = null; emit(); cur.reject(new VerifyCancelled()); };
  const done = () => { pending = null; emit(); cur.resolve(); };
  const run = async () => {
    setBusy(true); setError("");
    try {
      if (method === "passkey") await api.reauth();
      else if (method === "totp") await api.verifyTOTP(value);
      else if (method === "password") await api.verifyPassword(value);
      done();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setValue("");
    } finally { setBusy(false); }
  };

  return (
    <Modal title={cur.l2 ? t("验证身份后批准") : t("先验证身份")} onClose={close}
      footer={<>
        <button className="btn" onClick={close}>{t("取消")}</button>
        {method && <button className="btn primary" disabled={busy || (method !== "passkey" && !value.trim())} form="verify-form" type="submit">
          <ShieldCheck />{method === "passkey" ? t("使用 Passkey 验证") : t("验证")}
        </button>}
      </>}>
      <form id="verify-form" className="col" style={{ gap: 14 }} onSubmit={(e) => { e.preventDefault(); if (method) run(); }}>
        <p className="dim">{cur.l2 ? t("高危操作要当场用 Passkey 或动态码确认身份，验证一次只能批准一次。") : t("修改登录和验证方式之前要确认是你本人。验证后 5 分钟内不用再验证。")}</p>
        {info && list.length === 0 && (
          <div className="note warn"><TriangleAlert /><span>{cur.l2 ? t("这个账号还没有 Passkey 或动态码，批准不了高危操作。请先在“设置 → 安全”里添加一种。") : t("这个账号没有可以用来验证的方式。")}</span></div>
        )}
        {list.length > 1 && (
          <div className="seg" role="radiogroup" aria-label={t("验证方式")}>
            {list.map((m) => { const I = icons[m]; return (
              <button key={m} type="button" role="radio" aria-checked={method === m} className={method === m ? "on" : ""} onClick={() => { setMethod(m); setValue(""); setError(""); }}>
                <I size={13} style={{ verticalAlign: -2, marginRight: 5 }} />{t(labels[m])}
              </button>
            ); })}
          </div>
        )}
        {method === "totp" && (
          <div className="field" style={{ maxWidth: 260 }}>
            <label htmlFor="verify-code">{t("验证器 App 上的 6 位动态码")}</label>
            <input id="verify-code" className="input mono" inputMode="numeric" autoComplete="one-time-code" maxLength={7} autoFocus
              value={value} onChange={(e) => setValue(e.target.value.replace(/[^\d ]/g, ""))} />
          </div>
        )}
        {method === "password" && (
          <div className="field" style={{ maxWidth: 360 }}>
            <label htmlFor="verify-pass">{t("当前密码")}</label>
            <input id="verify-pass" className="input" type="password" autoComplete="current-password" autoFocus value={value} onChange={(e) => setValue(e.target.value)} />
          </div>
        )}
        {method === "passkey" && <div className="muted">{t("点下面的按钮，按浏览器或密码管理器的提示完成验证。")}</div>}
        {error && <div className="note crit" role="alert"><TriangleAlert /><span>{error}</span></div>}
      </form>
    </Modal>
  );
}
