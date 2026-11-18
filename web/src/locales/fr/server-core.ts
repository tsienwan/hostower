// Dictionnaire français des messages du serveur (partie principale). 键是服务端发出的中文原文；带 {a} 的是模板。
// 其余的按来源拆在 server-*.ts 里。
const dict: Record<string, string> = {
  "命令行执行": "Exécution depuis la CLI",
  "{a}（命令行）": "{a} (CLI)",
  "只读": "Lecture seule",
  // 审计和请求里的判级理由是 “等级 理由：命令” 的形式。
  "{level} {reason!}：{cmd!}": "{level} {reason!} : {cmd!}",
};

export default dict;
