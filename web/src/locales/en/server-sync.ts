// 服务端文案的英文词典：文件同步的校验错误和请求说明。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "一次分发 1 到 {a} 个文件": "Distribute 1 to {a} files at a time",
  "文件名不合法或重复": "File name is invalid or duplicated",
  "文件名规则只能用字母、数字、. _ - 和通配符 * ? [ ]，不能带路径": "A file-name pattern may only contain letters, digits, . _ - and the wildcards * ? [ ], and no path",
  "目录层数要在 1 到 {a} 之间": "Directory depth must be between 1 and {a}",
  "没有匹配的文件": "No matching files",
  "分发文件 {a}": "Distribute file {a}",
  "分发文件 {a}（{b} 台主机）": "Distribute file {a} ({b} host(s))",
};

export default dict;
