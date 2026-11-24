import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Cpu, Info, Link2, Plus, Search, Server, ShieldAlert, ShieldCheck, SquareTerminal, TriangleAlert } from "lucide-react";
import { api, type Host, type SelfStatus, type VerifyResult } from "../api";
import { useStore } from "../store";
import { SortTh, useSort, Empty, Sparkline, Usage, bytes, pctText, lastOnly, poll, rate, uptime } from "../ui";
import { CountUp } from "../fx";
import { cached, remember } from "../cache";
import { HostForm } from "../HostForm";
import { Tip } from "../controls";
import { QuotaMini, TrafficRanking } from "./Traffic";
import { ts, useT } from "../i18n";

const tone = (v?: number) => (v === undefined ? "" : v >= 90 ? "crit" : v >= 75 ? "warn" : "");

export function Overview() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const groups = useStore((s) => s.groups);
  const loaded = useStore((s) => s.hostsLoaded);
  const requests = useStore((s) => s.requests);
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState("");
  const [onlyIssues, setOnlyIssues] = useState(false);
  const [verify, setVerify] = useState<VerifyResult | null>(() => cached<VerifyResult>("audit.verify") ?? null);
  const [adding, setAdding] = useState(false);
  const nav = useNavigate();

  useEffect(() => { api.auditVerify().then((v) => setVerify(remember("audit.verify", v))).catch(() => {}); }, []);

  const hasIssue = (h: Host) => {
    const l = h.status.latest;
    return (h.status.known && !h.status.online) || (!!l && (l.cpu >= 90 || l.mem >= 90 || l.disk >= 90));
  };

  const shown = useMemo(() => {
    const members = group ? new Set(groups.find((g) => g.name === group)?.members) : null;
    const q = query.trim().toLowerCase();
    return hosts.filter((h) =>
      (!members || members.has(h.name)) &&
      (!onlyIssues || hasIssue(h)) &&
      (!q || h.name.toLowerCase().includes(q) || h.addr.includes(q) || h.tags.some((tag) => tag.includes(q)) || (h.status.os ?? "").toLowerCase().includes(q)));
  }, [hosts, groups, group, query, onlyIssues]);

  const sorter = useSort(shown, {
    name: (h) => h.name.toLowerCase(),
    cpu: (h) => (h.status.online ? h.status.latest?.cpu : undefined),
    mem: (h) => (h.status.online ? h.status.latest?.mem : undefined),
    disk: (h) => (h.status.online ? h.status.latest?.disk : undefined),
  });

  const online = hosts.filter((h) => h.status.online).length;
  const offline = hosts.filter((h) => h.status.known && !h.status.online).length;
  const issues = hosts.filter(hasIssue).length;
  const pending = requests.filter((r) => r.state === "pending").length;
  const dayAgo = Date.now() - 86400_000;
  const today = requests.filter((r) => Date.parse(r.createdAt) > dayAgo);
  const todayFailed = today.filter((r) => r.state === "failed").length;
  const todayDone = today.filter((r) => r.state === "done").length;

  return (
    <>
      <div className="topbar">
        <h1>{t("总览")}</h1>
        <span className="grow" />
        <button className="btn primary" onClick={() => setAdding(true)}><Plus />{t("添加主机")}</button>
      </div>
      {adding && <HostForm onClose={() => setAdding(false)} />}
      <div className="content">
        <div className="page">
          <div className="stats">
            <div className="card stat">
              <div className="stat-icon"><Server /></div>
              <div className="stat-label">{t("主机在线")}</div>
              <div className="stat-line">
                <div className="stat-value"><CountUp value={online} /><small>/ {hosts.length}</small></div>
                <div className="stat-foot">
                  {offline > 0 ? <><span className="dot crit" />{t("{n} 台离线", { n: offline })}</> : issues > 0 ? <><span className="dot warn" />{t("{n} 台告警", { n: issues })}</> : <><span className="dot ok" />{t("全部正常")}</>}
                </div>
              </div>
            </div>
            <Link to="/approvals" className="card stat">
              <div className="stat-icon"><ShieldCheck /></div>
              <div className="stat-label">{t("待审批")}</div>
              <div className="stat-line">
                <div className="stat-value"><CountUp value={pending} /></div>
                <div className="stat-foot">{pending > 0 ? <><span className="dot warn" />{t("等你批准")}</> : <><span className="dot ok" />{t("无需处理")}</>}</div>
              </div>
            </Link>
            <Link to="/activity" className="card stat">
              <div className="stat-icon"><SquareTerminal /></div>
              <div className="stat-label">{t("近 24 小时操作")}</div>
              <div className="stat-line">
                <div className="stat-value"><CountUp value={today.length} /><small>{t("次")}</small></div>
                {/* 数字是总数，旁边只说结果：有失败报失败，全部成功说全部成功，还有没做完的就说成功了几次；一次都没有就不写。 */}
                <div className="stat-foot">
                  {todayFailed > 0 ? <><span className="dot crit" />{t("{n} 次失败", { n: todayFailed })}</>
                    : today.length === 0 ? null
                    : todayDone === today.length ? <><span className="dot ok" />{t("全部成功")}</>
                    : todayDone > 0 ? <><span className="dot ok" />{t("{n} 次成功", { n: todayDone })}</>
                    : t("尚未完成")}
                </div>
              </div>
            </Link>
            <Link to="/audit" className="card stat">
              <div className="stat-icon"><Link2 /></div>
              <div className="stat-label">{t("审计哈希链")}</div>
              <div className="stat-line">
                <div className="stat-value">{verify ? <CountUp value={verify.count} /> : "—"}<small>{t("条")}</small></div>
                <div className="stat-foot">
                  {!verify ? t("校验中…") : verify.ok ? <><span className="dot ok" />{t("链完整")}</> : <><span className="dot crit" />{t("第 {n} 条断开", { n: verify.brokenAt ?? "" })}</>}
                </div>
              </div>
            </Link>
          </div>

          <SelfCard />

          <div className="card">
            <div className="card-head wrap">
              <h2>{t("主机@@列表")}</h2>
              <span className="muted num">{shown.length === hosts.length ? hosts.length : `${shown.length} / ${hosts.length}`}</span>
              <span className="grow" />
              <div className="row wrap" role="group" aria-label={t("按分组筛选")}>
                <button className={"tag" + (group === "" ? " on" : "")} onClick={() => setGroup("")}>{t("全部")}</button>
                {groups.filter((g) => g.name !== "all").map((g) => (
                  <button key={g.name} className={"tag" + (group === g.name ? " on" : "")} onClick={() => setGroup(group === g.name ? "" : g.name)} title={g.selector ?? g.hosts?.join(", ")}>
                    {g.name} <span className="muted" style={{ marginLeft: 4 }}>{g.members.length}</span>
                  </button>
                ))}
              </div>
              <label className="check"><input type="checkbox" checked={onlyIssues} onChange={(e) => setOnlyIssues(e.target.checked)} />{t("只看异常")}</label>
              <div className="search" style={{ width: 220, maxWidth: "100%" }}>
                <Search />
                <input className="input" placeholder={t("搜索名称、地址、标签")} value={query} onChange={(e) => setQuery(e.target.value)} aria-label={t("搜索主机")} />
              </div>
            </div>
            {!loaded ? (
              <div className="empty">{t("加载中…")}</div>
            ) : hosts.length === 0 ? (
              <Empty icon={<Server />} title={t("还没有主机")}>
                {t("点右上角的“添加主机”，或在配置目录的")} <span className="mono">inventory.yaml</span> {t("里声明。")}
              </Empty>
            ) : shown.length === 0 ? (
              <Empty icon={<Search />} title={t("没有匹配的主机")} />
            ) : (
              <div className="table-wrap">
                <table className="table host-table">
                  <thead>
                    <tr>
                      <SortTh s={sorter} k="name">{t("主机")}</SortTh>
                      <th className="hide-sm">{t("系统 · 运行时间")}</th>
                      <th className="hide-sm">{t("标签")}</th>
                      <SortTh s={sorter} k="cpu" desc>CPU</SortTh>
                      <SortTh s={sorter} k="mem" desc>{t("内存")}</SortTh>
                      <SortTh s={sorter} k="disk" desc className="hide-sm">{t("磁盘")}</SortTh>
                      <th className="hide-sm">{t("网络")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sorter.sorted.map((h) => {
                      const st = h.status, l = st.latest;
                      const state = !st.known ? t("尚未采样") : st.online ? t("在线") : t("离线");
                      return (
                        <tr key={h.name} className="click" onClick={() => nav(`/hosts/${h.name}`)}>
                          <td>
                            <div className="host-cell">
                              <span className={"dot " + (!st.known ? "" : st.online ? "ok" : "crit")} title={state} aria-label={state} />
                              <div className="cell-2">
                                <span className="row" style={{ gap: 7 }}>
                                <Link to={`/hosts/${h.name}`} style={{ fontWeight: 600 }} onClick={(e) => e.stopPropagation()}>{h.name}</Link>
                                {h.sensitivity === "high" && <span className="badge serious" title={t("高敏感：写操作自动升一级")}><ShieldAlert />{t("高敏感")}</span>}
                              </span>
                              <small className="mono">{h.addr}</small>
                            </div>
                            </div>
                          </td>
                          {st.known && !st.online ? (
                            <td colSpan={6}>
                              <span className="row" style={{ color: "var(--crit)" }}><TriangleAlert size={14} />{t("离线：")}<span className="dim ellipsis">{ts(st.error)}</span></span>
                            </td>
                          ) : (
                            <>
                              <td className="hide-sm">
                                <div className="cell-2">
                                  <span className="ellipsis" title={st.os ?? st.kernel}>{st.os ?? st.kernel ?? "—"}</span>
                                  <small className="ellipsis" title={st.kernel}>{t("已运行 {time}", { time: uptime(st.uptimeSec) })}</small>
                                </div>
                              </td>
                              <td className="hide-sm">
                                <div className="row" style={{ gap: 4 }}>
                                  {h.tags.slice(0, 3).map((tag) => <span key={tag} className="tag">{tag}</span>)}
                                  {h.tags.length > 3 && <span className="muted" title={h.tags.slice(3).join(" ")}>+{h.tags.length - 3}</span>}
                                </div>
                              </td>
                              <td>
                                <Tip content={<><div className="tip-title">CPU</div><div className="tip-grid"><span>{t("利用率")}</span><span>{l ? l.cpu.toFixed(1) + "%" : "—"}</span><span>{t("核数")}</span><span>{st.ncpu ?? "—"}</span><span>{t("1 分钟负载")}</span><span>{l ? l.load.toFixed(2) : "—"}</span><span>{t("进程数")}</span><span>{st.procs ?? "—"}</span></div></>}>
                                  <div className="cpu-cell">
                                    <Sparkline data={(st.history ?? []).map((s) => s.cpu)} width={84} height={24} />
                                    <b className={tone(l?.cpu)}>{l ? pctText(l.cpu) : "—"}</b>
                                  </div>
                                </Tip>
                              </td>
                              <td>
                                <Tip content={<><div className="tip-title">{t("内存")}</div><div className="tip-grid"><span>{t("已用")}</span><span>{l && st.memKB ? bytes(st.memKB * 1024 * l.mem / 100) : "—"}</span><span>{t("总量")}</span><span>{st.memKB ? bytes(st.memKB * 1024) : "—"}</span><span>{t("交换分区")}</span><span>{st.swapKB ? `${pctText(l?.swap ?? 0)} / ${bytes(st.swapKB * 1024)}` : t("未启用")}</span></div></>}>
                                  <Usage value={l?.mem} totalKB={st.memKB} />
                                </Tip>
                              </td>
                              <td className="hide-sm">
                                <Tip content={<><div className="tip-title">{t("根分区")}</div><div className="tip-grid"><span>{t("已用")}</span><span>{l && st.diskKB ? bytes(st.diskKB * 1024 * l.disk / 100) : "—"}</span><span>{t("可用")}</span><span>{l && st.diskKB ? bytes(st.diskKB * 1024 * (100 - l.disk) / 100) : "—"}</span><span>{t("总量")}</span><span>{st.diskKB ? bytes(st.diskKB * 1024) : "—"}</span></div></>}>
                                  <Usage value={l?.disk} totalKB={st.diskKB} />
                                </Tip>
                              </td>
                              <td className="hide-sm">
                                <div className="cell-2 num" style={{ fontSize: 12.5, whiteSpace: "nowrap" }}>
                                  <span><span className="muted">↓ </span>{rate(l?.rx)}</span>
                                  <span><span className="muted">↑ </span>{rate(l?.tx)}</span>
                                </div>
                                {st.quota && <QuotaMini q={st.quota} />}
                              </td>
                            </>
                          )}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
          <TrafficRanking />
        </div>
      </div>
    </>
  );
}

/** Hostower 自己所在这台机器的监测：整机的 CPU、内存、磁盘、网络，以及 Hostower 进程的占用。 */
function SelfCard() {
  const t = useT();
  // 宿主机版镜像里有一台内置的主机（local），它看到的才是整台机器。
  const builtin = useStore((st) => st.hosts.find((h) => h.source === "builtin"));
  const [self, setSelf] = useState<SelfStatus | null>(() => cached<SelfStatus>("self") ?? null);
  useEffect(() => {
    let on = true;
    const next = lastOnly();
    const load = () => { const fresh = next(); api.self().then((r) => on && fresh() && setSelf(remember("self", r))).catch(() => {}); };
    load();
    const stop = poll(load, 5000);
    return () => { on = false; stop(); };
  }, []);
  if (!self) return null;
  const l = self.latest;
  const diskPct = self.diskTotal ? (100 * self.diskUsed) / self.diskTotal : undefined;
  const cpuTone = !l ? "" : l.cpu >= 90 ? "crit" : l.cpu >= 75 ? "warn" : "";
  return (
    <div className="card">
      <div className="card-head wrap">
        <Cpu size={16} className="muted" />
        <h2>{t("本机")}</h2>
        <span className="muted hide-sm">{self.container ? t("Hostower 所在的容器") : t("Hostower 所在的这台机器")}</span>
        <span className="mono dim">{self.hostname}</span>
        <span className="muted hide-sm">{self.os} · {self.arch} · {t("{n} 核", { n: self.ncpu })}</span>
        <span className="grow" />
        <span className="muted" style={{ fontSize: 12.5 }} title={self.goVersion}>
          {t("进程内存 {mem} · {n} 个协程 · 数据库 {db}", { mem: bytes(self.procBytes), n: self.goroutines, db: bytes(self.dbBytes) })}
        </span>
      </div>
      {!self.system && (
        <div className="card-body" style={{ paddingBottom: 0 }}>
          <div className="note"><TriangleAlert /><span>{t("这台机器的系统不提供整机的 CPU、内存和网络数据（目前只支持 Linux）。下面只有磁盘和 Hostower 进程自身的数据。")}</span></div>
        </div>
      )}
      {self.container && self.system && (
        <div className="card-body" style={{ paddingBottom: 0 }}>
          <div className="note">
            <Info />
            <span>
              {t("Hostower 在容器里运行，这里是从容器里看到的：CPU、内存是整台机器的；网络只算这个容器自己的流量，磁盘是数据卷所在的磁盘，系统名是容器镜像的。")}
              {builtin && <>{t("整台机器的完整数据看主机 ")}<Link to={`/hosts/${builtin.name}`}>{builtin.name}</Link>{t("。")}</>}
            </span>
          </div>
        </div>
      )}
      <div className="facts self-facts">
        <div className="fact">
          <dt>CPU</dt>
          <dd>{self.system ? (
            <div className="cpu-cell">
              <Sparkline data={self.history.map((x) => x.cpu)} width={96} height={24} />
              <b className={cpuTone}>{l ? pctText(l.cpu) : "—"}</b>
              <span className="muted" style={{ fontSize: 12.5 }}>{t("负载 {load}", { load: l ? l.load.toFixed(2) : "—" })}</span>
            </div>
          ) : <span className="muted">—</span>}</dd>
        </div>
        <div className="fact">
          <dt>{t("内存")}</dt>
          <dd>{self.system ? <Usage value={l?.mem} totalKB={self.memBytes / 1024} /> : <span className="muted">—</span>}</dd>
        </div>
        <div className="fact">
          <dt title={self.diskPath}>{t("数据目录所在磁盘")}</dt>
          <dd><Usage value={diskPct} totalKB={self.diskTotal / 1024} /></dd>
        </div>
        <div className="fact">
          <dt>{self.container ? t("网络（这个容器）") : t("网络")}</dt>
          <dd>{self.system ? (
            <span className="num" style={{ fontSize: 13 }}><span className="muted">↓ </span>{rate(l?.rx)}　<span className="muted">↑ </span>{rate(l?.tx)}</span>
          ) : <span className="muted">—</span>}</dd>
        </div>
      </div>
    </div>
  );
}
