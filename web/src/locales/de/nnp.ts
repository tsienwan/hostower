// de 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Auf diesem Host führt der Hostower-Prozess die Befehle direkt aus, und der Dienst setzt NoNewPrivileges (Härtung gegen Rechteausweitung): sudo funktioniert nicht, Firewall und Caddy lassen sich nicht ändern.",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Empfohlen: Oben rechts auf Bearbeiten klicken und die Verbindung dieses Hosts auf SSH umstellen (Adresse 127.0.0.1, Zugangsdaten eines Kontos mit passwortlosem sudo). Es bleibt derselbe Host; ein weiterer muss nicht hinzugefügt werden. Man kann auch NoNewPrivileges=true aus der Dienstdatei entfernen, das schwächt aber die Härtung.",
};

export default dict;
