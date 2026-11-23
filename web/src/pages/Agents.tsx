// Agent 协作：把同一个任务派给多台主机上的 AI 编程助手，并排看结果，再让其中一个汇总；
// 上面的协作图实时画出塔台和各台主机之间正在发生的事（下发命令、分发文件、助手在思考、助手经由哪台网关调模型）。
// 这一页没有自己的执行通道：派发和汇总都是请求，判级、审批、审计照常；图上的东西全部来自已有的数据。
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Bot, Check, ChevronDown, ChevronUp, Columns3, Eye, Play, Sigma, TriangleAlert } from "lucide-react";
import { api, type AppInstalled, type HostApps, type HostEnv, type Preview, type Req } from "../api";
import { backAfterApproval, fail, useOutput, useStore } from "../store";
import { Empty, LevelBadge, Modal, ago } from "../ui";
import { PreviewPanel } from "../shared";
import { cached, remember } from "../cache";
import { ts, useT } from "../i18n";
import { HostPicker } from "./FileSync";

type AgentID = "claude" | "codex" | "gemini" | "dsh";
const agents: { id: AgentID; title: string; app: string; letter: string; color: string; summary: boolean; task: boolean }[] = [
  { id: "claude", title: "Claude Code", app: "claude-code", letter: "C", color: "#D97757", summary: true, task: true },
  { id: "codex", title: "Codex CLI", app: "codex", letter: "X", color: "#10A37F", summary: false, task: true },
  { id: "gemini", title: "Gemini CLI", app: "gemini-cli", letter: "G", color: "#4E87F6", summary: true, task: true },
  { id: "dsh", title: "DeepSeek Harness", app: "deepseek-harness", letter: "D", color: "#5786FE", summary: false, task: false },
];
const gateways: Record<string, { short: string; title: string }> = {
  litellm: { short: "LL", title: "LiteLLM" }, "new-api": { short: "NA", title: "New API" }, ollama: { short: "OL", title: "Ollama" },
};

/** 一台主机上的助手和网关（来自“已安装”的只读查询，再补上环境检测里看到的其他安装方式）。 */
interface HostCrew {
  agents: { id: AgentID; version?: string; provider?: string; model?: string; baseUrl?: string }[];
  gateways: { app: string; instance: string; port?: string; running: boolean }[];
}

function crewOf(installed: AppInstalled[] | undefined, env: HostEnv | undefined): HostCrew {
  const crew: HostCrew = { agents: [], gateways: [] };
  for (const x of installed ?? []) {
    const a = agents.find((g) => g.app === x.app);
    if (a && !crew.agents.some((c) => c.id === a.id)) crew.agents.push({ id: a.id, version: x.version, provider: x.config?.provider, model: x.config?.model, baseUrl: x.config?.base_url });
    if (gateways[x.app]) crew.gateways.push({ app: x.app, instance: x.instance, port: x.params?.port, running: x.containers.some((c) => c.state === "running") });
  }
  for (const tool of env?.tools ?? []) {
    const a = agents.find((g) => g.id === tool.name);
    if (a && tool.installed && !crew.agents.some((c) => c.id === a.id)) crew.agents.push({ id: a.id, version: tool.version });
  }
  return crew;
}

/** 从地址里取主机名（去掉账号、端口和方括号）。 */
function bareHost(addr: string) {
  let a = addr.trim().replace(/^[^@]*@/, "");
  if (a.startsWith("[")) return a.slice(1, a.indexOf("]"));
  if ((a.match(/:/g) ?? []).length === 1) a = a.replace(/:\d+$/, "");
  return a;
}

// Agent 任务和汇总请求的来源标记（agent:claude、agent-summary:gemini）。用户自己的 Snippet 也可能叫 agent-xxx，所以要整段匹配。
const agentRe = /^agent(-summary)?:(\w+)$/;
const isAgentReq = (r: Req) => agentRe.test(r.snippet ?? "");

/** 一次请求属于哪一类流动：给图上的小包裹上色。 */
function flowKind(r: Req): "agent" | "file" | "app" | "cmd" {
  if (isAgentReq(r)) return "agent";
  if (r.input?.kind === "upload" || r.kind === "file.write") return "file";
  if (r.snippet?.startsWith("app:")) return "app";
  return "cmd";
}

