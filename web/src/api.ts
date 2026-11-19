// API 层：类型定义和请求封装。写请求带自定义头，配合服务端的来源校验。
import { t, ts } from "./i18n";

export type Level = "L0" | "L1" | "L2" | "L3";

export interface Finding { command: string; level: Level; rule: string; reason: string }

export interface Sample { ts: number; cpu: number; mem: number; disk: number; load: number; swap: number; rx: number; tx: number; cache: number; load5: number; load15: number; dr: number; dw: number; temp: number }
export interface DiskInfo { mount: string; device: string; totalKB: number; used: number }
export interface ContainerStat { name: string; cpu: number; memBytes: number; mem: number; netRx: number; netTx: number; history?: number[] }

export interface ProcStat { pid: number; name: string; cpu: number; memBytes: number }
export interface HostStatus {
  host: string; online: boolean; known: boolean; error?: string; lastSeen?: number;
  os?: string; kernel?: string; ncpu?: number; memKB?: number; diskKB?: number; uptimeSec?: number; swapKB?: number; procs?: number; disks?: DiskInfo[]; containers?: ContainerStat[];
  /** 主机上增强采集程序的版本；没有表示用的是内置脚本。 */
  agent?: string; topProcs?: ProcStat[];
  /** CPU 型号、整机型号、磁盘：连上主机后才有，每 6 小时更新。 */
  hardware?: Hardware;
  latest?: Sample; history?: Sample[];
}

export interface Hardware {
  arch?: string; cpuVendor?: string; cpuModel?: string; sockets?: number; cores?: number; threads?: number; maxMHz?: number; l3?: string;
  hypervisor?: string; virtual?: boolean; vendor?: string; product?: string; board?: string;
  disks?: { name: string; bytes: number; kind?: "ssd" | "hdd"; model?: string }[];
  at: number;
}

export interface Host {
  name: string; addr: string; executor: string; credential?: string; tags: string[];
  sensitivity: "low" | "normal" | "high"; source: "file" | "managed" | "builtin"; status: HostStatus;
}

export interface Group { name: string; selector?: string; hosts?: string[]; members: string[] }

export interface Target {
  host: string; sensitivity: string; command: string; level: Level; findings: Finding[];
  state: "waiting" | "running" | "ok" | "failed" | "error" | "skipped";
  exitCode?: number; error?: string; output: string; outputBytes: number; truncated?: boolean;
  startedAt?: string; endedAt?: string; auditSeq?: number; backup?: string;
}

export type ReqState = "pending" | "running" | "done" | "failed" | "denied" | "rejected" | "expired" | "cancelled" | "interrupted";

export interface Actor { id: string; name: string; kind: string; entry: string }

/** 一台主机上 Docker 的情况（只读检查的结果）。 */
export interface DockerStatus { docker: boolean; compose: boolean; root: boolean; sudo: boolean; distro?: string; os?: string }

export interface Req {
  id: string; kind: "command" | "file.write" | "terminal"; intent: string; actor: Actor; snippet?: string;
  level: Level; digest: string; confirm?: string;
  strategy: { concurrency: number; canary: boolean; stopOnError: boolean };
  targets: Target[];
  file?: { path: string; content?: string; old?: string; oldExists: boolean };
  input?: { kind: "secret" | "upload" | "envfile"; label: string; size?: number; sha256?: string; env?: { env: string; ref: string }[] };
  terminal?: { attachedAt: string; closedAt?: string; bytes: number; recording: string; truncated?: boolean };
  state: ReqState; createdAt: string; expiresAt?: string; decidedAt?: string; finishedAt?: string;
  approvals?: { by: string; at: string; reauth: boolean; devBypass?: boolean }[];
  deniedBy?: string; denyReason?: string; auditSeq?: number;
}

export interface Preview { level: Level; targets: { host: string; command: string; level: Level; findings: Finding[] }[] }

export interface SnippetParam {
  name: string; type: "string" | "int" | "bool" | "enum" | "path"; title?: string; description?: string;
  required: boolean; default?: unknown; values?: string[]; pattern?: string; min?: number; max?: number;
}

export interface Snippet {
  name: string; title: string; description?: string; version: string; level: Level; selector?: string;
  params: SnippetParam[] | null; command: string; strategy: { concurrency: number; canary: boolean; stopOnError: boolean };
  source?: "file" | "managed"; yaml?: string;
}

export interface AuditBody {
  type: string; actor?: string; actorKind?: string; entry?: string; requestId?: string; parent?: number;
  host?: string; intent?: string; command?: string; level?: Level; reasons?: string[]; approver?: string;
  exitCode?: number; output?: string; backup?: string; snippet?: string; message?: string; detail?: Record<string, unknown>;
}

export interface AuditRecord { seq: number; ts: string; version: number; prevHash: string; hash: string; body: AuditBody }

export interface VerifyResult {
  /** 已经归档、不在数据库里的记录到第几条为止。 */
  archived?: number; ok: boolean; count: number; head: string; brokenAt?: number; message?: string }

