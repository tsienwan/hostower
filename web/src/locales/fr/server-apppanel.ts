// Dictionnaire français des textes serveur : réglage de l'adresse du panneau (internal/apps/panel.go, gateway/apppanel.go) et note sur le panneau de 3x-ui.
const dict: Record<string, string> = {
  "面板地址的访问方式不认识": "Mode d'accès au panneau inconnu",
  "面板端口不合法（1–65535）": "Port du panneau invalide (1–65535)",
  "面板路径只能用字母、数字和 . _ ~ / -，以 / 开头": "Le chemin du panneau ne peut contenir que des lettres, des chiffres et . _ ~ / -, et doit commencer par /",
  "本地端口不合法（1–65535）": "Port local invalide (1–65535)",
  "请填写面板地址": "Saisissez l'adresse du panneau",
  "面板地址不合法": "Adresse du panneau invalide",
  "面板地址要以 http:// 或 https:// 开头": "L'adresse du panneau doit commencer par http:// ou https://",
  "面板地址里不能带账号或密码": "L'adresse du panneau ne peut contenir ni identifiant ni mot de passe",
  "面板地址里不能带查询参数（? 和 # 后面的部分），以免把令牌放进链接": "L'adresse du panneau ne peut pas contenir de paramètres (ce qui suit ? ou #), pour qu'aucun jeton ne se retrouve dans le lien",
  "面板地址里的主机名不合法": "Nom d'hôte invalide dans l'adresse du panneau",
  "面板地址里的端口不合法": "Port invalide dans l'adresse du panneau",
  "面板地址里的路径只能用字母、数字和 . _ ~ / -": "Le chemin de l'adresse du panneau ne peut contenir que des lettres, des chiffres et . _ ~ / -",
  "这台主机上没有这个实例，或者它没有网页面板": "Cette instance n'est pas sur cet hôte, ou elle n'a pas de panneau web",
  "设置应用的面板地址": "Définir l'adresse du panneau de l'application",
  "恢复应用面板的默认地址": "Rétablir l'adresse par défaut du panneau de l'application",
  "面板的端口和路径在 3x-ui 自己的“面板设置”里改，Hostower 看不到；改过以后在“面板地址”里填上新的端口和路径。": "Le port et le chemin du panneau se modifient dans les « Réglages du panneau » de 3x-ui, que Hostower ne voit pas ; après les avoir changés, saisissez le nouveau port et le nouveau chemin dans « Adresse du panneau ».",
};

export default dict;
