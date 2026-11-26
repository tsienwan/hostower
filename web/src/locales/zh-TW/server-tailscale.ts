// zh-TW 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "改變這台主機的 Tailscale 組網（影響它能被誰連上）",
  "在 {a} 上安装 Tailscale":
    "在 {a} 上安裝 Tailscale",
  "把 {a} 加入 Tailscale 网络":
    "把 {a} 加入 Tailscale 網路",
  "修改 {a} 上的 Tailscale 设置":
    "修改 {a} 上的 Tailscale 設定",
  "断开 {a} 的 Tailscale":
    "中斷 {a} 的 Tailscale",
  "让 {a} 退出 Tailscale 登录":
    "讓 {a} 登出 Tailscale",
  "Tailscale 授权密钥 {a}":
    "Tailscale 授權密鑰 {a}",
  "管不了 Tailscale":
    "管不了 Tailscale",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "登入帳號不是 root，也沒有 sudo：管不了 Tailscale",
  "Tailscale 已经装好了":
    "Tailscale 已經裝好了",
  "这台主机还没有装 Tailscale":
    "這台主機還沒有裝 Tailscale",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "這台主機的系統不支援一鍵安裝 Tailscale，請按 Tailscale 官網的說明安裝",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "Tailscale 裡的名字只能用字母、數字和連字號，不能以連字號開頭或結尾，最長 63 個字元",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "這台主機已經在 Tailscale 網路裡了：要改設定用「設定」",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "這台主機還沒有登入 Tailscale：先「加入網路」",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower 就是經 Tailscale 連到這台主機的：中斷或登出會把它自己斷在外面。先把這台主機的位址改成不經過 Tailscale 的，再來操作",
  "这台主机现在没有连在 Tailscale 网络里":
    "這台主機現在沒有連在 Tailscale 網路裡",
  "这台主机没有登录 Tailscale":
    "這台主機沒有登入 Tailscale",
  "应用密钥的名称不合法":
    "應用程式密鑰的名稱不合法",
};

export default dict;
