import { Suspense, lazy, useEffect, useState } from "react";
import { Link, NavLink, Navigate, Route, Routes, useLocation } from "react-router-dom";
import {
  Activity, Box, Ellipsis, Gauge, LayoutDashboard, KeyRound, CircleAlert, CircleCheck, FileCode2, LayoutGrid, LogOut, OctagonPause, ScrollText,
  Settings as SettingsIcon, ShieldCheck, SquareTerminal, TriangleAlert,
} from "lucide-react";
import { api } from "./api";
import { connect, disconnect, fail, loadBoot, refreshHosts, refreshRequests, useStore } from "./store";
import { VerifyHost } from "./verify";
import { DialogHost } from "./dialogs";
import { Login } from "./pages/Login";
import { Overview } from "./pages/Overview";
import { HostDetail } from "./pages/HostDetail";
import { Run } from "./pages/Run";
import { Approvals, ActivityPage, RequestDetail } from "./pages/Requests";
import { Snippets } from "./pages/Snippets";
import { AuditPage } from "./pages/Audit";
import { SettingsPage } from "./pages/Settings";
import { Containers } from "./pages/Containers";
import { Credentials } from "./pages/Credentials";
import { SpeedtestPage } from "./pages/Speedtest";
import { AppsPage } from "./pages/Apps";
// 终端页带着一个不小的终端渲染库，按需加载。
const TerminalPage = lazy(() => import("./pages/Terminal").then((m) => ({ default: m.TerminalPage })));
import { stamp, uptime } from "./ui";
import { Backdrop, LangSwitch, Logo, ThemeSwitch } from "./fx";
import { timeZone, timeZoneOffset, useT } from "./i18n";

export function App() {
  const t = useT();
  const boot = useStore((s) => s.boot);
  const [error, setError] = useState("");

  useEffect(() => {
    loadBoot().catch((e) => setError(e.message));
    const onUnauthorized = () => { disconnect(); loadBoot().catch(() => {}); };
    window.addEventListener("hostower:unauthorized", onUnauthorized);
    return () => window.removeEventListener("hostower:unauthorized", onUnauthorized);
  }, []);

  const authed = !!boot?.user;
  useEffect(() => {
    if (!authed) return;
    refreshHosts().catch(fail);
    refreshRequests().catch(fail);
    connect();
    return disconnect;
  }, [authed]);

  return (
    <>
      <Backdrop />
      {error ? <div className="auth"><div className="card auth-card"><h1>{t("无法连接塔台")}</h1><p className="dim">{error}</p></div></div>
        : !boot ? <div className="auth" />
        : !boot.user ? <Login boot={boot} />
        : <Shell />}
    </>
  );
}

const nav = [
  // main：手机底栏上直接显示的几项；其余的收进“更多”里（底栏放不下十一个）。
  { to: "/", label: "总览", icon: LayoutGrid, end: true, main: true },
  { to: "/run", label: "执行", icon: SquareTerminal, main: true },
  { to: "/approvals", label: "审批", icon: ShieldCheck, count: true, main: true },
  { to: "/activity", label: "活动", icon: Activity },
  { to: "/containers", label: "容器", icon: Box },
  { to: "/apps", label: "应用", icon: LayoutDashboard, main: true },
  { to: "/speedtest", label: "测速", icon: Gauge },
  { to: "/snippets", label: "Snippet", icon: FileCode2 },
  { to: "/credentials", label: "凭据", icon: KeyRound },
  { to: "/audit", label: "审计", icon: ScrollText },
];

