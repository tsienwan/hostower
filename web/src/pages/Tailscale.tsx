// 网络页上的 Tailscale 卡片：把几台主机连进同一个加密的内网，互相访问不用把端口对公网开放。
// 和这一页别的卡片一样没有自己的执行通道：现状来自只读检测，安装、加入、改设置、断开、退出登录都变成请求，
// 而且都是 L2（它决定这台主机能被谁连上）。授权密钥只选应用密钥的名字，值不经过页面。
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { LogOut, Network, Play, Plus, Settings2, ShieldAlert, Square, TriangleAlert } from "lucide-react";
import { api, type AppPlanResult, type NetworkView, type Req, type TSOptions } from "../api";
import { useStore } from "../store";
import { Select } from "../controls";
import { useT } from "../i18n";

type Plan = (p: { title: string; send: (preview: boolean) => Promise<AppPlanResult>; onClose: () => void; children: React.ReactNode; ready?: boolean }) => React.ReactNode;
type ReqLinkC = (p: { req: Req; running: string }) => React.ReactNode;

const nameOK = (s: string) => s === "" || /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/.test(s);

function Check({ checked, onChange, children }: { checked: boolean; onChange: (v: boolean) => void; children: React.ReactNode }) {
  return <label className="check"><input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />{children}</label>;
}

/** 几项设置的表单：加入网络和改设置共用。 */
function Options({ host, value, onChange }: { host: string; value: TSOptions; onChange: (o: TSOptions) => void }) {
  const t = useT();
  const set = (p: Partial<TSOptions>) => onChange({ ...value, ...p });
  return (
    <>
      <div className="field"><label htmlFor="ts-name">{t("在 Tailscale 网络里的名字")}</label>
        <input id="ts-name" className="input mono" value={value.hostname} onChange={(e) => set({ hostname: e.target.value.trim() })} placeholder={host} spellCheck={false} />
        <span className="hint">{nameOK(value.hostname) ? t("留空就用系统的主机名。只能用字母、数字和连字符。") : t("名字只能用字母、数字和连字符，不能以连字符开头或结尾。")}</span></div>
      <div className="col" style={{ gap: 8 }}>
        <Check checked={value.acceptDns} onChange={(v) => set({ acceptDns: v })}>{t("使用 Tailscale 网络的 DNS（可以用名字访问别的主机；会接管这台主机的 DNS 设置）")}</Check>
        <Check checked={value.acceptRoutes} onChange={(v) => set({ acceptRoutes: v })}>{t("接受别的节点通告的子网路由")}</Check>
        <Check checked={value.ssh} onChange={(v) => set({ ssh: v })}>{t("开启 Tailscale SSH")}</Check>
      </div>
      {value.ssh && <div className="note warn"><TriangleAlert /><span>{t("开启后，Tailscale 网络里被访问规则允许的人可以直接登录这台主机，不经过 sshd 的密钥和这里的审批。只在你清楚那个网络的访问规则时开启。")}</span></div>}
    </>
  );
}

const defaults: TSOptions = { hostname: "", acceptRoutes: false, acceptDns: true, ssh: false };

