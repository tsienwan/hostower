// fr 词典：项目自己的确认框、输入框，以及 Passkey 的说明。
const dict: Record<string, string> = {
  "请确认": "Confirmation",
  "请输入": "Saisie",
  "确定": "OK",
  "下一步": "Suivant",
  "我的 Passkey": "Ma Passkey",
  "给这把 Passkey 起个名字，方便以后认出它（例如：工作电脑、YubiKey）": "Nommez cette Passkey pour la reconnaître plus tard (p. ex. Ordinateur pro, YubiKey)",
  "用 IP 地址访问时不能添加 Passkey": "Impossible d’ajouter une Passkey en accédant par adresse IP",
  "现在是用 IP 地址访问面板。浏览器规定 Passkey 只能绑定在域名上，用 IP 访问时添加不了，也用不了。给面板配一个域名（用 HTTPS 访问）后就能添加。": "Vous accédez au panneau par adresse IP. Les navigateurs n’autorisent les Passkeys que sur des noms de domaine : impossible d’en ajouter ou d’en utiliser par IP. Donnez un domaine au panneau (en HTTPS) pour en ajouter une.",
  "请修改密码。": "Veuillez changer votre mot de passe.",
  "还在用首次启动生成的初始密码，请修改密码。": "Le mot de passe initial généré au premier démarrage est toujours utilisé ; veuillez le changer.",
};

export default dict;
