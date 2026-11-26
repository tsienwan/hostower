// en 词典：Tailscale 卡片，以及站点转发到另一台主机的提示。
const dict: Record<string, string> = {
  "在 Tailscale 网络里的名字":
    "Name on the Tailscale network",
  "留空就用系统的主机名。只能用字母、数字和连字符。":
    "Leave empty to use the system host name. Letters, digits and hyphens only.",
  "名字只能用字母、数字和连字符，不能以连字符开头或结尾。":
    "The name may only contain letters, digits and hyphens, and cannot start or end with a hyphen.",
  "使用 Tailscale 网络的 DNS（可以用名字访问别的主机；会接管这台主机的 DNS 设置）":
    "Use the Tailscale network's DNS (reach other hosts by name; takes over this host's DNS settings)",
  "接受别的节点通告的子网路由":
    "Accept subnet routes advertised by other nodes",
  "开启 Tailscale SSH":
    "Enable Tailscale SSH",
  "开启后，Tailscale 网络里被访问规则允许的人可以直接登录这台主机，不经过 sshd 的密钥和这里的审批。只在你清楚那个网络的访问规则时开启。":
    "When enabled, people allowed by the Tailscale network's access rules can log in to this host directly, bypassing sshd keys and approval here. Enable it only if you know that network's access rules.",
  "让 {host} 重新连上 Tailscale":
    "Reconnect {host} to Tailscale",
  "把 {host} 加入 Tailscale 网络":
    "Join {host} to the Tailscale network",
  "怎么登录":
    "How to sign in",
  "在浏览器里登录":
    "Sign in in a browser",
  "不用密钥":
    "No key",
  "授权密钥":
    "Auth key",
  "用这条应用密钥里存的授权密钥登录。密钥在执行时从标准输入送给 tailscale，不会出现在命令、请求和审计里。":
    "Signs in with the auth key stored in this app secret. The key is passed to tailscale on standard input at run time and never appears in commands, requests or the audit log.",
  "提交并批准后，请求的输出里会出现一个登录链接：在浏览器里打开它并登录 Tailscale 账号，请求会一直等到登录完成（最多 9 分钟）。":
    "After you submit and approve, a sign-in link appears in the request output: open it in a browser and sign in to your Tailscale account. The request waits until sign-in completes (up to 9 minutes).",
  "想不用人登录，先在 Tailscale 后台生成授权密钥，到 ":
    "To sign in without a person, generate an auth key in the Tailscale admin console, save it as an app secret on the ",
  " 页存成应用密钥，再回来选。":
    " page, then come back and select it.",
  "想不用人登录，请所有者把授权密钥存成应用密钥。":
    "To sign in without a person, ask the owner to save an auth key as an app secret.",
  "加入以后，这台主机会多一个内网地址，同一个 Tailscale 网络里的设备按那边的访问规则可以连到它。这是高危操作，批准时要重新验证身份。":
    "After joining, this host gets an extra private address, and devices on the same Tailscale network can connect to it according to that network's access rules. This is high-risk and approval requires re-verifying your identity.",
  "{host} 上的 Tailscale 设置":
    "Tailscale settings on {host}",
  "读不出这台主机当前的 Tailscale 设置，下面显示的是默认值：提交会按这里的选择覆盖原来的设置。":
    "This host's current Tailscale settings could not be read; defaults are shown below. Submitting overwrites the existing settings with the choices here.",
  "Hostower 就是经 Tailscale 连到这台主机的，断开会把它自己断在外面":
    "Hostower reaches this host through Tailscale; disconnecting would lock it out",
  "Tailscale（主机之间的加密内网）":
    "Tailscale (encrypted private network between hosts)",
  "已连接":
    "Connected",
  "已断开":
    "Disconnected",
  "还没有登录":
    "Not signed in",
  "等待管理员批准":
    "Waiting for admin approval",
  "正在加入，等待登录…":
    "Joining, waiting for sign-in…",
  "断开":
    "Disconnect",
  "重新连上":
    "Reconnect",
  "加入网络":
    "Join network",
  "把几台主机连进同一个加密的内网：每台有一个 100.x 的地址，主机之间互相访问不用把端口对公网开放。安装、加入、改设置、断开都是高危操作，批准时要重新验证身份。":
    "Connects several hosts into one encrypted private network: each gets a 100.x address, and hosts reach each other without exposing ports to the public internet. Installing, joining, changing settings and disconnecting are high-risk operations; approval requires re-verifying your identity.",
  "这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装。":
    "One-click Tailscale installation is not supported on this system; follow the instructions on the Tailscale website.",
  "内网地址":
    "Private address",
  "名字":
    "Name",
  "所在网络":
    "Network",
  "别的主机加入同一个网络后，可以用上面的地址访问这台主机。在另一台主机上添加站点时，“转发到哪里”填这个地址，两台之间这一段就是加密的。":
    "Once other hosts join the same network, they can reach this host at the address above. When adding a site on another host, enter this address under “Forward to” and the hop between the two is encrypted.",
  "已经装好，还没有登录：点“加入网络”。":
    "Installed but not signed in: click “Join network”.",
  "这台主机已经登录，但那个 Tailscale 网络要求管理员批准新设备：到 Tailscale 后台批准它，再回来刷新。":
    "This host is signed in, but that Tailscale network requires an admin to approve new devices: approve it in the Tailscale admin console, then come back and refresh.",
  "在 {host} 上安装 Tailscale":
    "Install Tailscale on {host}",
  "添加 Tailscale 的官方软件源（pkgs.tailscale.com）和它的签名密钥，用系统的包管理器安装，并启动 tailscaled 服务。装完还没有登录，要再“加入网络”。":
    "Adds Tailscale's official package repository (pkgs.tailscale.com) and its signing key, installs with the system package manager and starts the tailscaled service. It is not signed in after installation; use “Join network” next.",
  "这台主机要能连上 pkgs.tailscale.com。Tailscale 决定主机能被谁连上，安装是高危操作，批准时要重新验证身份。":
    "This host must be able to reach pkgs.tailscale.com. Tailscale decides who can connect to the host, so installing is high-risk and approval requires re-verifying your identity.",
  "断开 {host} 的 Tailscale":
    "Disconnect {host} from Tailscale",
  "断开后这台主机的内网地址不再可用：经这个地址访问它的站点和连接都会断。登录状态还在，随时可以重新连上；要彻底离开那个网络，断开后再“退出登录”。":
    "After disconnecting, this host's private address stops working: sites and connections that reach it through that address will break. It stays signed in and can reconnect at any time; to leave that network completely, disconnect and then “Sign out”.",
  "让 {host} 退出 Tailscale 登录":
    "Sign {host} out of Tailscale",
  "退出登录后，这台主机不再属于那个 Tailscale 网络，再加入要重新登录或用新的授权密钥。":
    "After signing out, this host no longer belongs to that Tailscale network; joining again requires signing in again or a new auth key.",
  "{addr} 是 Tailscale 的内网地址，而这台主机还没有连上 Tailscale 网络，转发不过去：先在这一页的 Tailscale 卡片里加入网络。":
    "{addr} is a Tailscale private address, but this host is not connected to a Tailscale network yet, so traffic cannot be forwarded: join the network in the Tailscale card on this page first.",
  "流量会先到这台主机，再经 Tailscale 转到 {addr}：上传和下载都算这台主机的流量。两台主机之间这一段是加密的，那台主机的端口不用对公网开放；它上面的服务不能只监听 127.0.0.1。":
    "Traffic reaches this host first and is then forwarded over Tailscale to {addr}: uploads and downloads both count against this host’s traffic. The hop between the two hosts is encrypted and that host's port need not be exposed to the public internet; the service on it must not listen only on 127.0.0.1.",
};

export default dict;
