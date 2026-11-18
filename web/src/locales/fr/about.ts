// fr 词典：设置页“关于”里的构建封签、许可证和商标说明。
const dict: Record<string, string> = {
  "构建": "Build",
  "发布公钥指纹": "Empreinte de la clé de publication",
  "和官方公布的指纹一致，才是官方构建。也可以在主机上运行 hostower verify <官方公钥> 离线校验。": "C’est une build officielle seulement si l’empreinte correspond à celle publiée officiellement. Vous pouvez aussi vérifier hors ligne sur l’hôte avec hostower verify <clé publique officielle>.",
  "许可证": "Licence",
  "源代码": "Code source",
  "Hostower 名称和图标是项目的商标（™），改版后再发布请换名字和图标，见仓库里的 TRADEMARKS.md。": "Le nom et le logo Hostower sont des marques du projet (™). Pour redistribuer une version modifiée, changez de nom et de logo ; voir TRADEMARKS.md dans le dépôt.",
  "发布签名校验通过": "Signature de publication vérifiée",
  "封签对不上：文件被改过，或者不是用这把公钥签的": "Sceau invalide : le fichier a été modifié ou n’est pas signé avec cette clé",
  "自行构建（没有发布签名）": "Build personnelle (sans signature de publication)",
};

export default dict;
