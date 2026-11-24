import { useCallback, useEffect, useMemo, useState } from "react";
import { askConfirm } from "../dialogs";
import { useNavigate, useParams } from "react-router-dom";
import { ArrowDown, ArrowUp, Copy, FileCode2, Pencil, Play, Plus, Trash2, TriangleAlert } from "lucide-react";
import { api, type Level, type Preview, type Snippet, type SnippetParam } from "../api";
import { fail, toast, useStore } from "../store";
import { cached, remember } from "../cache";
import { PreviewPanel, StrategyFields, TargetPicker, type StrategyValue } from "../shared";
import { Empty, LevelBadge, Modal, useDebounced } from "../ui";
import { Select } from "../controls";
import { useT } from "../i18n";

const blank: Snippet = {
  name: "", title: "", description: "", version: "1.0.0", level: "L1", selector: "", params: [], command: "",
  strategy: { concurrency: 5, canary: true, stopOnError: true },
};

export function Snippets() {
  const t = useT();
  const { name } = useParams();
  const nav = useNavigate();
  // 再次进来先显示上次的列表，后台刷新；第一次加载完成前什么都不画，免得闪出一张空的列表卡片。
  const [list, setList] = useState<Snippet[] | null>(() => cached<Snippet[]>("snippets") ?? null);
  const [editing, setEditing] = useState<{ spec: Snippet; create: boolean } | null>(null);
  const [yaml, setYaml] = useState<Snippet | null>(null);

  const load = useCallback(() => api.snippets().then((r) => { setList(remember("snippets", r.snippets)); return r.snippets; }).catch((e) => { fail(e); return [] as Snippet[]; }), []);
  useEffect(() => { load(); }, [load]);
  const current = list?.find((s) => s.name === name) ?? null;
  useEffect(() => { if (list && list.length && !name && !editing) nav(`/snippets/${list[0].name}`, { replace: true }); }, [list, name, nav, editing]);

  const remove = async (s: Snippet) => {
    if (!(await askConfirm(t("删除 Snippet「{title}」？已经执行过的记录不受影响。", { title: s.title })))) return;
    try {
      await api.deleteSnippet(s.name);
      toast(t("已删除"));
      const rest = await load();
      nav(rest.length ? `/snippets/${rest[0].name}` : "/snippets", { replace: true });
    } catch (e) { fail(e); }
  };

  return (
    <>
      <div className="topbar">
        <h1>Snippet</h1><span className="muted hide-sm">{t("参数化的操作模板")}</span>
        <span className="grow" />
        <button className="btn primary" onClick={() => setEditing({ spec: blank, create: true })}><Plus />{t("新建 Snippet")}</button>
      </div>
      <div className="content">
        <div className="page">
          {editing ? (
            <Editor key={editing.spec.name + String(editing.create)} initial={editing.spec} create={editing.create} onCancel={() => setEditing(null)}
              onSaved={async (s) => { await load(); setEditing(null); nav(`/snippets/${s.name}`); }} />
          ) : !list ? null : list.length === 0 ? (
            <div className="card"><Empty icon={<FileCode2 />} title={t("还没有 Snippet")}>{t("把常做的操作做成模板：参数有类型和校验，渲染出的命令照样要判级和审批。点右上角“新建 Snippet”开始。")}</Empty></div>
          ) : (
            <div className="split even">
              <div className="card sticky-col">
                {list.map((s) => (
                  <div key={s.name} className={"list-item" + (s.name === name ? " active" : "")} onClick={() => nav(`/snippets/${s.name}`)}>
                    <div className="row"><span className="grow ellipsis" style={{ fontWeight: 560 }}>{s.title}</span><LevelBadge level={s.level} /></div>
                    <div className="muted ellipsis" style={{ fontSize: 12 }}>{s.description || s.name}</div>
                  </div>
                ))}
              </div>
              {current && (
                <SnippetForm key={current.name + current.version} s={current} actions={
                  <>
                    {current.source === "managed" && <button className="btn sm" onClick={() => setEditing({ spec: current, create: false })}><Pencil />{t("编辑")}</button>}
                    <button className="btn sm" title={t("以它为基础新建一个")} onClick={() => setEditing({ spec: { ...current, name: current.name + "-copy", title: current.title + t("（副本）") }, create: true })}><Copy /><span className="hide-sm">{t("复制")}</span></button>
                    {current.source === "managed" && <button className="btn sm" onClick={() => setYaml(current)}>YAML</button>}
                    {current.source === "managed" && <button className="btn ghost icon sm" title={t("删除")} aria-label={t("删除")} onClick={() => remove(current)}><Trash2 /></button>}
                  </>
                } />
              )}
            </div>
          )}
        </div>
      </div>
      {yaml && (
        <Modal wide title={`${yaml.name}.yaml`} onClose={() => setYaml(null)}
          footer={<><button className="btn" onClick={() => navigator.clipboard.writeText(yaml.yaml ?? "").then(() => toast(t("已复制")))}><Copy />{t("复制")}</button><button className="btn" onClick={() => setYaml(null)}>{t("关闭")}</button></>}>
          <p className="dim">{t("这就是它作为文件的样子。放进配置目录的")} <span className="mono">snippets/</span>{t(" 并纳入 Git，就变成只读的“配置文件”来源。")}</p>
          <pre className="code" style={{ maxHeight: "56vh", overflow: "auto" }}>{yaml.yaml}</pre>
        </Modal>
      )}
    </>
  );
}

