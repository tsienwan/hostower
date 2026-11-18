// zh-TW 词典：容器页的更新提示。
const dict: Record<string, string> = {
  "由应用市场更新": "由應用市場更新",
  "这个容器由你自己的 Compose 项目管理：在它的 compose 目录里执行 docker compose pull && docker compose up -d 更新，在这里重建会让它脱离 Compose。": "這個容器由你自己的 Compose 專案管理：在它的 compose 目錄裡執行 docker compose pull && docker compose up -d 更新，在這裡重建會讓它脫離 Compose。",
  "更新：拉取新镜像，按原来的设置重建容器": "更新：拉取新映像，按原來的設定重建容器",
  "更新 {name}：拉取新镜像并按原设置重建": "更新 {name}：拉取新映像並按原設定重建",
  "有新版本 {v}": "有新版本 {v}",
  "仓库里有同一大版本内更新的版本，更新会换成新标签重新部署，原来的设置不变。": "倉庫裡有同一大版本內更新的版本，更新會換成新標籤重新部署，原來的設定不變。",
  "更新到 {v}：换成新标签，按原来的设置重建容器": "更新到 {v}：換成新標籤，按原來的設定重建容器",
};

export default dict;