export interface User { id: string; name: string; role: string }

export type LoginMethod = "passkey" | "password" | "oidc";
export type VerifyMethod = "passkey" | "totp" | "password";

export interface Bootstrap {
  version: string; dev: boolean; setupRequired: boolean; user: User | null; devSession?: boolean; paused?: boolean; started: number;
  /** 这次会话是怎么登录的。 */
  login?: "passkey" | "password" | "password+totp" | "oidc" | "dev";
  /** 登录页能用的方式和默认显示哪一种。 */
  auth: { methods: LoginMethod[]; default: LoginMethod | "" };
  /** 还在用首次启动生成的初始密码。 */
  initialPassword?: boolean;
  /** 构建封签：signed 发布签名校验通过；unsealed / nokey 自己编译的；invalid 文件被改过。 */
  build?: { status: "signed" | "unsealed" | "invalid" | "nokey"; fingerprint?: string };
  license?: string;
  /** 源代码仓库地址（AGPL：通过网络提供服务时要告诉用户去哪里拿源代码）。 */
  source?: string;
  /** 已经设置了 Passkey 或动态码。 */
  secondFactor?: boolean;
  /** 当前访问地址能不能用 Passkey（浏览器只允许在域名上用；IP 访问时为 false）。 */
  passkey?: boolean;
  /** 所有者还在用默认用户名 admin。 */
  defaultUsername?: boolean;
  /** 启用了 OIDC 登录时有：登录按钮上的名字。 */
  oidc?: { name: string };
}

export interface FileEntry { name: string; size: number; mode: string; modTime: string; dir: boolean; link: boolean }

export class ApiError extends Error {
  status: number;
  /** 服务端返回的中文原文（未翻译），用来判断是哪一种错误。 */
  raw: string;
  constructor(status: number, message: string, raw = "") {
    super(message);
    this.status = status;
    this.raw = raw;
  }
}

export interface Methods { passkey: boolean; password: boolean; defaultLogin: LoginMethod | ""; defaultVerify: VerifyMethod | "" }
export interface AuthFactors {
  passkeys: { id: number; name: string; createdAt: string }[];
  factors: { passkeys: number; totp: boolean; password: boolean; passwordInitial: boolean };
  methods: Methods; oidc: boolean;
  /** 能用来验证身份的方式；strong 是能批 L2 的那几种。 */
  verify: VerifyMethod[]; strong: VerifyMethod[]; defaultVerify: VerifyMethod | "";
  verified: boolean; verifiedUntil?: number;
}

async function call<T>(method: string, path: string, body?: unknown, raw?: boolean): Promise<T> {
  const headers: Record<string, string> = {};
  if (method !== "GET") headers["X-Hostower"] = "1";
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const send = () => fetch("/api/v1" + path, {
    method, headers, credentials: "same-origin",
    body: body === undefined ? undefined : raw ? (body as string) : JSON.stringify(body),
  });
  let res: Response;
  try {
    res = await send();
  } catch {
    // 页面放久了，浏览器复用的连接可能已经被网络中间设备悄悄断掉，第一次请求会直接失败；
    // 只读请求重试一次就会换一条新连接。会改东西的请求不自动重试，免得重复执行。
    if (method !== "GET") throw new ApiError(0, t("无法连接塔台"));
    await new Promise((r) => setTimeout(r, 300));
    try {
      res = await send();
    } catch {
      throw new ApiError(0, t("无法连接塔台"));
    }
  }
  const text = await res.text();
  let data: unknown = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* 非 JSON 响应 */ }
  if (!res.ok) {
    // 服务端的错误提示是中文原文，在这里统一翻译成界面语言。
    const raw = (data as { error?: string } | null)?.error;
    const msg = raw ? ts(raw) : t("请求失败（{status}）", { status: res.status });
    if (res.status === 401 && !path.startsWith("/auth")) {
      window.dispatchEvent(new Event("hostower:unauthorized"));
    }
    throw new ApiError(res.status, msg, raw ?? "");
  }
  return data as T;
}

const q = (params: Record<string, string | number | undefined>) => {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") sp.set(k, String(v));
  const s = sp.toString();
  return s ? "?" + s : "";
};

type Ceremony = { ceremony: string; options: { publicKey: unknown } };

export interface SubmitBody {
  command?: string; snippet?: string; params?: Record<string, unknown>;
  selector?: string; hosts?: string[]; intent?: string; draft?: Snippet;
  strategy?: { concurrency: number; canary: boolean; stopOnError: boolean };
}

