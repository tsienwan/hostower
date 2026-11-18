// Deutsches Wörterbuch: Die Schlüssel sind der chinesische Originaltext. Diese Datei enthält seitenübergreifende Begriffe; die Begriffe einzelner Seiten liegen in den anderen Dateien dieses Verzeichnisses.
const dict: Record<string, string> = {
  "跟随系统": "System",
  "深色": "Dunkel",
  "浅色": "Hell",
  "主题": "Design",
  "语言": "Sprache",
  "时区": "Zeitzone",
  "无法连接塔台": "Tower nicht erreichbar",
  "请求失败（{status}）": "Anfrage fehlgeschlagen ({status})",
  "上传中断": "Upload unterbrochen",
  "上传失败（{status}）": "Upload fehlgeschlagen ({status})",
  "这个浏览器不支持 Passkey，请换用较新的浏览器": "Dieser Browser unterstützt keine Passkeys. Bitte verwenden Sie einen neueren Browser.",
  "Passkey 注册被取消": "Passkey-Registrierung abgebrochen",
  "Passkey 验证被取消": "Passkey-Prüfung abgebrochen",
  // Schlüssel mit Kontext: Dasselbe chinesische Wort braucht an diesen Stellen unterschiedliche Übersetzungen.
  "创建@@时间": "Erstellt",
  "提交@@时间": "Eingereicht",
  "主机@@列表": "Hosts",
  "系统@@操作系统": "Betriebssystem",
};

export default dict;
