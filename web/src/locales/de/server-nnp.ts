// de 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "Der Prozess, der die Befehle ausführt, darf keine Rechte ausweiten (der Dienst setzt NoNewPrivileges), sudo funktioniert nicht: {a}. Diesen Rechner per SSH neu hinzufügen (Adresse 127.0.0.1, Konto mit sudo) und darüber verwalten",
  "登录账号不是 root，也没有 sudo：{a}": "Das Login-Konto ist nicht root und hat kein sudo: {a}",
  "改不了防火墙": "Firewall nicht änderbar",
  "装不了 Caddy": "Caddy nicht installierbar",
  "改不了站点配置": "die Site-Konfiguration lässt sich nicht ändern",
  "装不了 Nginx": "Nginx lässt sich nicht installieren",
  "续期不了证书": "Zertifikate lassen sich nicht erneuern",
  "管不了 Fail2ban": "Fail2ban lässt sich nicht verwalten",
};

export default dict;