function Shell() {
  const t = useT();
  const boot = useStore((s) => s.boot)!;
  const live = useStore((s) => s.live);
  const paused = useStore((s) => s.paused);
  const pending = useStore((s) => s.requests.filter((r) => r.state === "pending").length);
  const loc = useLocation();
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 30_000); return () => clearInterval(timer); }, []);
  // 手机上的“更多”：底栏放不下的页面。换页面就收起来。
  const [more, setMore] = useState(false);
  useEffect(() => { setMore(false); }, [loc.pathname]);
  const secondary = [...nav.filter((n) => !n.main), { to: "/settings", label: "设置", icon: SettingsIcon }];
  const inMore = secondary.some((n) => loc.pathname.startsWith(n.to));
  const logout = async () => {
    await api.logout().catch(() => {});
    disconnect();
    loadBoot().catch(() => {});
  };

  return (
    <div className="shell">
      <aside className="side">
        <div className="brand">
          <Logo size={28} />
          <span className="brand-name">Hostower</span>
        </div>
        {nav.map((n) => (
          <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => "nav-item" + (n.main ? "" : " nav-sec") + (isActive || (n.to === "/" && loc.pathname.startsWith("/hosts")) ? " active" : "")}>
            <n.icon />
            <span className="t">{t(n.label)}</span>
            {n.count && pending > 0 && <span className="nav-count" aria-label={t("{n} 个待审批", { n: pending })}>{pending}</span>}
          </NavLink>
        ))}
        <div className="nav-label">{t("系统")}</div>
        <NavLink to="/settings" className={({ isActive }) => "nav-item nav-sec" + (isActive ? " active" : "")}>
          <SettingsIcon /><span className="t">{t("设置")}</span>
        </NavLink>
        <button type="button" className={"nav-item nav-more" + (inMore || more ? " active" : "")} aria-expanded={more} aria-haspopup="menu" onClick={() => setMore((v) => !v)}>
          <Ellipsis /><span className="t">{t("更多")}</span>
        </button>
        <div className="side-foot">
          <div className="row" style={{ alignItems: "flex-start", gap: 2 }}>
            <SideClock />
            <span className="grow" />
            <LangSwitch placement="top" />
            <ThemeSwitch placement="top" />
          </div>
          {/* 连接状态和运行时长合成一行；版本号只在正式版时显示，本地构建的版本名（比如 local）放进提示里。 */}
          <div className="conn" title={(live ? t("实时通道已连接") : t("实时通道断开，正在重连")) + "\n" + t("塔台版本 {version}，启动于 {time}", { version: boot.version, time: stamp(boot.started) })}>
            <span className={"dot " + (live ? "ok" : "warn pulse")} />
            <span>{live ? t("实时连接") : t("正在重连…")}</span>
            <span className="grow" />
            <span>{t("已运行 {time}", { time: uptime(Math.max(60, (now - boot.started) / 1000)) })}</span>
          </div>
          <div className="row" style={{ padding: "0 2px", gap: 2 }}>
            <div className="grow ellipsis" title={boot.user!.name}>
              <div className="ellipsis" style={{ fontWeight: 580 }}>{boot.user!.name}</div>
              {/^v?\d/.test(boot.version) && <div className="muted mono ellipsis" style={{ fontSize: 12 }}>{boot.version.startsWith("v") ? boot.version : "v" + boot.version}</div>}
            </div>
            <button className="btn ghost icon sm" onClick={logout} title={t("退出登录")} aria-label={t("退出登录")}><LogOut /></button>
          </div>
        </div>
      </aside>
      {more && (
        <div className="more-overlay" onClick={() => setMore(false)}>
          <div className="more-sheet" role="menu" aria-label={t("更多")} onClick={(e) => e.stopPropagation()}>
            {secondary.map((n) => (
              <NavLink key={n.to} to={n.to} role="menuitem" className={({ isActive }) => "more-item" + (isActive ? " active" : "")}>
                <n.icon /><span>{t(n.label)}</span>
              </NavLink>
            ))}
            <button type="button" className="more-item" role="menuitem" onClick={logout}><LogOut /><span>{t("退出登录")}</span></button>
          </div>
        </div>
      )}
      <div className="main">
        <div className="mobile-head">
          <Logo size={26} />
          <span className="brand-name">Hostower</span>
          <span className={"dot " + (live ? "ok" : "warn")} title={live ? t("实时连接") : t("正在重连")} />
          <span className="grow" />
          <MobileClock />
          <LangSwitch placement="bottom" />
          <ThemeSwitch placement="bottom" />
          <button className="btn ghost icon sm" onClick={logout} title={t("退出登录")} aria-label={t("退出登录")}><LogOut /></button>
        </div>
        {paused && (
          <div className="banner crit" role="alert">
            <OctagonPause />
            <b>{t("全部执行已暂停。")}</b>{t("新的操作、审批和指标采样都已停止，直到所有者在“设置”里恢复。")}
          </div>
        )}
        {(boot.initialPassword || boot.defaultUsername) && (
          <div className="banner warn" role="alert">
            <TriangleAlert />
            <span>
              {boot.initialPassword && <><b>{t("正在使用初始密码。")}</b>{boot.secondFactor ? t("请修改密码。") : t("请修改密码，并添加 Passkey 或动态码。")} </>}
              {boot.defaultUsername && <><b>{t("还在用默认用户名 {name}。", { name: boot.user?.name ?? "" })}</b>{t("改成别人猜不到的名字，暴力破解就要同时猜对用户名和密码。")} </>}
              <Link to="/settings?tab=security">{t("去设置")}</Link>
            </span>
          </div>
        )}
        {boot.dev && (
          <div className="banner warn">
            <TriangleAlert />
            <span><b>{t("开发模式")}</b>{boot.devSession ? t("：主机是模拟的，当前会话用启动令牌登录、未经 Passkey。") : t("：主机是模拟的。")}{t("不要在真实环境使用")} <span className="mono">--dev</span>{t("。")}</span>
          </div>
        )}
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/hosts/:name" element={<HostDetail />} />
          <Route path="/hosts/:name/terminal" element={<Suspense fallback={<div className="topbar"><h1>{t("终端")}</h1></div>}><TerminalPage /></Suspense>} />
          <Route path="/run" element={<Run />} />
          <Route path="/approvals" element={<Approvals />} />
          <Route path="/activity" element={<ActivityPage />} />
          <Route path="/requests/:id" element={<RequestDetail />} />
          <Route path="/containers" element={<Containers />} />
          <Route path="/apps" element={<AppsPage />} />
          <Route path="/speedtest" element={<SpeedtestPage />} />
          <Route path="/credentials" element={<Credentials />} />
          <Route path="/snippets" element={<Snippets />} />
          <Route path="/snippets/:name" element={<Snippets />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </div>
      <Toasts />
      <DialogHost />
      <VerifyHost />
    </div>
  );
}