/** 一个容器的镜像在仓库里的新情况：新构建、固定版本标签的新版本、上游的新大版本。 */
export interface ImageNews { image: string; rebuilt?: boolean; upstream?: string; newer?: string; newerImage?: string }
export interface Container { ID: string; Image: string; Names: string; Ports: string; State: string; Status: string; CreatedAt?: string; Labels?: string }
export interface Image { ID: string; Repository: string; Tag: string; Size: string; CreatedSince: string }
export interface Registry { name: string; server?: string; username?: string; updatedAt: string }
export interface DeployBody { name: string; image: string; ports: string[]; env: string[]; volumes: string[]; restart: string; replace: boolean; secrets: { env: string; secret: string }[] }
export interface NotifyChannel {
  id: string; name: string; kind: "telegram" | "feishu" | "discord" | "email"; enabled: boolean; lang: string; // 语言代码，见 locales/languages.json
  events: string[]; hosts?: string[]; hasSecret: boolean;
  telegram?: { chatId: string };
  email?: { host: string; port: number; security: "tls" | "starttls" | "none"; username?: string; from: string; to: string[] };
}
export interface NotifySecret { token?: string; webhook?: string; sign?: string; password?: string }
export interface NotifyStatus { lastOk?: number; lastError?: string; lastErrorAt?: number; sent: number; dropped: number }
export interface NotifyEvent { key: string; group: string; default: boolean }
export interface UpdateStatus {
  supported: boolean; reason?: string; current: string; auto: boolean; latest?: string; url?: string; newer: boolean;
  checkedAt?: number; state: "idle" | "checking" | "downloading" | "restarting"; error?: string;
}
export interface ContainerDetail {
  id: string; name: string; image: string; imageId: string; created: string; status: string; health?: string;
  startedAt?: string; finishedAt?: string; exitCode: number; oomKilled: boolean; restartCount: number; restart: string;
  command: string[]; workDir?: string; user?: string; privileged: boolean; networkMode?: string; memoryLimit: number; cpuLimit: number;
  project?: string; service?: string; labels: number; app?: string;
  ports: { container: string; proto: string; hostIp?: string; hostPort?: string }[];
  mounts: { type: string; source: string; destination: string; readOnly: boolean }[];
  env: { key: string; value: string; masked: boolean; fromImage: boolean }[];
  networks: { name: string; ip?: string; aliases?: string[] }[];
}
export interface ContainerAction { host: string; action: string; name?: string; image?: string; registry?: string; upload?: string; deploy?: DeployBody }
export interface UploadInfo { id: string; name: string; size: number; sha256: string }
export interface PanelField { key: string; env: string; stored: string; effective: string; fromEnv: boolean }
export interface PanelView {
  fields: PanelField[]; restartNeeded: boolean; certError?: string;
  cert?: { subject: string; names: string[] | null; issuer: string; notAfter: string; daysLeft: number };
}

export interface SelfSample { ts: number; cpu: number; mem: number; load: number; rx: number; tx: number }
export interface SelfStatus {
  hostname: string; os: string; arch: string; ncpu: number; system: boolean; memBytes: number;
  /** Hostower 自己跑在容器里：网络只算这个容器，系统名是镜像的，磁盘是数据卷。 */
  container?: boolean;
  diskPath: string; diskTotal: number; diskUsed: number; latest?: SelfSample; history: SelfSample[];
  procBytes: number; goroutines: number; dbBytes: number; goVersion: string;
}

export interface HostBody { name: string; addr: string; executor: string; credential: string; tags: string[]; sensitivity: string }

