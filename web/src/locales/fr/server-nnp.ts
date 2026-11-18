// fr 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "Le processus qui exécute les commandes ne peut pas élever ses privilèges (le service définit NoNewPrivileges), sudo ne peut pas fonctionner : {a}. Ajoutez à nouveau cette machine via SSH (adresse 127.0.0.1, compte avec sudo) et gérez-la ainsi",
  "登录账号不是 root，也没有 sudo：{a}": "Le compte de connexion n’est pas root et n’a pas sudo : {a}",
  "改不了防火墙": "pare-feu non modifiable",
  "改不了 Caddy 的配置": "configuration de Caddy non modifiable",
  "装不了 Caddy": "impossible d’installer Caddy",
};

export default dict;
