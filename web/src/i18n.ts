// 语言与时区。
//
// 翻译用“原文即键”的方式：代码里写 t("添加主机")，中文不需要词典；
// 其他语言在 locales/<语言>/ 下放词典，把中文原文映射成译文。没有译文时按清单里的 fallback 退。
// 支持哪些语言只登记在 locales/languages.json（服务端的通知和聊天机器人也读它）；
// 新增一种语言的步骤写在那个文件里，漏了什么由 go run ./tools/i18n 报出来。
import { useSyncExternalStore } from "react";
import registry from "./locales/languages.json";

export const languages: readonly { code: string; label: string }[] = registry.languages;

export type Lang = string;
export type LangMode = Lang | "system";

// 词典按文件拆开，构建时自动合并：locales/en/overview.ts、locales/en/common.ts……
const modules = import.meta.glob("./locales/*/*.ts", { eager: true }) as Record<string, { default: Record<string, string> }>;
const dicts: Record<string, Record<string, string>> = {};
const serverDicts: Record<string, Record<string, string>> = {};
for (const [path, mod] of Object.entries(modules)) {
  const [, , lang, file] = path.split("/");
  // 文件名以 server 开头的词典专门翻译服务端返回的文字（见下面的 ts）。
  if (file.startsWith("server")) serverDicts[lang] = { ...serverDicts[lang], ...mod.default };
  else dicts[lang] = { ...dicts[lang], ...mod.default };
}

const LANG_KEY = "hostower.lang";
const TZ_KEY = "hostower.tz";
const listeners = new Set<() => void>();
let version = 0;

function readLang(): LangMode {
  const v = localStorage.getItem(LANG_KEY);
  return languages.some((l) => l.code === v) ? (v as Lang) : "system";
}

let langMode: LangMode = readLang();
let tzMode: string = localStorage.getItem(TZ_KEY) || "system";

/** 按浏览器的语言选：清单里每种语言的 browser 前缀，最长匹配的那个生效；都不匹配用 browserDefault。 */
function systemLang(): Lang {
  for (const raw of navigator.languages?.length ? navigator.languages : [navigator.language || ""]) {
    const v = raw.toLowerCase();
    let best = "", len = 0;
    for (const l of registry.languages) for (const p of l.browser) {
      if ((v === p || v.startsWith(p + "-")) && p.length > len) { best = l.code; len = p.length; }
    }
    if (best) return best;
  }
  return registry.browserDefault;
}

/** 某种语言的词典里缺了某句时，先退到哪种语言（清单里的 fallback；空表示退回原文）。 */
const fallback = (l: Lang): string | null => registry.languages.find((x) => x.code === l)?.fallback || null;

export const lang = (): Lang => (langMode === "system" ? systemLang() : langMode);
export const langSetting = (): LangMode => langMode;

function changed() {
  version++;
  document.documentElement.lang = lang();
  listeners.forEach((l) => l());
}

export function setLang(next: LangMode) {
  langMode = next;
  if (next === "system") localStorage.removeItem(LANG_KEY);
  else localStorage.setItem(LANG_KEY, next);
  changed();
}

/** 翻译。键是中文原文；占位符写成 {名字}。 */
export function t(key: string, params?: Record<string, string | number>): string {
  // 同一个词在不同位置要译成不同的话时，键写成 "原文@@语境"：显示中文时去掉语境部分。
  const l = lang();
  const dict = dicts[l];
  const fb = fallback(l);
  const plain = key.includes("@@") ? key.slice(0, key.indexOf("@@")) : key;
  let s = dict?.[key] ?? dict?.[plain] ?? (fb ? dicts[fb]?.[key] ?? dicts[fb]?.[plain] : undefined) ?? plain;
  if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

/** 在组件里用它拿到 t：语言或时区变了，组件会重新渲染。 */
export function useT() {
  useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => version);
  return t;
}

// ---------- 时区 ----------

export const systemTimeZone = (): string => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
export const timeZoneSetting = (): string => tzMode;
export const timeZone = (): string => (tzMode === "system" ? systemTimeZone() : tzMode);

export function setTimeZone(next: string) {
  tzMode = next || "system";
  if (tzMode === "system") localStorage.removeItem(TZ_KEY);
  else localStorage.setItem(TZ_KEY, tzMode);
  formatter = null;
  changed();
}

