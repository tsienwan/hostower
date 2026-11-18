// 繁體中文詞典：服務端文案（核心部分）。鍵是服務端發出的簡體中文原文；帶 {a} 的是範本。
// 其餘的依來源拆在 server-*.ts 中。
const dict: Record<string, string> = {
  "命令行执行": "命令列執行",
  "{a}（命令行）": "{a}（命令列）",
  "只读": "唯讀",
  // 稽核和請求中的判級理由是「等級 理由：命令」的形式。
  "{level} {reason!}：{cmd!}": "{level} {reason!}：{cmd!}",
};

export default dict;