export const api = {
  bootstrap: () => call<Bootstrap>("GET", "/bootstrap"),
  hosts: () => call<{ hosts: Host[]; groups: Group[] }>("GET", "/hosts"),
  openTerminal: (host: string) => call<Req>("POST", "/terminal", { host }),
  hostMetrics: (name: string, range: string) => call<{ samples: Sample[] }>("GET", `/hosts/${encodeURIComponent(name)}/metrics` + q({ range })),
  createHost: (b: HostBody) => call("POST", "/hosts", b),
  updateHost: (b: HostBody) => call("PUT", "/hosts/" + encodeURIComponent(b.name), b),
  deleteHost: (name: string) => call("DELETE", "/hosts/" + encodeURIComponent(name)),
  select: (selector: string) => call<{ hosts: string[] }>("GET", "/select" + q({ selector })),
  classify: (b: SubmitBody) => call<Preview>("POST", "/classify", b),
  submit: (b: SubmitBody) => call<Req>("POST", "/requests", b),
  requests: () => call<{ requests: Req[] }>("GET", "/requests" + q({ limit: 200 })),
  request: (id: string) => call<Req>("GET", "/requests/" + id),
  approve: (id: string, digest: string, typed: string) => call<Req>("POST", `/requests/${id}/approve`, { digest, typed }),
  deny: (id: string, reason: string) => call<Req>("POST", `/requests/${id}/deny`, { reason }),
  cancel: (id: string) => call<Req>("POST", `/requests/${id}/cancel`),
  snippets: () => call<{ snippets: Snippet[] }>("GET", "/snippets"),
  createSnippet: (s: Snippet) => call<Snippet>("POST", "/snippets", s),
  updateSnippet: (s: Snippet) => call<Snippet>("PUT", "/snippets/" + encodeURIComponent(s.name), s),
  deleteSnippet: (name: string) => call("DELETE", "/snippets/" + encodeURIComponent(name)),
  listDir: (host: string, path: string) => call<{ path: string; entries: FileEntry[] }>("GET", "/files" + q({ host, path })),
  readFile: (host: string, path: string) =>
    call<{ path: string; size: number; content?: string; binary?: boolean; truncated: boolean }>("GET", "/files/content" + q({ host, path })),
  /** 下载地址：浏览器直接打开它，带着登录状态下载。 */
  hostAgent: (name: string, action: "install" | "remove") => call<Req>("POST", `/hosts/${encodeURIComponent(name)}/agent`, { action }),
  hostDocker: (name: string) => call<{ status: DockerStatus; ready: boolean; blocked?: string }>("GET", `/hosts/${encodeURIComponent(name)}/docker`),
  installDocker: (name: string) => call<Req>("POST", `/hosts/${encodeURIComponent(name)}/docker`, {}),
  hostReconnect: (name: string) => call<{ ok: boolean }>("POST", `/hosts/${encodeURIComponent(name)}/reconnect`, {}),
  searchFiles: (host: string, path: string, word: string) => call<{ paths: string[]; truncated: boolean }>("GET", "/files/search" + q({ host, path, q: word })),
  downloadURL: (host: string, path: string) => "/api/v1/files/download" + q({ host, path }),
  uploadFile: (host: string, dir: string, name: string, upload: string) => call<Req>("POST", "/files/upload", { host, dir, name, upload }),
  writeFile: (host: string, path: string, content: string, intent: string) => call<Req>("PUT", "/files/content", { host, path, content, intent }),
  audit: (o: { q?: string; types?: string; scope?: "key" | "all"; page?: number; size?: number }) => call<{ records: AuditRecord[]; total: number }>("GET", "/audit" + q(o)),
  speedtest: () => call<SpeedConfig>("GET", "/speedtest"),
  speedtestServers: (q: string) => call<{ servers: SpeedServer[]; service: string }>("GET", `/speedtest/servers?q=${encodeURIComponent(q)}`),
  speedtestOokla: (accept: boolean) => call<OoklaLicense>("PUT", "/speedtest/ookla", { accept }),
  speedtestGeo: (requestId: string, host: string, lang: string) => call<SpeedGeoView>("GET", `/speedtest/geo?request=${encodeURIComponent(requestId)}&host=${encodeURIComponent(host)}&lang=${encodeURIComponent(lang)}`),
  speedtestGeoSetting: (enabled: boolean) => call<{ enabled: boolean; service: string }>("PUT", "/speedtest/geo", { enabled }),
  speedtestReport: (requestId: string, host: string) => call<SpeedResult>("GET", `/speedtest/report?request=${encodeURIComponent(requestId)}&host=${encodeURIComponent(host)}`),
  speedtestTargets: (targets?: SpeedTarget[]) => call<{ targets: SpeedTarget[]; max: number }>(targets ? "PUT" : "GET", "/speedtest/routes", targets ? { targets } : undefined),
  speedtestRetention: (r?: SpeedRetention) => call<{ retention: SpeedRetention; days: number[]; perHost: number[]; stored: number }>(r ? "PUT" : "GET", "/speedtest/retention", r),
  speedtestDelete: (items: { requestId: string; host: string }[], all = false) => call<{ deleted: number }>("POST", "/speedtest/delete", { items, all }),
  runSpeedtest: (p: { hosts: string[]; method: SpeedMethod; sizeMb: number; server: number; install: string[]; routes: boolean; threads: SpeedThreads; ip: SpeedIP }) => call<Req>("POST", "/speedtest", p),
  speedtestCheck: (hosts: string[]) => call<{ hosts: { host: string; curl: boolean; trace: boolean; error?: string }[] }>("POST", "/speedtest/check", { hosts }),
  oidc: () => call<OIDCView>("GET", "/auth/oidc"),
  saveOIDC: (c: OIDCConfig & { secret: string; clearSecret: boolean }) => call<OIDCView>("PUT", "/auth/oidc", c),
  auditArchive: () => call<AuditArchiveView>("GET", "/audit/archive"),
  setAuditArchive: (p: { maxRecords: number; keepDays: number }) => call<AuditArchiveView>("PUT", "/audit/archive", p),
  runAuditArchive: () => call<{ checkpoint: AuditCheckpoint | null }>("POST", "/audit/archive/run"),
  retention: () => call<RetentionView>("GET", "/system/retention"),
  setRetention: (r: Retention) => call<RetentionView>("PUT", "/system/retention", r),
  auditRoutine: () => call<{ record: boolean }>("GET", "/audit/routine"),
  setAuditRoutine: (record: boolean) => call<{ record: boolean }>("PUT", "/audit/routine", { record }),
  auditVerify: () => call<VerifyResult>("GET", "/audit/verify"),
  system: () => call<{ version: string; dev: boolean; paused: boolean; hosts: number; groups: number; snippets: number; secretsAvailable: boolean; publicURL: string; executors: string[]; agentArchs: string[] }>("GET", "/system"),
  pause: () => call("POST", "/system/pause"),
  resume: () => call("POST", "/system/resume"),
  revokeSessions: () => call("POST", "/system/revoke-sessions"),
  reload: () => call("POST", "/system/reload"),
  secrets: () => call<{ secrets: { name: string; updatedAt: string }[]; available: boolean; autoKey?: boolean }>("GET", "/secrets"),
  putSSHSecret: (b: { name: string; user: string; privateKey: string; passphrase: string }) => call("PUT", "/secrets/ssh", b),
  deleteSecret: (name: string) => call("DELETE", "/secrets" + q({ name })),
  containers: (host: string) => call<{ available: boolean; message?: string; fix?: "docker-group"; user?: string; local?: boolean; containers: Container[]; images: Image[] }>("GET", "/containers" + q({ host })),
  containerInspect: (host: string, name: string) => call<ContainerDetail>("GET", "/containers/inspect" + q({ host, name })),
  containerLogs: (host: string, name: string) => call<{ logs: string }>("GET", "/containers/logs" + q({ host, name })),
  containerUpdates: (host: string) => call<{ containers: Record<string, ImageNews> }>("GET", "/containers/updates" + q({ host })),
  containerAction: (b: ContainerAction) => call<Req>("POST", "/containers/action", b),
  apps: () => call<{ apps: AppItem[]; categories: string[] }>("GET", "/apps"),
  appsInstalled: (hosts: string[]) => call<{ hosts: HostApps[] }>("GET", "/apps/installed" + q({ hosts: hosts.join(",") })),
  appInstall: (b: AppInstallBody) => call<AppPlanResult>("POST", "/apps/install", b),
  appAction: (b: AppActionBody) => call<AppPlanResult>("POST", "/apps/action", b),
  hostNetwork: (host: string) => call<NetworkView>("GET", `/hosts/${encodeURIComponent(host)}/network`),
  hostFirewall: (host: string, b: { action: "setup" | "allow" | "remove"; tool?: string; ports: NetPort[]; preview?: boolean }) => call<AppPlanResult>("POST", `/hosts/${encodeURIComponent(host)}/firewall`, b),
  hostSites: (host: string, b: { action: "add" | "remove" | "install-caddy" | "install-nginx"; preview?: boolean } & Partial<SiteForm>) => call<AppPlanResult>("POST", `/hosts/${encodeURIComponent(host)}/sites`, b),
  hostCerts: (host: string, b: { action: "dry-run" | "renew" | "force"; name?: string; preview?: boolean }) => call<AppPlanResult>("POST", `/hosts/${encodeURIComponent(host)}/certs`, b),
  hostFail2ban: (host: string, b: { action: "install" | "config" | "unban"; config?: F2BConfig; jail?: string; ip?: string; preview?: boolean }) => call<AppPlanResult>("POST", `/hosts/${encodeURIComponent(host)}/fail2ban`, b),
  hostNetLog: (host: string, q: { kind: "access" | "error" | "fail2ban"; server?: string; address?: string }) => call<{ text: string }>("GET", `/hosts/${encodeURIComponent(host)}/netlog?${new URLSearchParams(Object.entries(q).filter((e): e is [string, string] => !!e[1]))}`),
  appConfigure: (b: { host: string; instance: string; params?: Record<string, string>; secrets?: Record<string, string>; ai?: AIConfig; preview?: boolean }) => call<AppPlanResult>("POST", "/apps/configure", b),
  appAuto: () => call<AppAutoStatus>("GET", "/apps/autoupdate"),
  setAppAuto: (enabled: boolean) => call<AppAutoStatus>("PUT", "/apps/autoupdate", { enabled }),
  checkAppAuto: () => call<AppAutoStatus>("POST", "/apps/autoupdate/check"),
  appSecrets: () => call<{ secrets: { name: string; updatedAt: string }[]; available: boolean }>("GET", "/appsecrets"),
  putAppSecret: (name: string, value: string) => call("PUT", "/appsecrets", { name, value }),
  deleteAppSecret: (name: string) => call("DELETE", "/appsecrets" + q({ name })),
  registries: () => call<{ registries: Registry[]; available: boolean }>("GET", "/registries"),
  putRegistry: (b: { name: string; server: string; username: string; password: string }) => call("PUT", "/registries", b),
  deleteRegistry: (name: string) => call("DELETE", "/registries" + q({ name })),
  notifyList: () => call<{ channels: NotifyChannel[]; status: Record<string, NotifyStatus>; events: NotifyEvent[]; defaults: string[]; available: boolean }>("GET", "/notify"),
  notifySave: (c: NotifyChannel, secret: NotifySecret) =>
    c.id ? call<NotifyChannel>("PUT", `/notify/${c.id}`, { ...c, secret }) : call<NotifyChannel>("POST", "/notify", { ...c, secret }),
  notifyDelete: (id: string) => call("DELETE", `/notify/${id}`),
  notifyTest: (id: string) => call("POST", `/notify/${id}/test`),
  chatBot: () => call<ChatBot>("GET", "/chatbot"),
  setChatBot: (b: { enabled: boolean; channel: string; ttlDays: number }) => call<ChatBot>("PUT", "/chatbot", b),
  chatPair: () => call<ChatBot>("POST", "/chatbot/pair"),
  chatUnbind: () => call<ChatBot>("DELETE", "/chatbot/binding"),
  update: () => call<UpdateStatus>("GET", "/system/update"),
  setUpdate: (auto: boolean) => call<UpdateStatus>("PUT", "/system/update", { auto }),
  checkUpdate: () => call<UpdateStatus>("POST", "/system/update/check"),
  applyUpdate: () => call<UpdateStatus>("POST", "/system/update/apply"),
  self: () => call<SelfStatus>("GET", "/system/self"),
  panel: () => call<PanelView>("GET", "/system/panel"),
  savePanel: (values: Record<string, string>) => call<PanelView>("PUT", "/system/panel", values),
  restart: (force: boolean) => call<{ ok: boolean; started: number }>("POST", "/system/restart", { force }),
  /** 流式上传镜像包，带进度。 */
  upload: (file: File, onProgress: (fraction: number) => void) =>
    new Promise<UploadInfo>((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/v1/uploads" + q({ name: file.name }));
      xhr.setRequestHeader("X-Hostower", "1");
      xhr.setRequestHeader("Content-Type", "application/octet-stream");
      xhr.upload.onprogress = (e) => e.lengthComputable && onProgress(e.loaded / e.total);
      xhr.onerror = () => reject(new ApiError(0, t("上传中断")));
      xhr.onload = () => {
        let data: { error?: string } & Partial<UploadInfo> = {};
        try { data = JSON.parse(xhr.responseText); } catch { /* 非 JSON */ }
        if (xhr.status >= 200 && xhr.status < 300) resolve(data as UploadInfo);
        else reject(new ApiError(xhr.status, data.error ? ts(data.error) : t("上传失败（{status}）", { status: xhr.status })));
      };
      xhr.send(file);
    }),
  logout: () => call("POST", "/auth/logout"),
  devLogin: (token: string) => call<{ user: User }>("POST", "/auth/dev", { token }),
  passwordLogin: (name: string, password: string) => call<{ user?: User; totp?: boolean; ticket?: string }>("POST", "/auth/password/login", { name, password }),
  passwordTOTP: (ticket: string, code: string) => call<{ user: User }>("POST", "/auth/password/totp", { ticket, code }),
  factors: () => call<AuthFactors>("GET", "/auth/factors"),
  setMethods: (m: Methods) => call<{ methods: Methods }>("PUT", "/auth/methods", m),
  setPassword: (password: string) => call("POST", "/auth/password", { password }),
  setUsername: (name: string) => call<{ name: string }>("POST", "/auth/username", { name }),
  verifyTOTP: (code: string) => call("POST", "/auth/verify/totp", { code }),
  verifyPassword: (password: string) => call("POST", "/auth/verify/password", { password }),
  totpBegin: () => call<{ secret: string; uri: string }>("POST", "/auth/totp/begin"),
  totpFinish: (code: string) => call("POST", "/auth/totp/finish", { code }),
  totpRemove: () => call("DELETE", "/auth/totp"),
  deletePasskey: (id: number) => call("DELETE", `/auth/passkeys/${id}`),

  // ---- Passkey（WebAuthn）流程 ----
  async login() {
    const c = await call<Ceremony>("POST", "/auth/login/begin");
    const cred = await getCredential(c.options.publicKey);
    return call<{ user: User }>("POST", "/auth/login/finish" + q({ ceremony: c.ceremony }), cred, true);
  },
  async reauth() {
    const c = await call<Ceremony>("POST", "/auth/reauth/begin");
    const cred = await getCredential(c.options.publicKey);
    return call("POST", "/auth/reauth/finish" + q({ ceremony: c.ceremony }), cred, true);
  },
  async addPasskey(name: string) {
    const c = await call<Ceremony>("POST", "/auth/passkeys/begin");
    const cred = await createCredential(c.options.publicKey);
    return call("POST", "/auth/passkeys/finish" + q({ ceremony: c.ceremony, name }), cred, true);
  },
};

