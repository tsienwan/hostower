// 文件预览：图片、PDF、Markdown、CSV/TSV、Excel、Word、PowerPoint。
//
// 安全上的约定（见 AGENTS.md 第 10 条）：主机上的文件一律当不可信内容。
// - 文件以二进制取回（接口按附件返回），由页面自己解析，浏览器不会把它当网页打开；
// - 图片用 data: 地址放进 <img>（SVG 在 <img> 里不会执行脚本）；
// - PDF 用 pdf.js 画到 canvas 上，不用浏览器内置的阅读器，也不放进 iframe；
// - Markdown 渲染成 React 元素，不解析其中的原始 HTML；外部图片不加载；
// - Office 文档只取文字和表格结构，按纯文本显示。
// 内容安全策略不需要为此放宽。
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Maximize, TriangleAlert, ZoomIn, ZoomOut } from "lucide-react";
import { api } from "./api";
import { ts, useT } from "./i18n";
import { bytes } from "./ui";
import type { Block, Sheet, Slide } from "./office";

export type PreviewKind = "image" | "pdf" | "markdown" | "csv" | "tsv" | "xlsx" | "docx" | "pptx" | "legacy";

const IMAGE_TYPES: Record<string, string> = {
  png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp",
  bmp: "image/bmp", ico: "image/x-icon", avif: "image/avif", svg: "image/svg+xml",
};

const ext = (path: string) => path.slice(path.lastIndexOf(".") + 1).toLowerCase();

/** 按扩展名判断能不能预览、怎么预览。 */
export function previewKind(path: string): PreviewKind | null {
  const e = ext(path);
  if (IMAGE_TYPES[e]) return "image";
  if (e === "pdf") return "pdf";
  if (e === "md" || e === "markdown" || e === "mdx") return "markdown";
  if (e === "csv") return "csv";
  if (e === "tsv") return "tsv";
  if (e === "xlsx" || e === "xlsm") return "xlsx";
  if (e === "docx") return "docx";
  if (e === "pptx") return "pptx";
  if (e === "doc" || e === "xls" || e === "ppt") return "legacy";
  return null;
}

/** 这几种是文本，走原来的“读文件”接口（可以看源码、可以编辑）；其余的要取二进制。 */
export const textKinds: PreviewKind[] = ["markdown", "csv", "tsv"];

async function fetchBinary(host: string, path: string): Promise<Uint8Array> {
  const res = await fetch(api.downloadURL(host, path) + "&preview=1", { credentials: "same-origin" });
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try { msg = (await res.json()).error ?? msg; } catch { /* 不是 JSON */ }
    throw new Error(msg);
  }
  return new Uint8Array(await res.arrayBuffer());
}

/** 取二进制并交给 parse；加载、出错、结果三种状态。 */
function useBinary<T>(host: string, path: string, parse: (data: Uint8Array) => Promise<T> | T) {
  const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({ loading: true });
  useEffect(() => {
    let on = true;
    setState({ loading: true });
    fetchBinary(host, path)
      .then((d) => parse(d))
      .then((data) => on && setState({ data, loading: false }))
      .catch((e: Error) => on && setState({ error: e.message, loading: false }));
    return () => { on = false; };
    // parse 是按种类固定的函数
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host, path]);
  return state;
}

function Status({ loading, error, children }: { loading: boolean; error?: string; children: ReactNode }) {
  const t = useT();
  if (loading) return <div className="empty sm"><span>{t("正在读取文件…")}</span></div>;
  if (error) return <div className="note crit"><TriangleAlert /><span>{ts(error)}</span></div>;
  return <>{children}</>;
}

// ---------- 图片 ----------

function toDataURL(data: Uint8Array, type: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(r.result as string);
    r.onerror = () => reject(new Error("读取图片失败"));
    r.readAsDataURL(new Blob([data as BlobPart], { type }));
  });
}

// 缩放档位：按一下放大或缩小一档。
const ZOOMS = [0.1, 0.25, 0.33, 0.5, 0.67, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8];

