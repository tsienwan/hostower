// 服务端文案的德文词典：Agent 任务的校验错误、请求说明和判级理由。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "不认识的 AI 编程助手": "Unbekannter KI-Coding-Assistent",
  "任务不能为空，最长 {a} 个字": "Die Aufgabe darf nicht leer und höchstens {a} Zeichen lang sein",
  "这个 AI 编程助手不能用来汇总（要能从标准输入读材料）": "Dieser KI-Coding-Assistent kann nicht zum Zusammenfassen verwendet werden (er muss Material von der Standardeingabe lesen können)",
  "只能汇总 Agent 任务的结果": "Nur Ergebnisse von Agent-Aufgaben können zusammengefasst werden",
  "这个任务还没有执行完": "Diese Aufgabe ist noch nicht fertig ausgeführt",
  "Agent 任务（{a}）：{b}": "Agent-Aufgabe ({a}): {b}",
  "Agent 任务（{a}）：{b}（{c} 台主机）": "Agent-Aufgabe ({a}): {b} ({c} Host(s))",
  "汇总 {a} 的结果（{b}）": "Ergebnisse von {a} zusammenfassen ({b})",
  "运行 AI 编程助手：它会按自己的权限设置读写文件、访问网络": "Führt einen KI-Coding-Assistenten aus: Er liest und schreibt Dateien und greift aufs Netzwerk zu, gemäß seinen eigenen Berechtigungseinstellungen",
  "让 AI 编程助手跳过权限确认或沙箱，等于让它在主机上随意执行命令": "Einen KI-Coding-Assistenten Berechtigungsabfragen oder die Sandbox überspringen zu lassen heißt, ihn beliebige Befehle auf dem Host ausführen zu lassen",
  "AI 编程助手的参数看不全，无法确认有没有跳过权限确认": "Die Argumente des KI-Coding-Assistenten sind nicht vollständig einsehbar; ob Berechtigungsabfragen übersprungen werden, lässt sich nicht bestätigen",
  "写审计失败，没有暂存": "Audit-Eintrag konnte nicht geschrieben werden; die Datei wurde nicht zwischengespeichert",
  "写审计失败，没有取消：{a}": "Audit-Eintrag konnte nicht geschrieben werden; nicht abgebrochen: {a}",
};

export default dict;