type PKC = typeof PublicKeyCredential & {
  parseCreationOptionsFromJSON?: (o: unknown) => PublicKeyCredentialCreationOptions;
  parseRequestOptionsFromJSON?: (o: unknown) => PublicKeyCredentialRequestOptions;
};

function pkc(): PKC {
  const P = window.PublicKeyCredential as PKC | undefined;
  if (!P || !P.parseCreationOptionsFromJSON || !P.parseRequestOptionsFromJSON) {
    throw new ApiError(0, t("这个浏览器不支持 Passkey，请换用较新的浏览器"));
  }
  return P;
}

async function createCredential(options: unknown): Promise<string> {
  const publicKey = pkc().parseCreationOptionsFromJSON!(options);
  const cred = await navigator.credentials.create({ publicKey }).catch(() => null);
  if (!cred) throw new ApiError(0, t("Passkey 注册被取消"));
  return JSON.stringify((cred as PublicKeyCredential).toJSON());
}

async function getCredential(options: unknown): Promise<string> {
  const publicKey = pkc().parseRequestOptionsFromJSON!(options);
  const cred = await navigator.credentials.get({ publicKey }).catch(() => null);
  if (!cred) throw new ApiError(0, t("Passkey 验证被取消"));
  return JSON.stringify((cred as PublicKeyCredential).toJSON());
}

