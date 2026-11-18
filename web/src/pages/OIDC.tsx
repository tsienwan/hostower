// 单点登录（OIDC）设置：签发方、客户端、允许登录的账号。客户端密钥只写不读。
import { useEffect, useState } from "react";
import { Copy, LogIn, TriangleAlert } from "lucide-react";
import { api, type OIDCView } from "../api";
import { fail, toast } from "../store";
import { TagInput } from "../controls";
import { VerifyCancelled, withVerify } from "../verify";
import { ts, useT } from "../i18n";

export function OIDCSettings({ owner }: { owner: boolean }) {
  const t = useT();
  const [view, setView] = useState<OIDCView | null>(null);
  const [enabled, setEnabled] = useState(false);
  const [name, setName] = useState("");
  const [issuer, setIssuer] = useState("");
  const [clientId, setClientId] = useState("");
  const [allowed, setAllowed] = useState<string[]>([]);
  const [secret, setSecret] = useState("");
  const [clearSecret, setClearSecret] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const apply = (v: OIDCView) => {
    setView(v);
    setEnabled(v.config.enabled); setName(v.config.name ?? ""); setIssuer(v.config.issuer ?? ""); setClientId(v.config.clientId ?? "");
    setAllowed(v.config.allowed ?? []); setSecret(""); setClearSecret(false);
  };
  useEffect(() => { api.oidc().then(apply).catch(() => {}); }, []);
  if (!view) return null;

  const save = async () => {
    setBusy(true); setErr("");
    try {
      // 单点登录也是一种登录方式：改之前要先验证身份。
      apply(await withVerify(() => api.saveOIDC({ enabled, name, issuer, clientId, allowed, secret, clearSecret })));
      toast(t("已保存"));
    } catch (e) { if (!(e instanceof VerifyCancelled)) setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  const copy = () => navigator.clipboard?.writeText(view.redirectUrl).then(() => toast(t("已复制"))).catch(fail);
  const locked = !owner || busy;

  return (
    <div className="card">
      <div className="card-head">
        <LogIn size={16} className="muted" /><h2>{t("单点登录（OIDC）")}</h2>
        <span className="muted hide-sm">{t("用公司或个人的身份提供方登录")}</span>
        <span className="grow" />
        <span className={"badge " + (view.config.enabled ? "ok" : "neutral")}>{view.config.enabled ? t("已启用") : t("未启用")}</span>
      </div>
      <div className="card-body">
        <div className="note" style={{ marginBottom: 8 }}>
          <TriangleAlert />
          <span>{t("OIDC 只替代登录这一步：登录后是所有者的会话。高危操作（L2）的当场确认仍然要用 Passkey 或动态码，所以先保证至少设置了其中一种。")}</span>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("启用")}</div><div className="setting-desc">{t("打开后登录页会多一个按钮。保存时会去签发方读一次发现文档，读不到就不保存。")}</div></div>
          <div><button role="switch" aria-checked={enabled} aria-label={t("启用")} className={"switch" + (enabled ? " on" : "")} disabled={locked} onClick={() => setEnabled(!enabled)}><span /></button></div>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("回调地址")}</div><div className="setting-desc">{t("在身份提供方那边建应用时填这个地址（Redirect URI）。它跟着“面板 → 访问地址”走。")}</div></div>
          <div className="row" style={{ gap: 6, flexWrap: "nowrap" }}>
            <input className="input mono" readOnly value={view.redirectUrl} aria-label={t("回调地址")} onFocus={(e) => e.target.select()} />
            <button className="btn icon" onClick={copy} title={t("复制")} aria-label={t("复制")}><Copy /></button>
          </div>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("签发方地址")}</div><div className="setting-desc">{t("Issuer，例如 https://accounts.google.com、https://login.microsoftonline.com/<租户>/v2.0。必须是 https。")}</div></div>
          <input className="input mono" value={issuer} disabled={locked} onChange={(e) => setIssuer(e.target.value)} placeholder="https://idp.example.com" spellCheck={false} aria-label={t("签发方地址")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("客户端编号")}</div><div className="setting-desc">Client ID</div></div>
          <input className="input mono" value={clientId} disabled={locked} onChange={(e) => setClientId(e.target.value)} spellCheck={false} aria-label={t("客户端编号")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("客户端密钥")}</div><div className="setting-desc">{t("Client Secret。只写不读：留空表示不修改。公开客户端（只用 PKCE）可以不填。")}</div></div>
          <div className="col" style={{ gap: 8 }}>
            <input className="input mono" type="password" autoComplete="new-password" value={secret} disabled={locked || clearSecret || !view.secretsAvailable}
              onChange={(e) => setSecret(e.target.value)} placeholder={view.config.hasSecret ? t("已保存，留空表示不修改") : t("没有保存")} aria-label={t("客户端密钥")} />
            {view.config.hasSecret && <label className="check"><input type="checkbox" checked={clearSecret} disabled={locked} onChange={(e) => setClearSecret(e.target.checked)} />{t("删除已保存的密钥")}</label>}
          </div>
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("允许登录的账号")}</div><div className="setting-desc">{t("填邮箱（必须是身份提供方验证过的邮箱），或者 sub:<编号>。不在这里的账号即使在身份提供方那边登录成功，也进不来。")}</div></div>
          <TagInput value={allowed} onChange={setAllowed} suggestions={[]} placeholder={t("输入后按回车，可以填多个")} keepCase
            pattern={/^(sub:[\x21-\x7e]{1,255}|[^@\s<>,;"]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,24})$/} invalid={t("要填邮箱，或者 sub:<编号>")} />
        </div>
        <div className="setting-row">
          <div><div className="setting-name">{t("按钮名字")}</div><div className="setting-desc">{t("登录页上显示“使用 … 登录”。")}</div></div>
          <input className="input" value={name} disabled={locked} onChange={(e) => setName(e.target.value)} placeholder={t("例如：公司账号")} maxLength={30} aria-label={t("按钮名字")} />
        </div>
        {err && <div className="note crit" style={{ marginTop: 12 }}><TriangleAlert /><span>{ts(err)}</span></div>}
        <div className="row between" style={{ marginTop: 18 }}>
          <span className="muted" style={{ fontSize: 12.5 }}>{owner ? t("每次修改都会写审计。") : t("只有所有者可以修改。")}</span>
          <button className="btn primary" disabled={locked} onClick={save}>{t("保存")}</button>
        </div>
      </div>
    </div>
  );
}
