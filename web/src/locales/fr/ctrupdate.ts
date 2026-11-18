// fr 词典：容器页的更新提示。
const dict: Record<string, string> = {
  "由应用市场更新": "Mis à jour via le catalogue",
  "这个容器由你自己的 Compose 项目管理：在它的 compose 目录里执行 docker compose pull && docker compose up -d 更新，在这里重建会让它脱离 Compose。": "Ce conteneur est géré par votre propre projet Compose : mettez-le à jour avec docker compose pull && docker compose up -d dans son répertoire ; le recréer ici le détacherait de Compose.",
  "更新：拉取新镜像，按原来的设置重建容器": "Mettre à jour : télécharger la nouvelle image et recréer le conteneur avec les mêmes réglages",
  "更新 {name}：拉取新镜像并按原设置重建": "Mettre à jour {name} : nouvelle image, mêmes réglages",
  "有新版本 {v}": "Nouvelle version {v}",
  "仓库里有同一大版本内更新的版本，更新会换成新标签重新部署，原来的设置不变。": "Le registre propose une version plus récente dans la même version majeure ; la mise à jour redéploie avec la nouvelle étiquette en conservant vos réglages.",
  "更新到 {v}：换成新标签，按原来的设置重建容器": "Mettre à jour vers {v} : nouvelle étiquette, conteneur recréé avec les mêmes réglages",
};

export default dict;
