import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Activity, Ban, Check, ChevronRight, Fingerprint, KeyRound, ShieldAlert, ShieldCheck, ShieldX, Square, TriangleAlert } from "lucide-react";
import { api, ApiError, type Finding, type Req, type Target } from "../api";
import { fail, seedOutput, toast, upsertRequest, useOutput, useStore } from "../store";
import { verify } from "../verify";
import { Empty, LevelBadge, Modal, StateBadge, TargetIcon, ago, b64text, bytes, duration, levelMeta, stamp as clock, targetText } from "../ui";
import { t, ts, useT } from "../i18n";

function summary(r: Req): string {
  return r.kind === "file.write" ? t("写入 {path}", { path: r.file?.path ?? "" }) : r.kind === "command" ? r.targets[0]?.command ?? "" : ts(r.targets[0]?.command);
}

function hostsLabel(r: Req): string {
  const n = r.targets.length;
  return n === 1 ? r.targets[0].host : t("{host} 等 {n} 台", { host: r.targets[0].host, n });
}

function RequestTable({ rows }: { rows: Req[] }) {
  const t = useT();
  const nav = useNavigate();
  return (
    <div className="table-wrap">
      <table className="table req-table">
        <thead>
          <tr><th style={{ width: 64 }}>{t("等级")}</th><th>{t("操作")}</th><th style={{ width: "16%" }}>{t("目标")}</th><th className="hide-sm" style={{ width: "13%" }}>{t("发起人")}</th><th style={{ width: 128 }}>{t("状态")}</th><th style={{ width: 112 }}>{t("时间")}</th></tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="click" onClick={() => nav(`/requests/${r.id}`)}>
              <td><LevelBadge level={r.level} /></td>
              <td className="req-action">
                <div className="mono ellipsis">{summary(r)}</div>
                {(r.intent || r.snippet) && <div className="muted ellipsis" style={{ fontSize: 12 }}>{[ts(r.intent), r.snippet && `Snippet ${r.snippet}`].filter(Boolean).join(" · ")}</div>}
              </td>
              <td className="mono dim" style={{ whiteSpace: "nowrap" }}>{hostsLabel(r)}</td>
              <td className="hide-sm dim">{ts(r.actor.name)}</td>
              <td><StateBadge state={r.state} /></td>
              <td className="muted" style={{ whiteSpace: "nowrap" }} title={clock(r.createdAt)}>{ago(r.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Approvals() {
  const t = useT();
  const pending = useStore((s) => s.requests).filter((r) => r.state === "pending");
  return (
    <>
      <div className="topbar"><h1>{t("审批")}</h1><span className="muted num">{t("{n} 个等待中", { n: pending.length })}</span></div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("这里是所有等待人来决定的操作。批准的是你看到的那一份内容：命令或文件变了，批准自动作废。")}</p>
          <div className="card">
            {pending.length === 0
              ? <Empty icon={<ShieldCheck />} title={t("没有等待批准的操作")}>{t("只读操作会自动执行；变更和高危操作会出现在这里。")}</Empty>
              : <RequestTable rows={pending} />}
          </div>
        </div>
      </div>
    </>
  );
}

// label 是中文原文，显示时再翻译。
const filters: { key: string; label: string; match: (r: Req) => boolean }[] = [
  { key: "all", label: "全部", match: () => true },
  { key: "running", label: "进行中", match: (r) => r.state === "running" || r.state === "pending" },
  { key: "failed", label: "失败", match: (r) => r.state === "failed" },
  { key: "blocked", label: "被拒绝", match: (r) => r.state === "rejected" || r.state === "denied" },
];

export function ActivityPage() {
  const t = useT();
  const requests = useStore((s) => s.requests);
  const [filter, setFilter] = useState("all");
  const rows = requests.filter(filters.find((f) => f.key === filter)!.match);
  return (
    <>
      <div className="topbar"><h1>{t("活动")}</h1><span className="muted num">{t("最近 {n} 个请求", { n: requests.length })}</span></div>
      <div className="content">
        <div className="page">
          <div className="card">
            <div className="card-head">
              <div className="row" role="group" aria-label={t("按状态筛选")}>
                {filters.map((f) => <button key={f.key} className={"tag" + (filter === f.key ? " on" : "")} onClick={() => setFilter(f.key)}>{t(f.label)}</button>)}
              </div>
            </div>
            {rows.length === 0 ? <Empty icon={<Activity />} title={t("没有记录")} /> : <RequestTable rows={rows} />}
          </div>
        </div>
      </div>
    </>
  );
}

// ---------- 请求详情 ----------

function distinctFindings(targets: Target[]): Finding[] {
  const seen = new Set<string>();
  return targets.flatMap((tg) => tg.findings).filter((f) => {
    if (f.level === "L0") return false;
    const k = f.level + f.reason + f.command;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  }).sort((a, b) => b.level.localeCompare(a.level));
}

export function RequestDetail() {
  const t = useT();
  const { id = "" } = useParams();
  const live = useStore((s) => s.requests.find((r) => r.id === id));
  const [full, setFull] = useState<Req | null>(null);
  const [missing, setMissing] = useState(false);
  const [selected, setSelected] = useState("");

  // 完整内容（含输出和文件）取一次；之后的状态变化来自实时事件。
  const liveState = live?.state;
  useEffect(() => {
    let on = true;
    api.request(id).then((r) => { if (on) { setFull(r); seedOutput(r); upsertRequest(r); } })
      .catch((e) => { if (on && e instanceof ApiError && e.status === 404) setMissing(true); });
    return () => { on = false; };
  }, [id, liveState]);

  const r: Req | null = useMemo(() => {
    if (!full) return null;
    if (!live) return full;
    return { ...full, ...live, file: full.file, targets: live.targets.map((tg, i) => ({ ...full.targets[i], ...tg, output: full.targets[i]?.output ?? "" })) };
  }, [full, live]);

  if (missing) return <><div className="topbar"><h1>{t("请求不存在")}</h1></div><div className="content"><Empty icon={<TriangleAlert />} title={t("找不到这个请求")} /></div></>;
  if (!r) return <div className="topbar"><h1>{t("加载中…")}</h1></div>;

  const current = r.targets.find((tg) => tg.host === selected) ?? r.targets.find((tg) => tg.state === "running" || tg.state === "failed" || tg.state === "error") ?? r.targets[0];
  const commands = new Map<string, string[]>();
  for (const tg of r.targets) commands.set(tg.command, [...(commands.get(tg.command) ?? []), tg.host]);
  const findings = distinctFindings(r.targets);
  const started = r.state !== "pending" && r.state !== "rejected" && r.state !== "denied" && r.state !== "expired";
  const actorKinds: Record<string, string> = { human: t("人"), ai: "AI", token: t("API 令牌"), plugin: t("插件"), system: t("系统") };

  return (
    <>
      <div className="topbar">
        <Link to="/activity" className="crumb">{t("活动")}</Link>
        <ChevronRight size={14} className="crumb-sep" />
        <h1 className="mono">{r.id}</h1>
        <LevelBadge level={r.level} label />
        <StateBadge state={r.state} />
        <span className="grow" />
        {r.state === "running" && <button className="btn danger" onClick={() => api.cancel(r.id).catch(fail)}><Square />{t("停止")}</button>}
      </div>
      <div className="content">
        <div className="page">
          {r.state === "pending" && <ApprovalCard r={r} />}
          {r.state === "rejected" && (
            <div className="card rejected">
              <div className="card-head"><ShieldX size={16} style={{ color: "var(--crit)" }} /><h2>{t("内核拒绝了这个操作")}</h2></div>
              <div className="card-body dim">{t("它被判为 L3（禁止）。没有任何角色或入口能放行；这次尝试已经写入审计{ref}。", { ref: r.auditSeq ? t("（#{seq}）", { seq: r.auditSeq }) : "" })}</div>
            </div>
          )}
          {r.state === "denied" && (
            <div className="note"><Ban /><span>{r.denyReason ? t("{who} 拒绝了这个操作：{reason}", { who: r.deniedBy ?? "", reason: r.denyReason }) : t("{who} 拒绝了这个操作。", { who: r.deniedBy ?? "" })}</span></div>
          )}

          <div className="split">
            <div className="col" style={{ gap: 16 }}>
              <div className="card">
                <div className="card-head"><h2>{r.kind === "file.write" ? t("文件改动") : r.kind === "terminal" ? t("操作") : t("实际命令")}</h2><span className="muted">{r.kind === "file.write" ? r.file?.path : r.kind === "terminal" ? "" : t("渲染之后、将在主机上运行的内容")}</span></div>
                <div className="card-body col" style={{ gap: 12 }}>
                  {r.kind === "file.write" && r.file ? (
                    <Diff oldText={r.file.oldExists ? b64text(r.file.old) : ""} newText={b64text(r.file.content)} isNew={!r.file.oldExists} />
                  ) : (
                    [...commands.entries()].map(([cmd, hs]) => (
                      <div key={cmd}>
                        {commands.size > 1 && <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{hs.join(t("、"))}</div>}
                        <pre className="code"><span className="prompt">$ </span>{r.kind === "command" ? cmd : ts(cmd)}</pre>
                      </div>
                    ))
                  )}
                  {r.input && (
                    <div className="note">
                      <KeyRound />
                      <span>
                        {t("标准输入：")}{r.input.kind === "envfile" ? <>{t("{n} 条应用密钥（{names}），写成主机上只有本人可读的环境文件", { n: r.input.env?.length ?? 0, names: r.input.env?.map((e) => e.env).join(t("、")) ?? "" })} <span className="mono">{r.input.label}</span>{t("。值由塔台在执行时直接送过去，不出现在命令行、请求和审计里。")}</>
                          : r.input.kind === "secret" ? <>{t("凭据「{label}」。它由塔台在执行时直接送给命令，不出现在命令行、请求和审计里。", { label: r.input.label ?? "" })}</>
                          : <>{t("上传的文件")} <b>{r.input.label}</b>{t("（{size}，SHA-256", { size: bytes(r.input.size ?? 0) })} <span className="mono">{r.input.sha256?.slice(0, 16)}…</span>{t("）。")}</>}
                      </span>
                    </div>
                  )}
                  <div className="row wrap" style={{ gap: 4 }}>
                    <span className="muted" style={{ fontSize: 12 }}>{t("目标 {n} 台：", { n: r.targets.length })}</span>
                    {r.targets.map((tg) => (
                      <Link key={tg.host} to={`/hosts/${tg.host}`} className="tag mono" title={tg.sensitivity === "high" ? t("高敏感主机") : undefined}>
                        {tg.sensitivity === "high" && <ShieldAlert size={11} style={{ color: "var(--serious)", marginRight: 4 }} />}{tg.host}
                      </Link>
                    ))}
                  </div>
                </div>
              </div>

              {r.kind === "terminal" && started && (
                <div className="card">
                  <div className="card-head"><h2>{t("终端会话")}</h2></div>
                  <div className="card-body col" style={{ gap: 12 }}>
                    {!r.terminal ? <span className="dim">{t("已确认，等待连接（两分钟内不连就作废）。到")} <Link to={`/hosts/${r.targets[0].host}/terminal`} style={{ color: "var(--accent)" }}>{t("主机的终端页")}</Link>{t(" 重新打开。")}</span> : (
                      <>
                        <dl className="kv">
                          <dt>{t("主机")}</dt><dd className="mono">{r.targets[0].host}</dd>
                          <dt>{t("开始")}</dt><dd className="mono">{clock(r.terminal.attachedAt)}</dd>
                          <dt>{t("结束")}</dt><dd className="mono">{r.terminal.closedAt ? clock(r.terminal.closedAt) : t("进行中")}</dd>
                          <dt>{t("输出量")}</dt><dd>{bytes(r.terminal.bytes)}{r.terminal.truncated ? t("（录像达到上限，后面的没有录）") : ""}</dd>
                        </dl>
                        {r.targets[0].error && <div className="note crit"><TriangleAlert /><span>{ts(r.targets[0].error)}</span></div>}
                        {r.terminal.closedAt && <a className="btn" style={{ alignSelf: "flex-start" }} href={`/api/v1/terminal/${r.id}/recording`}>{t("下载录像")}</a>}
                        <span className="hint">{t("录像是 asciicast 格式，可以用")} <span className="mono">asciinema play</span>{t(" 回放。键盘输入不单独录制（它可能含口令），回显会出现在输出里。")}</span>
                      </>
                    )}
                  </div>
                </div>
              )}
              {r.kind !== "terminal" && started && (
                <div className="card">
                  <div className="card-head">
                    <h2>{t("执行")}</h2>
                    <span className="muted num">{t("{ok} / {n} 成功", { ok: r.targets.filter((tg) => tg.state === "ok").length, n: r.targets.length })}</span>
                  </div>
                  <div className={r.targets.length > 1 ? "split even" : ""} style={{ gap: 0 }}>
                    {r.targets.length > 1 && (
                      <div style={{ borderRight: "1px solid var(--border)" }}>
                        {r.targets.map((tg) => (
                          <div key={tg.host} className={"host-row" + (tg.host === current.host ? " active" : "")} onClick={() => setSelected(tg.host)}>
                            <TargetIcon state={tg.state} />
                            <span className="mono ellipsis">{tg.host}</span>
                            <span className="muted" style={{ fontSize: 12 }}>{tg.state === "ok" || tg.state === "failed" ? duration(tg.startedAt, tg.endedAt) : t(targetText[tg.state])}</span>
                          </div>
                        ))}
                      </div>
                    )}
                    <div className="card-body col" style={{ gap: 10, minWidth: 0 }}>
                      <div className="row wrap">
                        <TargetIcon state={current.state} />
                        <b className="mono">{current.host}</b>
                        <span className="dim">{t(targetText[current.state])}</span>
                        {current.exitCode !== undefined && <span className={"badge " + (current.exitCode === 0 ? "neutral" : "crit")}>{t("退出码 {code}", { code: current.exitCode })}</span>}
                        {current.endedAt && <span className="muted">{duration(current.startedAt, current.endedAt)}</span>}
                        <span className="grow" />
                        {current.auditSeq ? <Link to="/audit" className="muted" style={{ fontSize: 12 }}>{t("审计 #{seq}", { seq: current.auditSeq })}</Link> : null}
                      </div>
                      {current.error && <div className="note crit"><TriangleAlert /><span>{ts(current.error)}</span></div>}
                      {r.kind === "file.write"
                        ? current.state === "ok" && <div className="note ok"><Check /><span>{current.backup ? <>{t("已写入。旧内容备份在塔台数据目录：")}<span className="mono">{current.backup}</span></> : t("已写入。这是一个新文件，没有旧内容需要备份。")}</span></div>
                        : <Output id={r.id} target={current} />}
                    </div>
                  </div>
                </div>
              )}
            </div>

            <div className="col" style={{ gap: 16 }}>
              <div className="card">
                <div className="card-head"><h2>{t("判级")}</h2></div>
                <div className="card-body">
                  <div className="row" style={{ marginBottom: findings.length ? 8 : 0 }}>
                    <LevelBadge level={r.level} label /><span className="dim">{t(levelMeta[r.level].desc)}</span>
                  </div>
                  {findings.map((f, i) => (
                    <div key={i} className="finding">
                      <LevelBadge level={f.level} />
                      <div><div className="why">{ts(f.reason)}</div>{f.command && <div className="what">{ts(f.command)}</div>}</div>
                    </div>
                  ))}
                </div>
              </div>
              <div className="card">
                <div className="card-head"><h2>{t("详情")}</h2></div>
                <div className="card-body">
                  <dl className="kv">
                    <dt>{t("意图")}</dt><dd>{ts(r.intent) || <span className="muted">{t("未填写")}</span>}</dd>
                    <dt>{t("发起人")}</dt><dd>{ts(r.actor.name)} <span className="muted">· {actorKinds[r.actor.kind] ?? r.actor.kind} · {r.actor.entry}</span></dd>
                    {r.snippet && <><dt>Snippet</dt><dd className="mono">{r.snippet}</dd></>}
                    <dt>{t("目标")}</dt><dd>{t("{n} 台主机", { n: r.targets.length })}</dd>
                    {r.targets.length > 1 && <><dt>{t("策略")}</dt><dd>{[r.strategy.canary && t("先预演一台"), r.strategy.stopOnError && t("遇错停止"), r.targets.some((tg) => tg.sensitivity === "high") ? t("逐台滚动（含高敏感主机，排最后）") : t("并发 {n}", { n: r.strategy.concurrency || 10 })].filter(Boolean).join(" · ")}</dd></>}
                    <dt>{t("提交@@时间")}</dt><dd>{clock(r.createdAt)}</dd>
                    {r.approvals?.map((a, i) => <FragmentRow key={i} k={t("批准")} v={`${a.by} · ${clock(a.at)}${a.reauth ? " · " + t("已二次验证") : a.devBypass ? " · " + t("开发模式，未经二次验证") : ""}`} />)}
                    {r.finishedAt && <><dt>{t("结束")}</dt><dd>{clock(r.finishedAt)}</dd></>}
                    <dt>{t("内容摘要")}</dt><dd className="mono muted" title={r.digest}>{r.digest.slice(0, 16)}…</dd>
                  </dl>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

function FragmentRow({ k, v }: { k: string; v: string }) {
  return <><dt>{k}</dt><dd>{v}</dd></>;
}

function Output({ id, target }: { id: string; target: Target }) {
  const t = useT();
  const streamed = useOutput(id, target.host);
  const text = streamed ?? target.output;
  const ref = useRef<HTMLPreElement>(null);
  const stick = useRef(true);
  useEffect(() => {
    const el = ref.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [text]);
  return (
    <>
      <pre ref={ref} className="term" data-empty={t("（没有输出）")} tabIndex={0} aria-label={t("{host} 的输出", { host: target.host })}
        onScroll={(e) => { const el = e.currentTarget; stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24; }}>{text}</pre>
      {target.truncated && <div className="muted" style={{ fontSize: 12 }}>{t("输出超过上限，只保留了开头部分。")}</div>}
    </>
  );
}

export function ApprovalCard({ r, title, lead }: { r: Req; title?: string; lead?: React.ReactNode }) {
  const t = useT();
  const boot = useStore((s) => s.boot)!;
  const paused = useStore((s) => s.paused);
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [denying, setDenying] = useState(false);
  const [reason, setReason] = useState("");
  const l2 = r.level === "L2";
  const confirmOK = !l2 || typed.trim() === r.confirm;
  const [, tick] = useState(0);
  useEffect(() => { const timer = setInterval(() => tick((n) => n + 1), 15000); return () => clearInterval(timer); }, []);
  const minsLeft = r.expiresAt ? Math.max(0, Math.ceil((Date.parse(r.expiresAt) - Date.now()) / 60000)) : null;

  const approve = async () => {
    setBusy(true);
    try {
      // 高危操作：当场用 Passkey 或动态码验证身份。开发模式的令牌会话由服务端按开发模式处理。
      if (l2 && !boot.devSession) await verify({ l2: true });
      upsertRequest(await api.approve(r.id, r.digest, typed));
      if (r.kind !== "terminal") toast(t("已批准，开始执行"));
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const deny = async () => {
    setBusy(true);
    try {
      upsertRequest(await api.deny(r.id, reason));
      setDenying(false);
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <div className={"card approval" + (l2 ? " l2" : "")}>
      <div className="card-head">
        <ShieldCheck size={16} style={{ color: l2 ? "var(--serious)" : "var(--warn)" }} />
        <h2>{title ?? t("等待批准")}</h2>
        {!title && <span className="dim">{l2 ? t("{name} 请求在 {n} 台主机上执行高危操作", { name: ts(r.actor.name), n: r.targets.length }) : t("{name} 请求在 {n} 台主机上执行变更操作", { name: ts(r.actor.name), n: r.targets.length })}</span>}
        <span className="grow" />
        {minsLeft !== null && <span className="muted">{t("{n} 分钟后过期", { n: minsLeft })}</span>}
      </div>
      <div className="card-body col" style={{ gap: 14 }}>
        <div className="dim">{lead ?? (r.kind === "file.write" ? t("请先核对下面的文件改动、目标主机和判定理由。批准绑定到这份内容的摘要。") : t("请先核对下面的实际命令、目标主机和判定理由。批准绑定到这份内容的摘要。"))}</div>
        {l2 && (
          <div className="field" style={{ maxWidth: 420 }}>
            <label htmlFor="confirm">
              {r.targets.length === 1 ? <>{t("输入主机名")} <b className="mono">{r.confirm}</b>{t(" 确认")}</> : <>{t("输入主机数量")} <b className="mono">{r.confirm}</b>{t(" 确认")}</>}
            </label>
            <input id="confirm" className="input mono" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck={false} />
            <span className="hint">{boot.devSession ? t("开发模式会话：跳过身份验证。") : t("批准时要用 Passkey 或动态码验证身份。")}</span>
          </div>
        )}
        <div className="row">
          <button className={"btn lg " + (l2 ? "danger-solid" : "primary")} disabled={busy || paused || !confirmOK} onClick={approve}>
            {l2 ? <Fingerprint /> : <Check />}{r.kind === "terminal" ? t("确认并打开终端") : l2 ? t("验证身份并批准") : t("批准并执行")}
          </button>
          <button className="btn lg" disabled={busy} onClick={() => setDenying(true)}><Ban />{r.kind === "terminal" ? t("取消") : t("拒绝")}</button>
          {paused && <span className="muted">{t("执行已暂停，暂时不能批准。")}</span>}
        </div>
      </div>
      {denying && (
        <Modal title={t("拒绝这个操作")} onClose={() => setDenying(false)}
          footer={<><button className="btn" onClick={() => setDenying(false)}>{t("取消")}</button><button className="btn danger" disabled={busy} onClick={deny}>{t("确认拒绝")}</button></>}>
          <div className="field">
            <label htmlFor="reason">{t("理由（可选，会写进审计，也会反馈给发起人）")}</label>
            <textarea id="reason" className="textarea" rows={3} value={reason} onChange={(e) => setReason(e.target.value)} autoFocus />
          </div>
        </Modal>
      )}
    </div>
  );
}

// ---------- 行级 diff ----------

// gap 行的 text 是略过的行数，显示时再拼成文字。
type DiffLine = { kind: "add" | "del" | "ctx" | "gap"; text: string };

function lineDiff(a: string[], b: string[]): DiffLine[] {
  const n = a.length, m = b.length;
  if (n * m > 4_000_000) return [...a.map((x) => ({ kind: "del" as const, text: x })), ...b.map((x) => ({ kind: "add" as const, text: x }))];
  const w = m + 1;
  const dp = new Uint32Array((n + 1) * w);
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i * w + j] = a[i] === b[j] ? dp[(i + 1) * w + j + 1] + 1 : Math.max(dp[(i + 1) * w + j], dp[i * w + j + 1]);
  const out: DiffLine[] = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) { out.push({ kind: "ctx", text: a[i] }); i++; j++; }
    else if (dp[(i + 1) * w + j] >= dp[i * w + j + 1]) out.push({ kind: "del", text: a[i++] });
    else out.push({ kind: "add", text: b[j++] });
  }
  while (i < n) out.push({ kind: "del", text: a[i++] });
  while (j < m) out.push({ kind: "add", text: b[j++] });
  return out;
}

function Diff({ oldText, newText, isNew }: { oldText: string; newText: string; isNew: boolean }) {
  const t = useT();
  const lines = useMemo(() => {
    const split = (s: string) => (s === "" ? [] : s.replace(/\n$/, "").split("\n"));
    const all = lineDiff(split(oldText), split(newText));
    // 只保留改动附近 3 行上下文。
    const keep = new Array(all.length).fill(false);
    all.forEach((l, i) => { if (l.kind !== "ctx") for (let k = Math.max(0, i - 3); k <= Math.min(all.length - 1, i + 3); k++) keep[k] = true; });
    const out: DiffLine[] = [];
    let skipped = 0;
    all.forEach((l, i) => {
      if (keep[i]) { if (skipped) { out.push({ kind: "gap", text: String(skipped) }); skipped = 0; } out.push(l); }
      else skipped++;
    });
    if (skipped && out.length) out.push({ kind: "gap", text: String(skipped) });
    return out;
  }, [oldText, newText]);
  const adds = lines.filter((l) => l.kind === "add").length, dels = lines.filter((l) => l.kind === "del").length;
  return (
    <>
      <div className="row muted" style={{ fontSize: 12 }}>
        {isNew ? t("新文件") : t("相对当前内容")} · <span style={{ color: "var(--ok)" }}>{t("+{n} 行", { n: adds })}</span> <span style={{ color: "var(--crit)" }}>{t("−{n} 行", { n: dels })}</span>
      </div>
      {lines.length === 0 ? <div className="note"><TriangleAlert /><span>{t("内容没有变化。")}</span></div> : (
        <div className="diff" role="img" aria-label={t("改动：新增 {adds} 行，删除 {dels} 行", { adds, dels })}>
          {lines.map((l, i) => <div key={i} className={l.kind}>{l.kind === "add" ? "+ " : l.kind === "del" ? "− " : l.kind === "gap" ? "" : "  "}{l.kind === "gap" ? t("… {n} 行未改动 …", { n: l.text }) : l.text}</div>)}
        </div>
      )}
    </>
  );
}
