// 服务端文案的德文词典：月流量配额的校验错误和审计说明。
const dict: Record<string, string> = {
  "单位只能是 GB 或 TB": "Einheit muss GB oder TB sein",
  "换算只能按 1000 或 1024": "Umrechnung muss 1000 oder 1024 sein",
  "额度要大于 0": "Das Kontingent muss größer als 0 sein",
  "不认识的计费方式": "Unbekannte Abrechnungsart",
  "重置日要在 1 到 31 之间": "Der Rücksetztag muss zwischen 1 und 31 liegen",
  "网卡名不合法": "Ungültiger Schnittstellenname",
  "告警百分比要在 1 到 200 之间": "Warnschwellen müssen zwischen 1 und 200 liegen",
  "告警百分比最多 5 个": "Höchstens 5 Warnschwellen",
  "已用量不合法": "Ungültiger Verbrauch",
  "设置流量配额": "Traffic-Kontingent festlegen",
  "取消流量配额": "Traffic-Kontingent entfernen",
};

export default dict;
