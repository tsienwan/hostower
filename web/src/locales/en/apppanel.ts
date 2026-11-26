// 英文词典：应用的“打开面板”按钮和面板地址设置（pages/AppPanel.tsx）。
const dict: Record<string, string> = {
  "{app} 的面板地址": "Panel address of {app}",
  "恢复默认": "Restore default",
  "默认按这个实例当前的监听地址和端口拼出地址，在“修改设置”里改过以后会自动跟着变。下面的设置只存在塔台里，不改主机上的任何东西；改动会写进审计。": "By default the address is built from this instance's current listen address and port, and follows along when you change them under \"Edit settings\". The settings below are stored only in Hostower and change nothing on the host; every change is written to the audit log.",
  "怎么访问": "How to reach it",
  "直接访问主机": "Directly on the host",
  "填写完整地址": "Full address",
  "经由 SSH 隧道": "Through an SSH tunnel",
  "打开 主机地址:端口/路径。端口或路径在应用自己的页面里改过时，在下面填上新的；留空就照应用的设置。": "Opens host address:port/path. If the port or path was changed inside the app itself, enter the new one below; leave it empty to follow the app's settings.",
  "应用前面套了反向代理、用域名访问，或者只绑在 127.0.0.1 上时，直接填浏览器里要打开的地址。不能带账号、密码或 ? 后面的参数。": "If the app sits behind a reverse proxy, is reached by a domain, or listens only on 127.0.0.1, enter the address to open in the browser. It can't contain a user name, password or anything after ?.",
  "不对外开放端口：在自己电脑上运行下面这条命令建立隧道（用的是这台主机的 SSH 登录方式），按钮打开本机的端口。": "No port is opened to the outside: run the command below on your own computer to set up a tunnel (it uses this host's SSH login), and the button opens the port on your computer.",
  "面板地址": "Panel address",
  "网络页里反向代理到这个端口的站点：": "Sites on the Network page that proxy to this port:",
  "应用端口": "App port",
  "本地端口": "Local port",
  "先运行": "Run first",
  "复制命令": "Copy command",
  "按钮打开": "Button opens",
  "这个应用只在主机本机（127.0.0.1）监听，从外面打不开：填写完整地址或者经由 SSH 隧道。": "This app listens only on the host itself (127.0.0.1) and can't be opened from outside: enter a full address or use an SSH tunnel.",
  "这台主机上有站点反向代理到这个应用的端口：没有另外设置时，按钮自动打开站点的地址。想照主机地址打开，就在上面填上端口。":
    "A site on this host reverse-proxies to this app's port: with nothing else set, the button opens the site's address automatically. To open the host address instead, fill in the port above.",
  "只有所有者可以修改面板地址。": "Only the owner can change the panel address.",
  "这个应用只在主机本机（127.0.0.1）监听：先设置面板地址（反向代理的域名或 SSH 隧道）": "This app listens only on the host itself (127.0.0.1): set a panel address first (a reverse-proxy domain or an SSH tunnel)",
  "打开面板": "Open panel",
  "应用已停止": "The app is stopped",
  "经由 SSH 隧道：先在自己电脑上运行 {cmd}": "Through an SSH tunnel: first run {cmd} on your own computer",
  "面板地址：反向代理、SSH 隧道、改过的端口和路径": "Panel address: reverse proxy, SSH tunnel, changed port and path",
  "应用面板地址": "App panel address",
};

export default dict;
