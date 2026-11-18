// 主机的交互终端。打开终端是高危操作：先当场确认，确认后才连上；整段会话会被录制。
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, ChevronRight, Download, Highlighter, KeyRound, Power, RotateCw, SquareTerminal, TriangleAlert, Video } from "lucide-react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { api, type Req } from "../api";
import { toast, useStore } from "../store";
import { useResolvedTheme } from "../theme";
import { ts, useT } from "../i18n";
import { ApprovalCard } from "./Requests";
import { createHighlighter } from "../termHighlight";

const HL_KEY = "hostower.term.highlight";
const loadHL = () => { try { return localStorage.getItem(HL_KEY) !== "0"; } catch { return true; } };

const themes = {
  dark: { background: "#05070c", foreground: "#d6deee", cursor: "#74b4ff", selectionBackground: "#2a3b63",
    black: "#1b2233", red: "#ff6478", green: "#3fdc9b", yellow: "#f2b84b", blue: "#74b4ff", magenta: "#9d7dff", cyan: "#3fe3d0", white: "#d6deee",
    brightBlack: "#5d6884", brightRed: "#ff8b9a", brightGreen: "#6ee7b4", brightYellow: "#f7cd7c", brightBlue: "#9cc9ff", brightMagenta: "#bba4ff", brightCyan: "#7df0e2", brightWhite: "#ffffff" },
  light: { background: "#fbfcfe", foreground: "#1b2233", cursor: "#2f5fe0", selectionBackground: "#cfdcff",
    black: "#1b2233", red: "#d12c45", green: "#0c8f5c", yellow: "#a86a00", blue: "#2f5fe0", magenta: "#6d3fe6", cyan: "#0a8f88", white: "#7d87a0",
    brightBlack: "#49546f", brightRed: "#e0455d", brightGreen: "#11a56b", brightYellow: "#bf7c00", brightBlue: "#4a76ee", brightMagenta: "#8259f0", brightCyan: "#0ca39a", brightWhite: "#0e1426" },
};

type Phase = "requesting" | "confirm" | "connecting" | "open" | "closed" | "error";

