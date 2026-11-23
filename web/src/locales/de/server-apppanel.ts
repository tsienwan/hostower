// Deutsches Wörterbuch für Servertexte: Einstellung der Panel-Adresse (internal/apps/panel.go, gateway/apppanel.go) und Hinweis zum 3x-ui-Panel.
const dict: Record<string, string> = {
  "面板地址的访问方式不认识": "Unbekannter Zugriffsweg für das Panel",
  "面板端口不合法（1–65535）": "Ungültiger Panel-Port (1–65535)",
  "面板路径只能用字母、数字和 . _ ~ / -，以 / 开头": "Der Panel-Pfad darf nur Buchstaben, Ziffern und . _ ~ / - enthalten und muss mit / beginnen",
  "本地端口不合法（1–65535）": "Ungültiger lokaler Port (1–65535)",
  "请填写面板地址": "Bitte die Panel-Adresse eintragen",
  "面板地址不合法": "Ungültige Panel-Adresse",
  "面板地址要以 http:// 或 https:// 开头": "Die Panel-Adresse muss mit http:// oder https:// beginnen",
  "面板地址里不能带账号或密码": "Die Panel-Adresse darf keinen Benutzernamen und kein Passwort enthalten",
  "面板地址里不能带查询参数（? 和 # 后面的部分），以免把令牌放进链接": "Die Panel-Adresse darf keine Abfrageparameter (alles nach ? oder #) enthalten, damit kein Token im Link landet",
  "面板地址里的主机名不合法": "Ungültiger Hostname in der Panel-Adresse",
  "面板地址里的端口不合法": "Ungültiger Port in der Panel-Adresse",
  "面板地址里的路径只能用字母、数字和 . _ ~ / -": "Der Pfad in der Panel-Adresse darf nur Buchstaben, Ziffern und . _ ~ / - enthalten",
  "这台主机上没有这个实例，或者它没有网页面板": "Diese Instanz gibt es auf diesem Host nicht, oder sie hat kein Web-Panel",
  "设置应用的面板地址": "Panel-Adresse der App festlegen",
  "恢复应用面板的默认地址": "Standardadresse des App-Panels wiederherstellen",
  "面板的端口和路径在 3x-ui 自己的“面板设置”里改，Hostower 看不到；改过以后在“面板地址”里填上新的端口和路径。": "Port und Pfad des Panels werden in den eigenen „Panel-Einstellungen“ von 3x-ui geändert, die Hostower nicht sieht; tragen Sie danach den neuen Port und Pfad unter „Panel-Adresse“ ein.",
};

export default dict;