/** 聊天命令（Telegram 机器人）的设置、绑定和运行状态。 */
export type ChatBot = {
  enabled: boolean;
  channel: string;
  ttlDays: number;
  /** expiresAt 没有表示永久有效。 */
  binding?: { chatUserId: number; chatName: string; userId: string; userName: string; boundAt: string; expiresAt?: string };
  pairing?: { code: string; expiresAt: string };
  running: boolean;
  lastError?: string;
  botUsername?: string;
  channels: { id: string; name: string; enabled: boolean }[];
};

/** 指标历史的保留时长（天）。 */
export interface Retention { fineDays: number; coarseDays: number }
export interface RetentionView { retention: Retention; fine: number[]; coarse: number[]; stored: number }

/** 一台主机的一次测速结果（速率是比特每秒）。 */
export interface SpeedRetention { days: number; perHost: number }
export type SpeedMethod = "ookla" | "cloudflare";
export type SpeedThreads = "single" | "multi";
/** 空是自动（按系统默认）。 */
export type SpeedIP = "" | "4" | "6";
export interface SpeedResult { requestId: string; host: string; ts: number; pingMs: number; downBps: number; upBps: number; sizeMb: number; method: SpeedMethod; server?: string; threads?: SpeedThreads | ""; ip?: SpeedIP; error?: string; detail?: SpeedDetail }
export interface SpeedConfig {
  results: SpeedResult[]; sizes: number[]; server: string; retention: SpeedRetention; targets: SpeedTarget[]; maxTargets: number;
  geo: boolean; geoService: string; ookla: OoklaLicense; searchService: string; presets: SpeedPreset[];
}
/** 常用的路由追踪目标：城市和运营商是中文原文，显示时翻译。 */
export interface SpeedPreset { city: string; isp: string; target: string }
/** 所有者有没有接受 Speedtest by Ookla 的许可协议和隐私政策。 */
export interface OoklaLicense { accepted: boolean; by?: string; at?: string; version: string; terms: string; privacy: string }
/** 搜到的 Ookla 测速服务器（第三方给的文字，只按纯文本显示）。 */
export interface SpeedServer { id: number; sponsor: string; name: string; country: string; cc?: string; host?: string; distance?: number }
/** Ookla 测速的明细。 */
export interface SpeedOokla {
  server: { id: number; name?: string; location?: string; country?: string; host?: string; ip?: string };
  isp?: string; packetLoss?: number; jitter: number; low?: number; high?: number; downLatency?: number; upLatency?: number; url?: string;
  /** system：主机上已经装好的官方程序；pinned：Hostower 下载的锁定版本。 */
  binary?: "system" | "pinned"; version?: string;
}
/** 测速明细。时间都是毫秒；远端来的字段（IP、节点、路由）只按纯文本显示。 */
export interface SpeedDetail {
  v: number;
  pings: { dns: number; tcp: number; tls: number; wait: number }[];
  down?: SpeedTransfer;
  up?: SpeedTransfer;
  net: { ip?: string; loc?: string; colo?: string; http?: string; tls?: string; serverIp?: string };
  routes?: SpeedRoute[];
  ookla?: SpeedOokla;
}
export interface SpeedTransfer { bytes: number; ms: number; ttfb?: number; bps: number; conns?: number }
export interface SpeedHop { n: number; ip?: string; ms?: number; net?: string }
export interface SpeedRoute { target: string; name?: string; hops: SpeedHop[]; nets: string[]; line?: string; error?: string }
/** 节点地区：第三方查询服务给的文字，只按纯文本显示。 */
export interface SpeedGeo { country?: string; cc?: string; region?: string; city?: string; isp?: string; as?: string; net?: string }
export interface SpeedGeoView { enabled: boolean; service: string; hops?: Record<string, SpeedGeo>; routes?: SpeedRoute[]; error?: string }
export interface SpeedTarget { name: string; target: string }

