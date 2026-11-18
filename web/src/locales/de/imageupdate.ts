// de 词典：容器应用按镜像仓库检测更新。
const dict: Record<string, string> = {
  "拉取新构建并重建容器": "Neuen Build ziehen und Container neu erstellen",
  "同一个镜像标签在仓库里有了新的构建（通常是小版本或安全修复），更新会重新拉取并重建容器。": "Dasselbe Image-Tag zeigt in der Registry auf einen neuen Build (meist Minor-Version oder Sicherheitsfix); das Update zieht ihn und erstellt den Container neu.",
  "镜像有新构建": "Neuer Image-Build",
  "镜像仓库里已经有这个大版本，但目录还没收录：要先确认能不能原地升级，Hostower 不会自动换过去。": "Diese Hauptversion gibt es in der Registry, aber noch nicht im Katalog: Ob ein In-Place-Upgrade geht, muss erst geprüft werden, daher wechselt Hostower nicht automatisch.",
  "上游有新大版本 {v}": "Upstream-Hauptversion {v} verfügbar",
};

export default dict;
