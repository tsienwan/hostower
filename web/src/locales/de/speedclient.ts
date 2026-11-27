// de 词典：“这台设备”测速（浏览器和面板之间的速度、回程和去程路由）。
const dict: Record<string, string> = {
  "这台设备 ↔ 主机":
    "Dieses Gerät ↔ Host",
  "测这台设备":
    "Dieses Gerät testen",
  "把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。":
    "Behandelt das Gerät, auf dem das Panel gerade angezeigt wird (z. B. ein Handy, das zu Hause, im Büro oder unterwegs online ist), als Knoten: misst Latenz, Download und Upload zwischen ihm und einem beliebigen Host und verfolgt dann die Route von diesem Host zurück zu ihm, um Leitung und Knoten des Rückwegs zu zeigen.",
  "测哪台主机":
    "Zu testender Host",
  "没有在线的主机":
    "Keine Online-Hosts",
  "速度测到哪里":
    "Ziel der Geschwindigkeitsmessung",
  "这台主机（直连）":
    "Dieser Host (direkt)",
  "在 {host} 上临时开一个测速服务，这台设备的浏览器直接连它量延迟、下载、上传，再从 {host} 追踪回到这台设备。服务只认带一次性口令的地址，最多等 {w} 秒，测完自己退出。开端口是一次变更（L1），需要批准。":
    "Startet auf {host} vorübergehend einen Geschwindigkeitstest-Dienst; der Browser dieses Geräts verbindet sich direkt damit und misst Latenz, Download und Upload, danach wird die Route von {host} zurück zu diesem Gerät verfolgt. Der Dienst akzeptiert nur die Adresse mit Einmal-Token, wartet höchstens {w} Sekunden und beendet sich nach dem Test selbst. Das Öffnen eines Ports ist eine Änderung (L1) und muss genehmigt werden.",
  "延迟和速度量的是这台设备和面板之间；{host} 只用来追踪回到这台设备的路由。":
    "Latenz und Geschwindigkeit werden zwischen diesem Gerät und dem Panel gemessen; {host} dient nur dazu, die Route zurück zu diesem Gerät zu verfolgen.",
  "主机的地址和端口":
    "Adresse und Port des Hosts",
  "主机的地址":
    "Adresse des Hosts",
  "填这台设备能直接连到的地址（公网 IP 或域名）。主机的防火墙和云厂商的安全组要放行这个 TCP 端口，端口在 1024 到 65535 之间。":
    "Geben Sie eine Adresse an, die dieses Gerät direkt erreichen kann (öffentliche IP oder Domain). Die Firewall des Hosts und die Sicherheitsgruppe des Cloud-Anbieters müssen diesen TCP-Port freigeben; der Port muss zwischen 1024 und 65535 liegen.",
  "测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。":
    "Die Testseite wird direkt vom Host per HTTP ausgeliefert: Der Browser meldet möglicherweise „Nicht sicher“ – wählen Sie einfach „Trotzdem fortfahren“. Die Seite enthält nur Zufallsdaten für den Test und keine Anmeldedaten des Panels.",
  "这台设备的地址":
    "Adresse dieses Geräts",
  "这是面板看到的来源地址，回程路由就追踪到它。用了代理或 VPN 访问面板时，它是代理的出口地址。":
    "Das ist die Quelladresse, die das Panel sieht; die Rückroute wird bis zu ihr verfolgt. Beim Zugriff auf das Panel über einen Proxy oder ein VPN ist es die Ausgangsadresse des Proxys.",
  "面板前面有反向代理：延迟和速度量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "Vor dem Panel steht ein Reverse-Proxy: Latenz und Geschwindigkeit messen die Strecke von diesem Gerät zum Proxy (oder CDN) plus die vom Proxy zum Panel.",
  "读不出可以追踪的来源地址（比如从本机回环地址访问），测不了。":
    "Es lässt sich keine verfolgbare Quelladresse ermitteln (z. B. bei Zugriff über die lokale Loopback-Adresse); der Test ist nicht möglich.",
  "数据量上限":
    "Maximale Datenmenge",
  "下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，最多约 {total} MB。":
    "Download höchstens {d} MB, Upload höchstens {u} MB, jeweils höchstens {s} Sekunden – je nachdem, was zuerst erreicht ist. Verbraucht Datenvolumen des Hosts und dieses Geräts, höchstens etwa {total} MB.",
  "正在检查主机上有没有 traceroute…":
    "Prüfe, ob traceroute auf dem Host vorhanden ist…",
  "{host} 上没有 traceroute 或 tracepath，追踪不了路由。":
    "Auf {host} gibt es weder traceroute noch tracepath; die Route kann nicht verfolgt werden.",
  "先用系统的包管理器安装 traceroute":
    "traceroute zuerst mit dem Paketmanager des Systems installieren",
  "安装是一次变更（L1），需要批准；不装的话只有速度，没有回程路由。":
    "Die Installation ist eine Änderung (L1) und muss genehmigt werden; ohne sie gibt es nur die Geschwindigkeit, keine Rückroute.",
  "这台设备 ↔ {host}":
    "Dieses Gerät ↔ {host}",
  "正在测，请不要关闭这个页面…":
    "Test läuft, bitte diese Seite nicht schließen…",
  "测完了，可以关掉这个页面回到面板。":
    "Test abgeschlossen. Sie können diese Seite schließen und zum Panel zurückkehren.",
  "结果没有交回去，请回到面板重试。":
    "Das Ergebnis konnte nicht übermittelt werden. Kehren Sie zum Panel zurück und versuchen Sie es erneut.",
  "查看报告":
    "Bericht anzeigen",
  "批准后才会在主机上开测速服务":
    "Der Geschwindigkeitstest-Dienst wird erst nach der Genehmigung auf dem Host gestartet",
  "这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。":
    "Auf {host} wird vorübergehend ein Port ({port}) geöffnet, der auf die Verbindung dieses Geräts für den Test wartet; nach dem Test beendet sich der Dienst selbst.",
  "正在 {host} 上启动测速服务…":
    "Geschwindigkeitstest-Dienst wird auf {host} gestartet…",
  "测速服务已经在 {host} 上等着了。在这台设备上打开测速页，它会自己开始测。":
    "Der Geschwindigkeitstest-Dienst wartet auf {host}. Öffnen Sie die Testseite auf diesem Gerät, der Test startet von selbst.",
  "打开测速页":
    "Testseite öffnen",
  "打不开或一直转圈：主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者地址 {addr} 这台设备连不上。服务最多等 {w} 秒。":
    "Die Seite öffnet sich nicht oder lädt endlos: Die Firewall des Hosts oder die Sicherheitsgruppe des Cloud-Anbieters gibt den TCP-Port {port} nicht frei, oder dieses Gerät erreicht die Adresse {addr} nicht. Der Dienst wartet höchstens {w} Sekunden.",
  "这台设备已经连上，正在另一个页面里测…":
    "Dieses Gerät ist verbunden, der Test läuft auf einer anderen Seite…",
  "正在从 {host} 追踪到这台设备，最多要半分钟…":
    "Route von {host} zu diesem Gerät wird verfolgt, das dauert bis zu einer halben Minute…",
  "测完了，在报告里看速度、线路和每一跳。":
    "Test abgeschlossen; Geschwindigkeit, Leitung und alle Hops stehen im Bericht.",
  "没有测成，原因在报告或请求详情里。":
    "Der Test ist fehlgeschlagen; den Grund finden Sie im Bericht oder in den Anfragedetails.",
  "正在测…":
    "Test läuft…",
  "失败 {n} 次":
    "{n} fehlgeschlagen",
  "没有测出来":
    "Kein Ergebnis",
  "速度已经记下。":
    "Die Geschwindigkeit wurde gespeichert.",
  "批准后才会追踪回程路由":
    "Die Rückroute wird erst nach der Genehmigung verfolgt",
  "这次要先安装 traceroute，是一次变更；速度不受影响，已经在测。":
    "Diesmal muss zuerst traceroute installiert werden, das ist eine Änderung; die Geschwindigkeit ist nicht betroffen und wird bereits gemessen.",
  "回程路由追踪完了，在报告里看线路和每一跳。":
    "Die Rückroute ist verfolgt; Leitung und alle Hops stehen im Bericht.",
  "回程路由没有追踪成功，报告里只有速度。":
    "Die Rückroute konnte nicht verfolgt werden; der Bericht enthält nur die Geschwindigkeit.",
  "这台设备 ↔ 面板":
    "Dieses Gerät ↔ Panel",
  "这次没有收到浏览器量的延迟和速度（测到一半关掉了页面），只有回程路由。":
    "Diesmal kamen die vom Browser gemessenen Werte für Latenz und Geschwindigkeit nicht an (Seite während des Tests geschlossen); es gibt nur die Rückroute.",
  "失败的请求":
    "Fehlgeschlagene Anfragen",
  "共 {n} 次":
    "{n} insgesamt",
  "这次没有拿到回程路由。":
    "Diesmal wurde keine Rückroute ermittelt.",
  "回程是从主机追踪到这台设备的地址；去程是这台设备到服务器，浏览器追踪不了，要在设备那边跑一次路由追踪贴进来。来回两个方向走的线路常常不一样。线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。":
    "Der Rückweg wird vom Host zur Adresse dieses Geräts verfolgt; der Hinweg führt von diesem Gerät zum Server und lässt sich im Browser nicht verfolgen: Führen Sie auf dem Gerät eine Routenverfolgung aus und fügen Sie die Ausgabe ein. Hin- und Rückweg nehmen oft unterschiedliche Leitungen. Die Leitungen werden aus veröffentlichten Backbone-Bereichen der Netzbetreiber und dem autonomen System (AS) jedes Hops abgeleitet – nur ein Anhaltspunkt. Hops ohne Antwort erscheinen als *.",
  "蜂窝网络":
    "Mobilfunk",
  "有线网络":
    "Kabelgebundenes Netz",
  "这台设备的浏览器直接连到 {host} 上临时开的测速服务量的，不经过面板：延迟是连发小请求的往返时间，取中位数，里面含一点服务处理的时间；下载和上传是浏览器直接和这台主机传数据。":
    "Gemessen vom Browser dieses Geräts in direkter Verbindung mit dem vorübergehend auf {host} gestarteten Geschwindigkeitstest-Dienst, ohne Umweg über das Panel: Die Latenz ist die Umlaufzeit kleiner, nacheinander gesendeter Anfragen (Median), einschließlich etwas Verarbeitungszeit des Dienstes; Download und Upload werden durch direkte Datenübertragung zwischen Browser und diesem Host gemessen.",
  "延迟是浏览器向面板连发小请求量出来的往返时间，取中位数，里面含一点服务器处理的时间；下载和上传是浏览器直接和面板传数据量的。":
    "Die Latenz ist die Umlaufzeit kleiner Anfragen, die der Browser nacheinander an das Panel sendet (Median), einschließlich etwas Verarbeitungszeit des Servers; Download und Upload werden durch direkte Datenübertragung zwischen Browser und Panel gemessen.",
  "面板前面有反向代理：这些数字量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "Vor dem Panel steht ein Reverse-Proxy: Diese Werte messen die Strecke von diesem Gerät zum Proxy (oder CDN) plus die vom Proxy zum Panel.",
  "往返时间":
    "Umlaufzeit",
  "最小 {min}、中位 {med}、最大 {max} ms":
    "min. {min}, Median {med}, max. {max} ms",
  "连接类型":
    "Verbindungstyp",
  "从 {host} ping 它":
    "Ping von {host}",
  "不回应":
    "Keine Antwort",
  "平均 {avg} ms，丢包 {loss}%":
    "Mittel {avg} ms, Verlust {loss} %",
  "家用路由器和运营商的地址常常不回应 ping，不代表不通。":
    "Adressen von Heimroutern und Netzbetreibern antworten oft nicht auf ping; das heißt nicht, dass sie unerreichbar sind.",
  "移除去程路由":
    "Hinroute entfernen",
  "添加去程路由":
    "Hinroute hinzufügen",
  "这台设备 → 服务器":
    "Dieses Gerät → Server",
  "在和这台设备同一个网络里的电脑上运行下面的命令，把完整输出贴到这里。手机上可以用带路由追踪功能的网络工具，把结果按文字贴进来。":
    "Führen Sie den folgenden Befehl auf einem Rechner im selben Netz wie dieses Gerät aus und fügen Sie die vollständige Ausgabe hier ein. Auf dem Handy können Sie ein Netzwerk-Tool mit Routenverfolgung nutzen und das Ergebnis als Text einfügen.",
  "把 tracert、traceroute 或 mtr 的输出贴在这里":
    "Ausgabe von tracert, traceroute oder mtr hier einfügen",
  "去程路由的输出":
    "Ausgabe der Hinroute",
  "只留下每一跳的序号、IP 地址和时间，别的文字不保存。":
    "Es bleiben nur Nummer, IP-Adresse und Zeiten jedes Hops erhalten; anderer Text wird nicht gespeichert.",
  "回程：{host} → 这台设备":
    "Rückweg: {host} → dieses Gerät",
  "去程：这台设备 → 服务器":
    "Hinweg: dieses Gerät → Server",
  "最后一跳没有到达这台设备：家用路由器和运营商的地址常常不回应探测，前面的节点和线路仍然可以看。":
    "Der letzte Hop hat dieses Gerät nicht erreicht: Adressen von Heimroutern und Netzbetreibern antworten oft nicht auf Prüfpakete; die Knoten davor und die Leitung sind trotzdem sichtbar.",
};

export default dict;
