// de 词典：“这台设备”测速（浏览器和面板之间的速度、回程和去程路由）。
const dict: Record<string, string> = {
  "这台设备 ↔ 主机":
    "Dieses Gerät ↔ Host",
  "测这台设备":
    "Dieses Gerät testen",
  "把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。":
    "Behandelt das Gerät, auf dem das Panel gerade angezeigt wird (z. B. ein Handy, das zu Hause, im Büro oder unterwegs online ist), als Knoten: misst Latenz, Download und Upload zwischen ihm und einem beliebigen Host und verfolgt dann die Route von diesem Host zurück zu ihm, um Leitung und Knoten des Rückwegs zu zeigen.",
  "各网络最近一次":
    "Letzter Test je Netz",
  "没有拿到结果":
    "Kein Ergebnis erhalten",
  "速度测到哪里":
    "Ziel der Geschwindigkeitsmessung",
  "主机（直连）":
    "Hosts (direkt)",
  "在选中的主机上各临时开一个测速服务，这台设备的浏览器一台一台直接连过去量延迟、下载、上传，再从每台主机追踪回到这台设备，最后排成一张对比表。服务只认带一次性口令的地址，测完自己退出。开端口是一次变更（L1），需要批准，一次批准管这一批。":
    "Startet auf jedem ausgewählten Host vorübergehend einen Geschwindigkeitstest-Dienst; der Browser dieses Geräts verbindet sich nacheinander direkt mit jedem Host und misst Latenz, Download und Upload, danach wird die Route von jedem Host zurück zu diesem Gerät verfolgt, und am Ende entsteht eine Vergleichstabelle. Der Dienst akzeptiert nur die Adresse mit Einmal-Token und beendet sich nach dem Test selbst. Das Öffnen eines Ports ist eine Änderung (L1) und muss genehmigt werden; eine Genehmigung gilt für den ganzen Durchlauf.",
  "延迟和速度量的是这台设备和面板之间；选中的主机只用来追踪回到这台设备的路由。":
    "Latenz und Geschwindigkeit werden zwischen diesem Gerät und dem Panel gemessen; die ausgewählten Hosts dienen nur dazu, die Route zurück zu diesem Gerät zu verfolgen.",
  "测哪些主机（最多 {n} 台）":
    "Zu testende Hosts (höchstens {n})",
  "{host} 的地址":
    "Adresse von {host}",
  "地址填这台设备能直接连到的（公网 IP 或域名）。按勾选的顺序一台一台测。":
    "Geben Sie Adressen an, die dieses Gerät direkt erreichen kann (öffentliche IP oder Domain). Getestet wird nacheinander in der Reihenfolge der Auswahl.",
  "每台主机的防火墙和云厂商的安全组都要放行这个 TCP 端口，端口在 1024 到 65535 之间。":
    "Die Firewall jedes Hosts und die Sicherheitsgruppe des Cloud-Anbieters müssen diesen TCP-Port freigeben; der Port muss zwischen 1024 und 65535 liegen.",
  "测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。":
    "Die Testseite wird direkt vom Host per HTTP ausgeliefert: Der Browser meldet möglicherweise „Nicht sicher“ – wählen Sie einfach „Trotzdem fortfahren“. Die Seite enthält nur Zufallsdaten für den Test und keine Anmeldedaten des Panels.",
  "回程路由从哪台主机追踪":
    "Host, von dem die Rückroute verfolgt wird",
  "没有在线的主机":
    "Keine Online-Hosts",
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
  "每台下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，这次最多约 {total} MB。":
    "Pro Host: Download höchstens {d} MB, Upload höchstens {u} MB, jeweils höchstens {s} Sekunden – je nachdem, was zuerst erreicht ist. Verbraucht Datenvolumen der Hosts und dieses Geräts, diesmal höchstens etwa {total} MB.",
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
  "接着测下一台…":
    "Weiter mit dem nächsten Host…",
  "没有测成":
    "Test fehlgeschlagen",
  "测完了":
    "Test abgeschlossen",
  "正在追踪回程路由…":
    "Rückroute wird verfolgt…",
  "正在测…":
    "Test läuft…",
  "等这台设备连上来":
    "Warte auf die Verbindung dieses Geräts",
  "正在启动测速服务…":
    "Geschwindigkeitstest-Dienst wird gestartet…",
  "批准后才会在主机上开测速服务":
    "Der Geschwindigkeitstest-Dienst wird erst nach der Genehmigung auf dem Host gestartet",
  "这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。":
    "Auf {host} wird vorübergehend ein Port ({port}) geöffnet, der auf die Verbindung dieses Geräts für den Test wartet; nach dem Test beendet sich der Dienst selbst.",
  "正在主机上启动测速服务…":
    "Geschwindigkeitstest-Dienst wird auf den Hosts gestartet…",
  "测速服务已经在等着了。在这台设备上打开测速页，它会自己开始，一台一台测下去。":
    "Die Geschwindigkeitstest-Dienste warten bereits. Öffnen Sie die Testseite auf diesem Gerät; der Test startet von selbst und arbeitet die Hosts nacheinander ab.",
  "打开测速页（还有 {n} 台）":
    "Testseite öffnen (noch {n})",
  "打开测速页":
    "Testseite öffnen",
  "打不开或一直转圈：那台主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者填的地址这台设备连不上。回到这里可以接着测剩下的。服务最多等 {w} 秒。":
    "Die Seite öffnet sich nicht oder lädt endlos: Die Firewall dieses Hosts oder die Sicherheitsgruppe des Cloud-Anbieters gibt den TCP-Port {port} nicht frei, oder dieses Gerät erreicht die angegebene Adresse nicht. Hier können Sie mit den übrigen Hosts weitermachen. Der Dienst wartet höchstens {w} Sekunden.",
  "正在另一个页面里测 {host}…":
    "{host} wird auf einer anderen Seite getestet…",
  "下载时的延迟":
    "Latenz beim Download",
  "回程重传率":
    "Retransmissionsrate (Rückweg)",
  "回程线路":
    "Leitung des Rückwegs",
  "延迟最低":
    "Niedrigste Latenz",
  "延迟和速度是这台设备的浏览器直接和每台主机量的；回程线路是从主机追踪回来认出的，只能作参考。每台的明细在各自的报告里。":
    "Latenz und Geschwindigkeit misst der Browser dieses Geräts direkt mit jedem Host; die Leitung des Rückwegs wird aus der vom Host verfolgten Route abgeleitet und dient nur als Anhaltspunkt. Die Details zu jedem Host stehen im jeweiligen Bericht.",
  "查看报告":
    "Bericht anzeigen",
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
  "正在从 {host} 追踪到这台设备，最多要半分钟…":
    "Route von {host} zu diesem Gerät wird verfolgt, das dauert bis zu einer halben Minute…",
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
  "这是回程：从主机追踪到这台设备的地址。去程（这台设备到主机）在浏览器里追踪不了，所以这里没有；来回两个方向走的线路常常不一样。线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。":
    "Dies ist die Rückroute, verfolgt vom Host zur Adresse dieses Geräts. Die Hinroute (von diesem Gerät zum Host) lässt sich im Browser nicht verfolgen und fehlt daher hier; beide Richtungen nehmen oft unterschiedliche Leitungen. Die Leitungen werden aus öffentlichen Backbone-Bereichen der Netzbetreiber und dem autonomen System (AS) jedes Hops abgeleitet und dienen nur als Anhaltspunkt; Hops, die nicht antworten, werden als * angezeigt.",
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
  "上传时的延迟":
    "Latenz beim Upload",
  "连接类型":
    "Verbindungstyp",
  "从 {host} ping 它":
    "Ping von {host}",
  "不回应":
    "Keine Antwort",
  "平均 {avg} ms，丢包 {loss}%":
    "Mittel {avg} ms, Verlust {loss} %",
  "{a} / {b} 个数据段":
    "{a} / {b} Segmente",
  "TCP 往返时间":
    "TCP-Umlaufzeit",
  "重传率是下载时 {host} 重发的数据段占比，从它的 TCP 连接上读的：回程丢包越多它越高，到 1% 以上速度就会明显受影响。":
    "Die Retransmissionsrate ist der Anteil der Segmente, die {host} beim Download erneut senden musste, ausgelesen aus seiner TCP-Verbindung: Je mehr Verlust auf dem Rückweg, desto höher; ab 1 % leidet die Geschwindigkeit spürbar.",
  "家用路由器和运营商的地址常常不回应 ping，不代表不通。":
    "Adressen von Heimroutern und Netzbetreibern antworten oft nicht auf ping; das heißt nicht, dass sie unerreichbar sind.",
  "比空闲时高 {n} ms":
    "{n} ms höher als im Leerlauf",
  "和空闲时差不多":
    "Etwa wie im Leerlauf",
  "回程：{host} → 这台设备":
    "Rückweg: {host} → dieses Gerät",
  "每一跳探了 {n} 次，“丢包”是没回应的次数。中间某一跳丢、后面的跳不丢，多半是那台路由器限制了回应的速率，不是真的丢包；从某一跳开始一直丢到最后，问题才在那一段。":
    "Jeder Hop wurde {n}-mal geprüft; „Verlust“ ist die Zahl der unbeantworteten Prüfpakete. Verliert ein Hop in der Mitte, die folgenden aber nicht, begrenzt meist dieser Router seine Antwortrate – das ist kein echter Verlust. Erst wenn der Verlust ab einem Hop bis zum Ende anhält, liegt das Problem auf diesem Abschnitt.",
  "最后一跳没有到达这台设备：家用路由器和运营商的地址常常不回应探测，前面的节点和线路仍然可以看。":
    "Der letzte Hop hat dieses Gerät nicht erreicht: Adressen von Heimroutern und Netzbetreibern antworten oft nicht auf Prüfpakete; die Knoten davor und die Leitung sind trotzdem sichtbar.",
};

export default dict;
