// 英文词典：执行命令页（pages/Run.tsx）。
const dict: Record<string, string> = {
  "执行命令": "Run command",
  "命令先由规则引擎判级，再决定是自动执行、等人批准，还是直接拒绝。不管从哪个入口提交，走的都是同一条链路。": "The rule engine first assigns the command a risk level, then decides whether it runs automatically, waits for approval, or is denied outright. Every entry point goes through the same pipeline.",
  "命令": "Command",
  "例如 {example}": "e.g. {example}",
  "试试：": "Try:",
  "意图（可选，会写进审计）": "Intent (optional, recorded in the audit)",
  "为什么要做这件事": "Why are you doing this?",
  // {keys} 的位置放的是 Ctrl + Enter 两个按键。
  "{keys} 提交": "{keys} to submit",
  "执行已暂停": "Execution paused",
  "提交": "Submit",
  "提交（将被拒绝）": "Submit (will be denied)",
  "在 {n} 台主机上执行": "Run on {n} host(s)",
  "提交审批": "Submit for approval",
  "判级预览": "Risk level preview",
};

export default dict;
