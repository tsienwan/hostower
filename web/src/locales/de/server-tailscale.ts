// de 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "Tailscale-Vernetzung dieses Hosts ändern (beeinflusst, wer ihn erreichen kann)",
  "在 {a} 上安装 Tailscale":
    "Tailscale auf {a} installieren",
  "把 {a} 加入 Tailscale 网络":
    "{a} zum Tailscale-Netz hinzufügen",
  "修改 {a} 上的 Tailscale 设置":
    "Tailscale-Einstellungen auf {a} ändern",
  "断开 {a} 的 Tailscale":
    "Tailscale auf {a} trennen",
  "让 {a} 退出 Tailscale 登录":
    "{a} bei Tailscale abmelden",
  "Tailscale 授权密钥 {a}":
    "Tailscale-Auth-Schlüssel {a}",
  "管不了 Tailscale":
    "Tailscale lässt sich nicht verwalten",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "Das Anmeldekonto ist nicht root und hat kein sudo: Tailscale lässt sich nicht verwalten",
  "Tailscale 已经装好了":
    "Tailscale ist bereits installiert",
  "这台主机还没有装 Tailscale":
    "Tailscale ist auf diesem Host noch nicht installiert",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "Die Ein-Klick-Installation von Tailscale wird auf diesem System nicht unterstützt; folgen Sie der Anleitung auf der Tailscale-Website",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "Der Name in Tailscale darf nur Buchstaben, Ziffern und Bindestriche enthalten, nicht mit einem Bindestrich beginnen oder enden und höchstens 63 Zeichen lang sein",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "Dieser Host ist bereits im Tailscale-Netz: Einstellungen über „Einstellungen“ ändern",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "Dieser Host ist noch nicht bei Tailscale angemeldet: zuerst „Netz beitreten“",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower erreicht diesen Host über Tailscale: Trennen oder Abmelden würde es selbst aussperren. Zuerst die Adresse dieses Hosts auf eine ändern, die nicht über Tailscale läuft, und dann erneut versuchen",
  "这台主机现在没有连在 Tailscale 网络里":
    "Dieser Host ist derzeit nicht mit dem Tailscale-Netz verbunden",
  "这台主机没有登录 Tailscale":
    "Dieser Host ist nicht bei Tailscale angemeldet",
  "应用密钥的名称不合法":
    "Ungültiger Name des App-Secrets",
};

export default dict;
