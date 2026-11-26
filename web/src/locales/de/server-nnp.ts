// de 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以编辑这台主机，把连接方式改成 SSH（地址 127.0.0.1，选一条能 sudo 的账号的凭据），再来操作": "Der Prozess, der die Befehle ausführt, darf keine Rechte ausweiten (der Dienst setzt NoNewPrivileges), sudo funktioniert nicht: {a}. Diesen Host bearbeiten und die Verbindung auf SSH umstellen (Adresse 127.0.0.1, Zugangsdaten eines Kontos mit sudo), dann erneut versuchen",
  "登录账号不是 root，也没有 sudo：{a}": "Das Login-Konto ist nicht root und hat kein sudo: {a}",
  "改不了防火墙": "Firewall nicht änderbar",
  "装不了 Caddy": "Caddy nicht installierbar",
  "改不了站点配置": "die Site-Konfiguration lässt sich nicht ändern",
  "装不了 Nginx": "Nginx lässt sich nicht installieren",
  "续期不了证书": "Zertifikate lassen sich nicht erneuern",
  "管不了 Fail2ban": "Fail2ban lässt sich nicht verwalten",
  "启动不了 Caddy": "Caddy nicht startbar",
  "启动不了 Nginx": "Nginx nicht startbar",
  "宿主机模式下本机上的命令已经以 root 身份执行，不需要改成 SSH": "Im Host-Modus laufen Befehle auf dem lokalen Host bereits als root; eine Umstellung auf SSH ist nicht nötig",
  "本机改成 SSH 连接时，地址只能是 127.0.0.1 或 localhost（可以带端口）": "Wenn der lokale Host per SSH verbunden wird, muss die Adresse 127.0.0.1 oder localhost sein (Port erlaubt)",
};

export default dict;