// ---------- 运行 ----------

function SnippetForm({ s, actions }: { s: Snippet; actions: React.ReactNode }) {
  const t = useT();
  const params = s.params ?? [];
  const [values, setValues] = useState<Record<string, unknown>>(() => Object.fromEntries(params.map((p) => [p.name, p.default ?? (p.type === "bool" ? false : "")])));
  const [selector, setSelector] = useState(s.selector ?? "");
  const [matched, setMatched] = useState<string[]>([]);
  const [strategy, setStrategy] = useState<StrategyValue>({ concurrency: s.strategy.concurrency || 5, canary: s.strategy.canary, stopOnError: s.strategy.stopOnError });
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const hosts = useStore((st) => st.hosts);
  const paused = useStore((st) => st.paused);
  const nav = useNavigate();

  const body = { snippet: s.name, params: values, selector };
  const debounced = useDebounced(JSON.stringify(body), 250);
  useEffect(() => {
    let live = true;
    const b = JSON.parse(debounced);
    if (!b.selector.trim()) { setPreview(null); setError(""); return; }
    api.classify(b).then((p) => { if (live) { setPreview(p); setError(""); } }).catch((e) => { if (live) { setPreview(null); setError(e.message); } });
    return () => { live = false; };
  }, [debounced]);

  const multi = matched.length > 1;
  const forcedRolling = multi && matched.some((n) => hosts.find((h) => h.name === n)?.sensitivity === "high");
  const stale = debounced !== JSON.stringify(body);

  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.submit({ ...body, strategy: multi ? strategy : undefined });
      nav(`/requests/${r.id}`);
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="card">
        <div className="card-head wrap">
          <h2>{s.title}</h2>
          <LevelBadge level={s.level} label />
          <span className="mono muted hide-sm">{s.name}@{s.version}</span>
          <span className="badge neutral">{s.source === "managed" ? t("页面管理") : t("配置文件")}</span>
          <span className="grow" />
          {actions}
        </div>
        <div className="card-body col" style={{ gap: 18 }}>
          {s.description && <p className="dim">{s.description}</p>}
          <div className="field">
            <label>{t("模板")}</label>
            <pre className="code">{s.command.trim()}</pre>
          </div>
          {params.map((p) => <ParamField key={p.name} p={p} value={values[p.name]} onChange={(v) => setValues({ ...values, [p.name]: v })} />)}
          <TargetPicker value={selector} onChange={setSelector} onMatched={setMatched} />
          {multi && <StrategyFields value={strategy} onChange={setStrategy} forcedRolling={forcedRolling} />}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <button className="btn primary lg" disabled={busy || paused || !preview || stale || preview.level === "L3"} onClick={submit}>
              <Play />{paused ? t("执行已暂停") : !preview ? t("提交") : preview.level === "L0" ? t("在 {n} 台主机上执行", { n: preview.targets.length }) : t("提交审批")}
            </button>
          </div>
        </div>
      </div>
      <div className="card">
        <div className="card-head"><h2>{t("渲染与判级预览")}</h2></div>
        <div className="card-body col" style={{ gap: 14 }}>
          {preview && new Set(preview.targets.map((tg) => tg.command)).size === 1 && <pre className="code"><span className="prompt">$ </span>{preview.targets[0].command}</pre>}
          <PreviewPanel preview={preview} error={error} />
        </div>
      </div>
    </div>
  );
}

