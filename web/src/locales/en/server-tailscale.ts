// en 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "Changes this host's Tailscale networking (affects who can connect to it)",
  "在 {a} 上安装 Tailscale":
    "Install Tailscale on {a}",
  "把 {a} 加入 Tailscale 网络":
    "Join {a} to the Tailscale network",
  "修改 {a} 上的 Tailscale 设置":
    "Change Tailscale settings on {a}",
  "断开 {a} 的 Tailscale":
    "Disconnect {a} from Tailscale",
  "让 {a} 退出 Tailscale 登录":
    "Sign {a} out of Tailscale",
  "Tailscale 授权密钥 {a}":
    "Tailscale auth key {a}",
  "管不了 Tailscale":
    "Tailscale cannot be managed",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "The login account is not root and has no sudo: Tailscale cannot be managed",
  "Tailscale 已经装好了":
    "Tailscale is already installed",
  "这台主机还没有装 Tailscale":
    "Tailscale is not installed on this host yet",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "One-click Tailscale installation is not supported on this system; follow the instructions on the Tailscale website",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "A Tailscale name may only contain letters, digits and hyphens, cannot start or end with a hyphen, and is at most 63 characters",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "This host is already on a Tailscale network: use “Settings” to change its settings",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "This host is not signed in to Tailscale yet: use “Join network” first",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower reaches this host through Tailscale: disconnecting or signing out would lock it out. Change this host's address to one that does not go through Tailscale first, then try again",
  "这台主机现在没有连在 Tailscale 网络里":
    "This host is not connected to a Tailscale network right now",
  "这台主机没有登录 Tailscale":
    "This host is not signed in to Tailscale",
  "应用密钥的名称不合法":
    "Invalid app secret name",
};

export default dict;
