// 应用的上手指南：装好以后从哪里打开、初始账号是什么、接下来做哪几步。
// 内容来自应用清单（服务端按这个实例的参数渲染好），远端读回来的东西一律按纯文本显示。
import { useState } from "react";
import { Link } from "react-router-dom";
import { BookOpen, Copy, ExternalLink, Eye, Globe, Info, KeyRound, TriangleAlert } from "lucide-react";
import { api, type AppGuide, type AppItem, type Host } from "../api";
import { fail, toast, useStore } from "../store";
import { Modal } from "../ui";
import { ts, useT } from "../i18n";

/** 浏览器里用来访问这台主机的地址：清单里的地址去掉账号和 SSH 端口；内置的本机用当前页面的地址。 */
export function browserHost(h?: Host): string {
  if (!h) return "";
  let a = h.addr.trim();
  if (h.executor === "local" || a === "localhost" || a === "127.0.0.1" || a === "") return window.location.hostname;
  a = a.replace(/^[^@]*@/, "");
  if (a.startsWith("[")) return a.replace(/\]:\d+$/, "]");
  if ((a.match(/:/g) ?? []).length === 1) a = a.replace(/:\d+$/, "");
  return a.includes(":") ? `[${a}]` : a;
}

export function guideUrl(h: Host | undefined, o: AppGuide["open"][number]): string {
  return `${o.scheme}://${browserHost(h)}:${o.port}${o.path}`;
}

function copy(text: string, done: string) {
  navigator.clipboard?.writeText(text).then(() => toast(done)).catch(() => {});
}

export function AppGuideDialog({ host, instance, app, guide, onClose }: { host: string; instance: string; app?: AppItem; guide: AppGuide; onClose: () => void }) {
  const t = useT();
  const h = useStore((s) => s.hosts.find((x) => x.name === host));
  const [revealed, setRevealed] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const reveal = () => {
    setBusy(true);
    api.appReveal(host, instance).then((r) => setRevealed(r.output)).catch(fail).finally(() => setBusy(false));
  };

  return (
    <Modal wide title={t("{app} 上手指南", { app: app?.title ?? instance })} onClose={onClose}
      footer={<>
        {app?.homepage && <a className="btn" href={app.homepage} target="_blank" rel="noopener noreferrer"><BookOpen />{t("官方文档")}</a>}
        <span className="grow" />
        <button className="btn primary" onClick={onClose}>{t("知道了")}</button>
      </>}>
      <div className="col" style={{ gap: 16 }}>
        <div className="muted" style={{ fontSize: 12.5 }}>{t("实例 {instance} · 主机 {host}", { instance, host })}</div>

        {guide.open.length > 0 && (
          <section className="col" style={{ gap: 8 }}>
            <h3 className="guide-h"><Globe />{t("访问地址")}</h3>
            {guide.open.map((o, i) => {
              const url = guideUrl(h, o);
              return (
                <div key={i} className="guide-link">
                  <span className="muted">{ts(o.title)}</span>
                  <a className="mono" href={url} target="_blank" rel="noopener noreferrer">{url}<ExternalLink /></a>
                  <button className="btn ghost icon sm" title={t("复制地址")} aria-label={t("复制地址")} onClick={() => copy(url, t("已复制"))}><Copy /></button>
                </div>
              );
            })}
            {guide.open.some((o) => o.local) && (
              <div className="note warn"><TriangleAlert /><span>
                {t("这个应用只在主机本机（127.0.0.1）监听，上面的地址从外面打不开。")}
                <Link to={`/hosts/${encodeURIComponent(host)}?tab=network&port=${guide.open[0].port}`}>{t("给它配一个域名（反向代理）")}</Link>
                {t("，或者在“修改设置”里把监听地址改成 0.0.0.0。")}
              </span></div>
            )}
            {guide.open.some((o) => !o.local) && (
              <div className="hint">{t("打不开时先看主机防火墙和云服务商的安全组有没有放行这个端口（主机页的“网络”里可以放行）。")}</div>
            )}
          </section>
        )}

        {(guide.account || guide.reveal) && (
          <section className="col" style={{ gap: 8 }}>
            <h3 className="guide-h"><KeyRound />{t("初始账号")}</h3>
            {guide.account && <div>{ts(guide.account)}</div>}
            {guide.reveal && (
              <>
                <div><button className="btn" disabled={busy} onClick={reveal}><Eye />{busy ? t("读取中…") : ts(guide.reveal.title)}</button></div>
                {revealed !== null && (revealed
                  ? <pre className="guide-out mono">{revealed}</pre>
                  : <div className="note warn"><TriangleAlert /><span>{t("主机上没有读到。")}{guide.reveal.hint ? ts(guide.reveal.hint) : ""}</span></div>)}
                {revealed === null && <div className="hint">{t("这是一次只读查询：从主机上读出来直接显示在这里，不会保存到塔台。")}</div>}
              </>
            )}
          </section>
        )}

        {guide.steps.length > 0 && (
          <section className="col" style={{ gap: 8 }}>
            <h3 className="guide-h"><Info />{t("接下来")}</h3>
            <ol className="guide-steps">{guide.steps.map((s, i) => <li key={i}>{ts(s)}</li>)}</ol>
          </section>
        )}
      </div>
    </Modal>
  );
}
