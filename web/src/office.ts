// 读 Office 文档（xlsx、docx、pptx）里的内容，只取文字和表格结构，不执行、不渲染文档里的任何东西。
// 这三种格式都是一个 zip 包，里面是 XML：用 fflate 解压，用浏览器自带的 XML 解析器读，结果是纯数据，
// 页面再按纯文本渲染。旧的二进制格式（.doc、.xls、.ppt）不支持。
import { unzipSync, strFromU8 } from "fflate";

type Files = Record<string, Uint8Array>;

function unzip(data: Uint8Array): Files {
  try {
    return unzipSync(data);
  } catch {
    throw new Error("文件不是有效的 Office 文档（解压失败）");
  }
}

function xml(files: Files, path: string): Document | null {
  const raw = files[path];
  if (!raw) return null;
  const doc = new DOMParser().parseFromString(strFromU8(raw), "application/xml");
  return doc.getElementsByTagName("parsererror").length ? null : doc;
}

/** 按本地名取子孙元素，不管命名空间前缀。 */
const all = (el: Element | Document, name: string) => Array.from(el.getElementsByTagNameNS("*", name));
const kids = (el: Element, name: string) => Array.from(el.children).filter((c) => c.localName === name);
const attr = (el: Element, name: string) => {
  for (const a of Array.from(el.attributes)) if (a.localName === name) return a.value;
  return null;
};

/** 关系编号（r:id）。不能只按本地名找：幻灯片列表里同一个元素上还有一个普通的 id 属性。 */
const REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships";
const rid = (el: Element) => el.getAttributeNS(REL_NS, "id") ?? attr(el, "id");

/** 关系文件：r:id → 目标路径（相对于 base 所在目录）。 */
function rels(files: Files, relsPath: string, base: string): Record<string, string> {
  const doc = xml(files, relsPath);
  const out: Record<string, string> = {};
  if (!doc) return out;
  const dir = base.slice(0, base.lastIndexOf("/") + 1);
  for (const r of all(doc, "Relationship")) {
    const id = attr(r, "Id"), target = attr(r, "Target");
    if (!id || !target) continue;
    out[id] = target.startsWith("/") ? target.slice(1) : normalize(dir + target);
  }
  return out;
}

function normalize(p: string) {
  const out: string[] = [];
  for (const part of p.split("/")) {
    if (part === "..") out.pop();
    else if (part && part !== ".") out.push(part);
  }
  return out.join("/");
}

// ---------- 表格 ----------

export interface Sheet { name: string; rows: string[][]; totalRows: number; totalCols: number; truncated: boolean }

const MAX_ROWS = 1000, MAX_COLS = 60;

function colIndex(ref: string): number {
  let n = 0;
  for (const ch of ref) {
    const c = ch.charCodeAt(0);
    if (c < 65 || c > 90) break;
    n = n * 26 + (c - 64);
  }
  return n - 1;
}

export function readXlsx(data: Uint8Array): Sheet[] {
  const files = unzip(data);
  const wb = xml(files, "xl/workbook.xml");
  if (!wb) throw new Error("不是有效的 Excel 文件（找不到工作簿）");
  const map = rels(files, "xl/_rels/workbook.xml.rels", "xl/workbook.xml");
  const shared: string[] = [];
  const sst = xml(files, "xl/sharedStrings.xml");
  if (sst) for (const si of all(sst, "si")) shared.push(all(si, "t").map((t) => t.textContent ?? "").join(""));

  return all(wb, "sheet").map((sh) => {
    const name = attr(sh, "name") ?? "";
    const path = map[rid(sh) ?? ""];
    const doc = path ? xml(files, path) : null;
    const rows: string[][] = [];
    let totalRows = 0, totalCols = 0;
    if (doc) {
      for (const row of all(doc, "row")) {
        const r = Number(attr(row, "r") ?? rows.length + 1) - 1;
        totalRows = Math.max(totalRows, r + 1);
        if (r >= MAX_ROWS) continue;
        for (const c of kids(row, "c")) {
          const ref = attr(c, "r") ?? "";
          const ci = ref ? colIndex(ref) : 0;
          totalCols = Math.max(totalCols, ci + 1);
          if (ci >= MAX_COLS || ci < 0) continue;
          const type = attr(c, "t");
          const v = kids(c, "v")[0]?.textContent ?? "";
          let text = v;
          if (type === "s") text = shared[Number(v)] ?? "";
          else if (type === "inlineStr") text = all(c, "t").map((t) => t.textContent ?? "").join("");
          else if (type === "b") text = v === "1" ? "TRUE" : "FALSE";
          // 数字按 Excel 的显示精度（15 位有效数字）去掉浮点误差：0.07000000000000001 显示成 0.07。
          else if ((!type || type === "n") && v !== "" && Number.isFinite(Number(v))) text = String(Number(Number(v).toPrecision(15)));
          while (rows.length <= r) rows.push([]);
          const line = rows[r];
          while (line.length <= ci) line.push("");
          line[ci] = text;
        }
      }
    }
    const width = Math.min(totalCols, MAX_COLS);
    for (const line of rows) while (line.length < width) line.push("");
    return { name, rows, totalRows, totalCols, truncated: totalRows > MAX_ROWS || totalCols > MAX_COLS };
  });
}

