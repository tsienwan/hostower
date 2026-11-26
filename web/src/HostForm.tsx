// 在页面上新增或编辑主机。配置文件里声明的主机是只读的，要改就改文件。
import { useEffect, useMemo, useState } from "react";
import { TriangleAlert } from "lucide-react";
import { api, type Host } from "./api";
import { fail, refreshHosts, toast, useStore } from "./store";
import { Modal } from "./ui";
import { Select, TagInput } from "./controls";
import { useT } from "./i18n";

export function HostForm({ host, onClose }: { host?: Host; onClose: () => void }) {
  const t = useT();
  const editing = !!host;
  // 内置的本机：名字固定；连接方式是本机直连，或者经 SSH 连回本机（用一个能 sudo 的账号管防火墙和站点）。
  const builtin = host?.source === "builtin";
  const all = useStore((s) => s.hosts);
  const [name, setName] = useState(host?.name ?? "");
  const [addr, setAddr] = useState(host?.addr ?? "");
  const [executor, setExecutor] = useState(host?.executor ?? "ssh");
  const [credential, setCredential] = useState(host?.credential ?? "");
  const [tags, setTags] = useState<string[]>(host?.tags ?? []);
  const [sensitivity, setSensitivity] = useState<string>(host?.sensitivity ?? "normal");
  const [executors, setExecutors] = useState<string[]>(["ssh"]);
  const [secrets, setSecrets] = useState<string[]>([]);
  const [secretsReady, setSecretsReady] = useState(true);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.system().then((s) => { setExecutors(s.executors); setSecretsReady(s.secretsAvailable); }).catch(fail);
    api.secrets().then((r) => setSecrets(r.secrets.filter((s) => s.name.startsWith("ssh/")).map((s) => "ref:" + s.name))).catch(fail);
  }, []);

  // 已有的标签，按使用次数排序，方便直接点选。
  const known = useMemo(() => {
    const count = new Map<string, number>();
    for (const h of all) for (const tag of h.tags) count.set(tag, (count.get(tag) ?? 0) + 1);
    return [...count.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([tag]) => tag);
  }, [all]);

  const ssh = executor === "ssh";
  const valid = name.trim() && addr.trim() && (!ssh || credential);

  const save = async () => {
    setBusy(true);
    try {
      const body = { name: name.trim(), addr: addr.trim(), executor, credential: ssh ? credential : "", tags, sensitivity };
      if (editing) await api.updateHost(body); else await api.createHost(body);
      await refreshHosts();
      toast(editing ? t("主机已更新") : t("主机已添加"));
      onClose();
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <Modal title={editing ? t("编辑主机 {name}", { name: host!.name }) : t("添加主机")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !valid} onClick={save}>{editing ? t("保存") : t("添加")}</button></>}>
      <div className="form-grid">
        <div className="field">
          <label htmlFor="h-name">{t("名称")}</label>
          <input id="h-name" className="input mono" value={name} onChange={(e) => setName(e.target.value)} disabled={editing} spellCheck={false} placeholder="web-1" autoFocus={!editing} />
        </div>
        <div className="field">
          <label htmlFor="h-addr">{t("地址")}</label>
          <input id="h-addr" className="input mono" value={addr} onChange={(e) => setAddr(e.target.value)} disabled={builtin && !ssh} spellCheck={false} placeholder={t("192.0.2.10 或 192.0.2.10:2222")} />
        </div>
        <div className="field">
          <label htmlFor="h-exec">{t("连接方式")}</label>
          {builtin ? <Select id="h-exec" value={executor} onChange={(e) => { setExecutor(e); setAddr(e === "ssh" ? (host?.executor === "ssh" ? host.addr : "127.0.0.1") : "localhost"); }}
            options={[{ value: "local", label: t("本机直连") }, { value: "ssh", label: "SSH", hint: "127.0.0.1" }]} /> : <Select id="h-exec" value={executor} onChange={setExecutor}
            options={executors.map((e) => ({ value: e, label: e === "ssh" ? "SSH" : e === "sim" ? t("模拟主机") : e, hint: e === "ssh" ? t("塔台 → 主机") : e === "sim" ? t("仅开发模式") : undefined }))} />}
        </div>
        <div className="field">
          <label htmlFor="h-sens">{t("敏感度")}</label>
          <Select id="h-sens" value={sensitivity} onChange={setSensitivity}
            options={[{ value: "low", label: t("低") }, { value: "normal", label: t("普通") }, { value: "high", label: t("高"), hint: t("写操作自动升一级") }]} />
        </div>
      </div>
      {builtin && ssh && <div className="hint">{t("经 SSH 连回这台机器：地址只能是 127.0.0.1 或 localhost（可以带端口）。选一条能免密 sudo 的账号的凭据，就能管防火墙和站点；随时可以改回本机直连。")}</div>}
      {ssh && (
        <div className="field">
          <label htmlFor="h-cred">{t("SSH 凭据")}</label>
          <Select id="h-cred" mono value={credential} onChange={setCredential}
            options={[...new Set([...secrets, ...(credential ? [credential] : [])])].map((s) => ({ value: s, label: s }))} />
          {secrets.length === 0 && (
            <div className="note warn"><TriangleAlert /><span>{secretsReady ? t("还没有凭据。先到“设置 → SSH 凭据”添加一条，再回来选。") : t("没有配置主密钥，凭据功能不可用（见“设置”页的说明）。")}</span></div>
          )}
        </div>
      )}
      <div className="field">
        <label htmlFor="h-tags">{t("标签")}</label>
        <TagInput id="h-tags" value={tags} onChange={setTags} suggestions={known} placeholder={t("输入后按回车新建，例如 web")} />
        <span className="hint">{t("分组按标签自动匹配：带")} <span className="mono">docker</span> {t("标签的主机会进入选择器是")} <span className="mono">tag:docker</span> {t("的分组。")}</span>
      </div>
    </Modal>
  );
}
