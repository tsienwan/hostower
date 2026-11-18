// 凭据：塔台里只存一份，按名字引用，用到时才取出来。保存之后任何界面和接口都取不回原文。
import { useCallback, useEffect, useState } from "react";
import { askConfirm } from "../dialogs";
import { Box, Info, KeyRound, Plus, RefreshCw, Server, Trash2, TriangleAlert } from "lucide-react";
import { api, type Registry } from "../api";
import { fail, toast, useStore } from "../store";
import { Modal, ago } from "../ui";
import { useT } from "../i18n";
import { AddSecret } from "./Settings";

type Item = { name: string; updatedAt: string };

export function Credentials() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const [available, setAvailable] = useState(true);
  const [autoKey, setAutoKey] = useState(false);
  const [ssh, setSsh] = useState<Item[]>([]);
  const [regs, setRegs] = useState<Registry[]>([]);
  const [apps, setApps] = useState<Item[]>([]);
  const [dialog, setDialog] = useState<null | "ssh" | "registry" | { app: string }>(null);

  const load = useCallback(() => {
    api.secrets().then((r) => { setSsh(r.secrets.filter((s) => s.name.startsWith("ssh/"))); setAvailable(r.available); setAutoKey(!!r.autoKey); }).catch(fail);
    api.registries().then((r) => setRegs(r.registries)).catch(fail);
    api.appSecrets().then((r) => setApps(r.secrets)).catch(fail);
  }, []);
  useEffect(load, [load]);

  const remove = async (what: string, fn: () => Promise<unknown>) => {
    if (!(await askConfirm(t("删除{what}？用到它的地方之后会失败。", { what })))) return;
    try { await fn(); toast(t("已删除")); load(); } catch (e) { fail(e); }
  };
  const usedBy = (name: string) => hosts.filter((h) => h.credential === "ref:" + name).map((h) => h.name);

  return (
    <>
      <div className="topbar"><h1>{t("凭据")}</h1><span className="muted hide-sm">{t("集中保存，按名字引用")}</span></div>
      <div className="content">
        <div className="page narrow">
          <p className="lede">{t("所有凭据只在这里存一份，用信封加密。保存后任何界面和接口都取不回原文——只能覆盖或删除。要换密钥就在这里改一处，再重新部署用到它的服务。")}</p>
          {!available && (
            <div className="note warn"><TriangleAlert /><span>{t("没有配置主密钥，凭据功能不可用。用 ")}<span className="mono">hostower keygen</span>{t(" 生成一把，存成文件并用环境变量 ")}<span className="mono">HOSTOWER_MASTER_KEY_FILE</span>{t(" 指向它，然后重启。主密钥要和数据目录分开备份：丢了它，这里的凭据都解不开。")}</span></div>
          )}
          {available && autoKey && (
            <div className="note"><Info /><span>{t("主密钥是第一次启动时自动生成的，放在数据目录的 ")}<span className="mono">master_key</span>{t(" 文件里。只拿到数据库的人解不开凭据，但拿到整个数据目录或它的备份的人可以。想分开保管：把这个文件移到数据目录以外，用环境变量 ")}<span className="mono">HOSTOWER_MASTER_KEY_FILE</span>{t(" 指向它，然后重启。不要删除它：丢了它，这里的凭据都解不开。")}</span></div>
          )}

          <div className="card">
            <div className="card-head"><Server size={16} className="muted" /><h2>{t("SSH 凭据")}</h2><span className="muted hide-sm">{t("塔台连主机用")}</span><span className="grow" />
              <button className="btn sm" disabled={!available} onClick={() => setDialog("ssh")}><Plus />{t("添加")}</button></div>
            <div className="card-body col" style={{ gap: 12 }}>
              {ssh.length === 0 ? <span className="muted">{t("还没有。添加主机时在“SSH 凭据”里选它。")}</span> : ssh.map((s) => {
                const used = usedBy(s.name);
                return (
                  <div key={s.name} className="row">
                    <div className="cell-2 grow"><span className="mono" style={{ fontWeight: 600 }}>{s.name}</span>
                      <small>{used.length ? t("{n} 台主机在用：{names}", { n: used.length, names: used.slice(0, 4).join(t("、")) + (used.length > 4 ? t(" 等") : "") }) : t("没有主机在用")} · {t("更新于 {when}", { when: ago(s.updatedAt) })}</small></div>
                    <button className="btn ghost icon sm" aria-label={t("删除 {name}", { name: s.name })} title={t("删除")} onClick={() => remove(t(" SSH 凭据 {name}", { name: s.name }), () => api.deleteSecret(s.name))}><Trash2 /></button>
                  </div>
                );
              })}
            </div>
          </div>

          <div className="card">
            <div className="card-head"><Box size={16} className="muted" /><h2>{t("镜像仓库")}</h2><span className="muted hide-sm">{t("拉取私有镜像用")}</span><span className="grow" />
              <button className="btn sm" disabled={!available} onClick={() => setDialog("registry")}><Plus />{t("添加")}</button></div>
            <div className="card-body col" style={{ gap: 12 }}>
              {regs.length === 0 ? <span className="muted">{t("还没有。拉取公开镜像不需要登录。")}</span> : regs.map((r) => (
                <div key={r.name} className="row">
                  <div className="cell-2 grow"><span className="mono" style={{ fontWeight: 600 }}>{r.name}</span><small className="mono">{r.username}@{r.server} · {t("更新于 {when}", { when: ago(r.updatedAt) })}</small></div>
                  <button className="btn ghost icon sm" aria-label={t("删除 {name}", { name: r.name })} title={t("删除")} onClick={() => remove(t("镜像仓库 {name}", { name: r.name }), () => api.deleteRegistry(r.name))}><Trash2 /></button>
                </div>
              ))}
              <span className="hint">{t("保存后到“容器”页的“镜像仓库”里，在需要的主机上登录一次。")}</span>
            </div>
          </div>

          <div className="card">
            <div className="card-head"><KeyRound size={16} className="muted" /><h2>{t("应用密钥")}</h2><span className="muted hide-sm">{t("数据库密码、接口密钥等")}</span><span className="grow" />
              <button className="btn sm" disabled={!available} onClick={() => setDialog({ app: "" })}><Plus />{t("添加")}</button></div>
            <div className="card-body col" style={{ gap: 12 }}>
              {apps.length === 0 ? <span className="muted">{t("还没有。添加之后，部署容器时可以把它作为环境变量注入。")}</span> : apps.map((a) => (
                <div key={a.name} className="row">
                  <div className="cell-2 grow"><span className="mono" style={{ fontWeight: 600 }}>{a.name}</span><small>{t("更新于 {when}", { when: ago(a.updatedAt) })}</small></div>
                  <button className="btn sm" onClick={() => setDialog({ app: a.name })}><RefreshCw />{t("换值")}</button>
                  <button className="btn ghost icon sm" aria-label={t("删除 {name}", { name: a.name })} title={t("删除")} onClick={() => remove(t("应用密钥 {name}", { name: a.name }), () => api.deleteAppSecret(a.name))}><Trash2 /></button>
                </div>
              ))}
              <span className="hint">{t("部署容器时选“变量名 ← 密钥”。塔台在部署那一刻把值写成主机上只有登录账号可读的文件（")}<span className="mono">{t("~/.hostower/env/容器名.env")}</span>{t("），容器从它读取。换值之后要重新部署才生效。")}</span>
            </div>
          </div>
        </div>
      </div>

      {dialog === "ssh" && <AddSecret onClose={() => setDialog(null)} onSaved={() => { setDialog(null); load(); toast(t("凭据已保存")); }} />}
      {dialog === "registry" && <RegistryForm onClose={() => setDialog(null)} onSaved={() => { setDialog(null); load(); toast(t("仓库凭据已保存")); }} />}
      {dialog && typeof dialog === "object" && <AppSecretForm name={dialog.app} onClose={() => setDialog(null)} onSaved={() => { setDialog(null); load(); toast(t("密钥已保存")); }} />}
    </>
  );
}

