// 한국어 사전：服务端文案（核心部分）。键是服务端发出的中文原文；带 {a} 的是模板。
// 其余的按来源拆在 server-*.ts 里。
const dict: Record<string, string> = {
  "命令行执行": "CLI에서 실행",
  "{a}（命令行）": "{a}(CLI)",
  "只读": "읽기 전용",
  // 审计和请求里的判级理由是 “等级 理由：命令” 的形式。
  "{level} {reason!}：{cmd!}": "{level} {reason!}: {cmd!}",
};

export default dict;
