// Deutsches Wörterbuch: shared.tsx – Zielauswahl, Risikostufen-Vorschau, Batch-Strategie.
const dict: Record<string, string> = {
  "目标主机": "Ziel-Hosts",
  "主机名、tag:web、group:prod，可用 and / or / not 组合": "Hostname, tag:web, group:prod; kombinierbar mit and / or / not",
  "匹配 {n} 台：": "{n} Treffer:",
  "高敏感主机": "Hochsensibler Host",
  "等 {n} 台": "… insgesamt {n}",
  "只读操作，提交后自动执行。": "Lesender Vorgang. Wird nach dem Einreichen automatisch ausgeführt.",
  "变更操作，提交后需要一个人批准才会执行。": "Ändernder Vorgang. Wird erst nach der Genehmigung durch eine Person ausgeführt.",
  "高危操作，批准时要重新验证 Passkey，并手动输入确认内容。": "Hochriskanter Vorgang. Zum Genehmigen müssen Sie Ihren Passkey erneut bestätigen und eine Bestätigung eintippen.",
  "禁止的操作。提交会被内核直接拒绝并写入审计，任何人都无法放行。": "Verbotener Vorgang. Der Kernel lehnt ihn beim Einreichen ab und erfasst ihn im Audit; niemand kann ihn freigeben.",
  "判级中…": "Risikostufe wird ermittelt…",
  "填好目标和命令后，这里会显示风险等级和判定理由。判级由规则引擎完成，预览没有任何副作用。": "Sobald Ziel und Befehl ausgefüllt sind, erscheinen hier die Risikostufe und ihre Begründung. Die Einstufung übernimmt die Regel-Engine; die Vorschau hat keinerlei Nebenwirkungen.",
  "判定理由": "Begründung",
  "每台主机上的实际命令": "Tatsächlicher Befehl je Host",
  "、": ", ",
  "批量执行方式": "Batch-Strategie",
  "先在一台上预演": "Zuerst auf einem Host testen",
  "遇错停止": "Bei Fehler anhalten",
  "并发": "Parallelität",
  "目标里有高敏感主机：强制逐台滚动执行，高敏感主机排在最后。": "Unter den Zielen sind hochsensible Hosts: Die Ausführung erfolgt zwingend Host für Host, hochsensible Hosts zuletzt.",
};

export default dict;
