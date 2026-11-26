// ko 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "이 호스트의 Tailscale 네트워크 구성 변경(누가 연결할 수 있는지에 영향을 줍니다)",
  "在 {a} 上安装 Tailscale":
    "{a}에 Tailscale 설치",
  "把 {a} 加入 Tailscale 网络":
    "{a}을(를) Tailscale 네트워크에 참여시키기",
  "修改 {a} 上的 Tailscale 设置":
    "{a}의 Tailscale 설정 변경",
  "断开 {a} 的 Tailscale":
    "{a}의 Tailscale 연결 끊기",
  "让 {a} 退出 Tailscale 登录":
    "{a}을(를) Tailscale에서 로그아웃",
  "Tailscale 授权密钥 {a}":
    "Tailscale 인증 키 {a}",
  "管不了 Tailscale":
    "Tailscale을 관리할 수 없습니다",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "로그인 계정이 root가 아니고 sudo도 없어 Tailscale을 관리할 수 없습니다",
  "Tailscale 已经装好了":
    "Tailscale이 이미 설치되어 있습니다",
  "这台主机还没有装 Tailscale":
    "이 호스트에는 아직 Tailscale이 설치되지 않았습니다",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "이 호스트의 시스템은 Tailscale 원클릭 설치를 지원하지 않습니다. Tailscale 공식 사이트의 안내에 따라 설치하세요",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "Tailscale에서 쓸 이름에는 영문자, 숫자, 하이픈만 쓸 수 있고 하이픈으로 시작하거나 끝날 수 없으며 최대 63자입니다",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "이 호스트는 이미 Tailscale 네트워크에 있습니다. 설정을 바꾸려면 “설정”을 쓰세요",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "이 호스트는 아직 Tailscale에 로그인하지 않았습니다. 먼저 “네트워크 참여”를 하세요",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower가 Tailscale을 거쳐 이 호스트에 연결하고 있어, 연결을 끊거나 로그아웃하면 Hostower 자신이 접속하지 못하게 됩니다. 먼저 이 호스트의 주소를 Tailscale을 거치지 않는 주소로 바꾼 뒤 진행하세요",
  "这台主机现在没有连在 Tailscale 网络里":
    "이 호스트는 지금 Tailscale 네트워크에 연결되어 있지 않습니다",
  "这台主机没有登录 Tailscale":
    "이 호스트는 Tailscale에 로그인하지 않았습니다",
  "应用密钥的名称不合法":
    "앱 시크릿 이름이 올바르지 않습니다",
};

export default dict;
