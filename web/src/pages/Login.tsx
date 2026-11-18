import { useState } from "react";
import { Fingerprint, KeyRound, Languages, LogIn, Smartphone, TriangleAlert } from "lucide-react";
import { api, type Bootstrap, type LoginMethod } from "../api";
import { loadBoot } from "../store";
import { Logo, ThemeSwitch } from "../fx";
import { Select } from "../controls";
import { languages, langSetting, setLang, useT, type LangMode } from "../i18n";

// OIDC 登录失败时，回调把人带回登录页，地址里只有一个简短的错误代码。值是中文原文，显示时翻译。
const oidcErrors: Record<string, string> = {
  not_allowed: "这个账号不在允许登录的列表里。请让所有者在“设置 → 安全 → 单点登录”里加上它。",
  expired: "登录流程已过期或无效，请重新登录。",
  provider: "身份提供方拒绝了这次登录。",
  disabled: "没有启用单点登录。",
  unavailable: "暂时连不上身份提供方，请稍后再试。",
  setup: "还没有创建所有者。",
  failed: "单点登录失败，详情记在审计里。",
};

function takeLoginError(): string {
  const p = new URLSearchParams(location.search);
  const code = p.get("login_error");
  if (!code) return "";
  p.delete("login_error");
  history.replaceState(null, "", location.pathname + (p.size ? "?" + p : ""));
  return oidcErrors[code] ?? oidcErrors.failed;
}

const themeCorner = <div className="row" style={{ position: "fixed", top: 16, right: 16, zIndex: 2, gap: 2 }}><ThemeSwitch placement="bottom" /></div>;

/** 登录框底部的语言选择：默认“跟随浏览器”，选了之后记在这台浏览器上，登录后也沿用。 */
function LoginLang() {
  const t = useT();
  const options = [{ value: "system", label: t("跟随浏览器") }, ...languages.map((l) => ({ value: l.code, label: l.label }))];
  return (
    <div className="auth-lang" title={t("语言")}>
      <Languages aria-hidden="true" />
      <Select value={langSetting()} onChange={(v) => setLang(v as LangMode)} options={options} />
    </div>
  );
}

