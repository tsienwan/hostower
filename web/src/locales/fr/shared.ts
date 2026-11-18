// Dictionnaire français：shared.tsx（目标选择、判级预览、批量策略）。
const dict: Record<string, string> = {
  "目标主机": "Hôtes cibles",
  "主机名、tag:web、group:prod，可用 and / or / not 组合": "Nom d'hôte, tag:web, group:prod ; combinables avec and / or / not",
  "匹配 {n} 台：": "{n} correspondant(s) :",
  "高敏感主机": "Hôte à sensibilité élevée",
  "等 {n} 台": "… {n} au total",
  "只读操作，提交后自动执行。": "Opération en lecture seule. S'exécute automatiquement une fois soumise.",
  "变更操作，提交后需要一个人批准才会执行。": "Opération de modification. Nécessite l'approbation d'une personne avant de s'exécuter.",
  "高危操作，批准时要重新验证 Passkey，并手动输入确认内容。": "Opération à risque élevé. Son approbation exige de revérifier votre Passkey et de saisir une confirmation.",
  "禁止的操作。提交会被内核直接拒绝并写入审计，任何人都无法放行。": "Opération interdite. Le noyau la rejette dès la soumission et la consigne dans l'audit ; personne ne peut l'autoriser.",
  "判级中…": "Évaluation du niveau de risque…",
  "填好目标和命令后，这里会显示风险等级和判定理由。判级由规则引擎完成，预览没有任何副作用。": "Une fois la cible et la commande renseignées, le niveau de risque et ses raisons s'affichent ici. Le moteur de règles détermine le niveau de risque, et l'aperçu n'a aucun effet de bord.",
  "判定理由": "Raisons",
  "每台主机上的实际命令": "Commande réelle sur chaque hôte",
  "、": ", ",
  "批量执行方式": "Stratégie d'exécution par lot",
  "先在一台上预演": "Tester d'abord sur un hôte",
  "遇错停止": "Arrêter en cas d'erreur",
  "并发": "Parallélisme",
  "目标里有高敏感主机：强制逐台滚动执行，高敏感主机排在最后。": "Les cibles comprennent des hôtes à sensibilité élevée : l'exécution se fait obligatoirement hôte par hôte, les hôtes à sensibilité élevée en dernier.",
};

export default dict;
