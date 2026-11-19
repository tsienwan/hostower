// 服务端文案的英文词典：月流量配额的校验错误和审计说明。
const dict: Record<string, string> = {
  "单位只能是 GB 或 TB": "Unit must be GB or TB",
  "换算只能按 1000 或 1024": "Units must be 1000 or 1024",
  "额度要大于 0": "Quota must be greater than 0",
  "不认识的计费方式": "Unknown billing mode",
  "重置日要在 1 到 31 之间": "Reset day must be between 1 and 31",
  "网卡名不合法": "Invalid interface name",
  "告警百分比要在 1 到 200 之间": "Alert percentages must be between 1 and 200",
  "告警百分比最多 5 个": "At most 5 alert percentages",
  "已用量不合法": "Invalid usage",
  "设置流量配额": "Set traffic quota",
  "取消流量配额": "Remove traffic quota",
};

export default dict;