function RegistryForm({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const t = useT();
  const [f, setF] = useState({ name: "", server: "docker.io", username: "", password: "" });
  const [busy, setBusy] = useState(false);
  const save = async () => { setBusy(true); try { await api.putRegistry(f); onSaved(); } catch (e) { fail(e); } finally { setBusy(false); } };
  return (
    <Modal title={t("添加镜像仓库")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !f.name || !f.server || !f.username || !f.password} onClick={save}>{t("加密保存")}</button></>}>
      <div className="form-grid">
        <div className="field"><label htmlFor="rg-name">{t("名称")}</label><input id="rg-name" className="input mono" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} spellCheck={false} placeholder="hub" autoFocus /></div>
        <div className="field"><label htmlFor="rg-server">{t("仓库地址")}</label><input id="rg-server" className="input mono" value={f.server} onChange={(e) => setF({ ...f, server: e.target.value })} spellCheck={false} /></div>
        <div className="field"><label htmlFor="rg-user">{t("用户名")}</label><input id="rg-user" className="input mono" value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} spellCheck={false} autoComplete="off" /></div>
        <div className="field"><label htmlFor="rg-pass">{t("密码或访问令牌")}</label><input id="rg-pass" className="input mono" type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" /></div>
      </div>
      <span className="hint">{t("Docker Hub 的地址填 ")}<span className="mono">docker.io</span>{t("；建议用只读的访问令牌，而不是账号密码。")}</span>
    </Modal>
  );
}

function AppSecretForm({ name: initial, onClose, onSaved }: { name: string; onClose: () => void; onSaved: () => void }) {
  const t = useT();
  const [name, setName] = useState(initial);
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => { setBusy(true); try { await api.putAppSecret(name, value); onSaved(); } catch (e) { fail(e); } finally { setBusy(false); } };
  return (
    <Modal title={initial ? t("给 {name} 换值", { name: initial }) : t("添加应用密钥")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !name || !value} onClick={save}>{t("加密保存")}</button></>}>
      <div className="field"><label htmlFor="as-name">{t("名称")}</label>
        <input id="as-name" className="input mono" value={name} disabled={!!initial} onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-"))} spellCheck={false} placeholder="db-password" autoFocus={!initial} />
        <span className="hint">{t("小写字母、数字、连字符。部署时按这个名字选。")}</span></div>
      <div className="field"><label htmlFor="as-value">{t("值")}</label>
        <input id="as-value" className="input mono" type="password" value={value} onChange={(e) => setValue(e.target.value)} autoComplete="new-password" autoFocus={!!initial} />
        <span className="hint">{t("保存后看不到原文。不能含换行。")}{initial ? t("换值后，重新部署用到它的容器才生效。") : ""}</span></div>
    </Modal>
  );
}