/** 浏览器认识的全部时区。 */
export function timeZones(): string[] {
  const intl = Intl as typeof Intl & { supportedValuesOf?: (key: string) => string[] };
  try { return intl.supportedValuesOf?.("timeZone") ?? ["UTC"]; } catch { return ["UTC"]; }
}

let formatter: Intl.DateTimeFormat | null = null;

/** 按所选时区格式化成 “YYYY-MM-DD HH:mm:ss”。 */
export function formatStamp(ts: string | number | Date): string {
  if (!formatter) {
    try {
      // sv-SE 的格式正好是 ISO 风格的 “年-月-日 时:分:秒”，与界面语言无关。
      formatter = new Intl.DateTimeFormat("sv-SE", { timeZone: timeZone(), year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
    } catch {
      formatter = new Intl.DateTimeFormat("sv-SE", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
    }
  }
  const d = new Date(ts);
  return Number.isNaN(d.getTime()) ? "—" : formatter.format(d);
}

/** 当前时区相对 UTC 的偏移，例如 “UTC+08:00”。 */
export function timeZoneOffset(tz: string = timeZone()): string {
  try {
    const part = new Intl.DateTimeFormat("en-US", { timeZone: tz, timeZoneName: "longOffset" }).formatToParts(new Date()).find((p) => p.type === "timeZoneName")?.value ?? "GMT";
    return part.replace("GMT", "UTC") || "UTC";
  } catch { return "UTC"; }
}

export function initI18n() {
  document.documentElement.lang = lang();
  window.addEventListener("storage", (e) => {
    if (e.key === LANG_KEY) { langMode = readLang(); changed(); }
    if (e.key === TZ_KEY) { tzMode = localStorage.getItem(TZ_KEY) || "system"; formatter = null; changed(); }
  });
}

// ---------- 服务端返回的文字 ----------
//
// 服务端只说中文：错误提示、判级理由、审计里的说明都以中文写出并存下来（审计只追加，写进去的字不能改）。
// 界面在显示时翻译它们。词典放在 locales/<语言>/server*.ts，键有两种：
//   - 整句：  "只读" → "Read-only"
//   - 模板：  "主机 {a} 不存在" → "Host {a} does not exist"
//     占位符匹配任意文字；名字以 ! 结尾的（如 {err!}）会把匹配到的那一段再翻译一遍，
//     用来处理一句套一句的情况（“写审计失败，操作中止：{err!}”）。

interface Template { re: RegExp; names: string[]; out: string; weight: number }
const templateCache: Record<string, Template[]> = {};

function templates(l: string): Template[] {
  if (templateCache[l]) return templateCache[l];
  const list: Template[] = [];
  for (const [key, out] of Object.entries(serverDicts[l] ?? {})) {
    if (!key.includes("{")) continue;
    const names: string[] = [];
    let literal = 0;
    const pattern = key.split(/(\{[A-Za-z0-9_]+!?\})/).map((part) => {
      if (/^\{[A-Za-z0-9_]+!?\}$/.test(part)) { names.push(part.slice(1, -1)); return "([\\s\\S]+?)"; }
      literal += part.length;
      return part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    }).join("");
    list.push({ re: new RegExp("^" + pattern + "$"), names, out, weight: literal });
  }
  // 固定文字多的模板更具体，先试。
  list.sort((a, b) => b.weight - a.weight);
  return (templateCache[l] = list);
}

const hasChinese = /[一-鿿]/;

/** 翻译服务端返回的文字。没有对应译文时原样返回。 */
export function ts(text: string | undefined | null, depth = 0): string {
  if (!text) return "";
  const l = lang();
  if (!hasChinese.test(text)) return text;
  // 先在这种语言里找；找不到再退到英文（繁体中文不退，原文就是中文）。
  for (const d of [l, fallback(l)]) {
    if (!d || !serverDicts[d]) continue;
    const out = tsIn(d, text, depth);
    if (out !== null) return out;
  }
  return text;
}

function tsIn(l: string, text: string, depth: number): string | null {
  const dict = serverDicts[l];
  const exact = dict[text] ?? dicts[l]?.[text];
  if (exact !== undefined) return exact;
  if (depth > 4) return null;
  for (const tpl of templates(l)) {
    const m = tpl.re.exec(text);
    if (!m) continue;
    let out = tpl.out;
    tpl.names.forEach((name, i) => {
      out = out.split(`{${name}}`).join(name.endsWith("!") ? ts(m[i + 1], depth + 1) : m[i + 1]);
    });
    return out;
  }
  return null;
}
