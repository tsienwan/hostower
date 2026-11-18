// 终端输出的关键词高亮。只在页面上着色，不改变发给主机的内容，也不影响录像。
//
// 规则很保守：只给“没有颜色的普通文本”着色。程序自己已经上了色的部分（ls --color、带色的提示符）、
// 全屏程序（vim、top、less 用的备用屏幕）都原样通过，不去碰。

const ESC = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]/g;

// 分组依次是：出错、警告、正常、提示级别、调试级别、网址、地址、时间、带单位的数量、百分比。
const WORDS = new RegExp(
  [
    "\\b(errors?|ERR|failed|failures?|fatal|panic|denied|refused|unhealthy|critical|crit|exited|dead|killed|not found|no such file or directory|cannot|unable to|timed out|unreachable|invalid|missing)\\b",
    "\\b(warn(?:ing)?s?|deprecated|timeout|retrying|restarting|degraded|pending|skipp(?:ed|ing)|inactive|disabled|stopped|unknown)\\b",
    "\\b(ok|success(?:ful(?:ly)?)?|succeeded|done|passed|active|running|healthy|started|enabled|ready|up to date|listening|connected|complete[d]?)\\b",
    "\\b(INFO|NOTICE)\\b",
    "\\b(DEBUG|TRACE)\\b",
    "(https?:\\/\\/[^\\s\"'<>]+)",
    "\\b(\\d{1,3}(?:\\.\\d{1,3}){3}(?::\\d{1,5}|\\/\\d{1,2})?)\\b",
    "\\b(\\d{4}-\\d{2}-\\d{2}(?:[T ]\\d{2}:\\d{2}(?::\\d{2})?(?:\\.\\d+)?(?:Z|[+-]\\d{2}:?\\d{2})?)?|\\d{2}:\\d{2}:\\d{2})\\b",
    "\\b(\\d+(?:\\.\\d+)?\\s?(?:[KMGT]i?B|[kMGT]B|ms|[mµu]s|ns)(?:\\/s)?)\\b",
    "(\\b\\d+(?:\\.\\d+)?%)",
  ].join("|"),
  "g",
);
// 级别类的词（INFO、DEBUG）只认大写；其余不分大小写，在 paint 里统一处理。
const INSENSITIVE = new RegExp(WORDS.source, "gi");
const COLORS = ["\x1b[1;31m", "\x1b[33m", "\x1b[32m", "\x1b[34m", "\x1b[90m", "\x1b[4;34m", "\x1b[36m", "\x1b[90m", "\x1b[35m", "\x1b[35m"];
const RESET = "\x1b[22;24;39m";

function paint(text: string): string {
  return text.replace(INSENSITIVE, (m, ...g) => {
    const i = g.slice(0, COLORS.length).findIndex((x) => x !== undefined);
    if (i < 0) return m;
    // 3、4 两组是日志级别，小写的 info / debug 多半只是普通单词。
    if ((i === 3 || i === 4) && m !== m.toUpperCase()) return m;
    return COLORS[i] + m + RESET;
  });
}

/** 有状态的高亮器：逐块喂入终端输出，返回着色后的文本。 */
export function createHighlighter() {
  // 程序自己设置的颜色和样式，逐项记着：很多程序复位时用的是 39（前景）、22（粗体）这类单项代码，而不是整体复位。
  const on = { fg: false, bg: false, weight: false, italic: false, underline: false, blink: false, inverse: false, conceal: false, strike: false };
  let colored = false;
  let alt = false; // 正在备用屏幕里（全屏程序）
  let carry = ""; // 上一块末尾没收完的转义序列

  const sgr = (params: string) => {
    const codes = params === "" ? [0] : params.split(/[;:]/).map(Number);
    for (let i = 0; i < codes.length; i++) {
      const c = codes[i];
      if (c === 0) { for (const k of Object.keys(on) as (keyof typeof on)[]) on[k] = false; }
      else if (c === 1 || c === 2) on.weight = true;
      else if (c === 22) on.weight = false;
      else if (c === 3) on.italic = true;
      else if (c === 23) on.italic = false;
      else if (c === 4 || c === 21) on.underline = true;
      else if (c === 24) on.underline = false;
      else if (c === 5 || c === 6) on.blink = true;
      else if (c === 25) on.blink = false;
      else if (c === 7) on.inverse = true;
      else if (c === 27) on.inverse = false;
      else if (c === 8) on.conceal = true;
      else if (c === 28) on.conceal = false;
      else if (c === 9) on.strike = true;
      else if (c === 29) on.strike = false;
      else if ((c >= 30 && c <= 37) || (c >= 90 && c <= 97)) on.fg = true;
      else if (c === 38) { on.fg = true; i += codes[i + 1] === 5 ? 2 : 4; }
      else if (c === 39) on.fg = false;
      else if ((c >= 40 && c <= 47) || (c >= 100 && c <= 107)) on.bg = true;
      else if (c === 48) { on.bg = true; i += codes[i + 1] === 5 ? 2 : 4; }
      else if (c === 49) on.bg = false;
      // 其余的（字体选择等）不影响判断。
    }
    colored = Object.values(on).some(Boolean);
  };

  return (chunk: string): string => {
    let text = carry + chunk;
    carry = "";
    // 末尾如果是半截转义序列，留到下一块。
    const last = text.lastIndexOf("\x1b");
    if (last >= 0) {
      ESC.lastIndex = last;
      const m = ESC.exec(text);
      if (!m || m.index !== last) { carry = text.slice(last); text = text.slice(0, last); }
    }
    let out = "", pos = 0;
    ESC.lastIndex = 0;
    for (let m = ESC.exec(text); m; m = ESC.exec(text)) {
      const plain = text.slice(pos, m.index);
      out += colored || alt ? plain : paint(plain);
      out += m[0];
      pos = m.index + m[0].length;
      const seq = m[0];
      if (seq.endsWith("m") && seq.startsWith("\x1b[")) sgr(seq.slice(2, -1));
      else if (/^\x1b\[\?(?:1049|1047|47)h$/.test(seq)) alt = true;
      else if (/^\x1b\[\?(?:1049|1047|47)l$/.test(seq)) { alt = false; sgr("0"); }
    }
    const rest = text.slice(pos);
    out += colored || alt ? rest : paint(rest);
    return out;
  };
}
