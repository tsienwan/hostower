// fr 词典：“这台设备”测速的服务端文字。
const dict: Record<string, string> = {
  "同时在测速的连接太多了，请稍后再试":
    "Trop de connexions testent le débit en même temps, réessayez plus tard",
  "数据量不对":
    "Volume de données incorrect",
  "生成数据失败":
    "Échec de la génération des données",
  "一次传得太多了":
    "Trop de données envoyées en une fois",
  "没有这条测速请求":
    "Cette demande de test de débit n’existe pas",
  "追踪从 {a} 到这台设备（{b}）的回程路由":
    "Tracer la route de retour de {a} vers cet appareil ({b})",
  "安装缺少的 traceroute 并追踪从 {a} 到这台设备（{b}）的回程路由":
    "Installer traceroute, absent, et tracer la route de retour de {a} vers cet appareil ({b})",
  "读不出这台设备的来源地址，没法追踪回程路由":
    "Impossible de déterminer l’adresse source de cet appareil ; la route de retour ne peut pas être tracée",
  "这台设备的来源地址不能用来追踪路由":
    "L’adresse source de cet appareil ne peut pas servir à tracer une route",
  "测量结果里的数字不对":
    "Les chiffres des résultats de mesure sont incorrects",
  "测量结果里的延迟次数不对":
    "Le nombre de mesures de latence dans les résultats est incorrect",
  "贴进来的内容太长了":
    "Le contenu collé est trop long",
  "没有从贴进来的内容里认出路由：请贴 tracert、traceroute 或 mtr 的完整输出":
    "Aucune route reconnue dans le contenu collé : collez la sortie complète de tracert, traceroute ou mtr",
  "端口应该在 1024 到 65535 之间":
    "Le port doit être compris entre 1024 et 65535",
  "口令不对":
    "Jeton incorrect",
  "在 {a} 上临时开一个测速服务（端口 {b}），测这台设备和它之间的速度并追踪回程路由":
    "Ouvrir temporairement un service de test de débit sur {a} (port {b}), mesurer le débit entre cet appareil et lui, puis tracer la route de retour",
  "主机上没有 python3，开不了临时测速服务":
    "python3 est absent de l’hôte : impossible d’ouvrir le service de test de débit temporaire",
  "主机上这个端口已经被占用，换一个端口再试":
    "Ce port est déjà utilisé sur l’hôte ; réessayez avec un autre port",
  "没有等到这台设备连上来：测速页没打开，或者主机的防火墙、云厂商的安全组没有放行这个端口":
    "Cet appareil ne s’est pas connecté : la page de test n’a pas été ouverte, ou le pare-feu de l’hôte ou le groupe de sécurité du fournisseur cloud n’autorise pas ce port",
  "这台设备连上来了，但没有在限定时间里测完":
    "Cet appareil s’est connecté, mais le test ne s’est pas terminé dans le temps imparti",
};

export default dict;