/** CSV / TSV：处理引号里的分隔符、换行和转义的引号。 */
export function readDelimited(text: string, sep: string): Sheet {
  const rows: string[][] = [];
  let row: string[] = [], cell = "", quoted = false, totalRows = 0;
  const push = () => { row.push(cell); cell = ""; };
  const end = () => { push(); totalRows++; if (rows.length < MAX_ROWS) rows.push(row.slice(0, MAX_COLS)); row = []; };
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') { cell += '"'; i++; }
      else if (ch === '"') quoted = false;
      else cell += ch;
    } else if (ch === '"' && cell === "") quoted = true;
    else if (ch === sep) push();
    else if (ch === "\n") end();
    else if (ch !== "\r") cell += ch;
  }
  if (cell !== "" || row.length) end();
  const totalCols = rows.reduce((m, r) => Math.max(m, r.length), 0);
  for (const r of rows) while (r.length < totalCols) r.push("");
  return { name: "", rows, totalRows, totalCols, truncated: totalRows > MAX_ROWS };
}

// ---------- Word ----------

export type Block =
  | { kind: "p"; level: number; list: boolean; runs: Run[] }
  | { kind: "table"; rows: string[][] };
export interface Run { text: string; bold: boolean; italic: boolean }

function runsOf(p: Element): Run[] {
  const out: Run[] = [];
  for (const r of all(p, "r")) {
    const rPr = kids(r, "rPr")[0];
    const on = (name: string) => {
      const el = rPr && kids(rPr, name)[0];
      return !!el && attr(el, "val") !== "0" && attr(el, "val") !== "false";
    };
    let text = "";
    for (const c of Array.from(r.children)) {
      if (c.localName === "t") text += c.textContent ?? "";
      else if (c.localName === "tab") text += "\t";
      else if (c.localName === "br" || c.localName === "cr") text += "\n";
    }
    if (text) out.push({ text, bold: on("b"), italic: on("i") });
  }
  return out;
}

export function readDocx(data: Uint8Array): Block[] {
  const files = unzip(data);
  const doc = xml(files, "word/document.xml");
  if (!doc) throw new Error("不是有效的 Word 文件（找不到正文）");
  const body = all(doc, "body")[0];
  if (!body) return [];
  const blocks: Block[] = [];
  for (const el of Array.from(body.children)) {
    if (el.localName === "p") {
      const pPr = kids(el, "pPr")[0];
      const style = pPr ? attr(kids(pPr, "pStyle")[0] ?? pPr, "val") ?? "" : "";
      const m = /^(?:heading|标题)\s*(\d)$/i.exec(style);
      const level = /^title$/i.test(style) ? 1 : m ? Math.min(Number(m[1]) + 1, 4) : 0;
      // 列表：段落带编号属性，或者用的是“列表”样式（很多文档只设样式、不设编号）。
      const list = !!(pPr && kids(pPr, "numPr").length) || /list/i.test(style);
      blocks.push({ kind: "p", level, list, runs: runsOf(el) });
    } else if (el.localName === "tbl") {
      const rows = kids(el, "tr").map((tr) => kids(tr, "tc").map((tc) => all(tc, "p").map((p) => runsOf(p).map((r) => r.text).join("")).join("\n")));
      blocks.push({ kind: "table", rows });
    }
  }
  return blocks;
}

// ---------- PowerPoint ----------

export interface Slide { index: number; title: string; paragraphs: string[]; notes: string[] }

function paragraphsOf(el: Element): string[] {
  return all(el, "p").map((p) => all(p, "t").map((t) => t.textContent ?? "").join("")).filter((s) => s.trim() !== "");
}

export function readPptx(data: Uint8Array): Slide[] {
  const files = unzip(data);
  const pres = xml(files, "ppt/presentation.xml");
  if (!pres) throw new Error("不是有效的 PowerPoint 文件（找不到演示文稿）");
  const map = rels(files, "ppt/_rels/presentation.xml.rels", "ppt/presentation.xml");
  return all(pres, "sldId").map((s, i) => {
    const path = map[rid(s) ?? ""];
    const doc = path ? xml(files, path) : null;
    let title = "";
    const paragraphs: string[] = [];
    if (doc) {
      for (const sp of all(doc, "sp")) {
        const ph = all(sp, "ph")[0];
        const type = ph ? attr(ph, "type") : null;
        const text = paragraphsOf(sp);
        if (!title && (type === "title" || type === "ctrTitle")) title = text.join(" ");
        else paragraphs.push(...text);
      }
      for (const tbl of all(doc, "tbl")) for (const tr of all(tbl, "tr")) paragraphs.push(all(tr, "tc").map((tc) => paragraphsOf(tc).join(" ")).join(" | "));
    }
    // 演讲者备注在单独的文件里，经由这一页的关系文件找到。
    const notes: string[] = [];
    if (path) {
      const slideRels = rels(files, path.replace(/([^/]+)$/, "_rels/$1.rels"), path);
      for (const target of Object.values(slideRels)) {
        if (!/notesSlide/.test(target)) continue;
        const n = xml(files, target);
        if (n) for (const sp of all(n, "sp")) if (attr(all(sp, "ph")[0] ?? sp, "type") === "body") notes.push(...paragraphsOf(sp));
      }
    }
    return { index: i + 1, title, paragraphs, notes };
  });
}
