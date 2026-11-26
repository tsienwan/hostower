// de 词典：Tailscale 卡片，以及站点转发到另一台主机的提示。
const dict: Record<string, string> = {
  "在 Tailscale 网络里的名字":
    "Name im Tailscale-Netz",
  "留空就用系统的主机名。只能用字母、数字和连字符。":
    "Leer lassen, um den Hostnamen des Systems zu verwenden. Nur Buchstaben, Ziffern und Bindestriche.",
  "名字只能用字母、数字和连字符，不能以连字符开头或结尾。":
    "Der Name darf nur Buchstaben, Ziffern und Bindestriche enthalten und nicht mit einem Bindestrich beginnen oder enden.",
  "使用 Tailscale 网络的 DNS（可以用名字访问别的主机；会接管这台主机的 DNS 设置）":
    "DNS des Tailscale-Netzes verwenden (andere Hosts sind per Name erreichbar; übernimmt die DNS-Einstellungen dieses Hosts)",
  "接受别的节点通告的子网路由":
    "Von anderen Knoten angekündigte Subnetz-Routen annehmen",
  "开启 Tailscale SSH":
    "Tailscale SSH aktivieren",
  "开启后，Tailscale 网络里被访问规则允许的人可以直接登录这台主机，不经过 sshd 的密钥和这里的审批。只在你清楚那个网络的访问规则时开启。":
    "Danach können sich Personen, denen die Zugriffsregeln des Tailscale-Netzes es erlauben, direkt an diesem Host anmelden – ohne die Schlüssel von sshd und ohne die Genehmigungen hier. Nur aktivieren, wenn Sie die Zugriffsregeln dieses Netzes genau kennen.",
  "让 {host} 重新连上 Tailscale":
    "{host} wieder mit Tailscale verbinden",
  "把 {host} 加入 Tailscale 网络":
    "{host} zum Tailscale-Netz hinzufügen",
  "怎么登录":
    "Anmeldeart",
  "在浏览器里登录":
    "Im Browser anmelden",
  "不用密钥":
    "Ohne Schlüssel",
  "授权密钥":
    "Auth-Schlüssel",
  "用这条应用密钥里存的授权密钥登录。密钥在执行时从标准输入送给 tailscale，不会出现在命令、请求和审计里。":
    "Anmeldung mit dem in diesem App-Secret gespeicherten Auth-Schlüssel. Der Schlüssel wird bei der Ausführung über die Standardeingabe an tailscale übergeben und erscheint weder im Befehl noch in der Anfrage noch im Audit.",
  "提交并批准后，请求的输出里会出现一个登录链接：在浏览器里打开它并登录 Tailscale 账号，请求会一直等到登录完成（最多 9 分钟）。":
    "Nach dem Absenden und Genehmigen erscheint in der Ausgabe der Anfrage ein Anmeldelink: im Browser öffnen und beim Tailscale-Konto anmelden; die Anfrage wartet, bis die Anmeldung abgeschlossen ist (höchstens 9 Minuten).",
  "想不用人登录，先在 Tailscale 后台生成授权密钥，到 ":
    "Für eine Anmeldung ohne menschliches Zutun zuerst in der Tailscale-Admin-Konsole einen Auth-Schlüssel erzeugen, ihn auf der Seite ",
  " 页存成应用密钥，再回来选。":
    " als App-Secret speichern und dann hier auswählen.",
  "想不用人登录，请所有者把授权密钥存成应用密钥。":
    "Für eine Anmeldung ohne menschliches Zutun bitten Sie den Eigentümer, einen Auth-Schlüssel als App-Secret zu speichern.",
  "加入以后，这台主机会多一个内网地址，同一个 Tailscale 网络里的设备按那边的访问规则可以连到它。这是高危操作，批准时要重新验证身份。":
    "Nach dem Beitritt hat dieser Host eine zusätzliche interne Adresse, und Geräte im selben Tailscale-Netz können ihn gemäß den dortigen Zugriffsregeln erreichen. Das ist ein Hochrisiko-Vorgang; bei der Genehmigung ist eine erneute Identitätsprüfung nötig.",
  "{host} 上的 Tailscale 设置":
    "Tailscale-Einstellungen auf {host}",
  "读不出这台主机当前的 Tailscale 设置，下面显示的是默认值：提交会按这里的选择覆盖原来的设置。":
    "Die aktuellen Tailscale-Einstellungen dieses Hosts sind nicht lesbar; unten stehen die Standardwerte: Beim Absenden werden die bisherigen Einstellungen mit der Auswahl hier überschrieben.",
  "Hostower 就是经 Tailscale 连到这台主机的，断开会把它自己断在外面":
    "Hostower erreicht diesen Host über Tailscale: Trennen würde es selbst aussperren",
  "Tailscale（主机之间的加密内网）":
    "Tailscale (verschlüsseltes internes Netz zwischen Hosts)",
  "已连接":
    "Verbunden",
  "已断开":
    "Getrennt",
  "还没有登录":
    "Noch nicht angemeldet",
  "等待管理员批准":
    "Wartet auf Genehmigung durch den Administrator",
  "正在加入，等待登录…":
    "Beitritt läuft, wartet auf Anmeldung…",
  "断开":
    "Trennen",
  "重新连上":
    "Wieder verbinden",
  "加入网络":
    "Netz beitreten",
  "把几台主机连进同一个加密的内网：每台有一个 100.x 的地址，主机之间互相访问不用把端口对公网开放。安装、加入、改设置、断开都是高危操作，批准时要重新验证身份。":
    "Verbindet mehrere Hosts zu einem gemeinsamen verschlüsselten internen Netz: Jeder erhält eine 100.x-Adresse, und die Hosts erreichen einander, ohne Ports ins Internet zu öffnen. Installieren, Beitreten, Einstellungen ändern und Trennen sind Hochrisiko-Vorgänge; bei der Genehmigung ist eine erneute Identitätsprüfung nötig.",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装。":
    "Die Ein-Klick-Installation von Tailscale wird auf diesem System nicht unterstützt; folgen Sie der Anleitung auf der Tailscale-Website.",
  "内网地址":
    "Interne Adresse",
  "名字":
    "Name",
  "所在网络":
    "Netz",
  "别的主机加入同一个网络后，可以用上面的地址访问这台主机。在另一台主机上添加站点时，“转发到哪里”填这个地址，两台之间这一段就是加密的。":
    "Sobald andere Hosts demselben Netz beigetreten sind, erreichen sie diesen Host über die obige Adresse. Beim Hinzufügen einer Site auf einem anderen Host diese Adresse unter „Weiterleiten an“ eintragen – dann ist die Strecke zwischen den beiden Hosts verschlüsselt.",
  "已经装好，还没有登录：点“加入网络”。":
    "Bereits installiert, noch nicht angemeldet: auf „Netz beitreten“ klicken.",
  "这台主机已经登录，但那个 Tailscale 网络要求管理员批准新设备：到 Tailscale 后台批准它，再回来刷新。":
    "Dieser Host ist angemeldet, aber dieses Tailscale-Netz verlangt, dass ein Administrator neue Geräte genehmigt: in der Tailscale-Admin-Konsole genehmigen und dann hier aktualisieren.",
  "在 {host} 上安装 Tailscale":
    "Tailscale auf {host} installieren",
  "添加 Tailscale 的官方软件源（pkgs.tailscale.com）和它的签名密钥，用系统的包管理器安装，并启动 tailscaled 服务。装完还没有登录，要再“加入网络”。":
    "Fügt die offizielle Paketquelle von Tailscale (pkgs.tailscale.com) und deren Signaturschlüssel hinzu, installiert mit dem Paketmanager des Systems und startet den Dienst tailscaled. Danach ist der Host noch nicht angemeldet; anschließend „Netz beitreten“.",
  "这台主机要能连上 pkgs.tailscale.com。Tailscale 决定主机能被谁连上，安装是高危操作，批准时要重新验证身份。":
    "Dieser Host muss pkgs.tailscale.com erreichen können. Tailscale bestimmt, wer den Host erreichen kann; die Installation ist ein Hochrisiko-Vorgang, bei der Genehmigung ist eine erneute Identitätsprüfung nötig.",
  "断开 {host} 的 Tailscale":
    "Tailscale auf {host} trennen",
  "断开后这台主机的内网地址不再可用：经这个地址访问它的站点和连接都会断。登录状态还在，随时可以重新连上；要彻底离开那个网络，断开后再“退出登录”。":
    "Nach dem Trennen ist die interne Adresse dieses Hosts nicht mehr nutzbar: Sites und Verbindungen, die über sie laufen, brechen ab. Die Anmeldung bleibt bestehen, Sie können jederzeit wieder verbinden; um das Netz ganz zu verlassen, nach dem Trennen noch „Abmelden“.",
  "让 {host} 退出 Tailscale 登录":
    "{host} bei Tailscale abmelden",
  "退出登录后，这台主机不再属于那个 Tailscale 网络，再加入要重新登录或用新的授权密钥。":
    "Nach dem Abmelden gehört dieser Host nicht mehr zu diesem Tailscale-Netz; für einen erneuten Beitritt ist eine neue Anmeldung oder ein neuer Auth-Schlüssel nötig.",
  "{addr} 是 Tailscale 的内网地址，而这台主机还没有连上 Tailscale 网络，转发不过去：先在这一页的 Tailscale 卡片里加入网络。":
    "{addr} ist eine interne Tailscale-Adresse, aber dieser Host ist noch nicht mit dem Tailscale-Netz verbunden – die Weiterleitung kommt nicht an. Zuerst über die Tailscale-Karte auf dieser Seite dem Netz beitreten.",
  "流量会先到这台主机，再经 Tailscale 转到 {addr}：上传和下载都算这台主机的流量。两台主机之间这一段是加密的，那台主机的端口不用对公网开放；它上面的服务不能只监听 127.0.0.1。":
    "Der Verkehr erreicht zuerst diesen Host und wird dann über Tailscale an {addr} weitergeleitet: Uploads und Downloads zählen beide zum Traffic dieses Hosts. Die Strecke zwischen den beiden Hosts ist verschlüsselt, und der Port des anderen Hosts muss nicht ins Internet geöffnet werden; der Dienst dort darf nicht nur auf 127.0.0.1 lauschen.",
};

export default dict;
