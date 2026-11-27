// 测速：给一台或多台主机测延迟、下载、上传。默认用 Speedtest by Ookla（可以指定测速服务器），也可以用 Cloudflare（数据量固定）。
// 发起后生成一个 L1 请求（会从主机向外下载、上传数据，消耗流量），批准后执行；结果从请求的输出里解析出来存下。
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { askConfirm } from "../dialogs";
import { Link } from "react-router-dom";
import { Check, FileText, Gauge, RefreshCw, Search, ShieldCheck, Trash2, TriangleAlert } from "lucide-react";
import { api, type OoklaLicense, type SpeedConfig, type SpeedIP, type SpeedMethod, type SpeedResult, type SpeedServer, type SpeedThreads } from "../api";
import { fail, toast, upsertRequest, useStore } from "../store";
import { Empty, LevelBadge, Modal, Pager, StateBadge, ago, stamp, usePaged } from "../ui";
import { Select } from "../controls";
import { ApprovalCard } from "./Requests";
import { cached, remember } from "../cache";
import { ts, useT } from "../i18n";
import { SpeedReport, SpeedTargets } from "./SpeedReport";
import { ClientCard } from "./SpeedClient";

const TAG = "builtin:speedtest@1";

/** 比特每秒 → “48.9 Mbps” */
export function mbps(bps: number): string {
  if (!bps) return "—";
  if (bps >= 1e9) return (bps / 1e9).toFixed(2) + " Gbps";
  if (bps >= 1e6) return (bps / 1e6).toFixed(bps >= 1e8 ? 0 : 1) + " Mbps";
  return (bps / 1e3).toFixed(0) + " Kbps";
}

type Step = "choose" | "approve";
type Check = { host: string; curl: boolean; trace: boolean; error?: string };
type Picked = { id: number; label: string };

const METHOD_KEY = "hostower.speedtest.method";
const SERVER_KEY = "hostower.speedtest.server";
const THREADS_KEY = "hostower.speedtest.threads";
const IP_KEY = "hostower.speedtest.ip";

/** 测速选项的显示名：“单线程 · IPv6”。没记下的（之前的结果）不显示。 */
export function optsLabel(t: (k: string) => string, threads?: string, ip?: string): string {
  const parts: string[] = [];
  if (threads === "single") parts.push(t("单线程"));
  if (threads === "multi") parts.push(t("多线程"));
  if (ip === "4" || ip === "6") parts.push("IPv" + ip);
  return parts.join(" · ");
}

/** 上次用的测速方式和服务器（只是这台浏览器的方便，读写失败就用默认）。 */
function remembered<T>(key: string, fallback: T): T {
  try { const v = localStorage.getItem(key); return v ? (JSON.parse(v) as T) : fallback; } catch { return fallback; }
}
function rememberChoice(key: string, v: unknown) {
  try { localStorage.setItem(key, JSON.stringify(v)); } catch { /* 存不下就算了 */ }
}

