// en 词典：容器页的更新提示。
const dict: Record<string, string> = {
  "由应用市场更新": "Updated through the app catalog",
  "这个容器由你自己的 Compose 项目管理：在它的 compose 目录里执行 docker compose pull && docker compose up -d 更新，在这里重建会让它脱离 Compose。": "This container is managed by your own Compose project: update it by running docker compose pull && docker compose up -d in its compose directory; recreating it here would detach it from Compose.",
  "更新：拉取新镜像，按原来的设置重建容器": "Update: pull the new image and recreate the container with the same settings",
  "更新 {name}：拉取新镜像并按原设置重建": "Update {name}: pull the new image and recreate with the same settings",
  "有新版本 {v}": "New version {v}",
  "仓库里有同一大版本内更新的版本，更新会换成新标签重新部署，原来的设置不变。": "The registry has a newer version within the same major version; updating redeploys with the new tag and keeps your settings.",
  "更新到 {v}：换成新标签，按原来的设置重建容器": "Update to {v}: switch to the new tag and recreate the container with the same settings",
};

export default dict;
