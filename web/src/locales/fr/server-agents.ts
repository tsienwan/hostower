// 服务端文案的法文词典：Agent 任务的校验错误、请求说明和判级理由。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "不认识的 AI 编程助手": "Assistant de code IA inconnu",
  "任务不能为空，最长 {a} 个字": "La tâche ne peut pas être vide et est limitée à {a} caractères",
  "这个 AI 编程助手不能用来汇总（要能从标准输入读材料）": "Cet assistant de code IA ne peut pas servir à la synthèse (il doit pouvoir lire des données sur l’entrée standard)",
  "只能汇总 Agent 任务的结果": "Seuls les résultats de tâches Agent peuvent être synthétisés",
  "这个任务还没有执行完": "Cette tâche n’a pas encore fini de s’exécuter",
  "Agent 任务（{a}）：{b}": "Tâche Agent ({a}) : {b}",
  "Agent 任务（{a}）：{b}（{c} 台主机）": "Tâche Agent ({a}) : {b} ({c} hôte(s))",
  "汇总 {a} 的结果（{b}）": "Synthèse des résultats de {a} ({b})",
  "运行 AI 编程助手：它会按自己的权限设置读写文件、访问网络": "Exécute un assistant de code IA : il lit et écrit des fichiers et accède au réseau selon ses propres réglages de permissions",
  "让 AI 编程助手跳过权限确认或沙箱，等于让它在主机上随意执行命令": "Faire ignorer à un assistant de code IA les confirmations de permission ou le bac à sable revient à le laisser exécuter n’importe quelle commande sur l’hôte",
  "AI 编程助手的参数看不全，无法确认有没有跳过权限确认": "Les arguments de l’assistant de code IA ne sont pas entièrement visibles ; impossible de confirmer s’il ignore les confirmations de permission",
  "写审计失败，没有暂存": "Impossible d'écrire le journal d'audit : le fichier n'a pas été mis en attente",
  "写审计失败，没有取消：{a}": "Impossible d'écrire le journal d'audit : non annulé : {a}",
};

export default dict;
