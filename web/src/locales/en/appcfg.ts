// en 词典：应用实例的设置弹窗、容器页和应用市场的融合。
const dict: Record<string, string> = {
  "在容器页查看": "View in Containers",
  "没有找到这个实例，或者目录里没有这个应用。": "This instance was not found, or the app is not in the catalog.",
  "按新设置重新生成 compose 文件，只重建设置变了的容器；数据目录不动，仍由应用市场管理。": "Regenerates the compose file from the new settings and recreates only the containers whose settings changed; the data directory is untouched and the app stays managed by the catalog.",
  "目录里的版本较新：保存设置会同时更新到 {v}（先拉取新镜像）。": "The catalog has a newer version: saving will also update to {v} (pulling the new image first).",
  "这个应用没有可以修改的参数。": "This app has no parameters to change.",
  "更换密钥（不勾选时保持不变）": "Change secrets (left unchanged if unchecked)",
  "由应用市场安装，在应用页管理": "Installed from the app catalog; managed on the Apps page",
  "卸载应用 {name}": "Uninstall app {name}",
  "修改应用 {name} 的设置": "Change settings of app {name}",
};

export default dict;