function ParamField({ p, value, onChange }: { p: SnippetParam; value: unknown; onChange: (v: unknown) => void }) {
  const t = useT();
  const id = "p-" + p.name;
  return (
    <div className="field" style={{ maxWidth: 440 }}>
      <label htmlFor={id}>{p.title || p.name}{p.required && <span style={{ color: "var(--crit)" }}> *</span>}</label>
      {p.type === "enum" ? (
        <Select id={id} value={String(value ?? "")} onChange={onChange}
          options={[...(p.required ? [] : [{ value: "", label: t("（不填）") }]), ...(p.values ?? []).map((v) => ({ value: v, label: v }))]} />
      ) : p.type === "bool" ? (
        <label className="check"><input id={id} type="checkbox" checked={value === true || value === "true"} onChange={(e) => onChange(e.target.checked)} />{t("启用")}</label>
      ) : (
        <input id={id} className={"input" + (p.type !== "string" ? " mono" : "")} type={p.type === "int" ? "number" : "text"} min={p.min} max={p.max}
          value={String(value ?? "")} spellCheck={false} onChange={(e) => onChange(p.type === "int" && e.target.value !== "" ? Number(e.target.value) : e.target.value)} />
      )}
      {(p.description || (p.type === "int" && p.min !== undefined && p.max !== undefined)) && (
        <span className="hint">{p.description}{p.type === "int" && p.min !== undefined && p.max !== undefined ? t(" 范围 {min}–{max}", { min: p.min, max: p.max }) : ""}</span>
      )}
    </div>
  );
}

// ---------- 新建 / 编辑 ----------

// label 和 hint 是中文原文，在组件里翻译后再交给 Select。
const typeOptions: { value: string; label: string; hint?: string }[] = [
  { value: "string", label: "文本", hint: "可加格式校验" },
  { value: "int", label: "整数", hint: "可限范围" },
  { value: "enum", label: "选项", hint: "只能从列表里选" },
  { value: "bool", label: "开关" },
  { value: "path", label: "路径", hint: "绝对路径，不含 .." },
];

const levelOptions: { value: Level; label: string; hint: string }[] = [
  { value: "L0", label: "L0 只读", hint: "自动执行" },
  { value: "L1", label: "L1 变更", hint: "一人批准" },
  { value: "L2", label: "L2 高危", hint: "二次验证" },
];

