// en 词典：“这台设备”测速（浏览器和面板之间的速度、回程和去程路由）。
const dict: Record<string, string> = {
  "这台设备 ↔ 主机":
    "This device ↔ host",
  "测这台设备":
    "Test this device",
  "把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。":
    "Treats the device viewing the panel (say, a phone online at home, at work or on the go) as a node: measures latency, download and upload between it and any host, then traces the route from that host back to it to show which line the return route takes and which hops it passes.",
  "测哪台主机":
    "Host to test",
  "没有在线的主机":
    "No hosts online",
  "速度测到哪里":
    "Speed test target",
  "这台主机（直连）":
    "This host (direct)",
  "在 {host} 上临时开一个测速服务，这台设备的浏览器直接连它量延迟、下载、上传，再从 {host} 追踪回到这台设备。服务只认带一次性口令的地址，最多等 {w} 秒，测完自己退出。开端口是一次变更（L1），需要批准。":
    "Starts a temporary speed test service on {host}. This device's browser connects to it directly to measure latency, download and upload, then the route is traced from {host} back to this device. The service only accepts the address carrying a one-time token, waits at most {w} seconds, and exits by itself when the test is done. Opening a port is a change (L1) and needs approval.",
  "延迟和速度量的是这台设备和面板之间；{host} 只用来追踪回到这台设备的路由。":
    "Latency and speed are measured between this device and the panel; {host} is only used to trace the route back to this device.",
  "主机的地址和端口":
    "Host address and port",
  "主机的地址":
    "Host address",
  "填这台设备能直接连到的地址（公网 IP 或域名）。主机的防火墙和云厂商的安全组要放行这个 TCP 端口，端口在 1024 到 65535 之间。":
    "Enter an address this device can reach directly (public IP or domain name). The host's firewall and the cloud provider's security group must allow this TCP port, which must be between 1024 and 65535.",
  "测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。":
    "The speed test page is served directly by the host over HTTP: the browser may warn that it is \"not secure\"; just choose to continue. The page carries only random data for the test and nothing about your panel sign-in.",
  "这台设备的地址":
    "This device's address",
  "这是面板看到的来源地址，回程路由就追踪到它。用了代理或 VPN 访问面板时，它是代理的出口地址。":
    "This is the source address the panel sees; the return route is traced to it. If you reach the panel through a proxy or VPN, it is the proxy's egress address.",
  "面板前面有反向代理：延迟和速度量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "The panel is behind a reverse proxy: latency and speed cover this device to the proxy (or CDN), plus the proxy to the panel.",
  "读不出可以追踪的来源地址（比如从本机回环地址访问），测不了。":
    "No traceable source address could be read (for example, when visiting from a loopback address), so the test can't run.",
  "数据量上限":
    "Data limit",
  "下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，最多约 {total} MB。":
    "Download up to {d} MB and upload up to {u} MB, each for at most {s} seconds, whichever comes first. This uses traffic on the host and on this device, up to about {total} MB.",
  "正在检查主机上有没有 traceroute…":
    "Checking whether the host has traceroute…",
  "{host} 上没有 traceroute 或 tracepath，追踪不了路由。":
    "{host} has neither traceroute nor tracepath, so the route can't be traced.",
  "先用系统的包管理器安装 traceroute":
    "Install traceroute with the system package manager first",
  "安装是一次变更（L1），需要批准；不装的话只有速度，没有回程路由。":
    "Installing is a change (L1) and needs approval; without it you get speed only, no return route.",
  "这台设备 ↔ {host}":
    "This device ↔ {host}",
  "正在测，请不要关闭这个页面…":
    "Testing; please keep this page open…",
  "测完了，可以关掉这个页面回到面板。":
    "Test finished. You can close this page and return to the panel.",
  "结果没有交回去，请回到面板重试。":
    "The result wasn't delivered. Return to the panel and try again.",
  "查看报告":
    "View report",
  "批准后才会在主机上开测速服务":
    "The speed test service starts on the host only after approval",
  "这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。":
    "This temporarily opens a port ({port}) on {host}, waits for this device to connect and run the test, then exits by itself.",
  "正在 {host} 上启动测速服务…":
    "Starting the speed test service on {host}…",
  "测速服务已经在 {host} 上等着了。在这台设备上打开测速页，它会自己开始测。":
    "The speed test service is waiting on {host}. Open the speed test page on this device and it will start by itself.",
  "打开测速页":
    "Open speed test page",
  "打不开或一直转圈：主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者地址 {addr} 这台设备连不上。服务最多等 {w} 秒。":
    "If it won't open or keeps loading: the host's firewall or the cloud provider's security group doesn't allow TCP port {port}, or this device can't reach {addr}. The service waits at most {w} seconds.",
  "这台设备已经连上，正在另一个页面里测…":
    "This device is connected; the test is running in another page…",
  "正在从 {host} 追踪到这台设备，最多要半分钟…":
    "Tracing from {host} to this device; this can take up to half a minute…",
  "测完了，在报告里看速度、线路和每一跳。":
    "Test finished; see speed, line and each hop in the report.",
  "没有测成，原因在报告或请求详情里。":
    "The test didn't complete; see the report or the request details for the reason.",
  "正在测…":
    "Testing…",
  "失败 {n} 次":
    "{n} failed",
  "没有测出来":
    "No result",
  "速度已经记下。":
    "Speed has been recorded.",
  "批准后才会追踪回程路由":
    "The return route is traced only after approval",
  "这次要先安装 traceroute，是一次变更；速度不受影响，已经在测。":
    "This run has to install traceroute first, which is a change; speed is not affected and is already being measured.",
  "回程路由追踪完了，在报告里看线路和每一跳。":
    "The return route has been traced; see the line and each hop in the report.",
  "回程路由没有追踪成功，报告里只有速度。":
    "The return route trace failed; the report has speed only.",
  "这台设备 ↔ 面板":
    "This device ↔ panel",
  "这次没有收到浏览器量的延迟和速度（测到一半关掉了页面），只有回程路由。":
    "The browser's latency and speed measurements weren't received this time (the page was closed partway), so there is only the return route.",
  "失败的请求":
    "Failed requests",
  "共 {n} 次":
    "{n} in total",
  "这次没有拿到回程路由。":
    "No return route was obtained this time.",
  "回程是从主机追踪到这台设备的地址；去程是这台设备到服务器，浏览器追踪不了，要在设备那边跑一次路由追踪贴进来。来回两个方向走的线路常常不一样。线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。":
    "The return route is traced from the host to this device's address; the forward route is from this device to the server, which a browser can't trace, so run a route trace on the device side and paste it in. The two directions often take different lines. Lines are inferred from published carrier backbone ranges and each hop's autonomous system (AS), so treat them as a hint; hops that don't reply show as *.",
  "蜂窝网络":
    "Cellular",
  "有线网络":
    "Wired",
  "这台设备的浏览器直接连到 {host} 上临时开的测速服务量的，不经过面板：延迟是连发小请求的往返时间，取中位数，里面含一点服务处理的时间；下载和上传是浏览器直接和这台主机传数据。":
    "Measured by this device's browser connecting directly to the temporary speed test service on {host}, without going through the panel: latency is the round-trip time of small requests sent in a row, taking the median, and includes a little service processing time; download and upload are the browser transferring data directly with this host.",
  "延迟是浏览器向面板连发小请求量出来的往返时间，取中位数，里面含一点服务器处理的时间；下载和上传是浏览器直接和面板传数据量的。":
    "Latency is the round-trip time of small requests the browser sends to the panel in a row, taking the median; it includes a little server processing time. Download and upload are measured by the browser transferring data directly with the panel.",
  "面板前面有反向代理：这些数字量的是这台设备到代理（或 CDN）那一段，加上代理到面板。":
    "The panel is behind a reverse proxy: these numbers cover this device to the proxy (or CDN), plus the proxy to the panel.",
  "往返时间":
    "Round-trip time",
  "最小 {min}、中位 {med}、最大 {max} ms":
    "Min {min}, median {med}, max {max} ms",
  "连接类型":
    "Connection type",
  "从 {host} ping 它":
    "Ping it from {host}",
  "不回应":
    "No reply",
  "平均 {avg} ms，丢包 {loss}%":
    "Average {avg} ms, {loss}% packet loss",
  "家用路由器和运营商的地址常常不回应 ping，不代表不通。":
    "Home routers and carrier addresses often don't reply to ping; that doesn't mean they are unreachable.",
  "移除去程路由":
    "Remove forward route",
  "添加去程路由":
    "Add forward route",
  "这台设备 → 服务器":
    "This device → server",
  "在和这台设备同一个网络里的电脑上运行下面的命令，把完整输出贴到这里。手机上可以用带路由追踪功能的网络工具，把结果按文字贴进来。":
    "Run the command below on a computer on the same network as this device and paste the full output here. On a phone, use a network tool with route tracing and paste the result as text.",
  "把 tracert、traceroute 或 mtr 的输出贴在这里":
    "Paste the output of tracert, traceroute or mtr here",
  "去程路由的输出":
    "Forward route output",
  "只留下每一跳的序号、IP 地址和时间，别的文字不保存。":
    "Only each hop's number, IP address and times are kept; other text is not saved.",
  "回程：{host} → 这台设备":
    "Return: {host} → this device",
  "去程：这台设备 → 服务器":
    "Forward: this device → server",
  "最后一跳没有到达这台设备：家用路由器和运营商的地址常常不回应探测，前面的节点和线路仍然可以看。":
    "The last hop didn't reach this device: home routers and carrier addresses often don't reply to probes, but the earlier hops and the line are still shown.",
};

export default dict;
