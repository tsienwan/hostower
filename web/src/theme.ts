// 主题：跟随系统 / 深色 / 浅色。选择存在本机；“跟随系统”时实时响应系统切换。
import { useSyncExternalStore } from "react";

export type ThemeMode = "system" | "dark" | "light";

const KEY = "hostower.theme";
const media = window.matchMedia("(prefers-color-scheme: light)");
const listeners = new Set<() => void>();

function read(): ThemeMode {
  const v = localStorage.getItem(KEY);
  return v === "dark" || v === "light" ? v : "system";
}

let mode: ThemeMode = read();

export const resolved = (): "dark" | "light" => (mode === "system" ? (media.matches ? "light" : "dark") : mode);

function apply() {
  const root = document.documentElement;
  root.dataset.theme = resolved();
  root.dataset.themeMode = mode;
  listeners.forEach((l) => l());
}

export function initTheme() {
  apply();
  media.addEventListener("change", () => mode === "system" && apply());
  // 另一个标签页改了主题，这里也跟着变。
  window.addEventListener("storage", (e) => {
    if (e.key === KEY) { mode = read(); apply(); }
  });
}

export function setThemeMode(next: ThemeMode) {
  mode = next;
  if (next === "system") localStorage.removeItem(KEY);
  else localStorage.setItem(KEY, next);
  apply();
}

const subscribe = (l: () => void) => { listeners.add(l); return () => listeners.delete(l); };

export const useThemeMode = () => useSyncExternalStore(subscribe, () => mode);
export const useResolvedTheme = () => useSyncExternalStore(subscribe, resolved);
