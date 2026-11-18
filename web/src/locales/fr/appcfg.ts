// fr 词典：应用实例的设置弹窗、容器页和应用市场的融合。
const dict: Record<string, string> = {
  "在容器页查看": "Voir dans Conteneurs",
  "没有找到这个实例，或者目录里没有这个应用。": "Instance introuvable, ou application absente du catalogue.",
  "按新设置重新生成 compose 文件，只重建设置变了的容器；数据目录不动，仍由应用市场管理。": "Régénère le fichier compose avec les nouveaux réglages et ne recrée que les conteneurs modifiés ; le répertoire de données n’est pas touché et l’application reste gérée par le catalogue.",
  "目录里的版本较新：保存设置会同时更新到 {v}（先拉取新镜像）。": "Le catalogue a une version plus récente : l’enregistrement mettra aussi à jour vers {v} (en téléchargeant d’abord la nouvelle image).",
  "这个应用没有可以修改的参数。": "Cette application n’a aucun paramètre modifiable.",
  "更换密钥（不勾选时保持不变）": "Changer les secrets (inchangés si décoché)",
  "由应用市场安装，在应用页管理": "Installé depuis le catalogue ; géré dans la page Applications",
  "卸载应用 {name}": "Désinstaller l’application {name}",
  "修改应用 {name} 的设置": "Modifier les réglages de l’application {name}",
};

export default dict;