function ImagePreview({ host, path }: { host: string; path: string }) {
  const t = useT();
  const s = useBinary(host, path, (d) => toDataURL(d, IMAGE_TYPES[ext(path)] ?? "application/octet-stream"));
  // zoom 为空表示“适应窗口”；否则是相对原始像素的倍数（1 = 原始大小）。
  const [zoom, setZoom] = useState<number | null>(null);
  const [size, setSize] = useState<[number, number] | null>(null);
  const [fitScale, setFitScale] = useState(1);
  const img = useRef<HTMLImageElement>(null);
  const box = useRef<HTMLDivElement>(null);

  // 适应窗口时实际缩放了多少：用来显示百分比，也作为从“适应窗口”开始放大缩小的起点。
  useEffect(() => {
    if (zoom !== null || !size || !img.current) return;
    const update = () => img.current && setFitScale(img.current.getBoundingClientRect().width / size[0]);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(img.current);
    return () => ro.disconnect();
  }, [zoom, size]);

  const current = zoom ?? fitScale;
  const step = (dir: 1 | -1) => {
    const next = dir > 0 ? ZOOMS.find((z) => z > current + 0.001) : [...ZOOMS].reverse().find((z) => z < current - 0.001);
    if (next !== undefined) setZoom(next);
  };
  // 按住 Ctrl（或 Mac 上的 ⌘）滚动滚轮缩放。
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      step(e.deltaY < 0 ? 1 : -1);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  });

  return (
    <Status {...s}>
      <div className="pv-toolbar">
        {size && <span className="muted num">{size[0]} × {size[1]}</span>}
        <span className="grow" />
        <div className="pv-zoom" role="group" aria-label={t("缩放")}>
          <button className="btn ghost icon sm" onClick={() => step(-1)} disabled={current <= ZOOMS[0] + 0.001} title={t("缩小")} aria-label={t("缩小")}><ZoomOut /></button>
          <button className="pv-zoom-val num" onClick={() => setZoom(1)} title={t("点击回到原始大小")}>{Math.round(current * 100)}%</button>
          <button className="btn ghost icon sm" onClick={() => step(1)} disabled={current >= ZOOMS[ZOOMS.length - 1] - 0.001} title={t("放大（也可以按住 Ctrl 滚动滚轮）")} aria-label={t("放大")}><ZoomIn /></button>
        </div>
        <button className={"btn sm" + (zoom === null ? " active" : "")} onClick={() => setZoom(null)} title={t("整张图放进显示区域")}><Maximize /><span className="hide-sm">{t("适应窗口")}</span></button>
        <button className={"btn sm" + (zoom === 1 ? " active" : "")} onClick={() => setZoom(1)}>1:1</button>
      </div>
      <div ref={box} className={"pv-image" + (zoom === null ? "" : " actual")}>
        {s.data && (
          <img ref={img} src={s.data} alt={path}
            style={zoom !== null && size ? { width: Math.round(size[0] * zoom), height: Math.round(size[1] * zoom) } : undefined}
            onLoad={(e) => setSize([e.currentTarget.naturalWidth, e.currentTarget.naturalHeight])} onError={() => setSize(null)} />
        )}
      </div>
    </Status>
  );
}

// ---------- PDF ----------

const MAX_PAGES = 50;

function PdfPreview({ host, path }: { host: string; path: string }) {
  const t = useT();
  const box = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<{ loading: boolean; error?: string; pages?: number }>({ loading: true });
  useEffect(() => {
    let on = true;
    let destroy: (() => void) | undefined;
    (async () => {
      try {
        const [data, pdfjs, worker] = await Promise.all([
          fetchBinary(host, path),
          import("pdfjs-dist"),
          import("pdfjs-dist/build/pdf.worker.min.mjs?url"),
        ]);
        pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
        // 只把页面画出来：不渲染 XFA 表单，也不运行文档里的脚本（这里没有加载 pdf.js 的脚本沙箱）。
        const task = pdfjs.getDocument({ data, enableXfa: false, verbosity: 0 });
        destroy = () => { task.destroy(); };
        const doc = await task.promise;
        if (!on) return;
        setState({ loading: false, pages: doc.numPages });
        // 按显示区域的真实宽度和屏幕倍率来画：画布的像素数要不少于它在屏幕上占的物理像素，否则会被拉伸发糊。
        // 显示区域一直挂在页面上，所以这里量得到真实宽度；再乘一点余量，放大页面时也还清楚。
        const width = Math.max(box.current?.clientWidth ?? 0, 320) - 4;
        const ratio = Math.min(Math.max(window.devicePixelRatio || 1, 1) * 1.5, 4);
        for (let i = 1; i <= Math.min(doc.numPages, MAX_PAGES) && on; i++) {
          const page = await doc.getPage(i);
          const base = page.getViewport({ scale: 1 });
          const vp = page.getViewport({ scale: (width / base.width) * ratio });
          const canvas = document.createElement("canvas");
          canvas.width = Math.floor(vp.width);
          canvas.height = Math.floor(vp.height);
          canvas.className = "pv-page";
          canvas.setAttribute("aria-label", t("第 {n} 页", { n: i }));
          box.current?.appendChild(canvas);
          await page.render({ canvas, viewport: vp }).promise;
        }
      } catch (e) {
        if (on) setState({ loading: false, error: (e as Error).message || t("PDF 打不开") });
      }
    })();
    return () => { on = false; destroy?.(); if (box.current) box.current.textContent = ""; };
  }, [host, path, t]);
  return (
    <>
      <Status loading={state.loading} error={state.error}>
        {state.pages !== undefined && (
          <div className="pv-toolbar">
            <span className="muted">{t("共 {n} 页", { n: state.pages })}{state.pages > MAX_PAGES ? t("，只显示前 {n} 页", { n: MAX_PAGES }) : ""}</span>
          </div>
        )}
      </Status>
      {/* 画页面的容器不放进 Status：它要在读取文件时就已经在页面上，才量得到宽度。 */}
      <div ref={box} className="pv-pdf" hidden={!!state.error} />
    </>
  );
}