/** OIDC 登录的设置（客户端密钥不在这里，只有有没有设置）。 */
export interface OIDCConfig { enabled: boolean; name: string; issuer: string; clientId: string; allowed: string[]; hasSecret?: boolean }
export interface OIDCView { config: OIDCConfig; redirectUrl: string; secretsAvailable: boolean }

/** 一次审计归档留下的检查点。 */
export interface AuditCheckpoint { upto: number; hash: string; first: number; file: string; sha256: string; records: number; createdAt: string }
export interface AuditArchiveView {
  policy: { maxRecords: number; keepDays: number }; maxRecordChoices: number[]; keepDayChoices: number[];
  checkpoints: AuditCheckpoint[]; archives: number; archivedRecords: number; records: number;
}

// ---------- 应用市场 ----------
export interface AppParam { name: string; title: string; type: "port" | "string" | "enum" | "secret"; default?: string; options?: string[]; hint?: string; env?: string }
export interface AppItem {
  name: string; version: string; title: string; category: string; icon: string; logo?: string; summary: string; description: string; preview?: boolean;
  type: "compose" | "npm"; params: AppParam[] | null; npm?: { package: string; version: string; bins: string[]; node: number }; notes?: string; permissions: Finding[];
  /** AI 命令行工具的配置方式：供应商预置、订阅登录命令。 */
  ai?: { kind: string; login?: string; loginHint?: string; providers: AIProvider[] };
  /** 升级说明；blockFrom 里的旧版本不能原地升级。 */
  upgrade?: { note?: string; blockFrom?: string[] };
}
export interface AIProvider { id: string; title: string; baseUrl?: string; models?: string[]; key: "required" | "optional"; note?: string; custom?: boolean; apis?: string[] }
export interface AIConfig { provider: string; baseUrl?: string; model?: string; smallModel?: string; secret?: string; api?: string }
export interface AppRecord { app: string; version: string; type: "compose" | "npm"; instance: string; params?: Record<string, string>; bins?: string[]; envs?: string[]; config?: Record<string, string> }
export interface AppInstalled extends AppRecord { containers: { name: string; state: string; status: string; ports: string }[]; update?: string; blocked?: string; rebuilt?: boolean; upstream?: string; known: boolean }
export interface HostApps { host: string; installed: AppInstalled[]; error?: string }
export interface AppInstallTarget { host: string; instance: string; params: Record<string, string>; secrets: Record<string, string>; version?: string; ai?: AIConfig }
export interface AppInstallBody { app: string; targets: AppInstallTarget[]; preview?: boolean }
export type AppActionName = "start" | "stop" | "restart" | "update" | "uninstall" | "purge";
export interface AppActionBody { action: AppActionName; targets: { host: string; instance: string }[]; version?: string; preview?: boolean }
export interface AppAutoStatus { enabled: boolean; running: boolean; intervalHours: number; submitted: number; lastRun?: number; error?: string }
export interface AppPlanResult {
  level?: Level; targets?: { host: string; command: string; compose?: string; level: Level; findings: Finding[] }[];
  requests?: Req[] | number; error?: string;
}