function useReducedMotion() {
  const [reduced, setReduced] = useState(() => typeof matchMedia !== "undefined" && matchMedia("(prefers-reduced-motion: reduce)").matches);
  useEffect(() => {
    if (typeof matchMedia === "undefined") return;
    const m = matchMedia("(prefers-reduced-motion: reduce)");
    const on = () => setReduced(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return reduced;
}

// ---------- 协作图 ----------

const W = 1000, H = 560, CX = W / 2, CY = H / 2;
// 主机画成圆：R 是圆的半径，RING 是外面那圈负载环的半径。
const R = 26, RING = R + 6, RING_LEN = 2 * Math.PI * RING;
// 助手排在圆的右侧、网关排在左侧（角度，度）。
const petAngle = (k: number) => (-25 + k * 30) * Math.PI / 180;
const gwAngle = (k: number) => (205 - k * 30) * Math.PI / 180;

function CrewMap({ crews }: { crews: Record<string, HostCrew> }) {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const requests = useStore((s) => s.requests);
  const reduced = useReducedMotion();
  const shown = hosts.slice(0, 14);
  const n = shown.length;
  // 主机排在一个椭圆上；只有一两台时也别挤在一边。
  const pos = (i: number) => {
    const a = -Math.PI / 2 + (i * 2 * Math.PI) / Math.max(n, 3);
    return { x: CX + Math.cos(a) * 390, y: CY + Math.sin(a) * 200 };
  };
  const at = new Map(shown.map((h, i) => [h.name, pos(i)]));
  const running = requests.filter((r) => r.state === "running");
  const pending = new Set(requests.filter((r) => r.state === "pending").flatMap((r) => r.targets.map((tg) => tg.host)));
  const busyAgents = new Map<string, Set<string>>(); // 主机 → 正在干活的助手
  const flows: { host: string; kind: ReturnType<typeof flowKind>; label: string; id: string }[] = [];
  for (const r of running) {
    for (const tg of r.targets) {
      if (tg.state !== "running" || !at.has(tg.host)) continue;
      flows.push({ host: tg.host, kind: flowKind(r), label: ts(r.intent), id: r.id });
      const m = agentRe.exec(r.snippet ?? "");
      if (m) busyAgents.set(tg.host, new Set([...(busyAgents.get(tg.host) ?? []), m[2]]));
    }
  }
  // 助手经由哪台网关调模型：配置里的接口地址指向另一台（或同一台）主机上的 LiteLLM / New API / Ollama。
  const links: { from: string; to: string; agent: AgentID; model?: string }[] = [];
  for (const h of shown) {
    for (const a of crews[h.name]?.agents ?? []) {
      if (!a.baseUrl) continue;
      let u: URL;
      try { u = new URL(a.baseUrl); } catch { continue; }
      const target = /^(127\.0\.0\.1|localhost|\[::1\])$/.test(u.hostname) ? h : shown.find((x) => bareHost(x.addr) === u.hostname.replace(/^\[|\]$/g, ""));
      if (!target) continue;
      // 端口对得上的网关优先；对不上（比如前面套了反向代理，走 443）就算这台主机上的第一个网关。
      const gws = crews[target.name]?.gateways ?? [];
      const gw = gws.find((g) => g.port === (u.port || (u.protocol === "https:" ? "443" : "80"))) ?? gws[0];
      if (gw) links.push({ from: h.name, to: target.name, agent: a.id, model: a.model });
    }
  }
  const busy = running.length > 0;

  if (n === 0) return <Empty icon={<Bot />} title={t("还没有主机")} />;
  return (
    <div className="crew-wrap">
      {/* 窄屏上图不缩到看不清：保持最小宽度，左右滑动看。 */}
      <div className="crew-scroll">
      <svg className={"crew-map" + (reduced ? " still" : "")} viewBox={`0 0 ${W} ${H}`} role="img" aria-label={t("协作图：塔台、主机和它们之间正在发生的事")}>
        <defs>
          <radialGradient id="crew-glow"><stop offset="0%" stopColor="var(--primary)" stopOpacity="0.35" /><stop offset="100%" stopColor="var(--primary)" stopOpacity="0" /></radialGradient>
        </defs>
        {/* 塔台和每台主机之间的线 */}
        {shown.map((h) => {
          const p = at.get(h.name)!;
          const active = flows.some((f) => f.host === h.name);
          // 线停在负载环外面，不压住主机的圆；包裹沿线飞到环边上。
          const dx = p.x - CX, dy = p.y - CY, len = Math.hypot(dx, dy) || 1;
          const ex = p.x - (dx / len) * (RING + 3), ey = p.y - (dy / len) * (RING + 3);
          return <path key={h.name} id={`crew-edge-${h.name}`} d={`M${CX},${CY} L${ex},${ey}`} className={"crew-edge" + (active ? " active" : "") + (!h.status.online && h.status.known ? " off" : "")} />;
        })}
        {/* 助手 → 网关 的模型调用 */}
        {links.map((l, i) => {
          const a = at.get(l.from)!, b = at.get(l.to)!;
          const self = l.from === l.to;
          // 往外弯的二次曲线；标签放在曲线的中点（0.25·起点 + 0.5·控制点 + 0.25·终点）。
          const qx = (a.x + b.x) / 2 + (CY - (a.y + b.y) / 2) * 0.35, qy = (a.y + b.y) / 2 + ((a.x + b.x) / 2 - CX) * 0.35;
          const mx = 0.25 * a.x + 0.5 * qx + 0.25 * b.x, my = 0.25 * a.y + 0.5 * qy + 0.25 * b.y;
          const d = self
            ? `M${a.x - 40},${a.y + 30} C${a.x - 60},${a.y + 90} ${a.x + 60},${a.y + 90} ${a.x + 40},${a.y + 30}`
            : `M${a.x},${a.y} Q${qx},${qy} ${b.x},${b.y}`;
          return (
            <g key={i} className="crew-link">
              <path id={`crew-link-${i}`} d={d} />
              {!reduced && [0, 1, 2].map((k) => (
                <circle key={k} r="3.2" className="crew-token"><animateMotion dur="2.4s" begin={`${k * 0.8}s`} repeatCount="indefinite"><mpath href={`#crew-link-${i}`} /></animateMotion></circle>
              ))}
              {l.model && !self && !links.slice(0, i).some((o) => o.to === l.to && o.model === l.model) && <text className="crew-link-label" x={mx} y={my - 6}>{l.model}</text>}
            </g>
          );
        })}
        {/* 正在下发的东西：一颗颗小包裹沿着线从塔台飞向主机 */}
        {!reduced && flows.map((f, i) => (
          <g key={f.id + f.host} className={"crew-packet " + f.kind}>
            {[0, 1].map((k) => (
              <rect key={k} x="-5" y="-5" width="10" height="10" rx="3"><animateMotion dur="1.6s" begin={`${(i % 4) * 0.2 + k * 0.8}s`} repeatCount="indefinite" rotate="auto"><mpath href={`#crew-edge-${f.host}`} /></animateMotion></rect>
            ))}
          </g>
        ))}
        {/* 塔台 */}
        <g className={"crew-tower" + (busy ? " busy" : "")} transform={`translate(${CX},${CY})`}>
          <circle r="70" fill="url(#crew-glow)" />
          <circle r="34" className="crew-tower-core" />
          {busy && <circle r="44" className="crew-tower-ring" />}
          <g transform="translate(-16,-18) scale(0.5)">
            <path d="M39 21 62 11v20z" fill="var(--warn)" opacity=".7" />
            <path d="M24 17 32 9l8 8z M26 19h12l3 24H23z" fill="var(--text)" />
            <path d="M9 47.5c5-4 9-4 14 0s9 4 14 0 9-4 14 0" fill="none" stroke="var(--accent)" strokeWidth="4" strokeLinecap="round" />
          </g>
          <text y="56" className="crew-tower-label">{busy ? t("{n} 个操作进行中", { n: running.length }) : t("塔台")}</text>
        </g>
        {/* 主机 */}
        {shown.map((h) => {
          const p = at.get(h.name)!;
          const crew = crews[h.name];
          const off = !h.status.online && h.status.known;
          const doing = busyAgents.get(h.name);
          const waiting = pending.has(h.name);
          const active = flows.some((f) => f.host === h.name);
          const cpu = off || !h.status.online ? undefined : h.status.latest?.cpu;
          const load = cpu === undefined ? "" : cpu >= 90 ? " crit" : cpu >= 70 ? " warn" : "";
          // 圆下面的第二行：状态不只靠颜色，也写成字。
          const sub = off ? t("离线") : waiting ? t("等待批准") : cpu !== undefined ? `CPU ${Math.round(cpu)}%` : "";
          return (
            <g key={h.name} transform={`translate(${p.x},${p.y})`} className={"crew-host" + (off ? " off" : "") + (waiting ? " waiting" : "") + (active ? " active" : "")}>
              <title>{h.name + " · " + (off ? t("离线") : t("在线")) + (cpu !== undefined ? ` · CPU ${cpu.toFixed(1)}%` : "") + (waiting ? " · " + t("等待批准") : "")}</title>
              {/* 负载环：弧长是 CPU 利用率，70% 以上变黄、90% 以上变红 */}
              <circle r={RING} className="crew-load-track" />
              {cpu !== undefined && <circle r={RING} className={"crew-load" + load} transform="rotate(-90)" strokeDasharray={`${(RING_LEN * Math.min(100, Math.max(0, cpu))) / 100} ${RING_LEN}`} />}
              {active && <circle r={RING + 7} className="crew-host-spin" />}
              <circle r={R} className="crew-host-box" />
              {/* 圆里是一台两层的小服务器，指示灯：在线绿色、执行中闪蓝色、离线灰色 */}
              <g className="crew-server">
                <rect x="-11" y="-10" width="22" height="8" rx="2" />
                <rect x="-11" y="2" width="22" height="8" rx="2" />
                <circle cx="6" cy="-6" r="1.7" className={"crew-led" + (off ? " off" : "")} />
                <circle cx="6" cy="6" r="1.7" className={"crew-led" + (off ? " off" : "")} />
              </g>
              <text y={R + 21} className="crew-host-name">{h.name.length > 16 ? h.name.slice(0, 15) + "…" : h.name}</text>
              {sub && <text y={R + 36} className="crew-host-sub">{sub}</text>}
              {waiting && <g transform={`translate(0,${-RING - 13})`} className="crew-wait"><circle r="9" /><text y="4">?</text></g>}
              {/* 助手：一圈小圆脸，正在干活的会上下晃、头上冒“…” */}
              {(crew?.agents ?? []).map((a, k) => {
                const meta = agents.find((g) => g.id === a.id)!;
                const thinking = doing?.has(a.id);
                return (
                  <g key={a.id} transform={`translate(${Math.cos(petAngle(k)) * (RING + 14)},${Math.sin(petAngle(k)) * (RING + 14)})`} className={"crew-pet" + (thinking ? " thinking" : "")}>
                    <title>{meta.title + (a.version ? " " + a.version : "") + (a.model ? " · " + a.model : "")}</title>
                    <g className="crew-pet-body">
                      <circle r="11" fill={meta.color} />
                      <circle cx="-3.5" cy="-2" r="1.6" className="crew-eye" />
                      <circle cx="3.5" cy="-2" r="1.6" className="crew-eye" />
                      <path d={thinking ? "M-3,4 Q0,6 3,4" : "M-3.5,3.5 Q0,7 3.5,3.5"} className="crew-mouth" />
                    </g>
                    {thinking && (
                      <g className="crew-bubble" transform="translate(10,-20)">
                        <rect x="-2" y="-9" width="26" height="15" rx="7" />
                        <circle cx="5" cy="-1.5" r="1.8" /><circle cx="11" cy="-1.5" r="1.8" /><circle cx="17" cy="-1.5" r="1.8" />
                      </g>
                    )}
                  </g>
                );
              })}
              {/* 网关：小六边形 */}
              {(crew?.gateways ?? []).map((g, k) => (
                <g key={g.instance} transform={`translate(${Math.cos(gwAngle(k)) * (RING + 14)},${Math.sin(gwAngle(k)) * (RING + 14)})`} className={"crew-gw" + (g.running ? "" : " off")}>
                  <title>{(gateways[g.app]?.title ?? g.app) + " · " + g.instance + " · " + (g.running ? t("运行中") : t("已停止"))}</title>
                  <path d="M-11,0 L-5.5,-9.5 L5.5,-9.5 L11,0 L5.5,9.5 L-5.5,9.5 Z" />
                  <text y="3.5">{gateways[g.app]?.short ?? "GW"}</text>
                </g>
              ))}
            </g>
          );
        })}
      </svg>
      </div>
      {hosts.length > shown.length && <div className="muted" style={{ fontSize: 12.5 }}>{t("图上只画了前 {n} 台主机。", { n: shown.length })}</div>}
      <div className="crew-legend">
        <span><i className="lg-packet agent" />{t("Agent 任务")}</span>
        <span><i className="lg-packet file" />{t("分发文件")}</span>
        <span><i className="lg-packet app" />{t("安装应用")}</span>
        <span><i className="lg-packet cmd" />{t("其他命令")}</span>
        <span><i className="lg-token" />{t("助手经由网关调用模型")}</span>
        <span><i className="lg-wait">?</i>{t("等待批准")}</span>
        <span><i className="lg-pet" />{t("助手（冒“…”的正在思考）")}</span>
      </div>
    </div>
  );
}

// ---------- 派任务 ----------

function TaskForm({ crews }: { crews: Record<string, HostCrew> }) {
  const t = useT();
  const nav = useNavigate();
  const [agent, setAgent] = useState<AgentID>("claude");
  const [hosts, setHosts] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [prompt, setPrompt] = useState("");
  const [write, setWrite] = useState(false);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [busy, setBusy] = useState(false);
  const missing = hosts.filter((h) => crews[h] && !crews[h].agents.some((a) => a.id === agent));
  const ready = hosts.length > 0 && dir.trim().startsWith("/") && prompt.trim() !== "" && !busy;
  const body = { agent, hosts, dir: dir.trim(), prompt: prompt.trim(), write };
  useEffect(() => { setPreview(null); }, [agent, hosts.join(","), dir, prompt, write]); // eslint-disable-line react-hooks/exhaustive-deps

  const check = () => { setBusy(true); api.agentTask({ ...body, preview: true }).then((r) => setPreview(r as unknown as Preview)).catch(fail).finally(() => setBusy(false)); };
  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.agentTask(body);
      const reqs = (r as { requests?: Req[] }).requests ?? [];
      if (reqs[0]) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <div className="card">
      <div className="card-head"><h2>{t("派任务")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("同一个任务交给多台主机上的助手各自去做；它们只在自己那台主机上看、自己那台主机上改")}</span></div>
      <div className="card-body col" style={{ gap: 16 }}>
        <div className="agent-picks" role="radiogroup" aria-label={t("AI 编程助手")}>
          {agents.filter((a) => a.task).map((a) => (
            <button key={a.id} role="radio" aria-checked={agent === a.id} className={"agent-pick" + (agent === a.id ? " on" : "")} onClick={() => setAgent(a.id)}>
              <span className="agent-face" style={{ background: a.color }}>{a.letter}</span>{a.title}
            </button>
          ))}
        </div>
        <div className="field">
          <label htmlFor="ag-prompt">{t("任务")}</label>
          <textarea id="ag-prompt" className="textarea" rows={4} maxLength={4000} placeholder={t("例如：检查这台主机上 nginx 的配置和最近的错误日志，有问题就说明原因和改法，不要改动任何文件。")} value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        </div>
        <div className="form-grid">
          <div className="field">
            <label htmlFor="ag-dir">{t("工作目录")}</label>
            <input id="ag-dir" className="input mono" placeholder="/srv/app" value={dir} onChange={(e) => setDir(e.target.value)} />
            <span className="hint">{t("助手在这个目录里干活；它能读到的东西以登录账号的权限为准。")}</span>
          </div>
          <div className="field">
            <label>{t("权限")}</label>
            <label className="row" style={{ gap: 8, fontWeight: 400 }}><input type="checkbox" checked={write} onChange={(e) => setWrite(e.target.checked)} />{t("允许修改工作目录里的文件")}</label>
            <span className="hint">{write ? t("只放开工作目录里的修改；执行别的命令、改别处的文件仍会被拒绝。") : t("只读：助手只能看、不能改，要它执行命令时会被拒绝。")}</span>
          </div>
        </div>
        <HostPicker picked={hosts} onChange={setHosts} />
        {missing.length > 0 && (
          <div className="note warn"><TriangleAlert /><span>
            {t("这几台主机上没看到 {agent}：{hosts}。可以先在应用目录里装上并配置好，否则那几台会执行失败。", { agent: agents.find((a) => a.id === agent)!.title, hosts: missing.join("、") })}
            {" "}<Link to={`/apps?install=${agents.find((a) => a.id === agent)!.app}`}>{t("去安装")}</Link>
          </span></div>
        )}
        {preview && <PreviewPanel preview={preview} />}
        <div className="row">
          <span className="muted" style={{ fontSize: 12.5 }}>{t("提交后变成一个请求，批准后才会运行；每台主机的输出实时显示在请求页上。")}</span>
          <span className="grow" />
          <button className="btn" disabled={!ready} onClick={check}><Eye />{t("预览判级")}</button>
          <button className="btn primary" disabled={!ready} onClick={submit}><Play />{t("提交任务")}</button>
        </div>
      </div>
    </div>
  );
}

// ---------- 最近的任务 ----------

function TargetTail({ id, host, fallback }: { id: string; host: string; fallback: string }) {
  const streamed = useOutput(id, host);
  const text = streamed ?? fallback;
  return <pre className="term agent-out">{text.split("\n").slice(-40).join("\n")}</pre>;
}

function TaskCard({ r }: { r: Req }) {
  const t = useT();
  const nav = useNavigate();
  const [open, setOpen] = useState(r.state === "running");
  const [full, setFull] = useState<Req | null>(null);
  const [summing, setSumming] = useState(false);
  const m = agentRe.exec(r.snippet ?? "");
  const meta = agents.find((a) => a.id === m?.[2]);
  useEffect(() => { if (open && r.state !== "running" && r.state !== "pending") api.request(r.id).then(setFull).catch(() => {}); }, [open, r.id, r.state]);
  const targets = full?.targets ?? r.targets;
  return (
    <div className={"agent-task" + (r.state === "running" ? " running" : "")}>
      <div className="row wrap" style={{ gap: 8 }}>
        {meta && <span className="agent-face sm" style={{ background: meta.color }}>{meta.letter}</span>}
        <Link to={`/requests/${r.id}`} className="ellipsis grow" style={{ fontWeight: 560, minWidth: 0 }}>{ts(r.intent)}</Link>
        <LevelBadge level={r.level} />
        <span className="muted" style={{ fontSize: 12 }}>{ago(r.createdAt)}</span>
        {m && !m[1] && (r.state === "done" || r.state === "failed") && <button className="btn sm" onClick={() => setSumming(true)}><Sigma />{t("汇总")}</button>}
        <button className="btn ghost icon sm" onClick={() => setOpen((v) => !v)} title={open ? t("收起") : t("并排看结果")} aria-label={open ? t("收起") : t("并排看结果")}>{open ? <ChevronUp /> : <Columns3 />}</button>
      </div>
      <div className="agent-chips">
        {r.targets.map((tg) => (
          <span key={tg.host} className={"agent-chip " + tg.state} title={t(stateText[tg.state] ?? tg.state)}>
            {tg.state === "running" ? <span className="think-dots" aria-hidden><i /><i /><i /></span> : tg.state === "ok" ? <Check /> : tg.state === "failed" || tg.state === "error" ? <TriangleAlert /> : null}
            <span className="mono">{tg.host}</span>
          </span>
        ))}
        {r.state === "pending" && <button className="btn sm" onClick={() => nav(`/requests/${r.id}`, { state: backAfterApproval })}>{t("去批准")}</button>}
      </div>
      {open && (
        <div className="agent-compare">
          {targets.map((tg) => (
            <div key={tg.host} className="agent-col">
              <div className="row" style={{ gap: 6 }}><b className="mono">{tg.host}</b><span className="muted" style={{ fontSize: 12 }}>{t(stateText[tg.state] ?? tg.state)}</span></div>
              <TargetTail id={r.id} host={tg.host} fallback={tg.output ?? ""} />
            </div>
          ))}
        </div>
      )}
      {summing && <SummaryDialog r={r} onClose={() => setSumming(false)} />}
    </div>
  );
}

const stateText: Record<string, string> = { waiting: "排队中", running: "思考中", ok: "完成", failed: "失败", error: "出错", skipped: "跳过" };

function SummaryDialog({ r, onClose }: { r: Req; onClose: () => void }) {
  const t = useT();
  const nav = useNavigate();
  const [host, setHost] = useState(r.targets[0]?.host ?? "");
  const [agent, setAgent] = useState<AgentID>("claude");
  const [prompt, setPrompt] = useState(() => t("下面是同一个任务在几台主机上分别执行的结果，按主机分段。请对比各台的结论：哪些一致、哪些不一致、需要人注意什么，最后给出简短的建议。"));
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    try {
      const res = await api.agentSummary({ from: r.id, host, agent, prompt: prompt.trim() });
      const reqs = (res as { requests?: Req[] }).requests ?? [];
      if (reqs[0]) nav(`/requests/${reqs[0].id}`, { state: backAfterApproval });
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <Modal title={t("汇总各台主机的结果")} onClose={onClose}
      footer={<><button className="btn" onClick={onClose}>{t("取消")}</button><button className="btn primary" disabled={busy || !prompt.trim() || !host} onClick={submit}><Sigma />{t("提交汇总")}</button></>}>
      <div className="col" style={{ gap: 14 }}>
        <div className="hint">{t("各台主机的输出会拼成一份材料，从标准输入交给选中的助手；这也是一个请求，批准后才运行。")}</div>
        <div className="form-grid">
          <div className="field">
            <label htmlFor="sum-host">{t("在哪台主机上汇总")}</label>
            <select id="sum-host" className="input" value={host} onChange={(e) => setHost(e.target.value)}>{r.targets.map((tg) => <option key={tg.host} value={tg.host}>{tg.host}</option>)}</select>
          </div>
          <div className="field">
            <label htmlFor="sum-agent">{t("用哪个助手")}</label>
            <select id="sum-agent" className="input" value={agent} onChange={(e) => setAgent(e.target.value as AgentID)}>{agents.filter((a) => a.summary).map((a) => <option key={a.id} value={a.id}>{a.title}</option>)}</select>
          </div>
        </div>
        <div className="field">
          <label htmlFor="sum-prompt">{t("要它做什么")}</label>
          <textarea id="sum-prompt" className="textarea" rows={4} maxLength={4000} value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        </div>
      </div>
    </Modal>
  );
}

// ---------- 各主机的助手 ----------

function CrewTable({ crews, loading }: { crews: Record<string, HostCrew>; loading: boolean }) {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const requests = useStore((s) => s.requests);
  const tasks = requests.filter(isAgentReq);
  return (
    <div className="card">
      <div className="card-head"><h2>{t("各主机的助手和网关")}</h2><span className="muted" style={{ fontSize: 12.5 }}>{t("装在哪、配的哪家模型、经由哪台网关、最近干了多少活")}</span></div>
      <div className="table-wrap">
        <table className="table">
          <thead><tr><th>{t("主机")}</th><th>{t("助手")}</th><th>{t("模型和接口")}</th><th>{t("网关")}</th><th style={{ textAlign: "right" }}>{t("最近的任务")}</th></tr></thead>
          <tbody>
            {hosts.map((h) => {
              const c = crews[h.name];
              const mine = tasks.filter((r) => r.targets.some((tg) => tg.host === h.name));
              return (
                <tr key={h.name}>
                  <td className="mono"><Link to={`/hosts/${encodeURIComponent(h.name)}?tab=env`}>{h.name}</Link></td>
                  <td>{c?.agents.length ? <div className="row wrap" style={{ gap: 4 }}>{c.agents.map((a) => { const m = agents.find((g) => g.id === a.id)!; return <span key={a.id} className="badge neutral"><span className="agent-face xs" style={{ background: m.color }}>{m.letter}</span>{m.title}{a.version ? " " + a.version : ""}</span>; })}</div> : <span className="muted">{loading && !c ? t("加载中…") : t("没有")}</span>}</td>
                  <td>{c?.agents.filter((a) => a.provider || a.model).map((a) => <div key={a.id} className="muted" style={{ fontSize: 12.5 }}>{agents.find((g) => g.id === a.id)!.title}{t("：")}{[a.provider, a.model].filter(Boolean).join(" · ")}{a.baseUrl && <span className="mono"> → {a.baseUrl}</span>}</div>)}</td>
                  <td>{c?.gateways.length ? c.gateways.map((g) => <span key={g.instance} className={"badge " + (g.running ? "ok" : "neutral")} style={{ marginRight: 4 }}>{gateways[g.app]?.title ?? g.app}{g.port ? ":" + g.port : ""}</span>) : <span className="muted">—</span>}</td>
                  <td className="num" style={{ textAlign: "right" }}>{mine.length || <span className="muted">0</span>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="card-body hint" style={{ paddingTop: 10 }}>{t("token 用量和花费在网关自己的管理界面里看：让各台主机上的助手都经由同一台 LiteLLM 或 New API，就能按密钥、按模型统计。")}</div>
    </div>
  );
}

export function AgentsPage() {
  const t = useT();
  const hosts = useStore((s) => s.hosts);
  const requests = useStore((s) => s.requests);
  const names = hosts.map((h) => h.name).join(",");
  const [installed, setInstalled] = useState<HostApps[] | null>(() => cached<HostApps[]>("apps:installed") ?? null);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    if (!names) return;
    let on = true;
    setLoading(true);
    api.appsInstalledAll(names.split(",")).then((r) => { if (on) setInstalled(remember("apps:installed", r)); }).catch(() => {}).finally(() => on && setLoading(false));
    return () => { on = false; };
  }, [names]);
  // 只跟着主机名单和“已安装”变：主机的指标每隔几秒推一次，不必每次都重算。
  const crews = useMemo(() => {
    const m: Record<string, HostCrew> = {};
    for (const name of names ? names.split(",") : []) {
      const inst = installed?.find((x) => x.host === name);
      const env = cached<HostEnv>("env:" + name);
      if (inst || env) m[name] = crewOf(inst?.installed, env);
    }
    return m;
  }, [names, installed]);
  const tasks = requests.filter(isAgentReq).slice(0, 12);
  const [showAll, setShowAll] = useState(false);
  return (
    <>
      <div className="topbar"><h1>{t("Agent 协作")}</h1></div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("把同一个任务交给多台主机上的 AI 编程助手，并排比较它们的结论，再让其中一个汇总。上面的协作图实时显示塔台在给哪台主机下发什么、哪个助手正在思考、助手经由哪台网关调用模型。")}</p>
          <div className="card"><div className="card-body"><CrewMap crews={crews} /></div></div>
          <TaskForm crews={crews} />
          <div className="card">
            <div className="card-head"><h2>{t("最近的 Agent 任务")}</h2><span className="muted num">{tasks.length || ""}</span></div>
            {tasks.length === 0 ? <Empty icon={<Bot />} title={t("还没有 Agent 任务")}>{t("在上面派一个任务试试：先只读，看看各台主机上的助手怎么说。")}</Empty> : (
              <div className="card-body col" style={{ gap: 10 }}>
                {(showAll ? tasks : tasks.slice(0, 5)).map((r) => <TaskCard key={r.id} r={r} />)}
                {tasks.length > 5 && <button className="btn ghost sm" onClick={() => setShowAll((v) => !v)}>{showAll ? <><ChevronUp />{t("收起")}</> : <><ChevronDown />{t("显示全部 {n} 个", { n: tasks.length })}</>}</button>}
              </div>
            )}
          </div>
          <CrewTable crews={crews} loading={loading} />
        </div>
      </div>
    </>
  );
}