// ---------- Markdown ----------

function MarkdownPreview({ text }: { text: string }) {
  const t = useT();
  const [mods, setMods] = useState<{ Md: typeof import("react-markdown").default; gfm: typeof import("remark-gfm").default } | null>(null);
  useEffect(() => {
    Promise.all([import("react-markdown"), import("remark-gfm")]).then(([a, b]) => setMods({ Md: a.default, gfm: b.default }));
  }, []);
  if (!mods) return <div className="empty sm"><span>{t("加载中…")}</span></div>;
  const { Md, gfm } = mods;
  return (
    <div className="pv-markdown">
      <Md
        remarkPlugins={[gfm]}
        skipHtml
        components={{
          // 外部图片不加载（也加载不了：内容安全策略不允许），只显示说明文字。
          img: ({ alt, src }) => <span className="pv-img-alt" title={typeof src === "string" ? src : undefined}>[{alt || t("图片")}]</span>,
          a: ({ href, children }) => /^https?:\/\//i.test(href ?? "")
            ? <a href={href} target="_blank" rel="noreferrer noopener">{children}</a>
            : <span className="pv-link">{children}</span>,
        }}
      >{text}</Md>
    </div>
  );
}

// ---------- 表格 ----------

function SheetTable({ sheet }: { sheet: Sheet }) {
  const t = useT();
  const cols = sheet.rows.reduce((m, r) => Math.max(m, r.length), 0);
  const letter = (i: number) => { let s = ""; for (i++; i > 0; i = Math.floor((i - 1) / 26)) s = String.fromCharCode(65 + ((i - 1) % 26)) + s; return s; };
  return (
    <>
      {sheet.truncated && <div className="note"><TriangleAlert /><span>{t("表格共 {r} 行 {c} 列，只显示前 1000 行、60 列。", { r: sheet.totalRows, c: sheet.totalCols })}</span></div>}
      {sheet.rows.length === 0 ? <div className="empty sm"><span>{t("空表")}</span></div> : (
        <div className="pv-sheet">
          <table>
            <thead><tr><th />{Array.from({ length: cols }, (_, i) => <th key={i}>{letter(i)}</th>)}</tr></thead>
            <tbody>
              {sheet.rows.map((r, i) => <tr key={i}><th>{i + 1}</th>{r.map((c, j) => <td key={j} title={c.length > 40 ? c : undefined}>{c}</td>)}</tr>)}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function XlsxPreview({ host, path }: { host: string; path: string }) {
  const s = useBinary(host, path, async (d) => (await import("./office")).readXlsx(d));
  const [cur, setCur] = useState(0);
  const sheets = s.data ?? [];
  const sheet = sheets[Math.min(cur, sheets.length - 1)];
  return (
    <Status {...s}>
      {sheets.length > 1 && (
        <div className="pv-toolbar" role="tablist">
          {sheets.map((sh, i) => <button key={i} role="tab" aria-selected={i === cur} className={"tag" + (i === cur ? " on" : "")} onClick={() => setCur(i)}>{sh.name || `Sheet${i + 1}`}</button>)}
        </div>
      )}
      {sheet && <SheetTable sheet={sheet} />}
    </Status>
  );
}

function DelimitedPreview({ text, sep }: { text: string; sep: string }) {
  const [sheet, setSheet] = useState<Sheet | null>(null);
  useEffect(() => { import("./office").then((m) => setSheet(m.readDelimited(text, sep))); }, [text, sep]);
  return sheet ? <SheetTable sheet={sheet} /> : null;
}

// ---------- Word ----------

function DocxPreview({ host, path }: { host: string; path: string }) {
  const t = useT();
  const s = useBinary(host, path, async (d) => (await import("./office")).readDocx(d));
  const blocks: Block[] = s.data ?? [];
  return (
    <Status {...s}>
      <div className="note"><TriangleAlert /><span>{t("只显示文字、标题、列表和表格；图片、排版和批注不显示。")}</span></div>
      <div className="pv-doc">
        {blocks.length === 0 && <div className="empty sm"><span>{t("文档是空的")}</span></div>}
        {blocks.map((b, i) => {
          if (b.kind === "table") {
            return <table key={i} className="pv-doc-table"><tbody>{b.rows.map((r, j) => <tr key={j}>{r.map((c, k) => <td key={k}>{c}</td>)}</tr>)}</tbody></table>;
          }
          const content = b.runs.map((r, j) => <span key={j} className={(r.bold ? "b " : "") + (r.italic ? "i" : "")}>{r.text}</span>);
          if (b.level) {
            const H = (["h1", "h2", "h3", "h4"] as const)[b.level - 1];
            return <H key={i}>{content}</H>;
          }
          return <p key={i} className={b.list ? "li" : undefined}>{content.length ? content : " "}</p>;
        })}
      </div>
    </Status>
  );
}

// ---------- PowerPoint ----------

function PptxPreview({ host, path }: { host: string; path: string }) {
  const t = useT();
  const s = useBinary(host, path, async (d) => (await import("./office")).readPptx(d));
  const slides: Slide[] = s.data ?? [];
  return (
    <Status {...s}>
      <div className="note"><TriangleAlert /><span>{t("只显示每一页的标题、文字和演讲者备注；图片、版式和动画不显示。")}</span></div>
      <div className="pv-slides">
        {slides.length === 0 && <div className="empty sm"><span>{t("演示文稿是空的")}</span></div>}
        {slides.map((sl) => (
          <section key={sl.index} className="pv-slide">
            <div className="pv-slide-no">{t("第 {n} 页", { n: sl.index })}</div>
            {sl.title && <h3>{sl.title}</h3>}
            {sl.paragraphs.map((p, i) => <p key={i}>{p}</p>)}
            {!sl.title && sl.paragraphs.length === 0 && <p className="muted">{t("（这一页没有文字）")}</p>}
            {sl.notes.length > 0 && <div className="pv-notes"><b>{t("备注")}</b>{sl.notes.map((n, i) => <p key={i}>{n}</p>)}</div>}
          </section>
        ))}
      </div>
    </Status>
  );
}

// ---------- 入口 ----------

/** 二进制格式的预览。文本格式（Markdown、CSV）由调用方拿到文字后用 TextPreview。 */
export function BinaryPreview({ host, path, kind, size }: { host: string; path: string; kind: PreviewKind; size: number }) {
  const t = useT();
  if (kind === "legacy") {
    return <div className="note"><TriangleAlert /><span>{t("这是旧版 Office 格式（.doc / .xls / .ppt），页面上不能预览。请下载后查看，或者另存为 .docx / .xlsx / .pptx。")}</span></div>;
  }
  if (size > 20 << 20) {
    return <div className="note"><TriangleAlert /><span>{t("文件有 {size}，超过 20 MB，页面上不预览。可以下载后查看。", { size: bytes(size) })}</span></div>;
  }
  switch (kind) {
    case "image": return <ImagePreview host={host} path={path} />;
    case "pdf": return <PdfPreview host={host} path={path} />;
    case "xlsx": return <XlsxPreview host={host} path={path} />;
    case "docx": return <DocxPreview host={host} path={path} />;
    case "pptx": return <PptxPreview host={host} path={path} />;
  }
  return null;
}

export function TextPreview({ kind, text }: { kind: PreviewKind; text: string }) {
  if (kind === "markdown") return <MarkdownPreview text={text} />;
  if (kind === "csv") return <DelimitedPreview text={text} sep="," />;
  if (kind === "tsv") return <DelimitedPreview text={text} sep={"\t"} />;
  return null;
}
