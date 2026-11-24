// 主机的环境检测：出口 IP 的归属和类型、各家模型接口能不能用、常用网站和流媒体能不能打开和在哪个区、
// 装了哪些 AI 命令行工具和运行环境。全部是只读查询，点“检测”才查。状态都带文字，不只靠颜色。
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { CircleCheck, CircleHelp, CircleSlash, CircleX, Download, Radar, RefreshCw, TriangleAlert } from "lucide-react";
import { api, type EnvService, type EnvSite, type HostEnv } from "../api";
import { useStore } from "../store";
import { Empty, ago } from "../ui";
import { cached, remember } from "../cache";
import { ts, useT } from "../i18n";

type State = EnvService["state"];
const stateMeta: Record<State, { cls: string; label: string; icon: typeof CircleCheck }> = {
  ok: { cls: "ok", label: "可用@@能访问", icon: CircleCheck },
  blocked: { cls: "crit", label: "受限", icon: CircleSlash },
  unreachable: { cls: "warn", label: "连不上", icon: CircleX },
  unknown: { cls: "neutral", label: "不确定", icon: CircleHelp },
};

function StateBadge({ state, code, extra, compact }: { state: State; code: number; extra?: string; compact?: boolean }) {
  const t = useT();
  const m = stateMeta[state];
  const Icon = m.icon;
  const title = state === "ok" ? t("有正常应答（HTTP {code}）", { code })
    : state === "blocked" ? t("被拒绝，或者被带到“所在地区不提供服务”的页面（HTTP {code}）", { code })
    : state === "unreachable" ? t("8 秒内没有连上：可能被网络挡住，或者主机上没有 curl")
    : t("连上了，但应答不是预期的样子（HTTP {code}），没法判断", { code });
  return (
    <span className={"badge " + m.cls} title={title}>
      <Icon />{compact ? <span className="sr-only">{t(m.label)}</span> : t(m.label)}{extra ? <span className="mono">{compact ? extra : " · " + extra}</span> : null}
    </span>
  );
}

export function ServiceBadge({ s, compact }: { s: EnvService; compact?: boolean }) {
  return <StateBadge state={s.state} code={s.code} compact={compact} />;
}

const ipTypes: Record<string, string> = { hosting: "机房（数据中心）", residential: "家庭宽带", business: "企业专线", wireless: "移动网络", vpn: "VPN", education: "教育网", government: "政府机构" };
/** 出口 IP 的类型：信息库没给时返回空。 */
export function ipTypeText(t: (s: string) => string, type?: string) {
  return type ? (ipTypes[type] ? t(ipTypes[type]) : type) : "";
}
const riskTone = (r: number) => (r < 0 ? "neutral" : r <= 33 ? "ok" : r <= 66 ? "warn" : "crit");
const riskText = (r: number) => (r <= 33 ? "低" : r <= 66 ? "中" : "高");

const aiTitles: Record<string, string> = { chatgpt: "ChatGPT / OpenAI", claude: "Claude / Anthropic", google: "Gemini / Google" };
const aiUsed: Record<string, string> = { chatgpt: "Codex CLI", claude: "Claude Code", google: "Gemini CLI" };
const riskMeta: Record<string, { title: string; ok: string; warn: string; why: string }> = {
  "chatgpt-challenge": { title: "ChatGPT 网页的人机验证", ok: "没有要求", warn: "要求人机验证", why: "Cloudflare 对信誉差的地址先要求人机验证；脚本、命令行工具这时会直接失败，网页登录也要反复验证。" },
  "google-captcha": { title: "Google 的异常流量拦截", ok: "没有拦截", warn: "被当成异常流量", why: "Google 搜索要求做人机验证，说明这个地址被 Google 标成了异常流量，Gemini 等服务也可能受影响。" },
  "region-mismatch": { title: "地区是否一致", ok: "一致", warn: "不一致", why: "IP 信息库说的位置和 AI 服务当成的区不一样时（比如 IP 在新加坡、服务当成香港），常常会被当成不支持的地区。" },
};
const siteGroups: [string, string][] = [["video", "视频与流媒体"], ["social", "社交"], ["ai", "AI 网页版"], ["search", "搜索与百科"], ["dev", "开发"]];
function siteNote(t: (s: string) => string, s: EnvSite) {
  return s.note === "full" ? t("非自制剧也能看") : s.note === "originals" ? t("只能看自制剧") : "";
}

