// fr 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以编辑这台主机，把连接方式改成 SSH（地址 127.0.0.1，选一条能 sudo 的账号的凭据），再来操作": "Le processus qui exécute les commandes ne peut pas élever ses privilèges (le service définit NoNewPrivileges), sudo ne peut pas fonctionner : {a}. Modifiez cet hôte et passez sa connexion en SSH (adresse 127.0.0.1, identifiant d’un compte avec sudo), puis réessayez",
  "登录账号不是 root，也没有 sudo：{a}": "Le compte de connexion n’est pas root et n’a pas sudo : {a}",
  "改不了防火墙": "pare-feu non modifiable",
  "装不了 Caddy": "impossible d’installer Caddy",
  "改不了站点配置": "impossible de modifier la configuration du site",
  "装不了 Nginx": "impossible d'installer Nginx",
  "续期不了证书": "impossible de renouveler les certificats",
  "管不了 Fail2ban": "impossible de gérer Fail2ban",
  "启动不了 Caddy": "impossible de démarrer Caddy",
  "启动不了 Nginx": "impossible de démarrer Nginx",
  "宿主机模式下本机上的命令已经以 root 身份执行，不需要改成 SSH": "En mode hôte, les commandes de l’hôte local s’exécutent déjà en root ; inutile de passer en SSH",
  "本机改成 SSH 连接时，地址只能是 127.0.0.1 或 localhost（可以带端口）": "Lorsque l’hôte local se connecte en SSH, l’adresse doit être 127.0.0.1 ou localhost (port autorisé)",
};

export default dict;
