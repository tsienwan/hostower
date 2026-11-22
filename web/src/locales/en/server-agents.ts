// 服务端文案的英文词典：Agent 任务的校验错误、请求说明和判级理由。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "不认识的 AI 编程助手": "Unknown AI coding assistant",
  "任务不能为空，最长 {a} 个字": "The task can't be empty and is limited to {a} characters",
  "这个 AI 编程助手不能用来汇总（要能从标准输入读材料）": "This AI coding assistant can't be used for summaries (it must be able to read input from standard input)",
  "只能汇总 Agent 任务的结果": "Only results of Agent tasks can be summarized",
  "这个任务还没有执行完": "This task hasn't finished running yet",
  "Agent 任务（{a}）：{b}": "Agent task ({a}): {b}",
  "Agent 任务（{a}）：{b}（{c} 台主机）": "Agent task ({a}): {b} ({c} hosts)",
  "汇总 {a} 的结果（{b}）": "Summarize results of {a} ({b})",
  "运行 AI 编程助手：它会按自己的权限设置读写文件、访问网络": "Runs an AI coding assistant: it reads and writes files and accesses the network according to its own permission settings",
  "让 AI 编程助手跳过权限确认或沙箱，等于让它在主机上随意执行命令": "Making an AI coding assistant skip permission prompts or the sandbox amounts to letting it run any command on the host",
  "AI 编程助手的参数看不全，无法确认有没有跳过权限确认": "The AI coding assistant's arguments can't be fully seen, so it can't be confirmed whether permission prompts are skipped",
  "写审计失败，没有暂存": "Could not write the audit log; the file was not staged",
  "写审计失败，没有取消：{a}": "Could not write the audit log; not cancelled: {a}",
};

export default dict;
