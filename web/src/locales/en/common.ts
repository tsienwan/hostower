// 英文词典：键是中文原文。这个文件放跨页面通用的词；各页面自己的词放在同目录的其他文件里。
const dict: Record<string, string> = {
  "跟随系统": "System",
  "深色": "Dark",
  "浅色": "Light",
  "主题": "Theme",
  "语言": "Language",
  "时区": "Time zone",
  "无法连接塔台": "Cannot reach the tower",
  "请求失败（{status}）": "Request failed ({status})",
  "上传中断": "Upload interrupted",
  "上传失败（{status}）": "Upload failed ({status})",
  "这个浏览器不支持 Passkey，请换用较新的浏览器": "This browser does not support Passkeys. Please use a newer browser.",
  "Passkey 注册被取消": "Passkey registration was cancelled",
  "Passkey 验证被取消": "Passkey verification was cancelled",
  // 带语境的键：同一个中文词在这些位置需要不同的英文。
  "创建@@时间": "Created",
  "提交@@时间": "Submitted",
  "主机@@列表": "Hosts",
  "系统@@操作系统": "OS",
};

export default dict;
