// 服务端文案的英文词典：应用的面板地址设置（internal/apps/panel.go、gateway/apppanel.go）和 3x-ui 的面板说明。
const dict: Record<string, string> = {
  "面板地址的访问方式不认识": "Unknown way of reaching the panel",
  "面板端口不合法（1–65535）": "Invalid panel port (1–65535)",
  "面板路径只能用字母、数字和 . _ ~ / -，以 / 开头": "The panel path may only use letters, digits and . _ ~ / -, and must start with /",
  "本地端口不合法（1–65535）": "Invalid local port (1–65535)",
  "请填写面板地址": "Enter the panel address",
  "面板地址不合法": "Invalid panel address",
  "面板地址要以 http:// 或 https:// 开头": "The panel address must start with http:// or https://",
  "面板地址里不能带账号或密码": "The panel address can't contain a user name or password",
  "面板地址里不能带查询参数（? 和 # 后面的部分），以免把令牌放进链接": "The panel address can't contain query parameters (anything after ? or #), so that no token ends up in the link",
  "面板地址里的主机名不合法": "Invalid host name in the panel address",
  "面板地址里的端口不合法": "Invalid port in the panel address",
  "面板地址里的路径只能用字母、数字和 . _ ~ / -": "The path in the panel address may only use letters, digits and . _ ~ / -",
  "这台主机上没有这个实例，或者它没有网页面板": "This instance isn't on this host, or it has no web panel",
  "设置应用的面板地址": "Set the app's panel address",
  "恢复应用面板的默认地址": "Restore the app panel's default address",
  "面板的端口和路径在 3x-ui 自己的“面板设置”里改，Hostower 看不到；改过以后在“面板地址”里填上新的端口和路径。": "The panel's port and path are changed in 3x-ui's own \"Panel Settings\", which Hostower can't see; after changing them, enter the new port and path under \"Panel address\".",
};

export default dict;
