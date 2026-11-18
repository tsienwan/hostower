// fr 词典：设置页“面板”：环境变量接管时的说明。
const dict: Record<string, string> = {
  "现在生效的是环境变量 ": "Actuellement en vigueur : la valeur de la variable d’environnement ",
  " 的值": "",
  "。这里保存的值要等去掉这个环境变量、重启后才生效。": ". Une valeur enregistrée ici ne prend effet qu’après suppression de cette variable et redémarrage.",
  "有 {n} 项设置由环境变量决定：": "{n} réglages sont fixés par des variables d’environnement : ",
  "。环境变量优先于这里保存的值。": ". Les variables d’environnement l’emportent sur les valeurs enregistrées ici.",
  "想改成在这里管理：先在下面填好并保存（可以点“带入环境变量的值”），再去掉这些环境变量、重启 Hostower。这样顺序做，不会断开访问。": "Pour les gérer ici : remplissez et enregistrez d’abord les valeurs ci-dessous (bouton « Reprendre les valeurs de l’environnement »), puis supprimez ces variables et redémarrez Hostower. Dans cet ordre, vous ne perdez pas l’accès.",
  "systemd 部署：运行 ": "systemd : lancez ",
  " 删掉对应的 Environment= 行（或检查 /etc/systemd/system/hostower.service），然后 ": " et supprimez les lignes Environment= correspondantes (voir aussi /etc/systemd/system/hostower.service), puis ",
  "。Docker 部署：删掉 compose 文件 environment 里对应的行，再 ": ". Docker : supprimez les lignes correspondantes sous environment dans le fichier compose, puis ",
  "带入环境变量的值": "Reprendre les valeurs de l’environnement",
  "谁来提供 HTTPS。对外监听时必须二选一，否则塔台拒绝启动。选“塔台自己提供”后填证书和私钥文件的路径。": "Qui fournit le HTTPS. En écoute publique, il faut choisir l’un des deux, sinon la tour refuse de démarrer. Choisissez « La tour elle-même » pour saisir les chemins du certificat et de la clé privée.",
};

export default dict;
