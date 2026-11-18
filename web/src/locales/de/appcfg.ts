// de 词典：应用实例的设置弹窗、容器页和应用市场的融合。
const dict: Record<string, string> = {
  "在容器页查看": "In Container ansehen",
  "没有找到这个实例，或者目录里没有这个应用。": "Instanz nicht gefunden oder App nicht im Katalog.",
  "按新设置重新生成 compose 文件，只重建设置变了的容器；数据目录不动，仍由应用市场管理。": "Erzeugt die Compose-Datei mit den neuen Einstellungen neu und baut nur geänderte Container neu; das Datenverzeichnis bleibt, die App bleibt vom Katalog verwaltet.",
  "目录里的版本较新：保存设置会同时更新到 {v}（先拉取新镜像）。": "Der Katalog hat eine neuere Version: Speichern aktualisiert auch auf {v} (zieht zuerst das neue Image).",
  "这个应用没有可以修改的参数。": "Diese App hat keine änderbaren Parameter.",
  "更换密钥（不勾选时保持不变）": "Geheimnisse ändern (sonst unverändert)",
  "由应用市场安装，在应用页管理": "Aus dem App-Katalog installiert; auf der Apps-Seite verwaltet",
  "卸载应用 {name}": "App {name} deinstallieren",
  "修改应用 {name} 的设置": "Einstellungen der App {name} ändern",
};

export default dict;