/** 上个版本缓存下来的结果格式不一样（没有 IPv4/IPv6 之分、没有风控信号）：不用它，重新检测。 */
function fresh(e: HostEnv | undefined): HostEnv | null {
  return e && e.risks && e.egress && !("ip" in e.egress) ? e : null;
}

/** 检测一台主机并缓存结果（切换页面再回来不用重新查）。 */
function useEnv(host: string, auto: boolean) {
  const key = "env:" + host;
  const [env, setEnv] = useState<HostEnv | null>(() => fresh(cached<HostEnv>(key)));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  // 切到另一台主机时：换成那台的缓存，晚到的旧主机结果不要串过来。
  const cur = useRef(host);
  useEffect(() => { cur.current = host; setEnv(fresh(cached<HostEnv>(key))); setErr(""); }, [host, key]);
  const check = useCallback(() => {
    setBusy(true);
    setErr("");
    return api.hostEnv(host).then((r) => { remember(key, r); if (cur.current === host) setEnv(r); })
      .catch((e) => { if (cur.current === host) setErr(e instanceof Error ? e.message : String(e)); })
      .finally(() => { if (cur.current === host) setBusy(false); });
  }, [host, key]);
  useEffect(() => { if (auto && !fresh(cached<HostEnv>(key))) check(); }, [auto, key, check]);
  return { env, busy, err, check };
}

