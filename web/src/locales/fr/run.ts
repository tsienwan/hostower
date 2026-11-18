// Dictionnaire français：执行命令页（pages/Run.tsx）。
const dict: Record<string, string> = {
  "执行命令": "Exécuter une commande",
  "命令先由规则引擎判级，再决定是自动执行、等人批准，还是直接拒绝。不管从哪个入口提交，走的都是同一条链路。": "Le moteur de règles attribue d'abord un niveau de risque à la commande, puis décide si elle s'exécute automatiquement, attend une approbation ou est refusée d'emblée. Tous les points d'entrée passent par le même circuit.",
  "命令": "Commande",
  "例如 {example}": "p. ex. {example}",
  "试试：": "Essayez :",
  "意图（可选，会写进审计）": "Intention (facultative, consignée dans l'audit)",
  "为什么要做这件事": "Pourquoi faites-vous cela ?",
  // {keys} 的位置放的是 Ctrl + Enter 两个按键。
  "{keys} 提交": "{keys} pour soumettre",
  "执行已暂停": "Exécution suspendue",
  "提交": "Soumettre",
  "提交（将被拒绝）": "Soumettre (sera refusée)",
  "在 {n} 台主机上执行": "Exécuter sur {n} hôte(s)",
  "提交审批": "Soumettre pour approbation",
  "判级预览": "Aperçu du niveau de risque",
};

export default dict;
