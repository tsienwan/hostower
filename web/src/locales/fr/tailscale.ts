// fr 词典：Tailscale 卡片，以及站点转发到另一台主机的提示。
const dict: Record<string, string> = {
  "在 Tailscale 网络里的名字":
    "Nom dans le réseau Tailscale",
  "留空就用系统的主机名。只能用字母、数字和连字符。":
    "Laissez vide pour utiliser le nom d’hôte du système. Lettres, chiffres et tirets uniquement.",
  "名字只能用字母、数字和连字符，不能以连字符开头或结尾。":
    "Le nom ne peut contenir que des lettres, des chiffres et des tirets, et ne peut ni commencer ni finir par un tiret.",
  "使用 Tailscale 网络的 DNS（可以用名字访问别的主机；会接管这台主机的 DNS 设置）":
    "Utiliser le DNS du réseau Tailscale (les autres hôtes sont joignables par leur nom ; prend la main sur les réglages DNS de cet hôte)",
  "接受别的节点通告的子网路由":
    "Accepter les routes de sous-réseau annoncées par d’autres nœuds",
  "开启 Tailscale SSH":
    "Activer Tailscale SSH",
  "开启后，Tailscale 网络里被访问规则允许的人可以直接登录这台主机，不经过 sshd 的密钥和这里的审批。只在你清楚那个网络的访问规则时开启。":
    "Une fois activé, les personnes autorisées par les règles d’accès du réseau Tailscale peuvent se connecter directement à cet hôte, sans passer par les clés de sshd ni par les approbations d’ici. N’activez que si vous connaissez bien les règles d’accès de ce réseau.",
  "让 {host} 重新连上 Tailscale":
    "Reconnecter {host} à Tailscale",
  "把 {host} 加入 Tailscale 网络":
    "Ajouter {host} au réseau Tailscale",
  "怎么登录":
    "Mode de connexion",
  "在浏览器里登录":
    "Connexion dans le navigateur",
  "不用密钥":
    "Sans clé",
  "授权密钥":
    "Clé d’authentification",
  "用这条应用密钥里存的授权密钥登录。密钥在执行时从标准输入送给 tailscale，不会出现在命令、请求和审计里。":
    "Connexion avec la clé d’authentification enregistrée dans ce secret d’application. La clé est transmise à tailscale par l’entrée standard lors de l’exécution et n’apparaît ni dans la commande, ni dans la demande, ni dans l’audit.",
  "提交并批准后，请求的输出里会出现一个登录链接：在浏览器里打开它并登录 Tailscale 账号，请求会一直等到登录完成（最多 9 分钟）。":
    "Après soumission et approbation, un lien de connexion apparaît dans la sortie de la demande : ouvrez-le dans un navigateur et connectez-vous à votre compte Tailscale ; la demande attend la fin de la connexion (9 minutes au plus).",
  "想不用人登录，先在 Tailscale 后台生成授权密钥，到 ":
    "Pour une connexion sans intervention humaine, générez d’abord une clé d’authentification dans la console d’administration Tailscale, enregistrez-la comme secret d’application sur la page ",
  " 页存成应用密钥，再回来选。":
    ", puis revenez la sélectionner ici.",
  "想不用人登录，请所有者把授权密钥存成应用密钥。":
    "Pour une connexion sans intervention humaine, demandez au propriétaire d’enregistrer une clé d’authentification comme secret d’application.",
  "加入以后，这台主机会多一个内网地址，同一个 Tailscale 网络里的设备按那边的访问规则可以连到它。这是高危操作，批准时要重新验证身份。":
    "Une fois dans le réseau, cet hôte reçoit une adresse interne supplémentaire, et les appareils du même réseau Tailscale peuvent s’y connecter selon les règles d’accès de ce réseau. C’est une opération à haut risque : l’approbation exige de revérifier votre identité.",
  "{host} 上的 Tailscale 设置":
    "Réglages Tailscale sur {host}",
  "读不出这台主机当前的 Tailscale 设置，下面显示的是默认值：提交会按这里的选择覆盖原来的设置。":
    "Impossible de lire les réglages Tailscale actuels de cet hôte ; les valeurs ci-dessous sont celles par défaut : la soumission remplacera les réglages existants par les choix faits ici.",
  "Hostower 就是经 Tailscale 连到这台主机的，断开会把它自己断在外面":
    "Hostower se connecte à cet hôte via Tailscale : le déconnecter lui couperait son propre accès",
  "Tailscale（主机之间的加密内网）":
    "Tailscale (réseau interne chiffré entre hôtes)",
  "已连接":
    "Connecté",
  "已断开":
    "Déconnecté",
  "还没有登录":
    "Pas encore authentifié",
  "等待管理员批准":
    "En attente d’approbation par l’administrateur",
  "正在加入，等待登录…":
    "Ajout en cours, en attente d’authentification…",
  "断开":
    "Déconnecter",
  "重新连上":
    "Reconnecter",
  "加入网络":
    "Rejoindre le réseau",
  "把几台主机连进同一个加密的内网：每台有一个 100.x 的地址，主机之间互相访问不用把端口对公网开放。安装、加入、改设置、断开都是高危操作，批准时要重新验证身份。":
    "Relie plusieurs hôtes dans un même réseau interne chiffré : chacun reçoit une adresse en 100.x et les hôtes communiquent entre eux sans ouvrir de ports sur Internet. Installer, rejoindre, modifier les réglages et déconnecter sont des opérations à haut risque : l’approbation exige de revérifier votre identité.",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装。":
    "L’installation en un clic de Tailscale n’est pas prise en charge sur ce système ; suivez les instructions du site de Tailscale.",
  "内网地址":
    "Adresse interne",
  "名字":
    "Nom",
  "所在网络":
    "Réseau",
  "别的主机加入同一个网络后，可以用上面的地址访问这台主机。在另一台主机上添加站点时，“转发到哪里”填这个地址，两台之间这一段就是加密的。":
    "Une fois d’autres hôtes dans le même réseau, ils peuvent joindre cet hôte à l’adresse ci-dessus. En ajoutant un site sur un autre hôte, indiquez cette adresse dans « Rediriger vers » : le tronçon entre les deux hôtes est alors chiffré.",
  "已经装好，还没有登录：点“加入网络”。":
    "Déjà installé, pas encore authentifié : cliquez sur « Rejoindre le réseau ».",
  "这台主机已经登录，但那个 Tailscale 网络要求管理员批准新设备：到 Tailscale 后台批准它，再回来刷新。":
    "Cet hôte est authentifié, mais ce réseau Tailscale exige qu’un administrateur approuve les nouveaux appareils : approuvez-le dans la console d’administration Tailscale, puis revenez actualiser.",
  "在 {host} 上安装 Tailscale":
    "Installer Tailscale sur {host}",
  "添加 Tailscale 的官方软件源（pkgs.tailscale.com）和它的签名密钥，用系统的包管理器安装，并启动 tailscaled 服务。装完还没有登录，要再“加入网络”。":
    "Ajoute le dépôt officiel de Tailscale (pkgs.tailscale.com) et sa clé de signature, installe avec le gestionnaire de paquets du système et démarre le service tailscaled. Après l’installation, l’hôte n’est pas encore authentifié : il faut ensuite « Rejoindre le réseau ».",
  "这台主机要能连上 pkgs.tailscale.com。Tailscale 决定主机能被谁连上，安装是高危操作，批准时要重新验证身份。":
    "Cet hôte doit pouvoir joindre pkgs.tailscale.com. Tailscale détermine qui peut se connecter à l’hôte : l’installation est une opération à haut risque et l’approbation exige de revérifier votre identité.",
  "断开 {host} 的 Tailscale":
    "Déconnecter Tailscale sur {host}",
  "断开后这台主机的内网地址不再可用：经这个地址访问它的站点和连接都会断。登录状态还在，随时可以重新连上；要彻底离开那个网络，断开后再“退出登录”。":
    "Après la déconnexion, l’adresse interne de cet hôte n’est plus utilisable : les sites et connexions qui passent par elle sont coupés. La session reste ouverte et vous pouvez reconnecter à tout moment ; pour quitter définitivement ce réseau, déconnectez puis utilisez « Se déconnecter ».",
  "让 {host} 退出 Tailscale 登录":
    "Fermer la session Tailscale de {host}",
  "退出登录后，这台主机不再属于那个 Tailscale 网络，再加入要重新登录或用新的授权密钥。":
    "Après la fermeture de session, cet hôte ne fait plus partie de ce réseau Tailscale ; pour le rejoindre à nouveau, il faudra s’authentifier de nouveau ou utiliser une nouvelle clé d’authentification.",
  "{addr} 是 Tailscale 的内网地址，而这台主机还没有连上 Tailscale 网络，转发不过去：先在这一页的 Tailscale 卡片里加入网络。":
    "{addr} est une adresse interne Tailscale, mais cet hôte n’est pas encore connecté au réseau Tailscale : la redirection est impossible. Rejoignez d’abord le réseau depuis la carte Tailscale de cette page.",
  "流量会先到这台主机，再经 Tailscale 转到 {addr}：上传和下载都算这台主机的流量。两台主机之间这一段是加密的，那台主机的端口不用对公网开放；它上面的服务不能只监听 127.0.0.1。":
    "Le trafic arrive d’abord sur cet hôte, puis est transmis à {addr} via Tailscale : envois et téléchargements comptent dans le trafic de cet hôte. Le tronçon entre les deux hôtes est chiffré et le port de l’autre hôte n’a pas besoin d’être ouvert sur Internet ; son service ne doit pas écouter uniquement sur 127.0.0.1.",
};

export default dict;
