// ja 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "このホストの Tailscale ネットワーク構成を変更（誰が接続できるかに影響します）",
  "在 {a} 上安装 Tailscale":
    "{a} に Tailscale をインストール",
  "把 {a} 加入 Tailscale 网络":
    "{a} を Tailscale ネットワークに参加させる",
  "修改 {a} 上的 Tailscale 设置":
    "{a} の Tailscale 設定を変更",
  "断开 {a} 的 Tailscale":
    "{a} の Tailscale を切断",
  "让 {a} 退出 Tailscale 登录":
    "{a} を Tailscale からログアウト",
  "Tailscale 授权密钥 {a}":
    "Tailscale 認証キー {a}",
  "管不了 Tailscale":
    "Tailscale を管理できません",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "ログインアカウントは root ではなく sudo もないため、Tailscale を管理できません",
  "Tailscale 已经装好了":
    "Tailscale はすでにインストールされています",
  "这台主机还没有装 Tailscale":
    "このホストには Tailscale がまだインストールされていません",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "このホストの OS では Tailscale のワンクリックインストールに対応していません。Tailscale 公式サイトの説明に従ってインストールしてください",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "Tailscale での名前に使えるのは英字、数字、ハイフンだけで、先頭と末尾にハイフンは使えず、最長 63 文字です",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "このホストはすでに Tailscale ネットワークに参加しています。設定を変えるには「設定」を使ってください",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "このホストはまだ Tailscale にログインしていません。先に「ネットワークに参加」してください",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower は Tailscale 経由でこのホストに接続しているため、切断やログアウトをすると Hostower 自身が締め出されます。先にこのホストのアドレスを Tailscale を経由しないものに変えてから操作してください",
  "这台主机现在没有连在 Tailscale 网络里":
    "このホストは現在 Tailscale ネットワークに接続していません",
  "这台主机没有登录 Tailscale":
    "このホストは Tailscale にログインしていません",
  "应用密钥的名称不合法":
    "アプリシークレットの名前が正しくありません",
};

export default dict;
