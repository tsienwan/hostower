import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Download, RefreshCw, ShieldAlert, TriangleAlert } from "lucide-react";
import { api, type DockerStatus, type Req } from "./api";
import { backAfterApproval, fail, useStore } from "./store";
import { ts, useT } from "./i18n";

/** 服务端“安装 Docker”请求的意图（固定写法），用来在请求列表里认出它。 */
const intents = ["安装 Docker 和 Docker Compose", "安装 Docker Compose"];

/** 这台主机上最近一次安装 Docker 的请求，还在等批准或执行中时返回它。 */
function pendingInstall(reqs: Req[], host: string): Req | null {
  let latest: Req | null = null;
  for (const r of reqs) {
    if (!intents.includes(r.intent) || r.targets.length !== 1 || r.targets[0].host !== host) continue;
    if (!latest || r.createdAt > latest.createdAt) latest = r;
  }
  return latest && (latest.state === "pending" || latest.state === "running") ? latest : null;
}

/**
 * 主机上没有 Docker（或缺 Compose）时的提示和“安装 Docker”按钮。已经齐了就什么都不显示。
 * onReady 报告检查结果（null 表示还在检查）：安装应用的对话框用它决定能不能提交。
 */
export function DockerSetup({ host, onReady }: { host: string; onReady?: (ready: boolean | null) => void }) {
  const t = useT();
  const nav = useNavigate();
  const reqs = useStore((s) => s.requests);
  const [info, setInfo] = useState<{ status: DockerStatus; ready: boolean; blocked?: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const pending = pendingInstall(reqs, host);
  // 安装请求结束（成功或失败）后重新检查一次。
  const doneKey = reqs.filter((r) => intents.includes(r.intent) && r.targets[0]?.host === host && r.finishedAt).map((r) => r.id).join();
  useEffect(() => {
    let on = true;
    setInfo(null);
    onReady?.(null);
    api.hostDocker(host).then((r) => { if (on) { setInfo(r); onReady?.(r.ready); } }).catch(() => { if (on) onReady?.(true); });
    return () => { on = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [host, doneKey]);
  if (!info || info.ready) return null;
  const what = info.status.docker ? t("{host} 上有 Docker，但没有 Docker Compose v2（docker compose 命令）。", { host }) : t("{host} 上没有安装 Docker。", { host });
  const install = async () => {
    setBusy(true);
    try { nav(`/requests/${(await api.installDocker(host)).id}`, { state: backAfterApproval }); } catch (e) { fail(e); } finally { setBusy(false); }
  };
  return (
    <div className="note warn docker-setup">
      <TriangleAlert />
      <div className="col grow" style={{ gap: 6 }}>
        <span>{what}</span>
        {info.blocked
          ? <span className="dim">{ts(info.blocked)}</span>
          : <span className="dim">{t("可以让 Hostower 装上：按这台主机的系统，用 Docker 官方的软件源安装并启动服务。会生成一个请求，你看过实际命令、批准后才执行。")}</span>}
        {!info.blocked && !info.status.root && <span className="hint">{t("装好后，登录账号还要加进 docker 组才能用（容器页会给出命令）。")}</span>}
      </div>
      {pending
        ? <Link className="btn sm" to={`/requests/${pending.id}`} state={backAfterApproval}>{pending.state === "pending" ? <ShieldAlert /> : <RefreshCw className="spin" />}{pending.state === "pending" ? t("等待批准") : t("正在安装…")}</Link>
        : !info.blocked && <button className="btn sm primary" disabled={busy} onClick={install}><Download />{t("安装 Docker")}</button>}
    </div>
  );
}
