// en 词典：设置页“关于”里的构建封签、许可证和商标说明。
const dict: Record<string, string> = {
  "构建": "Build",
  "发布公钥指纹": "Release key fingerprint",
  "和官方公布的指纹一致，才是官方构建。也可以在主机上运行 hostower verify <官方公钥> 离线校验。": "It is an official build only if this matches the officially published fingerprint. You can also verify offline on the host with hostower verify <official public key>.",
  "许可证": "License",
  "源代码": "Source code",
  "Hostower 名称和图标是项目的商标（™），改版后再发布请换名字和图标，见仓库里的 TRADEMARKS.md。": "The Hostower name and logo are the project's trademarks (™). If you redistribute a modified version, use a different name and logo; see TRADEMARKS.md in the repository.",
  "发布签名校验通过": "Release signature verified",
  "封签对不上：文件被改过，或者不是用这把公钥签的": "Seal mismatch: the file was modified, or not signed with this key",
  "自行构建（没有发布签名）": "Self-built (no release signature)",
};

export default dict;
