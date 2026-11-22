// 服务端文案的法文词典：文件同步的校验错误和请求说明。键是服务端发出的中文原文；带 {a} 的是模板。
const dict: Record<string, string> = {
  "一次分发 1 到 {a} 个文件": "Distribuez de 1 à {a} fichiers à la fois",
  "文件名不合法或重复": "Nom de fichier invalide ou en double",
  "文件名规则只能用字母、数字、. _ - 和通配符 * ? [ ]，不能带路径": "Un motif de nom de fichier ne peut contenir que des lettres, des chiffres, . _ - et les caractères génériques * ? [ ], sans chemin",
  "目录层数要在 1 到 {a} 之间": "La profondeur de répertoires doit être comprise entre 1 et {a}",
  "没有匹配的文件": "Aucun fichier correspondant",
  "分发文件 {a}": "Distribuer le fichier {a}",
  "分发文件 {a}（{b} 台主机）": "Distribuer le fichier {a} ({b} hôte(s))",
};

export default dict;
