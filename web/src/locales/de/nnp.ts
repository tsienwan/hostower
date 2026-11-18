// de 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Auf diesem Host führt der Hostower-Prozess die Befehle direkt aus, und der Dienst setzt NoNewPrivileges (Härtung gegen Rechteausweitung): sudo funktioniert nicht, Firewall und Caddy lassen sich nicht ändern.",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Empfohlen: Unter Übersicht diesen Rechner noch einmal per SSH hinzufügen (Adresse 127.0.0.1, Konto mit passwortlosem sudo) und darüber Firewall und Caddy verwalten. Man kann auch NoNewPrivileges=true aus der Dienstdatei entfernen, das schwächt aber die Härtung.",
};

export default dict;