/** 测速的整个流程都在这一个弹窗里：选方式和服务器（或数据量）、检查 curl（没有可以顺便装上）→ 核对命令并批准 → 看各台主机的进度和结果。 */
export function SpeedtestDialog({ hosts, config, onClose, requestId }: {
  hosts: string[]; config: SpeedConfig; onClose: () => void;
  /** 已经发起的测速请求：直接从批准这一步开始。 */
  requestId?: string;
}) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const sizes = config.sizes;
  const [license, setLicense] = useState(config.ookla);
  const [method, setMethod] = useState<SpeedMethod>(() => remembered<SpeedMethod>(METHOD_KEY, "ookla") === "cloudflare" ? "cloudflare" : "ookla");
  const [picked, setPicked] = useState<Picked | null>(() => remembered<Picked | null>(SERVER_KEY, null));
  const [threads, setThreads] = useState<SpeedThreads>(() => remembered<SpeedThreads>(THREADS_KEY, "multi") === "single" ? "single" : "multi");
  const [ip, setIP] = useState<SpeedIP>(() => { const v = remembered<string>(IP_KEY, ""); return v === "4" || v === "6" ? v : ""; });
  const [size, setSize] = useState(sizes.includes(25) ? 25 : sizes[0]);
  const [checks, setChecks] = useState<Check[] | null>(null);
  const [install, setInstall] = useState(true);
  const [busy, setBusy] = useState(false);
  const [step, setStep] = useState<Step>(requestId ? "approve" : "choose");
  const [reqId, setReqId] = useState(requestId ?? "");
  const req = useStore((st) => st.requests.find((r) => r.id === reqId));
  const [results, setResults] = useState<SpeedResult[] | null>(null);
  const targets = config.targets;
  const [routes, setRoutes] = useState(true);
  const [installTrace, setInstallTrace] = useState(true);
  const [report, setReport] = useState("");

  // 打开时先看看哪些主机上没有 curl（只读查询）。
  useEffect(() => {
    if (requestId) return;
    let on = true;
    api.speedtestCheck(hosts).then((r) => on && setChecks(r.hosts)).catch(() => on && setChecks(hosts.map((h) => ({ host: h, curl: true, trace: true }))));
    return () => { on = false; };
  }, [hosts, requestId]);
  const missing = (checks ?? []).filter((c) => !c.curl && !c.error).map((c) => c.host);
  const tracing = routes && targets.length > 0;
  // 追踪路由时，没有 traceroute 和 tracepath 的主机可以顺便装上 traceroute（和装 curl 在同一个请求里）。
  const noTrace = (checks ?? []).filter((c) => !c.trace && !c.error).map((c) => c.host);
  const installHosts = [...new Set([...(install ? missing : []), ...(tracing && installTrace ? noTrace : [])])];
  const unreachable = (checks ?? []).filter((c) => c.error);
  const per = size + size / 4;
  const ookla = method === "ookla";
  const blocked = ookla && !license.accepted;

  const pickMethod = (m: SpeedMethod) => { setMethod(m); rememberChoice(METHOD_KEY, m); };
  const pickServer = (p: Picked | null) => { setPicked(p); rememberChoice(SERVER_KEY, p); };
  const pickThreads = (v: SpeedThreads) => { setThreads(v); rememberChoice(THREADS_KEY, v); };
  const pickIP = (v: SpeedIP) => { setIP(v); rememberChoice(IP_KEY, v); };
  const accept = () => {
    setBusy(true);
    api.speedtestOokla(true).then(setLicense).catch(fail).finally(() => setBusy(false));
  };

  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.runSpeedtest({ hosts, method, sizeMb: size, server: ookla ? picked?.id ?? 0 : 0, install: installHosts, routes: tracing, threads, ip });
      upsertRequest(r);
      setReqId(r.id);
      setStep("approve");
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  // 请求结束后取这一次的结果。
  const finished = !!req && !["pending", "running"].includes(req.state);
  useEffect(() => {
    if (!finished || !reqId) return;
    const timer = setTimeout(() => api.speedtest().then((r) => setResults(r.results.filter((x) => x.requestId === reqId))).catch(() => {}), 600);
    return () => clearTimeout(timer);
  }, [finished, reqId]);

  const names = req ? req.targets.map((x) => x.host) : hosts;
  const title = names.length === 1 ? t("给 {host} 测速", { host: names[0] }) : t("给 {n} 台主机测速", { n: names.length });
  const commands = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const tg of req?.targets ?? []) m.set(tg.command, [...(m.get(tg.command) ?? []), tg.host]);
    return [...m.entries()];
  }, [req]);

  if (step === "choose") {
    return (
      <Modal wide title={title} onClose={onClose}
        footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !checks || blocked} onClick={submit}><Gauge />{t("下一步：核对并批准")}</button></>}>
        <div className="field">
          <label>{t("测速方式")}</label>
          <div className="seg" role="radiogroup" aria-label={t("测速方式")}>
            <button role="radio" aria-checked={ookla} className={ookla ? "on" : ""} onClick={() => pickMethod("ookla")}>Speedtest by Ookla</button>
            <button role="radio" aria-checked={!ookla} className={!ookla ? "on" : ""} onClick={() => pickMethod("cloudflare")}>Cloudflare</button>
          </div>
          <span className="hint">{ookla
            ? t("用 Ookla 官方的命令行程序测到指定或最近的测速服务器，能看到抖动和丢包。主机上装了官方的 speedtest（比如从 Ookla 的软件源装的，会跟着系统更新）就直接用它；没有的话，第一次会把锁定版本 {version} 下载到 ~/.hostower/ 下，并校验官方发布包的 SHA-256。", { version: license.version })
            : t("从主机向 {server} 下载和上传一段固定大小的数据。", { server: config.server })}</span>
        </div>
        {ookla && !license.accepted && (
          <div className="note warn">
            <TriangleAlert />
            <div className="col" style={{ gap: 8 }}>
              <span>{t("使用 Speedtest by Ookla 之前，需要所有者接受 Ookla 的许可协议和隐私政策：测速结果会上传到 speedtest.net，并且只能用于个人、非商业用途。")}</span>
              <span className="row wrap" style={{ gap: 12 }}>
                <a href={license.terms} target="_blank" rel="noreferrer noopener">{t("许可协议")}</a>
                <a href={license.privacy} target="_blank" rel="noreferrer noopener">{t("隐私政策")}</a>
              </span>
              {owner
                ? <span><button className="btn primary sm" disabled={busy} onClick={accept}><Check />{t("我已阅读并接受")}</button></span>
                : <span className="hint">{t("请所有者接受后再用；也可以先用 Cloudflare 测速。")}</span>}
            </div>
          </div>
        )}
        <div className="sp-opts">
          <div className="field">
            <label>{t("线程")}</label>
            <div className="seg" role="radiogroup" aria-label={t("线程")}>
              {(["single", "multi"] as const).map((v) => <button key={v} role="radio" aria-checked={threads === v} className={threads === v ? "on" : ""} onClick={() => pickThreads(v)}>{v === "single" ? t("单线程") : t("多线程")}</button>)}
            </div>
          </div>
          <div className="field">
            <label>{t("IP 协议")}</label>
            <div className="seg" role="radiogroup" aria-label={t("IP 协议")}>
              {([["", t("自动")], ["4", "IPv4"], ["6", "IPv6"]] as const).map(([v, l]) => <button key={v} role="radio" aria-checked={ip === v} className={ip === v ? "on" : ""} onClick={() => pickIP(v)}>{l}</button>)}
            </div>
          </div>
        </div>
        <span className="hint sp-opts-hint">
          {threads === "single" ? t("单线程：只开一条连接，看单个下载能跑多快（比如单线程下载、代理单连接的速度）。")
            : ookla ? t("多线程：程序按带宽自动开多条连接，测的是总带宽。") : t("多线程：同时开 {n} 条连接，数据量照旧，平分到每条连接上。", { n: 4 })}
          {" "}{ip === "" ? t("IP 协议自动：按系统默认的出口走。") : t("只用 {ip} 测；主机没有这种协议的出口地址时会报错。", { ip: "IPv" + ip })}
        </span>
        {ookla ? (
          <ServerPicker picked={picked} onPick={pickServer} service={config.searchService} />
        ) : (
          <div className="field">
            <label>{t("下载量")}</label>
            <div className="seg" role="radiogroup" aria-label={t("下载量")}>
              {sizes.map((x) => <button key={x} role="radio" aria-checked={size === x} className={size === x ? "on" : ""} onClick={() => setSize(x)}>{x} MB</button>)}
            </div>
            <span className="hint">{t("上传量是下载量的四分之一。数据量越大结果越准，也越耗流量。")}</span>
          </div>
        )}
        {!checks ? (
          <div className="note"><RefreshCw className="spin" /><span>{t("正在检查主机上有没有 curl…")}</span></div>
        ) : missing.length > 0 ? (
          <div className="note warn">
            <TriangleAlert />
            <div className="col" style={{ gap: 8 }}>
              <span>{ookla ? t("这些主机上没有 curl：如果也没装官方的 speedtest，就没法下载 Speedtest 程序（已经下载过的不受影响）：") : t("这些主机上没有 curl，测不了速：")}<b className="mono">{missing.join(", ")}</b></span>
              <label className="check"><input type="checkbox" checked={install} onChange={(e) => setInstall(e.target.checked)} />{t("先用系统的包管理器自动安装 curl，装好后接着测速")}</label>
              <span className="hint">{t("支持 apt、dnf、yum、apk、zypper、pacman；需要 root 或免密 sudo。安装和测速在同一个请求里，批准一次。")}</span>
            </div>
          </div>
        ) : (
          <div className="note ok"><Check /><span>{t("选中的主机上都有 curl。")}</span></div>
        )}
        {targets.length > 0 && (
          <div className="field">
            <label className="check"><input type="checkbox" checked={routes} onChange={(e) => setRoutes(e.target.checked)} />{t("顺便追踪路由（{n} 个目标）", { n: targets.length })}</label>
            <span className="hint">{t("从主机向这些目标跑一次 traceroute，看经过哪些节点、走的哪条线路：{list}", { list: targets.map((x) => x.name || x.target).join("、") })}</span>
          </div>
        )}
        {checks && tracing && noTrace.length > 0 && (
          <div className="note warn">
            <TriangleAlert />
            <div className="col" style={{ gap: 8 }}>
              <span>{t("这些主机上没有 traceroute 或 tracepath，看不到线路：")}<b className="mono">{noTrace.join(", ")}</b></span>
              <label className="check"><input type="checkbox" checked={installTrace} onChange={(e) => setInstallTrace(e.target.checked)} />{t("顺便用系统的包管理器安装 traceroute")}</label>
              <span className="hint">{t("需要 root 或免密 sudo；装不上也照样测速，只是没有线路。")}</span>
            </div>
          </div>
        )}
        {unreachable.length > 0 && <div className="note crit"><TriangleAlert /><span>{t("这些主机检查失败，测速多半也会失败：{hosts}", { hosts: unreachable.map((c) => c.host).join(", ") })}</span></div>}
        {ookla
          ? <div className="note warn"><ShieldCheck /><span>{t("Speedtest 按带宽自动决定传多少数据，数据量控制不了：一次可能用掉几百 MB，千兆线路可能超过 1 GB。有流量配额的主机建议用 Cloudflare。这是一次变更（L1），下一步核对命令后在这里直接批准。")}</span></div>
          : <div className="note"><ShieldCheck /><span>{t("每台主机大约消耗 {mb} MB 流量，共 {total} MB；会从主机向 {server} 下载和上传数据。这是一次变更（L1），下一步核对命令后在这里直接批准。", { mb: per.toFixed(1).replace(/\.0$/, ""), total: (per * hosts.length).toFixed(1).replace(/\.0$/, ""), server: config.server })}</span></div>}
      </Modal>
    );
  }

  // 第二步之后：同一个弹窗里批准、看进度、看结果。
  return (
    <Modal wide title={title} onClose={onClose}
      footer={<button className={"btn" + (finished ? " primary" : "")} onClick={onClose}>{finished ? t("完成") : t("在后台继续")}</button>}>
      {!req ? <div className="note"><RefreshCw className="spin" /><span>{t("加载中…")}</span></div> : (
        <>
          <div className="row" style={{ gap: 10 }}>
            <LevelBadge level={req.level} />
            <span className="dim">{ts(req.intent)}</span>
            <span className="grow" />
            <StateBadge state={req.state} />
            <Link to={`/requests/${req.id}`} className="muted" style={{ fontSize: 12.5 }} onClick={onClose}>{t("请求详情")}</Link>
          </div>
          <details className="sp-cmds" open={req.state === "pending"}>
            <summary>{t("实际执行的命令（{n} 种）", { n: commands.length })}</summary>
            {commands.map(([cmd, hs]) => (
              <div key={cmd} className="sp-cmd">
                <div className="muted" style={{ fontSize: 12 }}>{t("在 {hosts} 上：", { hosts: hs.join(", ") })}</div>
                <pre className="code">{cmd}</pre>
              </div>
            ))}
          </details>
          {req.state === "pending" && <ApprovalCard r={req} title={t("核对上面的命令后批准")} lead={t("批准绑定到上面这份命令的摘要；批准后在这里看进度和结果。")} />}
          {req.state !== "pending" && (
            <div className="sp-progress">
              {req.targets.map((tg) => {
                const res = results?.find((x) => x.host === tg.host);
                return (
                  <div key={tg.host} className="sp-row">
                    <span className={"dot " + (tg.state === "ok" ? "ok" : tg.state === "failed" || tg.state === "error" ? "crit" : tg.state === "running" ? "warn pulse" : "")} />
                    <span className="mono" style={{ fontWeight: 600 }}>{tg.host}</span>
                    {res ? (res.error ? <span className="crit-text">{ts(res.error)}</span> : (
                      <span className="sp-nums num">
                        <span>{t("延迟")} <b>{res.pingMs.toFixed(0)}</b> ms</span>
                        <span>↓ <b>{mbps(res.downBps)}</b></span>
                        <span>↑ <b>{mbps(res.upBps)}</b></span>
                        <button className="link-btn" onClick={() => setReport(tg.host)}>{t("报告")}</button>
                      </span>
                    )) : (
                      <span className="muted">{tg.state === "running" ? t("正在测速…") : tg.state === "waiting" ? t("排队中") : tg.state === "skipped" ? t("跳过") : finished ? t("整理结果…") : ""}</span>
                    )}
                  </div>
                );
              })}
            </div>
          )}
          {["denied", "expired", "cancelled", "rejected"].includes(req.state) && <div className="note crit"><TriangleAlert /><span>{t("这次测速没有执行。")}</span></div>}
        </>
      )}
      {report && <SpeedReport requestId={reqId} host={report} onClose={() => setReport("")} />}
    </Modal>
  );
}

