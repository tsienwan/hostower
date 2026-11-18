// shared.tsx：目标选择、判级预览、批量策略。
const dict: Record<string, string> = {
  "目标主机": "Target hosts",
  "主机名、tag:web、group:prod，可用 and / or / not 组合": "Host name, tag:web, group:prod; combine with and / or / not",
  "匹配 {n} 台：": "{n} matched:",
  "高敏感主机": "High-sensitivity host",
  "等 {n} 台": "… {n} in total",
  "只读操作，提交后自动执行。": "Read-only operation. Runs automatically once submitted.",
  "变更操作，提交后需要一个人批准才会执行。": "Change operation. Needs one person's approval before it runs.",
  "高危操作，批准时要重新验证 Passkey，并手动输入确认内容。": "High-risk operation. Approving it requires re-verifying your Passkey and typing a confirmation.",
  "禁止的操作。提交会被内核直接拒绝并写入审计，任何人都无法放行。": "Forbidden operation. The kernel rejects it on submission and records it in the audit log; nobody can allow it.",
  "判级中…": "Assessing risk level…",
  "填好目标和命令后，这里会显示风险等级和判定理由。判级由规则引擎完成，预览没有任何副作用。": "Once the target and command are filled in, the risk level and the reasons for it appear here. The rule engine decides the risk level, and previewing has no side effects.",
  "判定理由": "Reasons",
  "每台主机上的实际命令": "Actual command on each host",
  "、": ", ",
  "批量执行方式": "Batch run strategy",
  "先在一台上预演": "Canary on one host first",
  "遇错停止": "Stop on error",
  "并发": "Concurrency",
  "目标里有高敏感主机：强制逐台滚动执行，高敏感主机排在最后。": "The targets include high-sensitivity hosts: runs are forced to roll one host at a time, with high-sensitivity hosts last.",
};

export default dict;
