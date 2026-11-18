// Deutsches Wörterbuch für Servermeldungen (Kern). Die Schlüssel sind die chinesischen Originaltexte des Servers; Schlüssel mit {a} sind Vorlagen.
// Die übrigen sind nach Herkunft auf die server-*.ts-Dateien verteilt.
const dict: Record<string, string> = {
  "命令行执行": "Über die CLI ausführen",
  "{a}（命令行）": "{a} (CLI)",
  "只读": "Nur lesen",
  // Begründungen der Risikostufe in Audit und Anfragen haben die Form „Stufe Begründung: Befehl“.
  "{level} {reason!}：{cmd!}": "{level} {reason!}: {cmd!}",
};

export default dict;
