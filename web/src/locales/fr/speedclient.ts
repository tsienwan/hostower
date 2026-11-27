// fr 词典：“这台设备”测速（浏览器和面板之间的速度、回程和去程路由）。
const dict: Record<string, string> = {
  "这台设备 ↔ 主机":
    "Cet appareil ↔ hôte",
  "测这台设备":
    "Tester cet appareil",
  "把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。":
    "Traite l’appareil qui affiche le panneau (par exemple un téléphone connecté à la maison, au bureau ou en déplacement) comme un nœud : mesure la latence, le téléchargement et l’envoi entre lui et l’hôte de votre choix, puis trace la route de cet hôte jusqu’à lui pour voir la ligne empruntée au retour et les nœuds traversés.",
  "各网络最近一次":
    "Dernier test par réseau",
  "没有拿到结果":
    "Aucun résultat obtenu",
  "速度测到哪里":
    "Destination de la mesure de débit",
  "主机（直连）":
    "Hôtes (direct)",
  "在选中的主机上各临时开一个测速服务，这台设备的浏览器一台一台直接连过去量延迟、下载、上传，再从每台主机追踪回到这台设备，最后排成一张对比表。服务只认带一次性口令的地址，测完自己退出。开端口是一次变更（L1），需要批准，一次批准管这一批。":
    "Ouvre temporairement un service de test de débit sur chaque hôte sélectionné : le navigateur de cet appareil s’y connecte directement, un hôte après l’autre, pour mesurer la latence, le téléchargement et l’envoi, puis la route est tracée de chaque hôte jusqu’à cet appareil, et le tout est réuni dans un tableau comparatif. Le service n’accepte que l’adresse munie d’un jeton à usage unique et s’arrête de lui-même une fois le test terminé. Ouvrir un port est une modification (L1) et doit être approuvé ; une seule approbation couvre tout le lot.",
  "延迟和速度量的是这台设备和面板之间；选中的主机只用来追踪回到这台设备的路由。":
    "La latence et le débit sont mesurés entre cet appareil et le panneau ; les hôtes sélectionnés ne servent qu’à tracer la route de retour vers cet appareil.",
  "测哪些主机（最多 {n} 台）":
    "Hôtes à tester ({n} au plus)",
  "{host} 的地址":
    "Adresse de {host}",
  "地址填这台设备能直接连到的（公网 IP 或域名）。按勾选的顺序一台一台测。":
    "Indiquez des adresses que cet appareil peut joindre directement (IP publique ou nom de domaine). Les hôtes sont testés un par un, dans l’ordre où ils ont été cochés.",
  "每台主机的防火墙和云厂商的安全组都要放行这个 TCP 端口，端口在 1024 到 65535 之间。":
    "Le pare-feu de chaque hôte et le groupe de sécurité du fournisseur cloud doivent autoriser ce port TCP, compris entre 1024 et 65535.",
  "测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。":
    "La page de test est servie directement par l’hôte, en HTTP : le navigateur peut afficher « non sécurisé », choisissez de continuer. Elle ne contient que des données aléatoires destinées au test, sans aucune information de connexion au panneau.",
  "回程路由从哪台主机追踪":
    "Hôte d’où tracer la route de retour",
  "没有在线的主机":
    "Aucun hôte en ligne",
  "这台设备的地址":
    "Adresse de cet appareil",
  "这是面板看到的来源地址，回程路由就追踪到它。用了代理或 VPN 访问面板时，它是代理的出口地址。":
    "C’est l’adresse source vue par le panneau ; la route de retour est tracée jusqu’à elle. Si vous accédez au panneau via un proxy ou un VPN, c’est l’adresse de sortie du proxy.",
  "面板前面有反向代理：延迟和速度量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "Un proxy inverse se trouve devant le panneau : la latence et le débit mesurent le tronçon entre cet appareil et le proxy (ou le CDN), plus celui du proxy au panneau.",
  "读不出可以追踪的来源地址（比如从本机回环地址访问），测不了。":
    "Impossible de déterminer une adresse source traçable (par exemple en accédant depuis l’adresse de bouclage locale) : le test est impossible.",
  "数据量上限":
    "Volume maximal",
  "每台下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，这次最多约 {total} MB。":
    "Par hôte : téléchargement de {d} Mo au plus, envoi de {u} Mo au plus, {s} secondes au plus chacun, selon la première limite atteinte. Consomme le trafic des hôtes et de cet appareil : environ {total} Mo au maximum cette fois.",
  "正在检查主机上有没有 traceroute…":
    "Vérification de la présence de traceroute sur l’hôte…",
  "{host} 上没有 traceroute 或 tracepath，追踪不了路由。":
    "{host} n’a ni traceroute ni tracepath : la route ne peut pas être tracée.",
  "先用系统的包管理器安装 traceroute":
    "Installer d’abord traceroute avec le gestionnaire de paquets du système",
  "安装是一次变更（L1），需要批准；不装的话只有速度，没有回程路由。":
    "L’installation est une modification (L1) et doit être approuvée ; sans elle, seul le débit est mesuré, sans route de retour.",
  "这台设备 ↔ {host}":
    "Cet appareil ↔ {host}",
  "正在测，请不要关闭这个页面…":
    "Test en cours, ne fermez pas cette page…",
  "测完了，可以关掉这个页面回到面板。":
    "Test terminé : vous pouvez fermer cette page et revenir au panneau.",
  "结果没有交回去，请回到面板重试。":
    "Le résultat n’a pas pu être transmis ; revenez au panneau et réessayez.",
  "接着测下一台…":
    "Passage à l’hôte suivant…",
  "没有测成":
    "Échec du test",
  "测完了":
    "Test terminé",
  "正在追踪回程路由…":
    "Traçage de la route de retour…",
  "正在测…":
    "Test en cours…",
  "等这台设备连上来":
    "En attente de la connexion de cet appareil",
  "正在启动测速服务…":
    "Démarrage du service de test de débit…",
  "批准后才会在主机上开测速服务":
    "Le service de test de débit ne sera ouvert sur l’hôte qu’après approbation",
  "这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。":
    "Un port ({port}) sera ouvert temporairement sur {host} en attendant que cet appareil s’y connecte pour le test ; le service s’arrête de lui-même une fois le test terminé.",
  "正在主机上启动测速服务…":
    "Démarrage du service de test de débit sur les hôtes…",
  "测速服务已经在等着了。在这台设备上打开测速页，它会自己开始，一台一台测下去。":
    "Les services de test de débit sont en attente. Ouvrez la page de test sur cet appareil : le test démarre tout seul et enchaîne les hôtes un par un.",
  "打开测速页（还有 {n} 台）":
    "Ouvrir la page de test ({n} restant(s))",
  "打开测速页":
    "Ouvrir la page de test",
  "打不开或一直转圈：那台主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者填的地址这台设备连不上。回到这里可以接着测剩下的。服务最多等 {w} 秒。":
    "La page ne s’ouvre pas ou charge sans fin ? Le pare-feu de cet hôte ou le groupe de sécurité du fournisseur cloud n’autorise pas le port TCP {port}, ou cet appareil ne peut pas joindre l’adresse indiquée. Revenez ici pour poursuivre avec les hôtes restants. Le service attend {w} secondes au plus.",
  "正在另一个页面里测 {host}…":
    "Test de {host} en cours dans une autre page…",
  "下载时的延迟":
    "Latence pendant le téléchargement",
  "回程重传率":
    "Taux de retransmission au retour",
  "回程线路":
    "Ligne de retour",
  "延迟最低":
    "Latence la plus faible",
  "延迟和速度是这台设备的浏览器直接和每台主机量的；回程线路是从主机追踪回来认出的，只能作参考。每台的明细在各自的报告里。":
    "La latence et le débit sont mesurés directement par le navigateur de cet appareil avec chaque hôte ; la ligne de retour est déduite de la route tracée depuis l’hôte, à titre indicatif. Le détail de chaque hôte figure dans son rapport.",
  "查看报告":
    "Voir le rapport",
  "失败 {n} 次":
    "{n} échec(s)",
  "没有测出来":
    "Aucun résultat",
  "速度已经记下。":
    "Le débit a été enregistré.",
  "批准后才会追踪回程路由":
    "La route de retour ne sera tracée qu’après approbation",
  "这次要先安装 traceroute，是一次变更；速度不受影响，已经在测。":
    "Cette fois, traceroute doit d’abord être installé, ce qui est une modification ; le débit n’est pas concerné, sa mesure est déjà en cours.",
  "正在从 {host} 追踪到这台设备，最多要半分钟…":
    "Traçage de {host} vers cet appareil en cours, jusqu’à trente secondes…",
  "回程路由追踪完了，在报告里看线路和每一跳。":
    "La route de retour est tracée : la ligne et chaque saut figurent dans le rapport.",
  "回程路由没有追踪成功，报告里只有速度。":
    "Le traçage de la route de retour a échoué ; le rapport ne contient que le débit.",
  "这台设备 ↔ 面板":
    "Cet appareil ↔ panneau",
  "这次没有收到浏览器量的延迟和速度（测到一半关掉了页面），只有回程路由。":
    "La latence et le débit mesurés par le navigateur n’ont pas été reçus cette fois (page fermée en cours de test) ; seule la route de retour est disponible.",
  "失败的请求":
    "Requêtes échouées",
  "共 {n} 次":
    "{n} au total",
  "这次没有拿到回程路由。":
    "Aucune route de retour n’a été obtenue cette fois.",
  "这是回程：从主机追踪到这台设备的地址。去程（这台设备到主机）在浏览器里追踪不了，所以这里没有；来回两个方向走的线路常常不一样。线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。":
    "Ceci est la route de retour, tracée de l’hôte vers l’adresse de cet appareil. La route aller (de cet appareil vers l’hôte) ne peut pas être tracée depuis un navigateur, elle n’apparaît donc pas ici ; les deux sens empruntent souvent des lignes différentes. Les lignes sont déduites des plages de dorsale publiques des opérateurs et du système autonome (AS) de chaque saut, à titre indicatif ; les sauts qui ne répondent pas sont affichés comme *.",
  "蜂窝网络":
    "Réseau mobile",
  "有线网络":
    "Réseau filaire",
  "这台设备的浏览器直接连到 {host} 上临时开的测速服务量的，不经过面板：延迟是连发小请求的往返时间，取中位数，里面含一点服务处理的时间；下载和上传是浏览器直接和这台主机传数据。":
    "Mesuré par le navigateur de cet appareil, connecté directement au service de test de débit ouvert temporairement sur {host}, sans passer par le panneau : la latence est le temps d’aller-retour de petites requêtes envoyées à la suite (médiane), avec un peu de temps de traitement du service ; le téléchargement et l’envoi sont mesurés par un transfert direct de données entre le navigateur et cet hôte.",
  "延迟是浏览器向面板连发小请求量出来的往返时间，取中位数，里面含一点服务器处理的时间；下载和上传是浏览器直接和面板传数据量的。":
    "La latence est le temps d’aller-retour de petites requêtes envoyées à la suite par le navigateur au panneau (médiane), avec un peu de temps de traitement du serveur ; le téléchargement et l’envoi sont mesurés par un transfert direct de données entre le navigateur et le panneau.",
  "面板前面有反向代理：这些数字量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "Un proxy inverse se trouve devant le panneau : ces chiffres mesurent le tronçon entre cet appareil et le proxy (ou le CDN), plus celui du proxy au panneau.",
  "往返时间":
    "Temps d’aller-retour",
  "最小 {min}、中位 {med}、最大 {max} ms":
    "min {min}, médiane {med}, max {max} ms",
  "上传时的延迟":
    "Latence pendant l’envoi",
  "连接类型":
    "Type de connexion",
  "从 {host} ping 它":
    "Ping depuis {host}",
  "不回应":
    "Pas de réponse",
  "平均 {avg} ms，丢包 {loss}%":
    "Moyenne {avg} ms, perte {loss} %",
  "{a} / {b} 个数据段":
    "{a} / {b} segments",
  "TCP 往返时间":
    "Temps d’aller-retour TCP",
  "重传率是下载时 {host} 重发的数据段占比，从它的 TCP 连接上读的：回程丢包越多它越高，到 1% 以上速度就会明显受影响。":
    "Le taux de retransmission est la part des segments que {host} a dû renvoyer pendant le téléchargement, lue sur sa connexion TCP : plus il y a de pertes au retour, plus il est élevé ; au-delà de 1 %, le débit s’en ressent nettement.",
  "家用路由器和运营商的地址常常不回应 ping，不代表不通。":
    "Les adresses des routeurs domestiques et des opérateurs ne répondent souvent pas au ping ; cela ne signifie pas qu’elles sont injoignables.",
  "比空闲时高 {n} ms":
    "{n} ms de plus qu’au repos",
  "和空闲时差不多":
    "Comparable au repos",
  "回程：{host} → 这台设备":
    "Retour : {host} → cet appareil",
  "每一跳探了 {n} 次，“丢包”是没回应的次数。中间某一跳丢、后面的跳不丢，多半是那台路由器限制了回应的速率，不是真的丢包；从某一跳开始一直丢到最后，问题才在那一段。":
    "Chaque saut est sondé {n} fois ; les « pertes » sont les sondes restées sans réponse. Des pertes à un saut intermédiaire mais pas aux suivants indiquent le plus souvent que ce routeur limite son débit de réponses, pas une perte réelle ; le problème ne se situe sur un tronçon que si les pertes persistent de ce saut jusqu’au dernier.",
  "最后一跳没有到达这台设备：家用路由器和运营商的地址常常不回应探测，前面的节点和线路仍然可以看。":
    "Le dernier saut n’atteint pas cet appareil : les adresses des routeurs domestiques et des opérateurs ignorent souvent les sondes ; les nœuds précédents et la ligne restent consultables.",
};

export default dict;