function Editor({ initial, create, onCancel, onSaved }: { initial: Snippet; create: boolean; onCancel: () => void; onSaved: (s: Snippet) => void }) {
  const t = useT();
  const typeOpts = typeOptions.map((o) => ({ ...o, label: t(o.label), hint: o.hint ? t(o.hint) : undefined }));
  const levelOpts = levelOptions.map((o) => ({ ...o, label: t(o.label), hint: t(o.hint) }));
  const [spec, setSpec] = useState<Snippet>({ ...initial, params: initial.params ?? [], source: undefined, yaml: undefined });
  const [selector, setSelector] = useState(initial.selector || "*");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const params = spec.params ?? [];
  const set = (patch: Partial<Snippet>) => setSpec((s) => ({ ...s, ...patch }));
  const setParam = (i: number, patch: Partial<SnippetParam>) => set({ params: params.map((p, j) => (j === i ? { ...p, ...patch } : p)) });
  const move = (i: number, d: number) => { const next = [...params]; const [x] = next.splice(i, 1); next.splice(i + d, 0, x); set({ params: next }); };

  // 实时预览：把草稿发给服务端校验、渲染、判级（没有任何副作用）。参数用默认值，没有默认值的用示例值。
  const sample = useMemo(() => Object.fromEntries(params.map((p) => [p.name,
    p.default !== undefined && p.default !== "" ? p.default : p.type === "int" ? (p.min ?? 1) : p.type === "bool" ? false : p.type === "enum" ? (p.values?.[0] ?? "") : p.type === "path" ? "/tmp/example" : "example"])), [params]);
  const payload = useDebounced(JSON.stringify({ draft: spec, params: sample, selector }), 350);
  useEffect(() => {
    let live = true;
    const b = JSON.parse(payload);
    if (!b.draft.name || !b.draft.command.trim()) { setPreview(null); setError(""); return; }
    api.classify(b).then((p) => { if (live) { setPreview(p); setError(""); } }).catch((e) => { if (live) { setPreview(null); setError(e.message); } });
    return () => { live = false; };
  }, [payload]);

  const insert = (text: string) => set({ command: spec.command + (spec.command && !spec.command.endsWith(" ") ? " " : "") + text });
  const save = async () => {
    setBusy(true);
    try {
      const saved = create ? await api.createSnippet(spec) : await api.updateSnippet(spec);
      toast(create ? t("Snippet 已创建") : t("Snippet 已保存"));
      onSaved(saved);
    } catch (e) { fail(e); } finally { setBusy(false); }
  };
  const declaredLow = preview && preview.level > spec.level;

  return (
    <div className="split">
      <div className="col" style={{ gap: 16 }}>
        <div className="card">
          <div className="card-head"><h2>{create ? t("新建 Snippet") : t("编辑 {title}", { title: initial.title })}</h2></div>
          <div className="card-body col" style={{ gap: 16 }}>
            <div className="form-grid">
              <div className="field"><label htmlFor="e-title">{t("标题")}</label><input id="e-title" className="input" value={spec.title} onChange={(e) => set({ title: e.target.value })} placeholder={t("重启服务")} autoFocus /></div>
              <div className="field"><label htmlFor="e-name">{t("标识")}</label><input id="e-name" className="input mono" value={spec.name} disabled={!create} onChange={(e) => set({ name: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-") })} placeholder="restart-service" spellCheck={false} /></div>
              <div className="field"><label htmlFor="e-level">{t("声明等级")}</label><Select id="e-level" value={spec.level} onChange={(v) => set({ level: v as Level })} options={levelOpts} /></div>
              <div className="field"><label htmlFor="e-ver">{t("版本")}</label><input id="e-ver" className="input mono" value={spec.version} onChange={(e) => set({ version: e.target.value })} placeholder="1.0.0" spellCheck={false} /></div>
            </div>
            <div className="field"><label htmlFor="e-desc">{t("说明")}</label><input id="e-desc" className="input" value={spec.description ?? ""} onChange={(e) => set({ description: e.target.value })} placeholder={t("这个模板做什么、什么时候用")} /></div>
            <div className="field">
              <label htmlFor="e-cmd">{t("命令模板")}</label>
              <textarea id="e-cmd" className="textarea mono" rows={5} value={spec.command} onChange={(e) => set({ command: e.target.value })} spellCheck={false} placeholder={"systemctl restart {{ .service }}"} />
              <div className="row wrap" style={{ gap: 4 }}>
                <span className="hint">{t("点击插入：")}</span>
                {params.filter((p) => p.name).map((p) => <button key={p.name} type="button" className="tag mono" onClick={() => insert(`{{ .${p.name} }}`)}>{`{{ .${p.name} }}`}</button>)}
                <button type="button" className="tag mono" onClick={() => insert("{{ .host.name }}")}>{"{{ .host.name }}"}</button>
                <button type="button" className="tag mono" onClick={() => insert("{{ .host.addr }}")}>{"{{ .host.addr }}"}</button>
                <button type="button" className="tag mono" title={t("主机变量：每台主机可以不同，在清单文件的 vars 里定义")} onClick={() => insert("{{ .host.vars.NAME }}")}>{"{{ .host.vars.NAME }}"}</button>
              </div>
              <span className="hint">{t("参数和主机变量渲染时都会做 shell 转义，所以不要再给它们加引号。支持")} <span className="mono">{"{{ if .flag }}…{{ end }}"}</span>{t(" 这样的条件。")}</span>
            </div>
            <div className="field">
              <label htmlFor="e-sel">{t("默认目标")}</label>
              <input id="e-sel" className="input mono" value={spec.selector ?? ""} onChange={(e) => { set({ selector: e.target.value }); if (e.target.value.trim()) setSelector(e.target.value); }} placeholder={t("tag:web（可留空，运行时再选）")} spellCheck={false} />
            </div>
            <div className="field">
              <label>{t("默认批量策略")}</label>
              <div className="row wrap" style={{ gap: 16 }}>
                <label className="check"><input type="checkbox" checked={spec.strategy.canary} onChange={(e) => set({ strategy: { ...spec.strategy, canary: e.target.checked } })} />{t("先在一台上预演")}</label>
                <label className="check"><input type="checkbox" checked={spec.strategy.stopOnError} onChange={(e) => set({ strategy: { ...spec.strategy, stopOnError: e.target.checked } })} />{t("遇错停止")}</label>
                <label className="check">{t("并发")}<input className="input num" style={{ width: 64, height: 28, minHeight: 0 }} type="number" min={1} max={50} value={spec.strategy.concurrency || 1}
                  onChange={(e) => set({ strategy: { ...spec.strategy, concurrency: Math.max(1, Math.min(50, Number(e.target.value) || 1)) } })} /></label>
              </div>
            </div>
          </div>
        </div>

        <div className="card">
          <div className="card-head"><h2>{t("参数")}</h2><span className="muted num">{params.length}</span><span className="grow" />
            <button className="btn sm" onClick={() => set({ params: [...params, { name: "", type: "string", required: false }] })}><Plus />{t("添加参数")}</button></div>
          {params.length === 0 ? <div className="empty sm"><span>{t("没有参数：每次运行的都是同一条命令。")}</span></div> : (
            <div className="card-body col" style={{ gap: 0, padding: 0 }}>
              {params.map((p, i) => (
                <div key={i} className="param-row">
                  <div className="form-grid" style={{ gridTemplateColumns: "repeat(3, minmax(0, 1fr))" }}>
                    <div className="field"><label>{t("参数名")}</label><input className="input mono" value={p.name} onChange={(e) => setParam(i, { name: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_") })} placeholder="service" spellCheck={false} /></div>
                    <div className="field"><label>{t("显示名")}</label><input className="input" value={p.title ?? ""} onChange={(e) => setParam(i, { title: e.target.value })} placeholder={t("服务名")} /></div>
                    <div className="field"><label>{t("类型")}</label><Select value={p.type} onChange={(v) => setParam(i, { type: v as SnippetParam["type"], default: undefined, values: v === "enum" ? p.values ?? [] : undefined, pattern: undefined, min: undefined, max: undefined })} options={typeOpts} /></div>
                    {p.type === "enum" && <div className="field" style={{ gridColumn: "span 2" }}><label>{t("可选值（用空格或逗号分隔）")}</label><input className="input mono" value={(p.values ?? []).join(" ")} onChange={(e) => setParam(i, { values: e.target.value.split(/[\s,，]+/).filter(Boolean) })} placeholder="nginx docker cron" spellCheck={false} /></div>}
                    {p.type === "string" && <div className="field" style={{ gridColumn: "span 2" }}><label>{t("格式校验（正则，可选）")}</label><input className="input mono" value={p.pattern ?? ""} onChange={(e) => setParam(i, { pattern: e.target.value || undefined })} placeholder="[a-z0-9._-]+" spellCheck={false} /></div>}
                    {p.type === "int" && <>
                      <div className="field"><label>{t("最小值")}</label><input className="input mono" type="number" value={p.min ?? ""} onChange={(e) => setParam(i, { min: e.target.value === "" ? undefined : Number(e.target.value) })} /></div>
                      <div className="field"><label>{t("最大值")}</label><input className="input mono" type="number" value={p.max ?? ""} onChange={(e) => setParam(i, { max: e.target.value === "" ? undefined : Number(e.target.value) })} /></div>
                    </>}
                    {p.type !== "bool" && (
                      <div className="field"><label>{t("默认值")}</label>
                        {p.type === "enum"
                          ? <Select value={String(p.default ?? "")} onChange={(v) => setParam(i, { default: v || undefined })} options={[{ value: "", label: t("（无）") }, ...(p.values ?? []).map((v) => ({ value: v, label: v }))]} />
                          : <input className="input mono" type={p.type === "int" ? "number" : "text"} value={String(p.default ?? "")} onChange={(e) => setParam(i, { default: e.target.value === "" ? undefined : p.type === "int" ? Number(e.target.value) : e.target.value })} spellCheck={false} />}
                      </div>
                    )}
                  </div>
                  <div className="row wrap" style={{ marginTop: 10 }}>
                    <input className="input grow" style={{ minWidth: 160 }} value={p.description ?? ""} onChange={(e) => setParam(i, { description: e.target.value })} placeholder={t("给使用者的提示（可选）")} aria-label={t("参数提示")} />
                    {p.type === "bool"
                      ? <label className="check"><input type="checkbox" checked={p.default === true} onChange={(e) => setParam(i, { default: e.target.checked })} />{t("默认开启")}</label>
                      : <label className="check"><input type="checkbox" checked={p.required} onChange={(e) => setParam(i, { required: e.target.checked })} />{t("必填")}</label>}
                    <button className="btn ghost icon sm" disabled={i === 0} onClick={() => move(i, -1)} aria-label={t("上移")} title={t("上移")}><ArrowUp /></button>
                    <button className="btn ghost icon sm" disabled={i === params.length - 1} onClick={() => move(i, 1)} aria-label={t("下移")} title={t("下移")}><ArrowDown /></button>
                    <button className="btn ghost icon sm" onClick={() => set({ params: params.filter((_, j) => j !== i) })} aria-label={t("删除参数")} title={t("删除参数")}><Trash2 /></button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className="row" style={{ justifyContent: "flex-end" }}>
          <button className="btn lg" onClick={onCancel}>{t("取消")}</button>
          <button className="btn primary lg" disabled={busy || !spec.name || !spec.title || !spec.command.trim() || !!error} onClick={save}>{create ? t("创建") : t("保存")}</button>
        </div>
      </div>

      <div className="card" style={{ position: "sticky", top: 0 }}>
        <div className="card-head"><h2>{t("实时预览")}</h2></div>
        <div className="card-body col" style={{ gap: 14 }}>
          <div className="field">
            <label htmlFor="e-prev">{t("在这些主机上试渲染")}</label>
            <input id="e-prev" className="input mono" value={selector} onChange={(e) => setSelector(e.target.value)} spellCheck={false} />
            <span className="hint">{t("参数用默认值；没有默认值的用示例值。只渲染和判级，不执行。")}</span>
          </div>
          {preview && new Set(preview.targets.map((tg) => tg.command)).size === 1 && <pre className="code"><span className="prompt">$ </span>{preview.targets[0].command}</pre>}
          <PreviewPanel preview={preview} error={error} />
          {declaredLow && <div className="note warn"><TriangleAlert /><span>{t("渲染出的命令被判为 {actual}，高于声明的 {declared}。执行时按较高的等级处理——声明等级只能加严，不能放宽。", { actual: preview!.level, declared: spec.level })}</span></div>}
        </div>
      </div>
    </div>
  );
}
