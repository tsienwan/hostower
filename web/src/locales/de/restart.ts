// de 词典：设置页的“重启 Hostower”。
const dict: Record<string, string> = {
  "立即重启": "Jetzt neu starten",
  "重启 Hostower": "Hostower neu starten",
  "重启": "Neu starten",
  "刷新页面": "Seite neu laden",
  "重启会让面板断开几秒钟，保存过的面板设置（监听地址、访问地址、证书、会话时长）在重启后生效。需要当场验证一次身份。": "Der Neustart trennt das Panel für einige Sekunden; gespeicherte Panel-Einstellungen (Lauschadresse, Zugriffs-URL, Zertifikat, Sitzungsdauer) gelten danach. Einmal Identität bestätigen.",
  "有 {n} 个请求正在执行，重启会中断它们（之后标成“中断”，不会自动重跑）。": "{n} Anfragen laufen; der Neustart unterbricht sie (sie werden als „unterbrochen“ markiert und nicht neu gestartet).",
  "访问地址会变成 ": "Die Zugriffs-URL wird zu ",
  "。重启后请到新地址打开；如果前面有反向代理，记得一起改它转发的端口。": ". Nach dem Neustart unter der neuen Adresse öffnen; bei einem vorgeschalteten Reverse-Proxy auch dessen Zielport ändern.",
  "正在重启。稍等几秒后到新地址打开：": "Neustart läuft. Nach einigen Sekunden die neue Adresse öffnen: ",
  "正在重启，回来后会自动刷新页面…": "Neustart läuft; die Seite lädt neu, sobald er zurück ist…",
  "一分钟了还没连上。可能是新的面板设置有问题，或者反向代理转发的端口没跟着改。到主机上看日志：": "Nach einer Minute noch nicht erreichbar. Die neuen Panel-Einstellungen sind evtl. falsch oder der Reverse-Proxy leitet noch an den alten Port. Log auf dem Host prüfen: ",
  "；面板设置写错时，可以临时设置环境变量 HOSTOWER_IGNORE_PANEL=1 跳过它启动。": "; sind die Panel-Einstellungen falsch, vorübergehend HOSTOWER_IGNORE_PANEL=1 setzen, um ohne sie zu starten.",
};

export default dict;
