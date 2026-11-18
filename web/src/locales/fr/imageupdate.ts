// fr 词典：容器应用按镜像仓库检测更新。
const dict: Record<string, string> = {
  "拉取新构建并重建容器": "Télécharger la nouvelle build et recréer le conteneur",
  "同一个镜像标签在仓库里有了新的构建（通常是小版本或安全修复），更新会重新拉取并重建容器。": "Le même tag pointe désormais vers une nouvelle build dans le registre (souvent une version mineure ou un correctif de sécurité) ; la mise à jour la télécharge et recrée le conteneur.",
  "镜像有新构建": "Nouvelle build d’image",
  "镜像仓库里已经有这个大版本，但目录还没收录：要先确认能不能原地升级，Hostower 不会自动换过去。": "Cette version majeure existe déjà dans le registre mais pas encore dans le catalogue : il faut d’abord vérifier la mise à niveau sur place, Hostower ne bascule donc pas automatiquement.",
  "上游有新大版本 {v}": "Version majeure {v} disponible en amont",
};

export default dict;
