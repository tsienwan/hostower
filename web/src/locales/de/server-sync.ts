// 服务端文案的德文词典：文件同步的校验错误和请求说明。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "一次分发 1 到 {a} 个文件": "1 bis {a} Dateien auf einmal verteilen",
  "文件名不合法或重复": "Dateiname ungültig oder doppelt",
  "文件名规则只能用字母、数字、. _ - 和通配符 * ? [ ]，不能带路径": "Ein Dateinamenmuster darf nur Buchstaben, Ziffern, . _ - und die Platzhalter * ? [ ] enthalten, keinen Pfad",
  "目录层数要在 1 到 {a} 之间": "Die Verzeichnistiefe muss zwischen 1 und {a} liegen",
  "没有匹配的文件": "Keine passenden Dateien",
  "分发文件 {a}": "Datei {a} verteilen",
  "分发文件 {a}（{b} 台主机）": "Datei {a} verteilen ({b} Host(s))",
};

export default dict;
