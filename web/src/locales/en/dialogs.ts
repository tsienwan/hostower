// en 词典：项目自己的确认框、输入框，以及 Passkey 的说明。
const dict: Record<string, string> = {
  "请确认": "Please confirm",
  "请输入": "Enter a value",
  "确定": "OK",
  "下一步": "Next",
  "我的 Passkey": "My Passkey",
  "给这把 Passkey 起个名字，方便以后认出它（例如：工作电脑、YubiKey）": "Name this Passkey so you can recognize it later (e.g. Work laptop, YubiKey)",
  "用 IP 地址访问时不能添加 Passkey": "Passkeys cannot be added when accessing by IP address",
  "现在是用 IP 地址访问面板。浏览器规定 Passkey 只能绑定在域名上，用 IP 访问时添加不了，也用不了。给面板配一个域名（用 HTTPS 访问）后就能添加。": "You are accessing the panel by IP address. Browsers only allow Passkeys on domain names, so they cannot be added or used over IP. Give the panel a domain (accessed over HTTPS) to add one.",
  "请修改密码。": "Please change your password.",
  "还在用首次启动生成的初始密码，请修改密码。": "Still using the initial password generated on first start; please change it.",
};

export default dict;