/** Speedtest by Ookla 的许可协议：所有者接受后才能用；也可以撤回。 */
function OoklaCard({ license, reload }: { license: OoklaLicense; reload: () => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const [busy, setBusy] = useState(false);
  const set = (accept: boolean) => {
    setBusy(true);
    api.speedtestOokla(accept).then(() => { toast(t("已保存")); reload(); }).catch(fail).finally(() => setBusy(false));
  };
  return (
    <div className="card">
      <div className="card-head"><h2>Speedtest by Ookla</h2><span className="grow" />
        {license.accepted ? <span className="badge ok"><Check />{t("已接受许可")}</span> : <span className="badge warn"><TriangleAlert />{t("未接受许可")}</span>}
      </div>
      <div className="card-body col" style={{ gap: 10 }}>
        <span className="hint">{license.accepted
          ? t("{by} 在 {at} 接受了 Ookla 的许可协议和隐私政策。测速结果会上传到 speedtest.net，只能用于个人、非商业用途。", { by: license.by ?? "—", at: license.at ? stamp(license.at) : "—" })
          : t("默认的测速方式。用之前需要所有者接受 Ookla 的许可协议和隐私政策：测速结果会上传到 speedtest.net，只能用于个人、非商业用途。")}</span>
        <span className="row wrap" style={{ gap: 12, fontSize: 13 }}>
          <a href={license.terms} target="_blank" rel="noreferrer noopener">{t("许可协议")}</a>
          <a href={license.privacy} target="_blank" rel="noreferrer noopener">{t("隐私政策")}</a>
          <span className="muted">{t("程序版本 {v}", { v: license.version })}</span>
        </span>
        {owner && (license.accepted
          ? <span><button className="btn ghost sm" disabled={busy} onClick={async () => (await askConfirm(t("撤回后就不能用 Speedtest 测速了，只能用 Cloudflare。撤回？"))) && set(false)}>{t("撤回接受")}</button></span>
          : <span><button className="btn primary sm" disabled={busy} onClick={() => set(true)}><Check />{t("我已阅读并接受")}</button></span>)}
      </div>
    </div>
  );
}

/** Ookla 的测速服务器：自动选最近的，或者搜索、填编号指定一台。 */
function ServerPicker({ picked, onPick, service }: { picked: Picked | null; onPick: (p: Picked | null) => void; service: string }) {
  const t = useT();
  const [mode, setMode] = useState<"auto" | "pick">(picked ? "pick" : "auto");
  const [q, setQ] = useState("");
  const [list, setList] = useState<SpeedServer[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [manual, setManual] = useState("");
  const search = (e: FormEvent) => {
    e.preventDefault();
    if (!q.trim()) return;
    setBusy(true); setErr("");
    api.speedtestServers(q.trim()).then((r) => setList(r.servers)).catch((x) => { setList(null); setErr(x instanceof Error ? x.message : String(x)); }).finally(() => setBusy(false));
  };
  const id = Number(manual);
  const manualOk = /^\d{1,7}$/.test(manual) && id > 0;
  return (
    <div className="field">
      <label>{t("测速服务器")}</label>
      <div className="seg" role="radiogroup" aria-label={t("测速服务器")}>
        <button role="radio" aria-checked={mode === "auto"} className={mode === "auto" ? "on" : ""} onClick={() => { setMode("auto"); onPick(null); }}>{t("自动选最近的")}</button>
        <button role="radio" aria-checked={mode === "pick"} className={mode === "pick" ? "on" : ""} onClick={() => setMode("pick")}>{t("指定服务器")}</button>
      </div>
      {mode === "auto" ? <span className="hint">{t("每台主机按自己的位置选最近的服务器。")}</span> : (
        <div className="col sp-pick">
          {picked
            ? <div className="sp-picked"><Check /><span>{t("已选：")}<b>{picked.label}</b></span><span className="mono muted">#{picked.id}</span></div>
            : <span className="hint">{t("还没选服务器；不选就自动选最近的。")}</span>}
          <form className="row sp-search" onSubmit={search}>
            <input className="input" value={q} maxLength={40} placeholder={t("按城市、运营商或国家搜索，比如 Tokyo、Hong Kong")} onChange={(e) => setQ(e.target.value)} />
            <button className="btn" type="submit" disabled={busy || !q.trim()}>{busy ? <RefreshCw className="spin" /> : <Search />}{t("搜索")}</button>
          </form>
          {err && <div className="note warn"><TriangleAlert /><span>{ts(err)}</span></div>}
          {list && (list.length === 0 ? <span className="muted">{t("没有搜到。")}</span> : (
            <div className="sp-servers" role="listbox" aria-label={t("测速服务器")}>
              {list.map((s) => {
                const label = `${s.sponsor} · ${s.name}`;
                return (
                  <button key={s.id} type="button" role="option" aria-selected={picked?.id === s.id} className={"sp-server" + (picked?.id === s.id ? " on" : "")}
                    onClick={() => onPick({ id: s.id, label })}>
                    <b>{s.sponsor}</b>
                    <span className="muted">{s.name}{s.country ? ", " + s.country : ""}</span>
                    <span className="grow" />
                    <span className="mono muted">#{s.id}</span>
                  </button>
                );
              })}
            </div>
          ))}
          <div className="row sp-search">
            <input className="input mono" inputMode="numeric" value={manual} placeholder={t("或者直接填服务器编号")} onChange={(e) => setManual(e.target.value.replace(/\D/g, "").slice(0, 7))} />
            <button className="btn" type="button" disabled={!manualOk} onClick={() => { onPick({ id, label: t("服务器 {id}", { id }) }); setManual(""); }}>{t("使用这个编号")}</button>
          </div>
          <span className="hint">{t("搜索时面板把搜索词发给 {service}。它的搜索不返回中国大陆的服务器；这类服务器知道编号的话可以直接填。", { service })}</span>
        </div>
      )}
    </div>
  );
}

export function SpeedtestPage() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const requests = useStore((s) => s.requests);
  const [data, setData] = useState<SpeedConfig | null>(() => cached<SpeedConfig>("speedtest") ?? null);
  const [picked, setPicked] = useState<string[]>([]);
  const [dialog, setDialog] = useState<string[] | null>(null);
  const [openReq, setOpenReq] = useState("");
  const [report, setReport] = useState<SpeedResult | null>(null);
  const load = useCallback(() => api.speedtest().then((r) => setData(remember("speedtest", r))).catch(fail), []);
  useEffect(() => { load(); }, [load]);

  // 有测速请求在等待或执行时，每 5 秒刷新一次结果。
  const active = requests.filter((r) => r.snippet === TAG && (r.state === "pending" || r.state === "running"));
  useEffect(() => {
    if (!active.length) return;
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [active.length, load]);
  // 一个请求刚结束时也刷新一次。
  const doneKey = requests.filter((r) => r.snippet === TAG && r.state !== "pending" && r.state !== "running").map((r) => r.id).join();
  useEffect(() => { load(); }, [doneKey, load]);

  const latest = useMemo(() => {
    const m = new Map<string, SpeedResult>();
    // “这台设备”测速量的是设备和面板（或主机）之间，不算这台主机自己的测速结果。
    for (const r of data?.results ?? []) if (r.method !== "client" && r.method !== "direct" && !m.has(r.host)) m.set(r.host, r);
    return m;
  }, [data]);
  const online = hosts.filter((h) => h.status.online).map((h) => h.name);
  const all = picked.length === online.length && online.length > 0;

  return (
    <>
      <div className="topbar">
        <h1>{t("测速")}</h1>
        <span className="muted hide-sm">{data ? (data.ookla?.accepted ? t("默认用 Speedtest by Ookla，也可以选 Cloudflare") : t("Speedtest by Ookla 需要所有者先接受许可协议；在那之前用 Cloudflare")) : ""}</span>
        <span className="grow" />
        <button className="btn" disabled={!data || picked.length === 0} onClick={() => setDialog(picked)}><Gauge />{t("测速所选（{n}）", { n: picked.length })}</button>
        <button className="btn primary" disabled={!data || online.length === 0} onClick={() => setDialog(online)}><Gauge />{t("全部在线主机")}</button>
      </div>
      <div className="content">
        <div className="page">
          {active.map((r) => (
            <div key={r.id} className={"note " + (r.state === "pending" ? "warn" : "")}>
              <Gauge />
              <span>
                {r.state === "pending" ? t("有一个测速请求（{n} 台）在等待批准。", { n: r.targets.length }) : t("正在测速（{n} 台），完成后结果会出现在下面。", { n: r.targets.length })}
                <button className="link-btn" onClick={() => setOpenReq(r.id)}>{r.state === "pending" ? t("去批准") : t("查看进度")}</button>
              </span>
            </div>
          ))}
          <div className="speed-layout">
          <div className="speed-main">
          <ClientCard onDone={load} />
          <div className="card">
            <div className="card-head"><h2>{t("各主机最近一次结果")}</h2></div>
            {hosts.length === 0 ? <Empty icon={<Gauge />} title={t("还没有主机")} /> : (
              <div className="table-wrap">
                <table className="table speed-table">
                  <thead>
                    <tr>
                      <th style={{ width: 44 }}>
                        <input type="checkbox" aria-label={t("全选在线主机")} checked={all} onChange={() => setPicked(all ? [] : online)} />
                      </th>
                      <th>{t("主机")}</th><th>{t("延迟")}</th><th>{t("下载")}</th><th>{t("上传")}</th><th className="hide-sm">{t("测试时间")}</th><th className="act">{t("操作")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {hosts.map((h) => {
                      const r = latest.get(h.name);
                      const on = h.status.online;
                      return (
                        <tr key={h.name}>
                          <td><input type="checkbox" aria-label={t("选择 {host}", { host: h.name })} disabled={!on} checked={picked.includes(h.name)}
                            onChange={(e) => setPicked(e.target.checked ? [...picked, h.name] : picked.filter((x) => x !== h.name))} /></td>
                          <td>
                            <div className="host-cell">
                              <span className={"dot " + (!h.status.known ? "" : on ? "ok" : "crit")} />
                              <Link to={`/hosts/${h.name}`} style={{ fontWeight: 600 }}>{h.name}</Link>
                            </div>
                          </td>
                          {!r ? <td colSpan={3} className="muted">{t("还没测过")}</td>
                            : r.error ? <td colSpan={3}><span className="crit-text">{ts(r.error)}</span></td>
                            : <>
                              <td className="num"><b>{r.pingMs ? r.pingMs.toFixed(0) : "—"}</b> <span className="muted">ms</span></td>
                              <td className="num"><b>{mbps(r.downBps)}</b></td>
                              <td className="num"><b>{mbps(r.upBps)}</b></td>
                            </>}
                          <td className="hide-sm muted" title={r ? stamp(r.ts) : ""}>{r ? ago(r.ts) : "—"}</td>
                          <td className="act">
                            {r && <button className="btn ghost icon sm" title={t("测速报告")} aria-label={t("测速报告")} onClick={() => setReport(r)}><FileText /></button>}
                            <button className="btn ghost sm" disabled={!on || !data} onClick={() => setDialog([h.name])}><Gauge />{t("测速")}</button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
          {data && <SpeedHistory results={data.results} reload={load} onReport={setReport} />}
          </div>
          {data && (
            <section className="speed-settings" aria-label={t("测速设置")}>
              <h2 className="speed-section">{t("测速设置")}</h2>
              <div className="speed-settings-grid">
                <OoklaCard license={data.ookla} reload={load} />
                <SpeedTargets targets={data.targets ?? []} max={data.maxTargets ?? 6} presets={data.presets ?? []} geo={data.geo ?? true} geoService={data.geoService ?? ""} reload={load} />
              </div>
            </section>
          )}
          </div>
        </div>
      </div>
      {dialog && data && <SpeedtestDialog hosts={dialog} config={data} onClose={() => { setDialog(null); load(); }} />}
      {openReq && data && <SpeedtestDialog hosts={[]} requestId={openReq} config={data} onClose={() => { setOpenReq(""); load(); }} />}
      {report && <SpeedReport requestId={report.requestId} host={report.host} onClose={() => setReport(null)} />}
    </>
  );
}

/** 测速历史：可以删单条、清空，所有者可以设定保留规则。删的只是测速结果的副本，请求和输出仍在审计里。 */
function SpeedHistory({ results, reload, onReport }: { results: SpeedResult[]; reload: () => void; onReport: (r: SpeedResult) => void }) {
  const t = useT();
  const owner = useStore((s) => s.boot?.user?.role === "owner");
  const paged = usePaged(results, 20);
  const [ret, setRet] = useState<{ retention: { days: number; perHost: number }; days: number[]; perHost: number[] } | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => { api.speedtestRetention().then(setRet).catch(() => setRet(null)); }, []);
  const save = (r: { days: number; perHost: number }) => {
    setBusy(true);
    api.speedtestRetention(r).then((x) => { setRet(x); toast(t("已保存")); reload(); }).catch(fail).finally(() => setBusy(false));
  };
  const remove = (items: { requestId: string; host: string }[], all = false) => {
    setBusy(true);
    api.speedtestDelete(items, all).then((x) => { toast(t("已删除 {n} 条", { n: x.deleted })); reload(); }).catch(fail).finally(() => setBusy(false));
  };
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("历史")}</h2><span className="muted num">{results.length || ""}</span>
        <span className="grow" />
        {owner && ret && (
          <div className="row wrap speed-keep" title={t("测速结果只是副本：删掉后，对应的请求和输出仍在审计里")}>
            <span className="muted">{t("保留")}</span>
            <Select value={String(ret.retention.days)} disabled={busy} onChange={(v) => save({ ...ret.retention, days: +v })}
              options={ret.days.map((d) => ({ value: String(d), label: d === 0 ? t("不按时间删") : t("{n} 天内", { n: d }) }))} />
            <Select value={String(ret.retention.perHost)} disabled={busy} onChange={(v) => save({ ...ret.retention, perHost: +v })}
              options={ret.perHost.map((n) => ({ value: String(n), label: t("每台最多 {n} 条", { n }) }))} />
          </div>
        )}
        {owner && results.length > 0 && (
          <button className="btn ghost sm danger" disabled={busy} onClick={async () => (await askConfirm(t("清空全部测速历史？对应的请求和输出仍在审计里。"))) && remove([], true)}><Trash2 />{t("清空")}</button>
        )}
      </div>
      {results.length === 0 ? <div className="empty sm"><span>{t("还没有测速记录")}</span></div> : (
        <>
          <div className="table-wrap">
            <table className="table speed-table speed-history">
              <thead><tr><th>{t("主机")}</th><th>{t("延迟")}</th><th>{t("下载")}</th><th>{t("上传")}</th><th className="hide-sm">{t("测速服务器")}</th><th className="hide-sm">{t("数据量")}</th><th>{t("时间")}</th><th className="act" aria-label={t("操作")} /></tr></thead>
              <tbody>
                {paged.rows.map((r) => (
                  <tr key={r.requestId + r.host} className="click" onClick={() => onReport(r)}>
                    <td className="mono">{r.host}</td>
                    {r.error ? <td colSpan={3}><span className="crit-text">{ts(r.error)}</span></td> : <>
                      <td className="num">{r.pingMs.toFixed(0)} <span className="muted">ms</span></td>
                      <td className="num">{mbps(r.downBps)}</td>
                      <td className="num">{mbps(r.upBps)}</td>
                    </>}
                    <td className="hide-sm muted sp-via" title={[r.server, optsLabel(t, r.threads, r.ip)].filter(Boolean).join(" · ")}>{r.method === "client" ? t("这台设备 ↔ 面板") : r.method === "direct" ? t("这台设备 ↔ {host}", { host: r.host }) : r.method === "ookla" ? r.server || "Speedtest" : "Cloudflare"}{optsLabel(t, r.threads, r.ip) && <small className="sp-opt"> · {optsLabel(t, r.threads, r.ip)}</small>}</td>
                    <td className="hide-sm muted num">{r.sizeMb ? r.sizeMb + " MB" : "—"}</td>
                    <td className="muted" title={stamp(r.ts)}>{ago(r.ts)}</td>
                    <td className="act" onClick={(e) => e.stopPropagation()}>
                      <button className="btn ghost icon sm" title={t("测速报告")} aria-label={t("测速报告")} onClick={() => onReport(r)}><FileText /></button>
                      {owner && <button className="btn ghost icon sm" disabled={busy} title={t("删除这条记录")} aria-label={t("删除这条记录")} onClick={() => remove([{ requestId: r.requestId, host: r.host }])}><Trash2 /></button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager p={paged} />
        </>
      )}
    </div>
  );
}