export function TerminalPage() {
  const t = useT();
  const { name = "" } = useParams();
  // ?login=<应用名>：订阅登录。登录命令按应用名从内置目录里查（不接受地址栏里写的任意命令），连上后预先填进终端，回车由人来按。
  const [sp] = useSearchParams();
  const loginApp = sp.get("login") ?? "";
  const [login, setLogin] = useState<{ title: string; command: string; hint?: string } | null>(null);
  useEffect(() => {
    if (!loginApp) return;
    api.apps().then((c) => {
      const a = c.apps.find((x) => x.name === loginApp);
      if (a?.ai?.login) setLogin({ title: a.title, command: a.ai.login, hint: a.ai.loginHint });
    }).catch(() => {});
  }, [loginApp]);
  const loginRef = useRef<string>("");
  loginRef.current = login?.command ?? "";
  const host = useStore((s) => s.hosts.find((h) => h.name === name));
  const paused = useStore((s) => s.paused);
  const [req, setReq] = useState<Req | null>(null);
  const live = useStore((s) => s.requests.find((r) => r.id === req?.id));
  const [phase, setPhase] = useState<Phase>("requesting");
  const [message, setMessage] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const theme = useResolvedTheme();
  const term = useRef<XTerm | null>(null);
  const sock = useRef<WebSocket | null>(null);
  const ended = useRef(false); // 是用户自己点了“结束会话”
  // 关键词高亮：只影响页面上的显示。用 ref 让它在会话中途也能开关。
  const [highlight, setHighlight] = useState(loadHL);
  const hl = useRef(highlight);
  const toggleHL = () => setHighlight((v) => { hl.current = !v; try { localStorage.setItem(HL_KEY, !v ? "1" : "0"); } catch { /* 存不了就只在这次生效 */ } return !v; });
  const end = () => { ended.current = true; sock.current?.close(1000); };

  // 进入页面就提交一个“开终端”的请求；它要当场确认之后才生效。
  const request = useCallback(() => {
    setPhase("requesting");
    setMessage("");
    setReq(null);
    api.openTerminal(name).then((r) => { setReq(r); setPhase("confirm"); }).catch((e) => { setMessage(e.message); setPhase("error"); });
  }, [name]);
  useEffect(() => { request(); }, [request]);

  // 请求被批准（或被拒绝、过期）后切换阶段。
  const state = live?.state ?? req?.state;
  useEffect(() => {
    if (phase !== "confirm" || !state) return;
    if (state === "running") setPhase("connecting");
    else if (state !== "pending") { setMessage(state === "denied" ? t("已取消。") : state === "expired" ? t("确认超时了。") : t("请求没有通过。")); setPhase("error"); }
  }, [state, phase, t]);

  // 连上终端。
  useEffect(() => {
    if (phase !== "connecting" || !req || !box.current) return;
    const xt = new XTerm({
      fontFamily: '"Geist Mono Variable", "Noto Sans SC Variable", ui-monospace, Consolas, monospace', fontSize: 13.5, lineHeight: 1.25,
      cursorBlink: true, scrollback: 10000, theme: themes[theme], allowProposedApi: false,
    });
    const fit = new FitAddon();
    xt.loadAddon(fit);
    xt.open(box.current);
    // 用 WebGL 绘制：不依赖运行时插入的内联样式，和严格的内容安全策略兼容。
    try { xt.loadAddon(new WebglAddon()); } catch { /* 不支持 WebGL 时退回默认绘制 */ }
    fit.fit();
    term.current = xt;

    // 复制和粘贴：拖动选中后松开鼠标就复制；有选区时 Ctrl+C 是复制而不是中断；Ctrl+V 交给浏览器粘贴。
    const copy = () => {
      const text = xt.getSelection();
      if (!text) return;
      const done = () => toast(t("已复制 {n} 个字符", { n: text.length }));
      // 浏览器不给剪贴板权限时，退回到老办法：借一个看不见的文本框复制。
      const fallback = () => {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.setAttribute("readonly", "");
        ta.className = "clip-helper";
        document.body.appendChild(ta);
        ta.select();
        let ok = false;
        try { ok = document.execCommand("copy"); } catch { ok = false; }
        ta.remove();
        xt.focus();
        if (ok) done(); else toast(t("没能自动复制，请用右键菜单里的“复制”"));
      };
      if (navigator.clipboard?.writeText) navigator.clipboard.writeText(text).then(done).catch(fallback); else fallback();
    };
    const onMouseUp = () => setTimeout(copy, 0);
    box.current.addEventListener("mouseup", onMouseUp);
    xt.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown" || !(e.ctrlKey || e.metaKey) || e.altKey) return true;
      const k = e.key.toLowerCase();
      if (k === "c" && xt.hasSelection()) { copy(); xt.clearSelection(); return false; }
      if (k === "v") return false;
      return true;
    });
    const host = box.current;

    const ws = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/v1/terminal/${req.id}/ws?cols=${xt.cols}&rows=${xt.rows}`);
    ws.binaryType = "arraybuffer";
    sock.current = ws;
    ended.current = false;
    const enc = new TextEncoder();
    const dec = new TextDecoder("utf-8");
    const paint = createHighlighter();
    ws.onopen = () => {
      setPhase("open");
      xt.focus();
      // 等提示符出来再填登录命令（不带回车）。
      if (loginRef.current) setTimeout(() => { if (ws.readyState === WebSocket.OPEN && loginRef.current) ws.send(enc.encode(loginRef.current)); }, 900);
    };
    ws.onmessage = (e) => {
      if (!(e.data instanceof ArrayBuffer)) return;
      // 高亮器要一直跟着输出走（它记着当前有没有颜色、是不是全屏程序），关掉高亮时只是不用它的结果。
      const text = dec.decode(new Uint8Array(e.data), { stream: true });
      const painted = paint(text);
      xt.write(hl.current ? painted : text);
    };
    ws.onclose = (e) => { setMessage(ended.current ? t("你结束了会话。") : e.reason || t("连接已断开。")); setPhase((p) => (p === "open" || p === "connecting" ? "closed" : p)); };
    const onData = xt.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
    const onResize = xt.onResize(({ cols, rows }) => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ cols, rows })));
    // 尺寸变了之后要整屏重绘一次：否则 WebGL 画布会留白，直到下一次有输出或点击。
    const ro = new ResizeObserver(() => { try { fit.fit(); requestAnimationFrame(() => xt.refresh(0, xt.rows - 1)); } catch { /* 容器还没有尺寸 */ } });
    ro.observe(box.current);
    return () => { host.removeEventListener("mouseup", onMouseUp); ro.disconnect(); onData.dispose(); onResize.dispose(); ws.close(); xt.dispose(); term.current = null; sock.current = null; };
    // 主题变化由下面单独处理，不重连。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase === "connecting" || phase === "open" || phase === "closed", req?.id]);

  useEffect(() => { if (term.current) term.current.options.theme = themes[theme]; }, [theme]);

  const showTerm = phase === "connecting" || phase === "open" || phase === "closed";
  return (
    <>
      <div className="topbar">
        <Link to="/" className="crumb hide-sm">{t("总览")}</Link>
        <ChevronRight size={14} className="crumb-sep hide-sm" />
        <Link to={`/hosts/${name}`} className="crumb">{name}</Link>
        <ChevronRight size={14} className="crumb-sep" />
        <h1>{t("终端")}</h1>
        {phase === "open" && <span className="badge ok"><Video />{t("录制中")}</span>}
        {phase === "connecting" && <span className="badge neutral">{t("连接中…")}</span>}
        {phase === "closed" && <span className="badge neutral">{t("已结束")}</span>}
        <span className="grow" />
        {showTerm && <span className="muted hide-sm" style={{ fontSize: 12.5 }}>{t("选中即复制 · Ctrl+V 粘贴 · 滚轮翻看历史")}</span>}
        {showTerm && <button className={"btn" + (highlight ? " active" : "")} aria-pressed={highlight} onClick={toggleHL} title={t("给输出里的错误、警告、成功、地址等关键内容上色（只影响显示，不影响录像）")}><Highlighter /><span className="hide-sm">{t("高亮")}</span></button>}
        {(phase === "open" || phase === "connecting") && <button className="btn danger" onClick={end} title={t("断开并结束这次会话；也可以在终端里输入 exit")}><Power /><span className="hide-sm">{t("结束会话")}</span></button>}
        {(phase === "closed" || phase === "error") && <Link className="btn" to={`/hosts/${name}`}><ArrowLeft /><span className="hide-sm">{t("返回主机")}</span></Link>}
        {(phase === "closed" || phase === "error") && <button className="btn primary" onClick={request} disabled={paused}><RotateCw />{t("重新打开")}</button>}
        {req && phase === "closed" && <a className="btn" href={`/api/v1/terminal/${req.id}/recording`}><Download /><span className="hide-sm">{t("下载录像")}</span></a>}
      </div>
      <div className="content term-content">
        {phase === "requesting" && <div className="card"><div className="empty sm"><span>{t("正在准备…")}</span></div></div>}
        {phase === "error" && (
          <div className="card"><div className="empty"><TriangleAlert /><strong>{t("终端没有打开")}</strong><span>{ts(message)}</span></div></div>
        )}
        {phase === "confirm" && req && (
          <div className="page narrow">
            <ApprovalCard r={live ?? req} title={t("打开 {name} 的终端", { name })}
              lead={<>{t("终端里敲的命令")}<b>{t("不会逐条判级")}</b>{t("，包括平时会被禁止的命令，所以打开终端按高危操作处理。整段会话会被录制并写入审计")}{host?.sensitivity === "high" ? t("；这是一台高敏感主机") : ""}{t("。")}</>} />
          </div>
        )}
        {login && showTerm && (
          <div className="note" style={{ margin: "0 0 10px" }}><KeyRound /><span>
            {t("订阅登录 {app}：终端里已经填好 ", { app: login.title })}<span className="mono">{login.command}</span>{t("，按回车开始。")}{login.hint ? " " + ts(login.hint) : ""}
            {" "}{t("登录完成后输入 exit 结束会话。")}
          </span></div>
        )}
        {showTerm && (
          <div className="term-frame">
            <div ref={box} className="term-box" />
            {phase === "closed" && <div className="term-closed"><SquareTerminal size={16} />{t("会话已结束：{message}　录像已保存，可以在右上角下载，或到“活动”里查看这次会话。", { message: ts(message) })}</div>}
          </div>
        )}
      </div>
    </>
  );
}