/** 主机页的“环境”页签。 */
export function EnvPanel({ host }: { host: string }) {
  const t = useT();
  const online = useStore((s) => !!s.hosts.find((h) => h.name === host)?.status.online);
  const { env, busy, err, check } = useEnv(host, online);
  const ai = env?.tools.filter((x) => x.kind === "ai") ?? [];
  const rt = env?.tools.filter((x) => x.kind === "runtime") ?? [];
  const e = env?.egress;
  // 判断“机房地址、风险高”这些提示时看哪个出口：有 IPv4 看 IPv4，没有再看 IPv6。
  const p = e?.v4 ?? e?.v6;

  return (
    <>
      <div className="row wrap">
        <span className="muted" style={{ fontSize: 12.5 }}>{env ? t("检测于 {when}", { when: ago(env.checkedAt) }) : busy ? t("检测中…") : t("还没有检测")}</span>
        <span className="grow" />
        <button className="btn sm" disabled={busy || !online} onClick={check}><RefreshCw className={busy ? "spin" : ""} />{t("重新检测")}</button>
      </div>
      {err && <div className="note crit"><CircleX /><span>{t("检测失败：{error}", { error: ts(err) })}</span></div>}
      {!env || !e ? <div className="card"><Empty icon={<Radar />} title={busy ? t("检测中…") : online ? t("还没有检测") : t("主机离线，没法检测")} /></div> : (
        <>
          <div className="card">
            <div className="card-head"><h2>{t("出口 IP")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("网站看到的是这个地址：它的类型和风险分影响会不会被限制、要不要人机验证")}</span>
              {e.warp && <span className="badge neutral">Cloudflare WARP</span>}</div>
            <div className="table-wrap">
              <table className="table egress-table">
                <thead><tr><th /><th>IPv4</th><th>IPv6</th></tr></thead>
                <tbody>
                  <tr><th>{t("地址")}</th>{[e.v4, e.v6].map((x, i) => <td key={i} className="mono">{x?.ip ?? <span className="muted">{i ? t("没有 IPv6 出口") : t("没有 IPv4 出口")}</span>}</td>)}</tr>
                  <tr><th>{t("位置")}</th>{[e.v4, e.v6].map((x, i) => <td key={i}>{x ? <>{[x.city, x.region].filter(Boolean).filter((v, k, a) => a.indexOf(v) === k).join(" · ") || "—"}{x.country && <span className="mono muted">　{x.country}{x.colo ? ` · ${x.colo}` : ""}</span>}</> : "—"}</td>)}</tr>
                  <tr><th>{t("类型")}</th>{[e.v4, e.v6].map((x, i) => <td key={i}>{x?.type
                    ? <span className={"badge " + (x.type === "residential" || x.type === "wireless" ? "ok" : x.type === "vpn" ? "warn" : "neutral")}>{ipTypeText(t, x.type)}</span>
                    : x ? <span className="muted">{t("没有查到")}</span> : "—"}</td>)}</tr>
                  <tr><th>{t("风险分")}</th>{[e.v4, e.v6].map((x, i) => <td key={i}>{x && x.risk >= 0
                    ? <span className={"badge " + riskTone(x.risk)} title={t("信息库给的 0 到 100 的分数，越高越容易被网站风控")}>{x.risk} · {t(riskText(x.risk))}</span>
                    : x ? <span className="muted">{t("没有查到")}</span> : "—"}</td>)}</tr>
                  <tr><th>{t("运营方")}</th>{[e.v4, e.v6].map((x, i) => <td key={i}>{x?.provider || "—"}{x?.asn && <span className="mono muted">　{x.asn}</span>}</td>)}</tr>
                  <tr><th>{t("代理标记")}</th>{[e.v4, e.v6].map((x, i) => <td key={i}>{x?.proxy
                    ? <span className="badge warn"><TriangleAlert />{t("被标成代理或 VPN")}</span>
                    : x?.type ? <span className="badge ok"><CircleCheck />{t("没有被标记")}</span> : x ? <span className="muted">{t("没有查到")}</span> : "—"}</td>)}</tr>
                </tbody>
              </table>
            </div>
            {p && (p.type === "hosting" || p.proxy || p.risk > 33) && (
              <div className="card-body" style={{ paddingTop: 12, paddingBottom: 0 }}>
                <div className="note warn"><TriangleAlert /><span>
                  {[p.type === "hosting" && t("这是机房的地址：流媒体常常只给看自制剧或直接拒绝，部分网站会频繁要求人机验证，AI 服务的订阅登录也更容易被风控。"),
                    p.proxy && t("这个地址被标成了代理或 VPN 出口，很多网站会直接限制。"),
                    p.risk > 33 && t("风险分偏高，说明这个地址或它所在的网段有过滥用记录。")].filter(Boolean).join(" ")}
                </span></div>
              </div>
            )}
            <div className="card-body hint" style={{ paddingTop: 10 }}>{t("IPv4 和 IPv6 分开查：有 IPv6 的主机，很多网站会优先走 IPv6，两个出口的归属和信誉可能不一样。归属、类型和风险分来自公开的 IP 信息库（由这台主机自己去查），只供参考。")}</div>
          </div>

          <div className="card">
            <div className="card-head"><h2>{t("AI 服务的地区与风控")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("各家 AI 服务把这台主机当成哪个区、有没有对它做额外的风控")}</span></div>
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("服务")}</th><th>{t("当成的区")}</th><th>{t("这个区能不能用")}</th><th className="hide-sm">{t("影响")}</th></tr></thead>
                <tbody>
                  {(env.ai ?? []).map((a) => (
                    <tr key={a.id}>
                      <td style={{ fontWeight: 560 }}>{aiTitles[a.id] ?? a.id}</td>
                      <td className="mono">{a.region || <span className="muted">{t("没有查到")}</span>}
                        {a.region && p?.country && a.region !== p.country && <span className="badge warn" style={{ marginLeft: 6 }} title={t("和 IP 信息库的位置（{loc}）不一致", { loc: p.country })}>{t("与 IP 位置不一致")}</span>}</td>
                      <td>{!a.region ? <span className="muted">—</span> : a.restricted
                        ? <span className="badge crit"><CircleSlash />{t("不在服务范围")}</span>
                        : <span className="badge ok"><CircleCheck />{t("在服务范围")}</span>}</td>
                      <td className="hide-sm muted">{aiUsed[a.id] ?? ""}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="risk-list">
              {(env.risks ?? []).map((r) => {
                const m = riskMeta[r.id];
                if (!m) return null;
                return (
                  <div key={r.id} className={"risk-item " + r.state}>
                    {r.state === "warn" ? <TriangleAlert /> : r.state === "ok" ? <CircleCheck /> : <CircleHelp />}
                    <div>
                      <b>{t(m.title)}</b>
                      <span className={"badge " + (r.state === "warn" ? "warn" : r.state === "ok" ? "ok" : "neutral")}>{r.state === "warn" ? t(m.warn) : r.state === "ok" ? t(m.ok) : t("没有查出来")}</span>
                      <div className="muted">{t(m.why)}</div>
                    </div>
                  </div>
                );
              })}
            </div>
            <div className="card-body hint" style={{ paddingTop: 0 }}>{t("“服务范围”按内置的常见不支持地区名单判断（{list}），以各家官方公布的为准。", { list: (env.restricted ?? []).join("、") })}</div>
          </div>

          <div className="card">
            <div className="card-head"><h2>{t("大模型接口")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("从这台主机不带密钥请求各家接口，看它是“缺少密钥”还是按地区拒绝")}</span></div>
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("服务")}</th><th>{t("状态")}</th><th>{t("应答")}</th><th className="hide-sm">{t("用它的工具")}</th></tr></thead>
                <tbody>
                  {env.services.map((s) => (
                    <tr key={s.id}>
                      <td style={{ fontWeight: 560 }}>{ts(s.title)}</td>
                      <td><ServiceBadge s={s} /></td>
                      <td className="mono muted">{s.code ? `HTTP ${s.code}` : "—"}</td>
                      <td className="hide-sm muted">{s.used ?? "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="card-body hint" style={{ paddingTop: 10 }}>{t("这里查的是接口（API）。各家的网页版和订阅登录另有自己的地区和风控规则，接口可用不代表订阅登录一定能用。")}</div>
          </div>

          <div className="card">
            <div className="card-head"><h2>{t("网站和流媒体")}</h2><span className="muted num">{env.sites.filter((s) => s.state === "ok").length} / {env.sites.length}</span>
              <span className="muted" style={{ fontSize: 12.5 }}>{t("从这台主机访问各个网站，看能不能打开、落在哪个区")}</span></div>
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>{t("网站")}</th><th>{t("状态")}</th><th>{t("所在的区")}</th><th>{t("说明")}</th><th className="hide-sm">{t("应答")}</th></tr></thead>
                <tbody>
                  {siteGroups.flatMap(([g, label]) => {
                    const list = env.sites.filter((s) => s.group === g);
                    return list.length === 0 ? [] : [
                      <tr key={g} className="env-group"><td colSpan={5}>{t(label)}</td></tr>,
                      ...list.map((s) => (
                        <tr key={s.id}>
                          <td style={{ fontWeight: 560 }}>{ts(s.title)}</td>
                          <td><StateBadge state={s.state} code={s.code} /></td>
                          <td className="mono">{s.region || <span className="muted">—</span>}</td>
                          <td>{siteNote(t, s) || <span className="muted">—</span>}</td>
                          <td className="mono muted hide-sm">{s.code ? `HTTP ${s.code}` : "—"}</td>
                        </tr>
                      )),
                    ];
                  })}
                </tbody>
              </table>
            </div>
            <div className="card-body hint" style={{ paddingTop: 10 }}>{t("这里只看网站怎么应答，没有登录账号：能打开不等于账号一定能用。Netflix 用一部非自制剧来判断这个区的片库能不能看。")}</div>
          </div>

          <div className="card">
            <div className="card-head"><h2>{t("AI 命令行工具")}</h2><span className="muted num">{ai.filter((x) => x.installed).length} / {ai.length}</span></div>
            <ToolTable host={host} tools={ai} ai />
          </div>
          <div className="card">
            <div className="card-head"><h2>{t("运行环境")}</h2><span className="muted num">{rt.filter((x) => x.installed).length} / {rt.length}</span></div>
            <ToolTable host={host} tools={rt} />
          </div>
        </>
      )}
    </>
  );
}

function ToolTable({ host, tools, ai }: { host: string; tools: HostEnv["tools"]; ai?: boolean }) {
  const t = useT();
  return (
    <div className="table-wrap">
      <table className="table">
        <thead><tr><th>{t("名称")}</th><th>{t("状态")}</th>{ai && <th>{t("版本")}</th>}<th className="hide-sm">{t("位置")}</th>{ai && <th className="act">{t("操作")}</th>}</tr></thead>
        <tbody>
          {tools.map((x) => (
            <tr key={x.name}>
              <td><span style={{ fontWeight: 560 }}>{x.title}</span> <span className="mono muted" style={{ fontSize: 12 }}>{x.name}</span></td>
              <td>
                {x.installed ? <span className="badge ok"><CircleCheck />{t("已安装")}</span> : <span className="badge neutral">{t("没有安装")}</span>}
                {ai && x.installed && x.app && (x.signed
                  ? <span className="badge ok" style={{ marginLeft: 6 }} title={t("找到了登录凭据或面板写的配置（只看文件在不在，不读内容）")}>{t("已登录或已配置")}</span>
                  : <span className="badge warn" style={{ marginLeft: 6 }} title={t("没有找到登录凭据或配置：在应用页点“配置”或“订阅登录”")}>{t("还没配置")}</span>)}
              </td>
              {ai && <td className="mono">{x.version || <span className="muted">—</span>}</td>}
              <td className="mono muted hide-sm">{x.path || "—"}</td>
              {ai && <td className="act">{!x.installed && x.app
                ? <Link className="btn sm" to={`/apps?install=${encodeURIComponent(x.app)}&host=${encodeURIComponent(host)}`}><Download />{t("安装")}</Link>
                : x.installed && x.app ? <Link className="btn ghost sm" to="/apps?tab=installed">{t("去配置")}</Link> : null}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// 总表里只放最常问的几个网站，其余的在主机页看。
const matrixSites = ["google", "youtube", "netflix", "disney", "tiktok", "chatgpt"];

/** 应用页的“环境”页签：所有主机一张表，一眼看出哪台的出口是什么类型、能用哪家、装了什么。 */
export function EnvMatrix() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const [data, setData] = useState<Record<string, HostEnv>>(() => {
    const m: Record<string, HostEnv> = {};
    for (const h of hosts) { const c = fresh(cached<HostEnv>("env:" + h.name)); if (c) m[h.name] = c; }
    return m;
  });
  const [errs, setErrs] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string[]>([]);

  const checkOne = useCallback(async (name: string) => {
    setBusy((b) => [...b, name]);
    try {
      const r = await api.hostEnv(name);
      remember("env:" + name, r);
      setData((d) => ({ ...d, [name]: r }));
      setErrs((e) => { const n = { ...e }; delete n[name]; return n; });
    } catch (e) {
      setErrs((x) => ({ ...x, [name]: e instanceof Error ? e.message : String(e) }));
    } finally { setBusy((b) => b.filter((x) => x !== name)); }
  }, []);
  // 一次查三台：每台主机要并行发二十多个请求，全部一起查会挤占连接。
  // 离开这一页后，还没轮到的主机就不再检测（每台主机要发二十多个请求）。
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const checkAll = async () => {
    const queue = hosts.filter((h) => h.status.online).map((h) => h.name);
    await Promise.all([0, 1, 2].map(async () => { for (let n = queue.shift(); n && alive.current; n = queue.shift()) await checkOne(n); }));
  };

  const sample = Object.values(data)[0];
  const services = sample?.services ?? [];
  const sites = matrixSites.map((id) => sample?.sites.find((s) => s.id === id)).filter((s): s is EnvSite => !!s);
  const tools = (sample?.tools ?? []).filter((x) => x.kind === "ai" && x.app);
  const short = (title: string) => ts(title).replace(/[（(].*[）)]/, "").trim();
  return (
    <div className="card">
      <div className="card-head wrap">
        <h2>{t("各主机的环境")}</h2>
        <span className="muted" style={{ fontSize: 12.5 }}>{t("出口 IP 的类型和风险、能用哪家模型接口、常用网站在哪个区、装了哪些 AI 命令行工具")}</span>
        <span className="grow" />
        <button className="btn sm" disabled={busy.length > 0 || hosts.length === 0} onClick={checkAll}><Radar className={busy.length ? "spin" : ""} />{busy.length ? t("检测中…") : t("检测全部在线主机")}</button>
      </div>
      {hosts.length === 0 ? <Empty icon={<Radar />} title={t("还没有主机")} />
        : !sample ? <Empty icon={<Radar />} title={busy.length ? t("检测中…") : t("还没有检测")}>{t("点“检测全部在线主机”：每台主机上只做只读查询。")}</Empty>
        : (
          <div className="table-wrap">
            <table className="table env-matrix">
              <thead><tr>
                <th>{t("主机")}</th><th>{t("出口")}</th><th>{t("IP 类型")}</th><th>{t("风险分")}</th><th>{t("风控")}</th>
                {services.map((s) => <th key={s.id} title={ts(s.title)}>{short(s.title)}</th>)}
                {sites.map((s) => <th key={s.id}>{short(s.title)}</th>)}
                {tools.map((x) => <th key={x.name}>{x.title}</th>)}
                <th className="act" />
              </tr></thead>
              <tbody>
                {hosts.map((h) => {
                  const e = data[h.name];
                  // 旧版本缓存下来的结果没有 v4/v6：当成没查过。
                  const ep = e?.egress?.v4 ?? e?.egress?.v6;
                  const warns = e?.risks?.filter((r) => r.state === "warn").length ?? 0;
                  return (
                    <tr key={h.name}>
                      <td><Link to={`/hosts/${encodeURIComponent(h.name)}?tab=env`} style={{ fontWeight: 560 }}>{h.name}</Link>
                        {errs[h.name] && <div className="crit" style={{ fontSize: 12 }}>{ts(errs[h.name])}</div>}
                        {!h.status.online && <div className="muted" style={{ fontSize: 12 }}>{t("离线")}</div>}</td>
                      {/* 两种出口都标出来：国家、IP 类型和风险分取 IPv4（没有 IPv4 时取 IPv6）；只标一个“v6”会被看成只有 IPv6。 */}
                      <td className="mono">{ep?.country ?? <span className="muted">—</span>}
                        {(["v4", "v6"] as const).map((f) => { const x = e?.egress?.[f]; return x && <span key={f} className="badge neutral" style={{ marginLeft: 6 }} title={x.ip + (x.country ? " · " + x.country : "")}>{f}</span>; })}</td>
                      <td>{ep?.type ? ipTypeText(t, ep.type) : <span className="muted">—</span>}</td>
                      <td>{ep && ep.risk >= 0 ? <span className={"badge " + riskTone(ep.risk)}>{ep.risk} · {t(riskText(ep.risk))}</span> : <span className="muted">—</span>}</td>
                      <td>{warns > 0 ? <span className="badge warn" title={e!.risks!.filter((r) => r.state === "warn").map((r) => t(riskMeta[r.id]?.title ?? r.id)).join("\n")}><TriangleAlert />{warns}</span> : e?.risks ? <span className="badge ok"><CircleCheck /><span className="sr-only">{t("没有风控信号")}</span></span> : <span className="muted">—</span>}</td>
                      {services.map((s) => { const x = e?.services.find((y) => y.id === s.id); return <td key={s.id}>{x ? <ServiceBadge s={x} compact /> : <span className="muted">—</span>}</td>; })}
                      {sites.map((s) => {
                        const x = e?.sites.find((y) => y.id === s.id);
                        return <td key={s.id}>{x ? <span title={siteNote(t, x) || undefined}><StateBadge state={x.state} code={x.code} compact extra={x.region ? x.region + (x.note === "originals" ? "*" : "") : undefined} /></span> : <span className="muted">—</span>}</td>;
                      })}
                      {tools.map((x) => {
                        const y = e?.tools.find((z) => z.name === x.name);
                        return <td key={x.name}>{!y ? <span className="muted">—</span> : y.installed
                          ? <span className={"badge " + (y.signed ? "ok" : "warn")} title={y.signed ? t("已安装，已登录或已配置") : t("已安装，还没配置")}><CircleCheck />{y.version || t("已安装")}</span>
                          : <span className="muted">{t("没有")}</span>}</td>;
                      })}
                      <td className="act"><button className="btn ghost icon sm" disabled={busy.includes(h.name) || !h.status.online} title={t("重新检测")} aria-label={t("重新检测")} onClick={() => checkOne(h.name)}><RefreshCw className={busy.includes(h.name) ? "spin" : ""} /></button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      {sample && <div className="card-body hint" style={{ paddingTop: 10 }}>{t("图标：对勾是可用，斜杠是受限，叉是连不上，问号是不确定（鼠标停上去有说明）。Netflix 的区后面带 * 表示这个区只能看自制剧。更多网站和出口 IP 的详情点主机名查看。")}</div>}
    </div>
  );
}
