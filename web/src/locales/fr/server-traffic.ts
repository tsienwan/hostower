// 服务端文案的法文词典：月流量配额的校验错误和审计说明。
const dict: Record<string, string> = {
  "单位只能是 GB 或 TB": "L'unité doit être GB ou TB",
  "换算只能按 1000 或 1024": "La conversion doit être 1000 ou 1024",
  "额度要大于 0": "Le quota doit être supérieur à 0",
  "不认识的计费方式": "Mode de facturation inconnu",
  "重置日要在 1 到 31 之间": "Le jour de remise à zéro doit être entre 1 et 31",
  "网卡名不合法": "Nom d'interface invalide",
  "告警百分比要在 1 到 200 之间": "Les seuils d'alerte doivent être entre 1 et 200",
  "告警百分比最多 5 个": "5 seuils d'alerte au maximum",
  "已用量不合法": "Consommation invalide",
  "设置流量配额": "Définir un quota de trafic",
  "取消流量配额": "Supprimer un quota de trafic",
};

export default dict;
