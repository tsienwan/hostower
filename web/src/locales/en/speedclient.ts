// en 词典：“这台设备”测速（浏览器和面板之间的速度、回程和去程路由）。
const dict: Record<string, string> = {
  "这台设备 ↔ 主机":
    "This device ↔ host",
  "测这台设备":
    "Test this device",
  "把正在看面板的这台设备（比如家里、公司或外面连着网的手机）当成一个节点：量它和任意一台主机之间的延迟、下载、上传，再从那台主机追踪回到它的路由，看回程走的哪条线路、经过哪些节点。":
    "Treats the device viewing the panel (say, a phone online at home, at work or on the go) as a node: measures latency, download and upload between it and any host, then traces the route from that host back to it to show which line the return route takes and which hops it passes.",
  "各网络最近一次":
    "Latest for each network",
  "没有拿到结果":
    "No result received",
  "速度测到哪里":
    "Speed test target",
  "主机（直连）":
    "Hosts (direct)",
  "在选中的主机上各临时开一个测速服务，这台设备的浏览器一台一台直接连过去量延迟、下载、上传，再从每台主机追踪回到这台设备，最后排成一张对比表。服务只认带一次性口令的地址，测完自己退出。开端口是一次变更（L1），需要批准，一次批准管这一批。":
    "Starts a temporary speed test service on each selected host. This device's browser connects to them directly, one at a time, to measure latency, download and upload, then the route is traced from each host back to this device and everything is lined up in one comparison table. The service only accepts the address carrying a one-time token and exits by itself when the test is done. Opening a port is a change (L1) and needs approval; one approval covers the whole batch.",
  "延迟和速度量的是这台设备和面板之间；选中的主机只用来追踪回到这台设备的路由。":
    "Latency and speed are measured between this device and the panel; the selected hosts are only used to trace the route back to this device.",
  "测哪些主机（最多 {n} 台）":
    "Hosts to test (up to {n})",
  "{host} 的地址":
    "Address of {host}",
  "地址填这台设备能直接连到的（公网 IP 或域名）。按勾选的顺序一台一台测。":
    "Enter addresses this device can reach directly (public IP or domain name). Hosts are tested one at a time, in the order you selected them.",
  "每台主机的防火墙和云厂商的安全组都要放行这个 TCP 端口，端口在 1024 到 65535 之间。":
    "Each host's firewall and the cloud provider's security group must allow this TCP port, which must be between 1024 and 65535.",
  "测速页由主机直接提供，用的是 HTTP：浏览器可能提示“不安全”，选择继续访问即可。页面上只有测速用的随机数据，不涉及面板的登录信息。":
    "The speed test page is served directly by the host over HTTP: the browser may warn that it is \"not secure\"; just choose to continue. The page carries only random data for the test and nothing about your panel sign-in.",
  "回程路由从哪台主机追踪":
    "Host to trace the return route from",
  "没有在线的主机":
    "No hosts online",
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
  "每台下载最多 {d} MB、上传最多 {u} MB，各自最多跑 {s} 秒，先到哪个算哪个。会消耗主机和这台设备的流量，这次最多约 {total} MB。":
    "Per host, download up to {d} MB and upload up to {u} MB, each for at most {s} seconds, whichever comes first. This uses traffic on the hosts and on this device, up to about {total} MB for this run.",
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
  "接着测下一台…":
    "Moving on to the next host…",
  "没有测成":
    "Test didn't complete",
  "测完了":
    "Test finished",
  "正在追踪回程路由…":
    "Tracing the return route…",
  "正在测…":
    "Testing…",
  "等这台设备连上来":
    "Waiting for this device to connect",
  "正在启动测速服务…":
    "Starting the speed test service…",
  "批准后才会在主机上开测速服务":
    "The speed test service starts on the host only after approval",
  "这会在 {host} 上临时开一个端口（{port}），等这台设备连上来测速，测完自己退出。":
    "This temporarily opens a port ({port}) on {host}, waits for this device to connect and run the test, then exits by itself.",
  "正在主机上启动测速服务…":
    "Starting the speed test service on the host…",
  "测速服务已经在等着了。在这台设备上打开测速页，它会自己开始，一台一台测下去。":
    "The speed test service is waiting. Open the speed test page on this device and it will start by itself, testing the hosts one at a time.",
  "打开测速页（还有 {n} 台）":
    "Open speed test page ({n} left)",
  "打开测速页":
    "Open speed test page",
  "打不开或一直转圈：那台主机的防火墙或云厂商的安全组没有放行 TCP 端口 {port}，或者填的地址这台设备连不上。回到这里可以接着测剩下的。服务最多等 {w} 秒。":
    "If it won't open or keeps loading: that host's firewall or the cloud provider's security group doesn't allow TCP port {port}, or this device can't reach the address you entered. Come back here to continue with the remaining hosts. The service waits at most {w} seconds.",
  "正在另一个页面里测 {host}…":
    "Testing {host} in another page…",
  "下载时的延迟":
    "Latency during download",
  "回程重传率":
    "Return retransmission rate",
  "回程线路":
    "Return line",
  "延迟最低":
    "Lowest latency",
  "延迟和速度是这台设备的浏览器直接和每台主机量的；回程线路是从主机追踪回来认出的，只能作参考。每台的明细在各自的报告里。":
    "Latency and speed are measured by this device's browser directly with each host; the return line is identified by tracing back from the host and is only a reference. Details for each host are in its own report.",
  "查看报告":
    "View report",
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
  "正在从 {host} 追踪到这台设备，最多要半分钟…":
    "Tracing from {host} to this device; this can take up to half a minute…",
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
  "这是回程：从主机追踪到这台设备的地址。去程（这台设备到主机）在浏览器里追踪不了，所以这里没有；来回两个方向走的线路常常不一样。线路按公开的运营商骨干网段和节点所属的自治系统（AS）推测，只能作参考；不回应的节点显示为 *。":
    "This is the return route, traced from the host to this device's address. The forward route (this device to the host) cannot be traced from a browser, so it is not shown; the two directions often take different lines. Lines are inferred from public carrier backbone ranges and each hop's autonomous system (AS) and are only a reference; hops that do not reply are shown as *.",
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
  "上传时的延迟":
    "Latency during upload",
  "连接类型":
    "Connection type",
  "从 {host} ping 它":
    "Ping it from {host}",
  "不回应":
    "No reply",
  "平均 {avg} ms，丢包 {loss}%":
    "Average {avg} ms, {loss}% packet loss",
  "{a} / {b} 个数据段":
    "{a} / {b} segments",
  "TCP 往返时间":
    "TCP round-trip time",
  "重传率是下载时 {host} 重发的数据段占比，从它的 TCP 连接上读的：回程丢包越多它越高，到 1% 以上速度就会明显受影响。":
    "The retransmission rate is the share of segments {host} had to resend during the download, read from its TCP connection: the more loss on the return path, the higher it gets, and above 1% speed is noticeably affected.",
  "家用路由器和运营商的地址常常不回应 ping，不代表不通。":
    "Home routers and carrier addresses often don't reply to ping; that doesn't mean they are unreachable.",
  "比空闲时高 {n} ms":
    "{n} ms higher than idle",
  "和空闲时差不多":
    "About the same as idle",
  "回程：{host} → 这台设备":
    "Return: {host} → this device",
  "每一跳探了 {n} 次，“丢包”是没回应的次数。中间某一跳丢、后面的跳不丢，多半是那台路由器限制了回应的速率，不是真的丢包；从某一跳开始一直丢到最后，问题才在那一段。":
    "Each hop was probed {n} times; \"Loss\" is the number of probes that got no reply. If a middle hop shows loss but later hops don't, that router is most likely rate-limiting its replies rather than really dropping packets; only when loss starts at a hop and continues to the end is the problem in that stretch.",
  "最后一跳没有到达这台设备：家用路由器和运营商的地址常常不回应探测，前面的节点和线路仍然可以看。":
    "The last hop didn't reach this device: home routers and carrier addresses often don't reply to probes, but the earlier hops and the line are still shown.",
};

export default dict;