/** 加入网络：选一条授权密钥（应用密钥），或者不选、提交后在请求的输出里打开登录链接。 */
function JoinDialog({ host, st, Plan, onClose }: { host: string; st: NetworkView["status"]; Plan: Plan; onClose: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const again = st.tailscale.state === "Stopped";
  const [o, setO] = useState<TSOptions>(st.tailscale.options ?? defaults);
  const [secret, setSecret] = useState("");
  const [secrets, setSecrets] = useState<string[]>([]);
  useEffect(() => { api.appSecrets().then((r) => setSecrets(r.secrets.map((x) => x.name))).catch(() => {}); }, []);
  return (
    <Plan title={again ? t("让 {host} 重新连上 Tailscale", { host }) : t("把 {host} 加入 Tailscale 网络", { host })} onClose={onClose} ready={nameOK(o.hostname)}
      send={(preview) => api.hostTailscale(host, { action: "up", options: o, secret: again ? "" : secret, preview })}>
      {!again && (
        <div className="field"><label>{t("怎么登录")}</label>
          <Select value={secret} onChange={setSecret} options={[{ value: "", label: t("在浏览器里登录"), hint: t("不用密钥") }, ...secrets.map((s) => ({ value: s, label: s, hint: t("授权密钥") }))]} />
          <span className="hint">{secret
            ? t("用这条应用密钥里存的授权密钥登录。密钥在执行时从标准输入送给 tailscale，不会出现在命令、请求和审计里。")
            : <>{t("提交并批准后，请求的输出里会出现一个登录链接：在浏览器里打开它并登录 Tailscale 账号，请求会一直等到登录完成（最多 9 分钟）。")}
                {secrets.length === 0 && <> {owner ? <>{t("想不用人登录，先在 Tailscale 后台生成授权密钥，到 ")}<Link to="/credentials" style={{ color: "var(--accent)" }}>{t("凭据")}</Link>{t(" 页存成应用密钥，再回来选。")}</> : t("想不用人登录，请所有者把授权密钥存成应用密钥。")}</>}</>}</span>
        </div>
      )}
      <Options host={host} value={o} onChange={setO} />
      <div className="note warn"><ShieldAlert /><span>{t("加入以后，这台主机会多一个内网地址，同一个 Tailscale 网络里的设备按那边的访问规则可以连到它。这是高危操作，批准时要重新验证身份。")}</span></div>
    </Plan>
  );
}

function SettingsDialog({ host, st, Plan, onClose }: { host: string; st: NetworkView["status"]; Plan: Plan; onClose: () => void }) {
  const t = useT();
  const [o, setO] = useState<TSOptions>(st.tailscale.options ?? defaults);
  return (
    <Plan title={t("{host} 上的 Tailscale 设置", { host })} onClose={onClose} ready={nameOK(o.hostname)} send={(preview) => api.hostTailscale(host, { action: "set", options: o, preview })}>
      {!st.tailscale.options && <div className="note warn"><TriangleAlert /><span>{t("读不出这台主机当前的 Tailscale 设置，下面显示的是默认值：提交会按这里的选择覆盖原来的设置。")}</span></div>}
      <Options host={host} value={o} onChange={setO} />
    </Plan>
  );
}

export function TailscaleCard({ host, data, canRoot, loading, reqs, activeReq, Plan, ReqLink }: {
  host: string; data: NetworkView; canRoot: boolean; loading: boolean; reqs: Req[];
  activeReq: (reqs: Req[], host: string, match: (intent: string) => boolean) => Req | null; Plan: Plan; ReqLink: ReqLinkC;
}) {
  const t = useT();
  const st = data.status;
  const ts = st.tailscale;
  const [dialog, setDialog] = useState<null | "install" | "join" | "set" | "down" | "logout">(null);
  const installReq = activeReq(reqs, host, (i) => i === `在 ${host} 上安装 Tailscale`);
  const joinReq = activeReq(reqs, host, (i) => i === `把 ${host} 加入 Tailscale 网络`);
  const close = () => setDialog(null);
  const locked = ts.viaTailscale ? t("Hostower 就是经 Tailscale 连到这台主机的，断开会把它自己断在外面") : undefined;
  const on = ts.state === "Running", off = ts.state === "Stopped";
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2><Network size={16} style={{ verticalAlign: -2, marginRight: 6 }} />{t("Tailscale（主机之间的加密内网）")}</h2>
        {!ts.installed ? <span className="badge neutral">{t("没有安装")}</span>
          : !ts.active ? <span className="badge warn">{t("没有运行")}</span>
          : on ? <span className="badge ok">{t("已连接")}</span>
          : off ? <span className="badge neutral">{t("已断开")}</span>
          : ts.state === "NeedsLogin" || ts.state === "NeedsMachineAuth" ? <span className="badge warn">{ts.state === "NeedsLogin" ? t("还没有登录") : t("等待管理员批准")}</span>
          : <span className="badge warn">{t("读不出状态")}</span>}
        <span className="grow" />
        {!ts.installed
          ? installReq ? <ReqLink req={installReq} running={t("正在安装…")} />
            : ts.installable && <button className="btn primary sm" onClick={() => setDialog("install")} disabled={!canRoot || loading}><Plus />{t("安装")}</button>
          : joinReq ? <ReqLink req={joinReq} running={t("正在加入，等待登录…")} />
          : on ? <>
              <button className="btn sm" onClick={() => setDialog("set")} disabled={!canRoot}><Settings2 />{t("设置")}</button>
              <button className="btn sm" onClick={() => setDialog("down")} disabled={!canRoot || !!locked} title={locked}><Square />{t("断开")}</button>
            </>
          : off ? <>
              <button className="btn primary sm" onClick={() => setDialog("join")} disabled={!canRoot || loading}><Play />{t("重新连上")}</button>
              <button className="btn sm" onClick={() => setDialog("logout")} disabled={!canRoot || !!locked} title={locked}><LogOut />{t("退出登录")}</button>
            </>
          : <button className="btn primary sm" onClick={() => setDialog("join")} disabled={!canRoot || loading}><Play />{t("加入网络")}</button>}
      </div>
      <div className="card-body">
        <p className="dim" style={{ marginTop: 0 }}>{t("把几台主机连进同一个加密的内网：每台有一个 100.x 的地址，主机之间互相访问不用把端口对公网开放。安装、加入、改设置、断开都是高危操作，批准时要重新验证身份。")}</p>
        {!ts.installed && !ts.installable && <div className="muted">{t("这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装。")}</div>}
        {ts.installed && (on || off) && (
          <div className="row wrap" style={{ gap: "8px 28px", fontSize: 13 }}>
            {ts.ips.length > 0 && <span><span className="muted">{t("内网地址")}　</span><span className="mono">{ts.ips.join("　")}</span></span>}
            {(ts.dnsName || ts.name) && <span><span className="muted">{t("名字")}　</span><span className="mono">{ts.dnsName || ts.name}</span></span>}
            {ts.tailnet && <span><span className="muted">{t("所在网络")}　</span><span className="mono">{ts.tailnet}</span></span>}
            {ts.version && <span><span className="muted">{t("版本")}　</span><span className="mono">{ts.version}</span></span>}
          </div>
        )}
        {on && <p className="hint" style={{ margin: "10px 0 0" }}>{t("别的主机加入同一个网络后，可以用上面的地址访问这台主机。在另一台主机上添加站点时，“转发到哪里”填这个地址，两台之间这一段就是加密的。")}</p>}
        {ts.installed && ts.state === "NeedsLogin" && !joinReq && <div className="muted">{t("已经装好，还没有登录：点“加入网络”。")}</div>}
        {ts.state === "NeedsMachineAuth" && <div className="note warn"><TriangleAlert /><span>{t("这台主机已经登录，但那个 Tailscale 网络要求管理员批准新设备：到 Tailscale 后台批准它，再回来刷新。")}</span></div>}
      </div>
      {dialog === "install" && (
        <Plan title={t("在 {host} 上安装 Tailscale", { host })} onClose={close} send={(preview) => api.hostTailscale(host, { action: "install", preview })}>
          <div>{t("添加 Tailscale 的官方软件源（pkgs.tailscale.com）和它的签名密钥，用系统的包管理器安装，并启动 tailscaled 服务。装完还没有登录，要再“加入网络”。")}</div>
          <div className="note warn"><ShieldAlert /><span>{t("这台主机要能连上 pkgs.tailscale.com。Tailscale 决定主机能被谁连上，安装是高危操作，批准时要重新验证身份。")}</span></div>
        </Plan>
      )}
      {dialog === "join" && <JoinDialog host={host} st={st} Plan={Plan} onClose={close} />}
      {dialog === "set" && <SettingsDialog host={host} st={st} Plan={Plan} onClose={close} />}
      {dialog === "down" && (
        <Plan title={t("断开 {host} 的 Tailscale", { host })} onClose={close} send={(preview) => api.hostTailscale(host, { action: "down", preview })}>
          <div>{t("断开后这台主机的内网地址不再可用：经这个地址访问它的站点和连接都会断。登录状态还在，随时可以重新连上；要彻底离开那个网络，断开后再“退出登录”。")}</div>
        </Plan>
      )}
      {dialog === "logout" && (
        <Plan title={t("让 {host} 退出 Tailscale 登录", { host })} onClose={close} send={(preview) => api.hostTailscale(host, { action: "logout", preview })}>
          <div>{t("退出登录后，这台主机不再属于那个 Tailscale 网络，再加入要重新登录或用新的授权密钥。")}</div>
        </Plan>
      )}
    </div>
  );
}
