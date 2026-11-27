// de 词典：“这台设备”测速的服务端文字。
const dict: Record<string, string> = {
  "同时在测速的连接太多了，请稍后再试":
    "Zu viele Verbindungen testen gleichzeitig die Geschwindigkeit, bitte später erneut versuchen",
  "数据量不对":
    "Ungültige Datenmenge",
  "生成数据失败":
    "Daten konnten nicht erzeugt werden",
  "一次传得太多了":
    "Zu viele Daten auf einmal übertragen",
  "没有这条测速请求":
    "Diese Testanfrage gibt es nicht",
  "追踪从 {a} 到这台设备（{b}）的回程路由":
    "Rückroute von {a} zu diesem Gerät ({b}) verfolgen",
  "安装缺少的 traceroute 并追踪从 {a} 到这台设备（{b}）的回程路由":
    "Fehlendes traceroute installieren und Rückroute von {a} zu diesem Gerät ({b}) verfolgen",
  "读不出这台设备的来源地址，没法追踪回程路由":
    "Die Quelladresse dieses Geräts lässt sich nicht ermitteln; die Rückroute kann nicht verfolgt werden",
  "这台设备的来源地址不能用来追踪路由":
    "Die Quelladresse dieses Geräts eignet sich nicht zur Routenverfolgung",
  "测量结果里的数字不对":
    "Die Zahlen im Messergebnis sind ungültig",
  "测量结果里的延迟次数不对":
    "Die Anzahl der Latenzmessungen im Messergebnis ist ungültig",
  "端口应该在 1024 到 65535 之间":
    "Der Port muss zwischen 1024 und 65535 liegen",
  "口令不对":
    "Token ungültig",
  "在 {a} 上临时开一个测速服务（端口 {b}），测这台设备和它之间的速度并追踪回程路由":
    "Auf {a} vorübergehend einen Geschwindigkeitstest-Dienst starten (Port {b}), die Geschwindigkeit zwischen diesem Gerät und dem Host messen und die Rückroute verfolgen",
  "主机上没有 python3，开不了临时测速服务":
    "Auf dem Host fehlt python3; der temporäre Geschwindigkeitstest-Dienst kann nicht gestartet werden",
  "主机上这个端口已经被占用，换一个端口再试":
    "Dieser Port ist auf dem Host bereits belegt; versuchen Sie es mit einem anderen Port",
  "没有等到这台设备连上来：测速页没打开，或者主机的防火墙、云厂商的安全组没有放行这个端口":
    "Dieses Gerät hat sich nicht verbunden: Die Testseite wurde nicht geöffnet, oder die Firewall des Hosts bzw. die Sicherheitsgruppe des Cloud-Anbieters gibt diesen Port nicht frei",
  "这台设备连上来了，但没有在限定时间里测完":
    "Dieses Gerät hat sich verbunden, der Test wurde aber nicht innerhalb des Zeitlimits abgeschlossen",
  "一次最多测 {a} 台主机":
    "Höchstens {a} Hosts pro Durchlauf",
};

export default dict;