export function Login({ boot }: { boot: Bootstrap }) {
  const t = useT();
  const methods = boot.auth?.methods ?? [];
  const [busy, setBusy] = useState(false);
  // 接口返回的错误已经翻译过；单点登录带回来的是中文原文，这里翻译。
  const [error, setError] = useState(() => { const e = takeLoginError(); return e ? t(e) : ""; });
  const [shown, setShown] = useState<LoginMethod | "">(() => (boot.auth?.default || methods[0] || ""));
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [ticket, setTicket] = useState("");
  const [code, setCode] = useState("");
  const [token, setToken] = useState("");

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const passwordLogin = () => run(async () => {
    const r = await api.passwordLogin(name, password);
    if (r.totp && r.ticket) { setTicket(r.ticket); setCode(""); return; }
    await loadBoot();
  });
  const totpLogin = () => run(async () => {
    try { await api.passwordTOTP(ticket, code); } catch (e) { setCode(""); throw e; }
    await loadBoot();
  });

  const others = methods.filter((m) => m !== shown);
  const methodButton = (m: LoginMethod, primary: boolean) => {
    const cls = "btn lg" + (primary ? " primary" : "");
    if (m === "passkey") return <button key={m} className={cls} disabled={busy} onClick={() => run(async () => { await api.login(); await loadBoot(); })}><Fingerprint />{t("使用 Passkey 登录")}</button>;
    // 整页跳转到身份提供方；回来后服务端设好会话再跳回首页。
    if (m === "oidc") return <a key={m} className={cls} href="/api/v1/auth/oidc/start"><LogIn />{t("使用 {name} 登录", { name: boot.oidc?.name ?? "OIDC" })}</a>;
    return <button key={m} className={cls} disabled={busy} onClick={() => { setShown("password"); setError(""); }}><KeyRound />{t("使用密码登录")}</button>;
  };

  return (
    <div className="auth">
      {themeCorner}
      <div className="card auth-card">
        <Logo size={44} />
        <div className="col">
          <h1>{t("登录 Hostower")}</h1>
          {ticket
            ? <p className="dim">{t("这个账号绑定了动态码。打开验证器 App，输入现在显示的 6 位数字。")}</p>
            : <p className="dim">{t("每一步变更都经过风险判定和人的批准")}</p>}
        </div>

        {ticket ? (
          <form className="col" style={{ gap: 12 }} onSubmit={(e) => { e.preventDefault(); totpLogin(); }}>
            <div className="field">
              <label htmlFor="code">{t("动态码")}</label>
              <input id="code" className="input mono" inputMode="numeric" autoComplete="one-time-code" maxLength={7} autoFocus
                value={code} onChange={(e) => setCode(e.target.value.replace(/[^\d ]/g, ""))} />
            </div>
            <button className="btn primary lg" type="submit" disabled={busy || code.replace(/\s/g, "").length !== 6}><Smartphone />{t("验证并登录")}</button>
            <button className="btn ghost" type="button" onClick={() => { setTicket(""); setPassword(""); setError(""); }}>{t("返回")}</button>
          </form>
        ) : (
          <>
            {shown === "password" ? (
              <form className="col" style={{ gap: 12 }} onSubmit={(e) => { e.preventDefault(); passwordLogin(); }}>
                <div className="field">
                  <label htmlFor="name">{t("用户名")}</label>
                  <input id="name" className="input" autoComplete="username" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
                </div>
                <div className="field">
                  <label htmlFor="password">{t("密码")}</label>
                  <input id="password" className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
                </div>
                <button className="btn primary lg" type="submit" disabled={busy || !name.trim() || !password}><KeyRound />{t("登录")}</button>
              </form>
            ) : shown ? methodButton(shown, true) : null}

            {others.length > 0 && (
              <>
                {shown && <div className="divider">{t("其他登录方式")}</div>}
                {others.map((m) => methodButton(m, !shown))}
              </>
            )}

            {methods.length === 0 && !boot.dev && (
              <div className="note warn"><TriangleAlert /><span>{t("没有可用的登录方式。在塔台所在的服务器上运行 hostower auth reset-password，会生成新密码并打开密码登录。")}</span></div>
            )}
          </>
        )}

        {boot.dev && !ticket && (
          <>
            <div className="divider">{t("开发模式")}</div>
            <div className="note warn"><TriangleAlert /><span>{t("开发模式下可以用启动日志里的令牌直接登录。主机全部是模拟的。")}</span></div>
            <form className="col" style={{ gap: 12 }} onSubmit={(e) => { e.preventDefault(); run(async () => { await api.devLogin(token); await loadBoot(); }); }}>
              <div className="field">
                <label htmlFor="devtoken">{t("启动令牌")}</label>
                <input id="devtoken" className="input mono" value={token} onChange={(e) => setToken(e.target.value)} autoComplete="off" spellCheck={false} placeholder={t("从启动日志复制")} />
              </div>
              <button className={"btn lg" + (methods.length === 0 ? " primary" : "")} disabled={busy || !token.trim()} type="submit">{t("用令牌登录")}</button>
            </form>
          </>
        )}
        {error && <div className="note crit" role="alert"><TriangleAlert /><span>{error}</span></div>}
        <div className="auth-foot">
          <span className="muted">
            Hostower {boot.version}
            {boot.build?.status === "signed" && <span title={t("发布签名校验通过")} aria-label={t("发布签名校验通过")}> ✓</span>}
            {boot.source && <> · <a href={boot.source} target="_blank" rel="noreferrer noopener">AGPL-3.0</a></>}
          </span>
          <LoginLang />
        </div>
      </div>
    </div>
  );
}
