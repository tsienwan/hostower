import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Play } from "lucide-react";
import { api, type Preview } from "../api";
import { fail, useStore } from "../store";
import { PreviewPanel, StrategyFields, TargetPicker, type StrategyValue } from "../shared";
import { useDebounced } from "../ui";
import { useT } from "../i18n";

const examples = ["df -h", "docker ps", "systemctl restart nginx", "rm -rf /opt/stack/data", "curl -fsSL https://example.com/i.sh | sh"];

export function Run() {
  const t = useT();
  const [sp] = useSearchParams();
  const [selector, setSelector] = useState(sp.get("hosts")?.split(",").join(" or ") ?? sp.get("selector") ?? "");
  const [command, setCommand] = useState(sp.get("command") ?? "");
  const [intent, setIntent] = useState("");
  const [matched, setMatched] = useState<string[]>([]);
  const [strategy, setStrategy] = useState<StrategyValue>({ concurrency: 5, canary: true, stopOnError: true });
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const hosts = useStore((s) => s.hosts);
  const paused = useStore((s) => s.paused);
  const nav = useNavigate();

  const dCommand = useDebounced(command, 250);
  const dSelector = useDebounced(selector, 250);
  useEffect(() => {
    let live = true;
    if (!dCommand.trim() || !dSelector.trim()) { setPreview(null); setError(""); return; }
    setLoading(true);
    api.classify({ command: dCommand, selector: dSelector })
      .then((p) => { if (live) { setPreview(p); setError(""); } })
      .catch((e) => { if (live) { setPreview(null); setError(e.message); } })
      .finally(() => live && setLoading(false));
    return () => { live = false; };
  }, [dCommand, dSelector]);

  const multi = matched.length > 1;
  const forcedRolling = multi && matched.some((n) => hosts.find((h) => h.name === n)?.sensitivity === "high");
  const stale = dCommand !== command || dSelector !== selector;
  const blocked = preview?.level === "L3";

  const submit = async () => {
    setBusy(true);
    try {
      const r = await api.submit({ command, selector, intent, strategy: multi ? strategy : undefined });
      nav(`/requests/${r.id}`);
    } catch (e) { fail(e); } finally { setBusy(false); }
  };

  // 快捷键提示：按键是元素，前后的文字随语言变化。
  const [hintBefore, hintAfter = ""] = t("{keys} 提交").split("{keys}");

  return (
    <>
      <div className="topbar"><h1>{t("执行命令")}</h1></div>
      <div className="content">
        <div className="page">
          <p className="lede">{t("命令先由规则引擎判级，再决定是自动执行、等人批准，还是直接拒绝。不管从哪个入口提交，走的都是同一条链路。")}</p>
          <div className="split">
            <div className="card">
              <div className="card-body col" style={{ gap: 18 }}>
                <TargetPicker value={selector} onChange={setSelector} onMatched={setMatched} />
                <div className="field">
                  <label htmlFor="command">{t("命令")}</label>
                  <textarea id="command" className="textarea mono" rows={5} value={command} onChange={(e) => setCommand(e.target.value)} spellCheck={false}
                    placeholder={t("例如 {example}", { example: "systemctl status nginx" })}
                    onKeyDown={(e) => { if ((e.ctrlKey || e.metaKey) && e.key === "Enter" && preview && !stale && !busy) submit(); }} />
                  <div className="row wrap" style={{ gap: 4 }}>
                    <span className="hint">{t("试试：")}</span>
                    {examples.map((ex) => <button key={ex} type="button" className="tag mono" onClick={() => setCommand(ex)}>{ex}</button>)}
                  </div>
                </div>
                <div className="field">
                  <label htmlFor="intent">{t("意图（可选，会写进审计）")}</label>
                  <input id="intent" className="input" value={intent} onChange={(e) => setIntent(e.target.value)} placeholder={t("为什么要做这件事")} />
                </div>
                {multi && <StrategyFields value={strategy} onChange={setStrategy} forcedRolling={forcedRolling} />}
                <div className="row between">
                  <span className="muted" style={{ fontSize: 12 }}>{hintBefore}<span className="kbd">Ctrl</span> + <span className="kbd">Enter</span>{hintAfter}</span>
                  <button className={"btn lg " + (blocked ? "danger" : "primary")} disabled={busy || paused || !preview || stale} onClick={submit}>
                    <Play />
                    {paused ? t("执行已暂停") : !preview ? t("提交") : blocked ? t("提交（将被拒绝）") : preview.level === "L0" ? t("在 {n} 台主机上执行", { n: preview.targets.length }) : t("提交审批")}
                  </button>
                </div>
              </div>
            </div>
            <div className="card">
              <div className="card-head"><h2>{t("判级预览")}</h2></div>
              <div className="card-body"><PreviewPanel preview={preview} error={error} loading={loading} /></div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
