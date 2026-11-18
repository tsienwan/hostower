// 提交操作时共用的部件：目标选择、判级预览、批量策略。
import { useEffect, useState } from "react";
import { ShieldAlert, ShieldCheck, ShieldX, TriangleAlert } from "lucide-react";
import { api, type Finding, type Level, type Preview } from "./api";
import { useStore } from "./store";
import { LevelBadge, levelMeta, useDebounced } from "./ui";
import { ts, useT } from "./i18n";

/** 目标选择：选择器表达式，或点分组快速填入。实时显示匹配到的主机。 */
export function TargetPicker({ value, onChange, onMatched }: { value: string; onChange: (v: string) => void; onMatched?: (hosts: string[]) => void }) {
  const t = useT();
  const groups = useStore((s) => s.groups);
  const hosts = useStore((s) => s.hosts);
  const [matched, setMatched] = useState<string[]>([]);
  const [error, setError] = useState("");
  const debounced = useDebounced(value, 200);

  useEffect(() => {
    let live = true;
    if (!debounced.trim()) { setMatched([]); setError(""); onMatched?.([]); return; }
    api.select(debounced)
      .then((r) => { if (live) { setMatched(r.hosts); setError(""); onMatched?.(r.hosts); } })
      .catch((e) => { if (live) { setMatched([]); setError(e.message); onMatched?.([]); } });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);

  const byName = new Map(hosts.map((h) => [h.name, h]));
  return (
    <div className="field">
      <label htmlFor="selector">{t("目标主机")}</label>
      <input id="selector" className="input mono" value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false}
        placeholder={t("主机名、tag:web、group:prod，可用 and / or / not 组合")} />
      <div className="row wrap" style={{ gap: 4 }}>
        {groups.map((g) => (
          <button key={g.name} type="button" className={"tag" + (value.trim() === `group:${g.name}` ? " on" : "")} onClick={() => onChange(`group:${g.name}`)}>
            {g.name} <span className="muted" style={{ marginLeft: 4 }}>{g.members.length}</span>
          </button>
        ))}
      </div>
      {error ? (
        <div className="hint" style={{ color: "var(--crit)" }}>{error}</div>
      ) : matched.length > 0 ? (
        <div className="row wrap" style={{ gap: 4 }}>
          <span className="hint">{t("匹配 {n} 台：", { n: matched.length })}</span>
          {matched.slice(0, 24).map((n) => {
            const h = byName.get(n);
            return (
              <span key={n} className="tag mono" title={h?.sensitivity === "high" ? t("高敏感主机") : undefined}>
                {h?.sensitivity === "high" && <ShieldAlert size={11} style={{ color: "var(--serious)", marginRight: 4 }} />}{n}
              </span>
            );
          })}
          {matched.length > 24 && <span className="hint">{t("等 {n} 台", { n: matched.length })}</span>}
        </div>
      ) : null}
    </div>
  );
}

function distinct(findings: Finding[]): Finding[] {
  const seen = new Set<string>();
  return findings.filter((f) => {
    if (f.level === "L0") return false;
    const k = f.level + f.rule + f.reason + f.command;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  }).sort((a, b) => b.level.localeCompare(a.level));
}

const levelIcon: Record<Level, React.ReactNode> = {
  L0: <ShieldCheck size={18} style={{ color: "var(--ok)" }} />,
  L1: <ShieldAlert size={18} style={{ color: "var(--warn)" }} />,
  L2: <ShieldAlert size={18} style={{ color: "var(--serious)" }} />,
  L3: <ShieldX size={18} style={{ color: "var(--crit)" }} />,
};

const outcome: Record<Level, string> = {
  L0: "只读操作，提交后自动执行。",
  L1: "变更操作，提交后需要一个人批准才会执行。",
  L2: "高危操作，批准时要重新验证 Passkey，并手动输入确认内容。",
  L3: "禁止的操作。提交会被内核直接拒绝并写入审计，任何人都无法放行。",
};

/** 判级预览：等级、会发生什么、为什么。 */
export function PreviewPanel({ preview, error, loading }: { preview: Preview | null; error?: string; loading?: boolean }) {
  const t = useT();
  if (error) return <div className="note crit"><TriangleAlert /><span>{error}</span></div>;
  if (!preview) return <div className="muted" style={{ fontSize: 12.5 }}>{loading ? t("判级中…") : t("填好目标和命令后，这里会显示风险等级和判定理由。判级由规则引擎完成，预览没有任何副作用。")}</div>;
  const all = distinct(preview.targets.flatMap((tgt) => tgt.findings));
  const commands = new Map<string, string[]>();
  for (const tgt of preview.targets) commands.set(tgt.command, [...(commands.get(tgt.command) ?? []), tgt.host]);
  return (
    <div className="col" style={{ gap: 14 }}>
      <div className="row" style={{ alignItems: "flex-start", gap: 12 }}>
        {levelIcon[preview.level]}
        <div className="grow">
          <div className="row" style={{ gap: 8 }}><LevelBadge level={preview.level} label /><span style={{ fontWeight: 560 }}>{t(levelMeta[preview.level].desc)}</span></div>
          <div className="dim" style={{ marginTop: 4 }}>{t(outcome[preview.level])}</div>
        </div>
      </div>
      {all.length > 0 && (
        <div>
          <div className="section-title" style={{ marginBottom: 2 }}>{t("判定理由")}</div>
          {all.map((f, i) => (
            <div key={i} className="finding">
              <LevelBadge level={f.level} />
              <div><div className="why">{ts(f.reason)}</div>{f.command && <div className="what">{ts(f.command)}</div>}</div>
            </div>
          ))}
        </div>
      )}
      {commands.size > 1 && (
        <div className="col" style={{ gap: 8 }}>
          <div className="section-title">{t("每台主机上的实际命令")}</div>
          {[...commands.entries()].map(([cmd, hs]) => (
            <div key={cmd}>
              <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{hs.join(t("、"))}</div>
              <pre className="code"><span className="prompt">$ </span>{cmd}</pre>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export interface StrategyValue { concurrency: number; canary: boolean; stopOnError: boolean }

export function StrategyFields({ value, onChange, forcedRolling }: { value: StrategyValue; onChange: (v: StrategyValue) => void; forcedRolling: boolean }) {
  const t = useT();
  return (
    <div className="field">
      <label>{t("批量执行方式")}</label>
      <div className="row wrap" style={{ gap: 16 }}>
        <label className="check"><input type="checkbox" checked={value.canary} onChange={(e) => onChange({ ...value, canary: e.target.checked })} />{t("先在一台上预演")}</label>
        <label className="check"><input type="checkbox" checked={value.stopOnError} onChange={(e) => onChange({ ...value, stopOnError: e.target.checked })} />{t("遇错停止")}</label>
        <label className="check">
          {t("并发")}
          <input className="input num" style={{ width: 64, height: 26 }} type="number" min={1} max={50} disabled={forcedRolling}
            value={forcedRolling ? 1 : value.concurrency} onChange={(e) => onChange({ ...value, concurrency: Math.max(1, Math.min(50, Number(e.target.value) || 1)) })} />
        </label>
      </div>
      {forcedRolling && <div className="hint">{t("目标里有高敏感主机：强制逐台滚动执行，高敏感主机排在最后。")}</div>}
    </div>
  );
}