/** 当前时间：按设置里选的时区显示，每秒走一次。悬停看时区。 */
function SideClock() {
  const t = useT();
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(id); }, []);
  const full = stamp(now); // YYYY-MM-DD HH:mm:ss
  const tz = timeZone();
  return (
    <div className="side-clock" title={t("时区：{tz}（{offset}），可以在“设置 → 通用 → 界面”里改", { tz, offset: timeZoneOffset() })}>
      <span className="side-clock-time num">{full.slice(11)}</span>
      <span className="side-clock-date num">{full.slice(0, 10)} · {timeZoneOffset().replace(":00", "")}</span>
    </div>
  );
}

/** 手机顶栏上的时钟：只显示时和分。 */
function MobileClock() {
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), 10_000); return () => clearInterval(id); }, []);
  return <span className="num muted" style={{ fontSize: 13, fontWeight: 600 }} title={stamp(now)}>{stamp(now).slice(11, 16)}</span>;
}

function Toasts() {
  const toasts = useStore((s) => s.toasts);
  return (
    <div className="toasts" aria-live="polite">
      {toasts.map((x) => (
        <div key={x.id} className={"toast " + x.kind}>
          {x.kind === "err" ? <CircleAlert /> : <CircleCheck />}
          <span>{x.text}</span>
        </div>
      ))}
    </div>
  );
}