// ---------- 主机的网络入口 ----------
export interface NetPort { port: number; proto: "tcp" | "udp" }
export type SiteServer = "caddy" | "nginx";
export type SiteKind = "proxy" | "static" | "redirect";
export type SiteTLS = "auto" | "internal" | "le" | "custom" | "http";
export interface NetSite { server: SiteServer; address: string; kind: SiteKind; target: string; tls: SiteTLS; maxBody?: number; hsts?: boolean; keepHttp?: boolean; perm?: boolean }
/** “添加站点”的表单，和服务端的 netcfg.SiteRequest 对应。 */
export interface SiteForm {
  server: SiteServer; address: string; kind: SiteKind; port: number; dir: string; url: string; tls: SiteTLS;
  email: string; agree: boolean; cert: string; key: string; maxBody: number; hsts: boolean; keepHttp: boolean; perm: boolean;
}
export interface NetCert { name: string; domains: string[]; expiry: string; days: number; valid: boolean }
export interface F2BJail { name: string; failed: number; totalFailed: number; banned: number; totalBanned: number; ips: string[] }
export interface F2BConfig { bantime: number; findtime: number; maxretry: number; ignore: string[]; jails: string[] }
export interface NetworkView {
  status: {
    os: string; family: string; root: boolean; sudo: boolean; noNewPrivs: boolean; sshPort: number;
    firewall: { tool: "" | "ufw" | "firewalld" | "conflict"; recommended: string; ufwInstalled: boolean; ufwActive: boolean; firewalldInstalled: boolean; firewalldActive: boolean; rules: string[]; unknown: boolean };
    caddy: { installed: boolean; active: boolean; imported: boolean; installable: boolean };
    nginx: { installed: boolean; active: boolean; installable: boolean };
    certbot: { installed: boolean; autoRenew: boolean };
    certs: NetCert[];
    fail2ban: { installed: boolean; active: boolean; installable: boolean; unknown: boolean; jails: F2BJail[]; config: F2BConfig | null; authLog: boolean };
    clientIp: string;
    sites: NetSite[];
    listen: { port: number; addr: string }[];
    docker: { container: string; port: number; proto: string }[];
  };
  keep: NetPort[];
}
