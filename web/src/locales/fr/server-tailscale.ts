// fr 词典：Tailscale 的服务端文字。
const dict: Record<string, string> = {
  "改变这台主机的 Tailscale 组网（影响它能被谁连上）":
    "Modifier le réseau Tailscale de cet hôte (change qui peut s’y connecter)",
  "在 {a} 上安装 Tailscale":
    "Installer Tailscale sur {a}",
  "把 {a} 加入 Tailscale 网络":
    "Ajouter {a} au réseau Tailscale",
  "修改 {a} 上的 Tailscale 设置":
    "Modifier les réglages Tailscale sur {a}",
  "断开 {a} 的 Tailscale":
    "Déconnecter Tailscale sur {a}",
  "让 {a} 退出 Tailscale 登录":
    "Fermer la session Tailscale de {a}",
  "Tailscale 授权密钥 {a}":
    "Clé d’authentification Tailscale {a}",
  "管不了 Tailscale":
    "Impossible de gérer Tailscale",
  "登录账号不是 root，也没有 sudo：管不了 Tailscale":
    "Le compte de connexion n’est pas root et n’a pas sudo : impossible de gérer Tailscale",
  "Tailscale 已经装好了":
    "Tailscale est déjà installé",
  "这台主机还没有装 Tailscale":
    "Tailscale n’est pas encore installé sur cet hôte",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装":
    "L’installation en un clic de Tailscale n’est pas prise en charge sur ce système ; suivez les instructions du site de Tailscale",
  "Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符":
    "Le nom dans Tailscale ne peut contenir que des lettres, des chiffres et des tirets, ne peut ni commencer ni finir par un tiret et compte au plus 63 caractères",
  "这台主机已经在 Tailscale 网络里了：要改设置用“设置”":
    "Cet hôte est déjà dans le réseau Tailscale : pour modifier les réglages, utilisez « Réglages »",
  "这台主机还没有登录 Tailscale：先“加入网络”":
    "Cet hôte n’est pas encore authentifié auprès de Tailscale : utilisez d’abord « Rejoindre le réseau »",
  "Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作":
    "Hostower se connecte à cet hôte via Tailscale : le déconnecter ou fermer la session lui couperait son propre accès. Remplacez d’abord l’adresse de cet hôte par une adresse qui ne passe pas par Tailscale, puis réessayez",
  "这台主机现在没有连在 Tailscale 网络里":
    "Cet hôte n’est pas connecté au réseau Tailscale actuellement",
  "这台主机没有登录 Tailscale":
    "Cet hôte n’est pas authentifié auprès de Tailscale",
  "应用密钥的名称不合法":
    "Nom du secret d’application invalide",
};

export default dict;
