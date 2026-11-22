// 主机的环境检测：出口 IP 的归属和类型、各家模型接口能不能用、常用网站和流媒体能不能打开和在哪个区、
// 装了哪些 AI 命令行工具和运行环境。全部是只读查询，点“检测”才查。状态都带文字，不只靠颜色。
import { useCallback, useEffect, useState } from "react";
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

const siteGroups: [string, string][] = [["video", "视频与流媒体"], ["social", "社交"], ["ai", "AI 网页版"], ["search", "搜索与百科"], ["dev", "开发"]];
function siteNote(t: (s: string) => string, s: EnvSite) {
  return s.note === "full" ? t("非自制剧也能看") : s.note === "originals" ? t("只能看自制剧") : "";
}

/** 检测一台主机并缓存结果（切换页面再回来不用重新查）。 */
function useEnv(host: string, auto: boolean) {
  const key = "env:" + host;
  const [env, setEnv] = useState<HostEnv | null>(() => cached<HostEnv>(key) ?? null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const check = useCallback(() => {
    setBusy(true);
    setErr("");
    return api.hostEnv(host).then((r) => setEnv(remember(key, r))).catch((e) => setErr(e instanceof Error ? e.message : String(e))).finally(() => setBusy(false));
  }, [host, key]);
  useEffect(() => { if (auto && !cached<HostEnv>(key)) check(); }, [auto, key, check]);
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
            <div className="card-head"><h2>{t("出口 IP")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("网站看到的是这个地址：它的类型和风险分影响会不会被限制、要不要人机验证")}</span></div>
            <div className="facts">
              <div className="fact"><dt>{t("地址")}</dt><dd className="mono">{e.ip || "—"}</dd></div>
              <div className="fact"><dt>{t("位置")}</dt><dd>{[e.city, e.region].filter(Boolean).filter((x, i, a) => a.indexOf(x) === i).join(" · ") || "—"}{e.country && <span className="mono muted">　{e.country}{e.colo ? ` · ${e.colo}` : ""}</span>}</dd></div>
              <div className="fact"><dt>{t("类型")}</dt><dd>{e.type
                ? <span className={"badge " + (e.type === "residential" || e.type === "wireless" ? "ok" : e.type === "vpn" ? "warn" : "neutral")}>{ipTypeText(t, e.type)}</span>
                : <span className="muted">{t("没有查到")}</span>}</dd></div>
              <div className="fact"><dt>{t("风险分")}</dt><dd>{e.risk >= 0
                ? <span className={"badge " + riskTone(e.risk)} title={t("信息库给的 0 到 100 的分数，越高越容易被网站风控")}>{e.risk} · {t(riskText(e.risk))}</span>
                : <span className="muted">{t("没有查到")}</span>}</dd></div>
              <div className="fact"><dt>{t("运营方")}</dt><dd>{e.provider || "—"}{e.asn && <span className="mono muted">　{e.asn}</span>}</dd></div>
              <div className="fact"><dt>{t("代理标记")}</dt><dd>{e.proxy
                ? <span className="badge warn"><TriangleAlert />{t("被标成代理或 VPN")}</span>
                : e.type ? <span className="badge ok"><CircleCheck />{t("没有被标记")}</span> : <span className="muted">{t("没有查到")}</span>}
                {e.warp && <span className="badge neutral" style={{ marginLeft: 6 }}>Cloudflare WARP</span>}</dd></div>
              <div className="fact"><dt>IPv6</dt><dd>{e.ipv6 ? <span className="mono">{e.ipv6}</span> : <span className="muted">{t("没有 IPv6 出口")}</span>}</dd></div>
            </div>
            {(e.type === "hosting" || e.proxy || e.risk > 33) && (
              <div className="card-body" style={{ paddingTop: 0 }}>
                <div className="note warn"><TriangleAlert /><span>
                  {[e.type === "hosting" && t("这是机房的地址：流媒体常常只给看自制剧或直接拒绝，部分网站会频繁要求人机验证，AI 服务的订阅登录也更容易被风控。"),
                    e.proxy && t("这个地址被标成了代理或 VPN 出口，很多网站会直接限制。"),
                    e.risk > 33 && t("风险分偏高，说明这个地址或它所在的网段有过滥用记录。")].filter(Boolean).join(" ")}
                </span></div>
              </div>
            )}
            <div className="card-body hint" style={{ paddingTop: 0 }}>{t("归属、类型和风险分来自公开的 IP 信息库（由这台主机自己去查），只供参考：不同的库结论可能不一样，网站各自的风控也不完全看这些。")}</div>
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
    for (const h of hosts) { const c = cached<HostEnv>("env:" + h.name); if (c) m[h.name] = c; }
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
  const checkAll = async () => {
    const queue = hosts.filter((h) => h.status.online).map((h) => h.name);
    await Promise.all([0, 1, 2].map(async () => { for (let n = queue.shift(); n; n = queue.shift()) await checkOne(n); }));
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
                <th>{t("主机")}</th><th>{t("出口")}</th><th>{t("IP 类型")}</th><th>{t("风险分")}</th>
                {services.map((s) => <th key={s.id} title={ts(s.title)}>{short(s.title)}</th>)}
                {sites.map((s) => <th key={s.id}>{short(s.title)}</th>)}
                {tools.map((x) => <th key={x.name}>{x.title}</th>)}
                <th className="act" />
              </tr></thead>
              <tbody>
                {hosts.map((h) => {
                  const e = data[h.name];
                  return (
                    <tr key={h.name}>
                      <td><Link to={`/hosts/${encodeURIComponent(h.name)}?tab=env`} style={{ fontWeight: 560 }}>{h.name}</Link>
                        {errs[h.name] && <div className="crit" style={{ fontSize: 12 }}>{ts(errs[h.name])}</div>}
                        {!h.status.online && <div className="muted" style={{ fontSize: 12 }}>{t("离线")}</div>}</td>
                      <td className="mono">{e?.egress.country ?? <span className="muted">—</span>}</td>
                      <td>{e?.egress.type ? ipTypeText(t, e.egress.type) : <span className="muted">—</span>}</td>
                      <td>{e && e.egress.risk >= 0 ? <span className={"badge " + riskTone(e.egress.risk)}>{e.egress.risk} · {t(riskText(e.egress.risk))}</span> : <span className="muted">—</span>}</td>
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
