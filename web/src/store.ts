// 全局状态：主机、请求和实时事件。一条 WebSocket 多路复用所有实时数据。
import { useSyncExternalStore } from "react";
import { api, type Bootstrap, type Group, type Host, type HostStatus, type Req } from "./api";
import { clearCache } from "./cache";

export interface Toast { id: number; kind: "ok" | "err"; text: string }

interface State {
  boot: Bootstrap | null;
  hosts: Host[];
  groups: Group[];
  hostsLoaded: boolean;
  requests: Req[];
  paused: boolean;
  live: boolean;
  toasts: Toast[];
}

let state: State = { boot: null, hosts: [], groups: [], hostsLoaded: false, requests: [], paused: false, live: false, toasts: [] };
const listeners = new Set<() => void>();

function set(patch: Partial<State>) {
  state = { ...state, ...patch };
  listeners.forEach((l) => l());
}

export function useStore<T>(selector: (s: State) => T): T {
  return useSyncExternalStore(
    (l) => { listeners.add(l); return () => listeners.delete(l); },
    () => selector(state),
  );
}

export const getState = () => state;

// ---- 流式输出：按 请求/主机 累积，单独订阅，避免每个分片都重渲染整页 ----
const outputs = new Map<string, string>();
const outputListeners = new Set<() => void>();
let outputVersion = 0;
const key = (id: string, host: string) => id + "\u0000" + host;

export function useOutput(id: string, host: string): string | undefined {
  useSyncExternalStore((l) => { outputListeners.add(l); return () => outputListeners.delete(l); }, () => outputVersion);
  return outputs.get(key(id, host));
}

export function seedOutput(req: Req) {
  for (const t of req.targets) {
    const k = key(req.id, t.host);
    if ((outputs.get(k)?.length ?? 0) < t.output.length) outputs.set(k, t.output);
  }
  outputVersion++;
  outputListeners.forEach((l) => l());
}

let toastId = 0;
export function toast(text: string, kind: "ok" | "err" = "ok") {
  // 同一条提示已经在显示：不再叠一条（断网时每个请求都会报“无法连接塔台”）。
  if (state.toasts.some((t) => t.text === text && t.kind === kind)) return;
  const id = ++toastId;
  set({ toasts: [...state.toasts, { id, kind, text }] });
  setTimeout(() => set({ toasts: state.toasts.filter((t) => t.id !== id) }), kind === "err" ? 6000 : 3500);
}

// 用户自己关掉了验证框：不算出错，不弹提示。
export const fail = (e: unknown) => { if (e instanceof Error && e.name === "VerifyCancelled") return; toast(e instanceof Error ? e.message : String(e), "err"); };

export async function loadBoot() {
  const boot = await api.bootstrap();
  set({ boot, paused: !!boot.paused });
  return boot;
}

export async function refreshHosts() {
  const { hosts, groups } = await api.hosts();
  set({ hosts, groups, hostsLoaded: true });
}

export async function refreshRequests() {
  const { requests } = await api.requests();
  set({ requests });
}

export function upsertRequest(r: Req) {
  const i = state.requests.findIndex((x) => x.id === r.id);
  const next = [...state.requests];
  if (i >= 0) next[i] = { ...r, targets: r.targets.map((t, j) => ({ ...t, output: next[i].targets[j]?.output ?? "" })) };
  else next.unshift(r);
  set({ requests: next });
}

export const setPaused = (paused: boolean) => set({ paused });

let ws: WebSocket | null = null;
let retry = 0;
let stopped = true;
let timer: ReturnType<typeof setTimeout> | undefined;

// 电脑睡眠唤醒、网络恢复、切回这个页面时：不等退避的定时器，马上重连一次。
function reconnectNow() {
  if (stopped || ws || (typeof document !== "undefined" && document.hidden)) return;
  clearTimeout(timer);
  retry = 0;
  connect();
}
if (typeof window !== "undefined") {
  window.addEventListener("online", reconnectNow);
  document.addEventListener("visibilitychange", reconnectNow);
}

export function connect() {
  stopped = false;
  if (ws) return;
  const url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/api/v1/stream";
  const sock = new WebSocket(url);
  ws = sock;
  sock.onopen = () => {
    retry = 0;
    set({ live: true });
    // 断线期间可能漏了事件，重连后整体刷新一次。
    refreshHosts().catch(() => {});
    refreshRequests().catch(() => {});
    // 实时连接通了说明塔台又能连上：让各个定时刷新的卡片立即补一次，清掉断网期间的报错（见 ui.tsx 的 poll）。
    window.dispatchEvent(new Event("hostower:reconnected"));
  };
  sock.onmessage = (m) => {
    let ev: { type: string; data: unknown };
    try { ev = JSON.parse(m.data); } catch { return; }
    switch (ev.type) {
      case "request":
        upsertRequest(ev.data as Req);
        break;
      case "output": {
        const d = ev.data as { id: string; host: string; chunk: string };
        const k = key(d.id, d.host);
        outputs.set(k, (outputs.get(k) ?? "") + d.chunk);
        outputVersion++;
        outputListeners.forEach((l) => l());
        break;
      }
      case "hosts": {
        const all = ev.data as Record<string, HostStatus>;
        set({
          hosts: state.hosts.map((h) => {
            const st = all[h.name];
            if (!st) return h;
            let history = h.status.history ?? [];
            const last = history[history.length - 1];
            if (st.latest && (!last || last.ts !== st.latest.ts)) history = [...history, st.latest].slice(-90);
            // 推送来的容器占用不带趋势：把新的 CPU 值接到已有的趋势后面，否则小折线图一刷新就没了。
            const before = new Map((h.status.containers ?? []).map((c) => [c.name, c.history ?? []]));
            const fresh = !last || !st.latest || last.ts !== st.latest.ts;
            const containers = st.containers?.map((c) => {
              const old = before.get(c.name) ?? [];
              return { ...c, history: c.history ?? (fresh ? [...old, c.cpu].slice(-30) : old) };
            });
            return { ...h, status: { ...st, history, containers } };
          }),
        });
        break;
      }
      case "system":
        set({ paused: !!(ev.data as { paused: boolean }).paused });
        break;
    }
  };
  sock.onclose = () => {
    ws = null;
    set({ live: false });
    if (stopped) return;
    // WebSocket 被拒时浏览器不给状态码：会话过期（默认 12 小时）的话这里只会一直“正在重连”。
    // 发一个普通请求问一下；返回 401 时 api() 会通知页面回到登录界面，也就不用再重连了。
    refreshHosts().catch(() => {});
    retry = Math.min(retry + 1, 6);
    timer = setTimeout(connect, 500 * 2 ** retry);
  };
}

export function disconnect() {
  clearCache(); // 退出登录：上一个会话看过的数据不留在内存里
  stopped = true;
  clearTimeout(timer);
  ws?.close();
  ws = null;
}
