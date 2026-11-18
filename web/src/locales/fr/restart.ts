// fr 词典：设置页的“重启 Hostower”。
const dict: Record<string, string> = {
  "立即重启": "Redémarrer maintenant",
  "重启 Hostower": "Redémarrer Hostower",
  "重启": "Redémarrer",
  "刷新页面": "Recharger la page",
  "重启会让面板断开几秒钟，保存过的面板设置（监听地址、访问地址、证书、会话时长）在重启后生效。需要当场验证一次身份。": "Le redémarrage coupe le panneau quelques secondes ; les réglages enregistrés (adresse d’écoute, URL d’accès, certificat, durée de session) s’appliquent après. Une vérification d’identité est demandée.",
  "有 {n} 个请求正在执行，重启会中断它们（之后标成“中断”，不会自动重跑）。": "{n} demandes sont en cours ; le redémarrage les interrompt (marquées « interrompues », non relancées).",
  "访问地址会变成 ": "L’URL d’accès devient ",
  "。重启后请到新地址打开；如果前面有反向代理，记得一起改它转发的端口。": ". Après le redémarrage, ouvrez la nouvelle adresse ; s’il y a un proxy inverse devant, changez aussi le port vers lequel il redirige.",
  "正在重启。稍等几秒后到新地址打开：": "Redémarrage. Patientez quelques secondes puis ouvrez la nouvelle adresse : ",
  "正在重启，回来后会自动刷新页面…": "Redémarrage ; la page se recharge au retour…",
  "一分钟了还没连上。可能是新的面板设置有问题，或者反向代理转发的端口没跟着改。到主机上看日志：": "Toujours injoignable après une minute. Les nouveaux réglages sont peut-être faux, ou le proxy inverse redirige encore vers l’ancien port. Consultez le journal sur l’hôte : ",
  "；面板设置写错时，可以临时设置环境变量 HOSTOWER_IGNORE_PANEL=1 跳过它启动。": " ; si les réglages du panneau sont faux, définissez temporairement HOSTOWER_IGNORE_PANEL=1 pour démarrer sans eux.",
};

export default dict;
