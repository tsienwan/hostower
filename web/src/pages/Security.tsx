// 登录与验证：登录方式和默认方式、密码、动态码（验证器 App）、Passkey。
// 这里的每个改动都要先验证身份（见 verify.tsx）；服务端会拒绝会让所有者登录不进来的改动。
import { useCallback, useEffect, useMemo, useState } from "react";
import { askConfirm, askText } from "../dialogs";
import qrcode from "qrcode-generator";
import { Copy, Dices, Eye, EyeOff, Fingerprint, KeyRound, Plus, ShieldCheck, Smartphone, Trash2, TriangleAlert } from "lucide-react";
import { api, type AuthFactors, type Methods } from "../api";
import { fail, loadBoot, toast, useStore } from "../store";
import { useCached } from "../cache";
import { Modal, ago } from "../ui";
import { Select } from "../controls";
import { withVerify } from "../verify";
import { ts, useT } from "../i18n";

export function LoginSecurity() {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const defaultName = useStore((s) => !!s.boot?.defaultUsername);
  const [info, setInfo] = useCached<AuthFactors>("security.factors");
  const [busy, setBusy] = useState(false);
  const [dialog, setDialog] = useState<"password" | "totp" | "username" | null>(null);
  // 当前访问地址能不能用 Passkey（用 IP 访问时不能）。
  const passkeyOK = useStore((s) => s.boot?.passkey !== false);
  const userName = useStore((s) => s.boot?.user?.name ?? "");
  const load = useCallback(() => api.factors().then(setInfo).catch(fail), []);
  useEffect(() => { load(); }, [load]);

  const act = async (fn: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    try { await withVerify(fn); if (done) toast(done); await load(); loadBoot(); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  if (!info) return null;
  const f = info.factors;
  const m = info.methods;
  const saveMethods = (next: Partial<Methods>) => act(() => api.setMethods({ ...m, ...next }), t("已保存"));

  const loginOptions = [
    { value: "", label: t("自动") },
    { value: "passkey", label: "Passkey" },
    { value: "password", label: t("密码") },
    ...(info.oidc ? [{ value: "oidc", label: t("单点登录") }] : []),
  ];
  const verifyOptions = [
    { value: "", label: t("自动") },
    { value: "passkey", label: "Passkey" },
    { value: "totp", label: t("动态码") },
    { value: "password", label: t("密码") },
  ];

  return (
    <>
      <div className="card">
        <div className="card-head"><h2>{t("账号")}</h2></div>
        <div className="card-body">
          <div className="setting-row">
            <div>
              <div className="setting-name">{t("用户名")}</div>
              <div className="setting-desc"><span className="mono">{userName}</span>{defaultName && <span className="badge warn" style={{ marginLeft: 6 }}>{t("默认用户名，建议修改")}</span>} · {t("用密码登录时输入的名字。改名不影响 Passkey、动态码和已登录的会话；审计里已经记下的旧名字保持原样。")}</div>
            </div>
            <div><button className={"btn" + (defaultName ? " primary" : "")} disabled={busy} onClick={() => setDialog("username")}>{t("修改用户名")}</button></div>
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-head"><h2>{t("登录方式")}</h2></div>
        <div className="card-body">
          {f.passwordInitial && (
            <div className="note warn" style={{ marginBottom: 16 }}><TriangleAlert /><span>{f.passkeys > 0 || f.totp ? t("还在用首次启动生成的初始密码，请修改密码。") : t("还在用首次启动生成的初始密码。请修改密码，并添加 Passkey 或动态码。")}</span></div>
          )}
          {info.strong.length === 0 && (
            <div className="note warn" style={{ marginBottom: 16 }}><TriangleAlert /><span>{t("还没有 Passkey 或动态码：批准不了高危操作（L2）。至少添加一种。")}</span></div>
          )}
          <div className="setting-row">
            <div><div className="setting-name">{t("Passkey 登录")}</div><div className="setting-desc">{f.passkeys > 0 ? t("已注册 {n} 把。", { n: f.passkeys }) : t("还没有注册 Passkey，开着也用不了。")}</div></div>
            <div><label className="check"><input type="checkbox" checked={m.passkey} disabled={!owner || busy} onChange={(e) => saveMethods({ passkey: e.target.checked })} />{m.passkey ? t("已开启") : t("已关闭")}</label></div>
          </div>
          <div className="setting-row">
            <div><div className="setting-name">{t("密码登录")}</div><div className="setting-desc">{f.password ? (f.totp ? t("登录时先输密码，再输动态码。") : t("绑定动态码后，登录时还要再输一次动态码。")) : t("还没有设置密码，开着也用不了。")}</div></div>
            <div><label className="check"><input type="checkbox" checked={m.password} disabled={!owner || busy} onChange={(e) => saveMethods({ password: e.target.checked })} />{m.password ? t("已开启") : t("已关闭")}</label></div>
          </div>
          <div className="setting-row">
            <div><div className="setting-name">{t("单点登录")}</div><div className="setting-desc">{t("在下面的“单点登录（OIDC）”里设置。")}</div></div>
            <div className="dim">{info.oidc ? t("已开启") : t("未开启")}</div>
          </div>
          <div className="setting-row">
            <div><div className="setting-name">{t("默认登录方式")}</div><div className="setting-desc">{t("登录页先显示哪一种；其他能用的方式放在下面。")}</div></div>
            <div><Select value={m.defaultLogin} disabled={!owner || busy} options={loginOptions} onChange={(v) => saveMethods({ defaultLogin: v as Methods["defaultLogin"] })} /></div>
          </div>
          <div className="setting-row">
            <div><div className="setting-name">{t("默认验证方式")}</div><div className="setting-desc">{t("改登录设置、批准高危操作时先用哪一种验证身份。有 Passkey 或动态码时，密码不能用来验证。")}</div></div>
            <div><Select value={m.defaultVerify} disabled={!owner || busy} options={verifyOptions} onChange={(v) => saveMethods({ defaultVerify: v as Methods["defaultVerify"] })} /></div>
          </div>
          <div className="hint" style={{ marginTop: 12 }}>{t("至少要保留一种开着、并且已经设置好的登录方式。所有方式都丢了时，在服务器上运行 hostower auth reset-password。")}</div>
        </div>
      </div>

      <div className="card">
        <div className="card-head"><h2>{t("密码与动态码")}</h2></div>
        <div className="card-body">
          <div className="setting-row">
            <div>
              <div className="setting-name"><KeyRound size={14} style={{ verticalAlign: -2, marginRight: 6 }} />{t("密码")}</div>
              <div className="setting-desc">{!f.password ? t("没有设置。") : f.passwordInitial ? t("正在用初始密码。") : t("已设置。")}{t("至少 12 个字符；改密码前要先验证身份。")}</div>
            </div>
            <div><button className="btn" disabled={busy} onClick={() => setDialog("password")}>{f.password ? t("修改密码") : t("设置密码")}</button></div>
          </div>
          <div className="setting-row">
            <div>
              <div className="setting-name"><Smartphone size={14} style={{ verticalAlign: -2, marginRight: 6 }} />{t("动态码（验证器 App）")}</div>
              <div className="setting-desc">{f.totp ? t("已绑定。用密码登录时要再输一次动态码，也可以用它验证身份、批准高危操作。") : t("用 Google Authenticator、1Password、Bitwarden 这类 App 生成 6 位动态码。它不能单独用来登录。")}</div>
            </div>
            <div>{f.totp
              ? <button className="btn danger" disabled={busy} onClick={async () => (await askConfirm(t("解绑动态码？之后用密码登录不再需要动态码。"))) && act(() => api.totpRemove(), t("已解绑动态码"))}>{t("解绑")}</button>
              : <button className="btn" disabled={busy} onClick={() => setDialog("totp")}>{t("绑定")}</button>}</div>
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>Passkey</h2><span className="muted num">{info.passkeys.length || ""}</span><span className="grow" />
          <button className="btn sm" disabled={busy || !passkeyOK} title={passkeyOK ? undefined : t("用 IP 地址访问时不能添加 Passkey")} onClick={async () => {
            // 第一把就叫“我的 Passkey”；已经有了再加的才是“备用”。
            const name = await askText(t("给这把 Passkey 起个名字，方便以后认出它（例如：工作电脑、YubiKey）"), {
              title: t("添加 Passkey"), confirmLabel: t("下一步"), initial: info.passkeys.length ? t("备用 Passkey") : t("我的 Passkey"),
            });
            if (name) act(() => api.addPasskey(name), t("已添加 Passkey"));
          }}><Plus />{t("添加 Passkey")}</button>
        </div>
        <div className="card-body col" style={{ gap: 12 }}>
          {!passkeyOK && (
            <div className="note warn"><TriangleAlert /><span>{t("现在是用 IP 地址访问面板。浏览器规定 Passkey 只能绑定在域名上，用 IP 访问时添加不了，也用不了。给面板配一个域名（用 HTTPS 访问）后就能添加。")}</span></div>
          )}
          {info.passkeys.length === 0 && <div className="muted">{t("还没有 Passkey。")}</div>}
          {info.passkeys.length === 1 && <div className="hint">{t("同一个密码管理器里只能存一把。要换一把，先删掉旧的再添加；删之前确认还有别的登录方式。")}</div>}
          {info.passkeys.map((p) => (
            <div key={p.id} className="row">
              <Fingerprint size={16} className="muted" /><span style={{ fontWeight: 520 }}>{p.name}</span><span className="grow" />
              <span className="muted">{t("添加于 {when}", { when: ago(p.createdAt) })}</span>
              <button className="btn ghost icon sm" disabled={busy} title={t("删除")} aria-label={t("删除 {name}", { name: p.name })}
                onClick={async () => (await askConfirm(t("删除 Passkey {name}？", { name: p.name }))) && act(() => api.deletePasskey(p.id), t("已删除"))}><Trash2 /></button>
            </div>
          ))}
        </div>
      </div>

      {dialog === "username" && <UsernameDialog current={userName} onClose={() => setDialog(null)} onDone={() => { setDialog(null); loadBoot(); }} />}
      {dialog === "password" && <PasswordDialog had={f.password} onClose={() => setDialog(null)} onDone={() => { setDialog(null); load(); loadBoot(); }} />}
      {dialog === "totp" && <TOTPDialog onClose={() => setDialog(null)} onDone={() => { setDialog(null); load(); }} />}
    </>
  );
}

function randomPassword(): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_.!@#%^*+=";
  const buf = new Uint32Array(24);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join("");
}

/** 修改自己的用户名：小写字母开头，3 到 32 个字符；保存前要验证身份。 */
function UsernameDialog({ current, onClose, onDone }: { current: string; onClose: () => void; onDone: () => void }) {
  const t = useT();
  const [name, setName] = useState(current);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const v = name.trim().toLowerCase();
  const ok = /^[a-z][a-z0-9._-]{2,31}$/.test(v) && v !== current;
  const save = async () => {
    setBusy(true); setErr("");
    try { await withVerify(() => api.setUsername(v)); toast(t("用户名已改成 {name}", { name: v })); onDone(); }
    catch (e) { if (!(e instanceof Error && e.name === "VerifyCancelled")) setErr(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };
  return (
    <Modal title={t("修改用户名")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" type="submit" form="un-form" disabled={busy || !ok}>{t("保存")}</button></>}>
      <form id="un-form" className="col" style={{ gap: 14 }} onSubmit={(e) => { e.preventDefault(); if (ok) save(); }}>
        <div className="field">
          <label htmlFor="un-new">{t("新用户名")}</label>
          <input id="un-new" className="input mono" autoComplete="username" maxLength={32} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          <span className="hint">{t("小写字母开头，3 到 32 个字符，只能用小写字母、数字、点、下划线和连字符。下次用密码登录时用新名字。")}</span>
        </div>
        {err && <div className="note crit" role="alert"><TriangleAlert /><span>{ts(err)}</span></div>}
      </form>
    </Modal>
  );
}

function PasswordDialog({ had, onClose, onDone }: { had: boolean; onClose: () => void; onDone: () => void }) {
  const t = useT();
  const userName = useStore((s) => s.boot?.user?.name ?? "");
  const [pw, setPw] = useState("");
  const [again, setAgain] = useState("");
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const ok = pw.length >= 12 && pw === again;
  const save = async () => {
    setBusy(true); setErr("");
    try { await withVerify(() => api.setPassword(pw)); toast(t("密码已更新")); onDone(); }
    catch (e) { if (!(e instanceof Error && e.name === "VerifyCancelled")) setErr(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };
  return (
    <Modal title={had ? t("修改密码") : t("设置密码")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" type="submit" form="pw-form" disabled={busy || !ok}>{t("保存")}</button></>}>
      <form id="pw-form" className="col" style={{ gap: 14 }} onSubmit={(e) => { e.preventDefault(); if (ok) save(); }}>
        <input type="text" autoComplete="username" value={userName} readOnly hidden />
        <div className="field">
          <label htmlFor="pw-new">{t("新密码")}</label>
          <div className="row" style={{ gap: 6 }}>
            <input id="pw-new" className="input mono grow" type={show ? "text" : "password"} autoComplete="new-password" value={pw} onChange={(e) => setPw(e.target.value)} autoFocus />
            <button type="button" className="btn icon" title={show ? t("隐藏") : t("显示")} aria-label={show ? t("隐藏") : t("显示")} onClick={() => setShow(!show)}>{show ? <EyeOff /> : <Eye />}</button>
            <button type="button" className="btn" onClick={() => { const p = randomPassword(); setPw(p); setAgain(p); setShow(true); }}><Dices />{t("随机生成")}</button>
          </div>
          <span className="hint">{pw && pw.length < 12 ? t("还差 {n} 个字符", { n: 12 - pw.length }) : t("至少 12 个字符。随机生成的密码请存进密码管理器。")}</span>
        </div>
        <div className="field">
          <label htmlFor="pw-again">{t("再输一次")}</label>
          <input id="pw-again" className="input mono" type={show ? "text" : "password"} autoComplete="new-password" value={again} onChange={(e) => setAgain(e.target.value)} />
          {again && pw !== again && <span className="hint crit-text">{t("两次输入不一样")}</span>}
        </div>
        {err && <div className="note crit" role="alert"><TriangleAlert /><span>{err}</span></div>}
      </form>
    </Modal>
  );
}

/** 二维码：用库算出模块，自己画成 SVG（不用 innerHTML）。 */
function QR({ text, size = 184 }: { text: string; size?: number }) {
  const cells = useMemo(() => {
    const q = qrcode(0, "M");
    q.addData(text);
    q.make();
    const n = q.getModuleCount();
    const out: string[] = [];
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (q.isDark(r, c)) out.push(`M${c + 2},${r + 2}h1v1h-1z`);
    return { n: n + 4, d: out.join("") };
  }, [text]);
  return (
    <svg className="qr" width={size} height={size} viewBox={`0 0 ${cells.n} ${cells.n}`} role="img" aria-label="QR" shapeRendering="crispEdges">
      <rect width={cells.n} height={cells.n} fill="#fff" />
      <path d={cells.d} fill="#000" />
    </svg>
  );
}

function TOTPDialog({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const t = useT();
  const [data, setData] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    withVerify(() => api.totpBegin()).then(setData).catch((e) => { fail(e); onClose(); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  if (!data) return null;
  const finish = async () => {
    setBusy(true); setErr("");
    try { await withVerify(() => api.totpFinish(code)); toast(t("已绑定动态码")); onDone(); }
    catch (e) { setCode(""); if (!(e instanceof Error && e.name === "VerifyCancelled")) setErr(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };
  const grouped = data.secret.replace(/(.{4})/g, "$1 ").trim();
  return (
    <Modal title={t("绑定动态码")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" type="submit" form="totp-form" disabled={busy || code.replace(/\s/g, "").length !== 6}><ShieldCheck />{t("确认绑定")}</button></>}>
      <form id="totp-form" className="col" style={{ gap: 14 }} onSubmit={(e) => { e.preventDefault(); finish(); }}>
        <p className="dim">{t("1. 用验证器 App 扫下面的二维码；扫不了就手动输入密钥。2. 输入 App 上显示的 6 位数字完成绑定。")}</p>
        <div className="totp-setup">
          <QR text={data.uri} />
          <div className="col" style={{ gap: 8, minWidth: 0 }}>
            <span className="muted" style={{ fontSize: 12.5 }}>{t("密钥（只显示这一次）")}</span>
            <code className="mono totp-secret">{grouped}</code>
            <button type="button" className="btn sm" style={{ alignSelf: "flex-start" }} onClick={() => navigator.clipboard?.writeText(data.secret).then(() => toast(t("已复制"))).catch(fail)}><Copy />{t("复制密钥")}</button>
          </div>
        </div>
        <div className="field" style={{ maxWidth: 260 }}>
          <label htmlFor="totp-code">{t("App 上显示的 6 位数字")}</label>
          <input id="totp-code" className="input mono" inputMode="numeric" autoComplete="one-time-code" maxLength={7} value={code} onChange={(e) => setCode(e.target.value.replace(/[^\d ]/g, ""))} autoFocus />
        </div>
        {err && <div className="note crit" role="alert"><TriangleAlert /><span>{err}</span></div>}
      </form>
    </Modal>
  );
}
