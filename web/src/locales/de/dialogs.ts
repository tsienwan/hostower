// de 词典：项目自己的确认框、输入框，以及 Passkey 的说明。
const dict: Record<string, string> = {
  "请确认": "Bitte bestätigen",
  "请输入": "Eingabe",
  "确定": "OK",
  "下一步": "Weiter",
  "我的 Passkey": "Mein Passkey",
  "给这把 Passkey 起个名字，方便以后认出它（例如：工作电脑、YubiKey）": "Benennen Sie diesen Passkey, um ihn später wiederzuerkennen (z. B. Arbeitslaptop, YubiKey)",
  "用 IP 地址访问时不能添加 Passkey": "Bei Zugriff über IP-Adresse lassen sich keine Passkeys hinzufügen",
  "现在是用 IP 地址访问面板。浏览器规定 Passkey 只能绑定在域名上，用 IP 访问时添加不了，也用不了。给面板配一个域名（用 HTTPS 访问）后就能添加。": "Sie greifen per IP-Adresse auf das Panel zu. Browser erlauben Passkeys nur auf Domains, per IP lassen sie sich weder hinzufügen noch nutzen. Geben Sie dem Panel eine Domain (per HTTPS), um einen hinzuzufügen.",
  "请修改密码。": "Bitte ändern Sie Ihr Passwort.",
  "还在用首次启动生成的初始密码，请修改密码。": "Noch das beim ersten Start erzeugte Anfangspasswort in Gebrauch; bitte ändern.",
};

export default dict;
