// Deutsches Wörterbuch: die Schaltfläche „Panel öffnen“ der Apps und die Einstellung der Panel-Adresse (pages/AppPanel.tsx).
const dict: Record<string, string> = {
  "{app} 的面板地址": "Panel-Adresse von {app}",
  "恢复默认": "Standard wiederherstellen",
  "默认按这个实例当前的监听地址和端口拼出地址，在“修改设置”里改过以后会自动跟着变。下面的设置只存在塔台里，不改主机上的任何东西；改动会写进审计。": "Standardmäßig wird die Adresse aus der aktuellen Lauschadresse und dem Port dieser Instanz gebildet und folgt Änderungen unter „Einstellungen bearbeiten“ automatisch. Die Einstellungen unten werden nur in Hostower gespeichert und ändern nichts auf dem Host; jede Änderung wird im Audit-Protokoll festgehalten.",
  "怎么访问": "Zugriffsweg",
  "直接访问主机": "Direkt auf dem Host",
  "填写完整地址": "Vollständige Adresse",
  "经由 SSH 隧道": "Über einen SSH-Tunnel",
  "打开 主机地址:端口/路径。端口或路径在应用自己的页面里改过时，在下面填上新的；留空就照应用的设置。": "Öffnet Host-Adresse:Port/Pfad. Wurden Port oder Pfad in der App selbst geändert, tragen Sie den neuen Wert unten ein; leer lassen, um den App-Einstellungen zu folgen.",
  "应用前面套了反向代理、用域名访问，或者只绑在 127.0.0.1 上时，直接填浏览器里要打开的地址。不能带账号、密码或 ? 后面的参数。": "Liegt die App hinter einem Reverse-Proxy, wird sie über eine Domain erreicht oder lauscht sie nur auf 127.0.0.1, tragen Sie die Adresse ein, die im Browser geöffnet werden soll. Sie darf keinen Benutzernamen, kein Passwort und nichts nach ? enthalten.",
  "不对外开放端口：在自己电脑上运行下面这条命令建立隧道（用的是这台主机的 SSH 登录方式），按钮打开本机的端口。": "Es wird kein Port nach außen geöffnet: Führen Sie den Befehl unten auf Ihrem eigenen Rechner aus, um einen Tunnel aufzubauen (er nutzt die SSH-Anmeldung dieses Hosts); die Schaltfläche öffnet dann den Port auf Ihrem Rechner.",
  "面板地址": "Panel-Adresse",
  "网络页里反向代理到这个端口的站点：": "Sites auf der Netzwerkseite, die auf diesen Port weiterleiten:",
  "应用端口": "App-Port",
  "本地端口": "Lokaler Port",
  "先运行": "Zuerst ausführen",
  "复制命令": "Befehl kopieren",
  "按钮打开": "Schaltfläche öffnet",
  "这个应用只在主机本机（127.0.0.1）监听，从外面打不开：填写完整地址或者经由 SSH 隧道。": "Diese App lauscht nur auf dem Host selbst (127.0.0.1) und lässt sich von außen nicht öffnen: Tragen Sie eine vollständige Adresse ein oder nutzen Sie einen SSH-Tunnel.",
  "这台主机上有站点反向代理到这个应用的端口：没有另外设置时，按钮自动打开站点的地址。想照主机地址打开，就在上面填上端口。":
    "Eine Site auf diesem Host leitet als Reverse-Proxy auf den Port dieser App weiter: Ohne weitere Einstellung öffnet die Schaltfläche automatisch die Adresse der Site. Um stattdessen die Host-Adresse zu öffnen, tragen Sie oben den Port ein.",
  "只有所有者可以修改面板地址。": "Nur der Eigentümer kann die Panel-Adresse ändern.",
  "这个应用只在主机本机（127.0.0.1）监听：先设置面板地址（反向代理的域名或 SSH 隧道）": "Diese App lauscht nur auf dem Host selbst (127.0.0.1): Legen Sie zuerst eine Panel-Adresse fest (Domain des Reverse-Proxys oder SSH-Tunnel)",
  "打开面板": "Panel öffnen",
  "应用已停止": "Die App ist gestoppt",
  "经由 SSH 隧道：先在自己电脑上运行 {cmd}": "Über einen SSH-Tunnel: zuerst {cmd} auf Ihrem eigenen Rechner ausführen",
  "面板地址：反向代理、SSH 隧道、改过的端口和路径": "Panel-Adresse: Reverse-Proxy, SSH-Tunnel, geänderter Port und Pfad",
  "应用面板地址": "App-Panel-Adresse",
};

export default dict;
