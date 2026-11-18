// de 词典：设置页“关于”里的构建封签、许可证和商标说明。
const dict: Record<string, string> = {
  "构建": "Build",
  "发布公钥指纹": "Fingerabdruck des Release-Schlüssels",
  "和官方公布的指纹一致，才是官方构建。也可以在主机上运行 hostower verify <官方公钥> 离线校验。": "Nur wenn er mit dem offiziell veröffentlichten Fingerabdruck übereinstimmt, ist es ein offizieller Build. Offline prüfen geht auf dem Host mit hostower verify <offizieller öffentlicher Schlüssel>.",
  "许可证": "Lizenz",
  "源代码": "Quellcode",
  "Hostower 名称和图标是项目的商标（™），改版后再发布请换名字和图标，见仓库里的 TRADEMARKS.md。": "Name und Logo von Hostower sind Marken des Projekts (™). Wer eine geänderte Version weitergibt, verwendet bitte einen anderen Namen und ein anderes Logo; siehe TRADEMARKS.md im Repository.",
  "发布签名校验通过": "Release-Signatur geprüft",
  "封签对不上：文件被改过，或者不是用这把公钥签的": "Siegel passt nicht: Datei wurde verändert oder nicht mit diesem Schlüssel signiert",
  "自行构建（没有发布签名）": "Eigener Build (ohne Release-Signatur)",
};

export default dict;
