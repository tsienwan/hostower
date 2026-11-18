// de 词典：设置页“面板”：环境变量接管时的说明。
const dict: Record<string, string> = {
  "现在生效的是环境变量 ": "Derzeit gilt der Wert der Umgebungsvariable ",
  " 的值": "",
  "。这里保存的值要等去掉这个环境变量、重启后才生效。": ". Ein hier gespeicherter Wert gilt erst, wenn diese Variable entfernt und Hostower neu gestartet wurde.",
  "有 {n} 项设置由环境变量决定：": "{n} Einstellungen kommen aus Umgebungsvariablen: ",
  "。环境变量优先于这里保存的值。": ". Umgebungsvariablen haben Vorrang vor hier gespeicherten Werten.",
  "想改成在这里管理：先在下面填好并保存（可以点“带入环境变量的值”），再去掉这些环境变量、重启 Hostower。这样顺序做，不会断开访问。": "Um sie hier zu verwalten: zuerst unten ausfüllen und speichern (Schaltfläche „Werte aus der Umgebung übernehmen“), dann die Variablen entfernen und Hostower neu starten. In dieser Reihenfolge verlieren Sie den Zugang nicht.",
  "systemd 部署：运行 ": "systemd: ",
  " 删掉对应的 Environment= 行（或检查 /etc/systemd/system/hostower.service），然后 ": " ausführen, die passenden Environment=-Zeilen löschen (auch /etc/systemd/system/hostower.service prüfen), dann ",
  "。Docker 部署：删掉 compose 文件 environment 里对应的行，再 ": ". Docker: die passenden Zeilen unter environment in der Compose-Datei löschen, dann ",
  "带入环境变量的值": "Werte aus der Umgebung übernehmen",
  "谁来提供 HTTPS。对外监听时必须二选一，否则塔台拒绝启动。选“塔台自己提供”后填证书和私钥文件的路径。": "Wer HTTPS bereitstellt. Bei öffentlichem Lauschen muss eines gewählt sein, sonst startet der Tower nicht. „Der Tower selbst“ wählen, um die Pfade zu Zertifikat und privatem Schlüssel einzutragen.",
};

export default dict;
