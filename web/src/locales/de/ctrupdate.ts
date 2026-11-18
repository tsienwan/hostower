// de 词典：容器页的更新提示。
const dict: Record<string, string> = {
  "由应用市场更新": "Über den App-Katalog aktualisiert",
  "这个容器由你自己的 Compose 项目管理：在它的 compose 目录里执行 docker compose pull && docker compose up -d 更新，在这里重建会让它脱离 Compose。": "Dieser Container gehört zu Ihrem eigenen Compose-Projekt: im Compose-Verzeichnis docker compose pull && docker compose up -d ausführen; hier neu erstellen würde ihn von Compose lösen.",
  "更新：拉取新镜像，按原来的设置重建容器": "Aktualisieren: neues Image ziehen und Container mit denselben Einstellungen neu erstellen",
  "更新 {name}：拉取新镜像并按原设置重建": "{name} aktualisieren: neues Image ziehen, mit denselben Einstellungen neu erstellen",
  "有新版本 {v}": "Neue Version {v}",
  "仓库里有同一大版本内更新的版本，更新会换成新标签重新部署，原来的设置不变。": "Die Registry hat eine neuere Version derselben Hauptversion; das Update stellt mit dem neuen Tag neu bereit und behält Ihre Einstellungen.",
  "更新到 {v}：换成新标签，按原来的设置重建容器": "Auf {v} aktualisieren: neues Tag, Container mit denselben Einstellungen neu erstellen",
};

export default dict;
