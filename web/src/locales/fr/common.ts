// Dictionnaire français：键是中文原文。这个文件放跨页面通用的词；各页面自己的词放在同目录的其他文件里。
const dict: Record<string, string> = {
  "跟随系统": "Système",
  "深色": "Sombre",
  "浅色": "Clair",
  "主题": "Thème",
  "语言": "Langue",
  "时区": "Fuseau horaire",
  "无法连接塔台": "Impossible de joindre la tour",
  "请求失败（{status}）": "Échec de la requête ({status})",
  "上传中断": "Téléversement interrompu",
  "上传失败（{status}）": "Échec du téléversement ({status})",
  "这个浏览器不支持 Passkey，请换用较新的浏览器": "Ce navigateur ne prend pas en charge les Passkeys. Veuillez utiliser un navigateur plus récent.",
  "Passkey 注册被取消": "Enregistrement de la Passkey annulé",
  "Passkey 验证被取消": "Vérification de la Passkey annulée",
  // 带语境的键：同一个中文词在这些位置需要不同的译文。
  "创建@@时间": "Créé",
  "提交@@时间": "Soumis",
  "主机@@列表": "Hôtes",
  "系统@@操作系统": "OS",
};

export default dict;
