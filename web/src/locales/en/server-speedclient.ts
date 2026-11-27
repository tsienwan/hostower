// en 词典：“这台设备”测速的服务端文字。
const dict: Record<string, string> = {
  "同时在测速的连接太多了，请稍后再试":
    "Too many connections are running speed tests at once; try again later",
  "数据量不对":
    "Invalid data size",
  "生成数据失败":
    "Failed to generate data",
  "一次传得太多了":
    "Too much data sent at once",
  "没有这条测速请求":
    "No such speed test request",
  "追踪从 {a} 到这台设备（{b}）的回程路由":
    "Trace the return route from {a} to this device ({b})",
  "安装缺少的 traceroute 并追踪从 {a} 到这台设备（{b}）的回程路由":
    "Install the missing traceroute and trace the return route from {a} to this device ({b})",
  "读不出这台设备的来源地址，没法追踪回程路由":
    "Can't read this device's source address, so the return route can't be traced",
  "这台设备的来源地址不能用来追踪路由":
    "This device's source address can't be used for route tracing",
  "测量结果里的数字不对":
    "Invalid numbers in the measurement results",
  "测量结果里的延迟次数不对":
    "Invalid latency sample count in the measurement results",
  "贴进来的内容太长了":
    "The pasted content is too long",
  "没有从贴进来的内容里认出路由：请贴 tracert、traceroute 或 mtr 的完整输出":
    "No route recognized in the pasted content: paste the full output of tracert, traceroute or mtr",
  "端口应该在 1024 到 65535 之间":
    "The port must be between 1024 and 65535",
  "口令不对":
    "Invalid token",
  "在 {a} 上临时开一个测速服务（端口 {b}），测这台设备和它之间的速度并追踪回程路由":
    "Start a temporary speed test service on {a} (port {b}) to measure the speed between this device and it and trace the return route",
  "主机上没有 python3，开不了临时测速服务":
    "The host has no python3, so the temporary speed test service can't be started",
  "主机上这个端口已经被占用，换一个端口再试":
    "This port is already in use on the host; try another port",
  "没有等到这台设备连上来：测速页没打开，或者主机的防火墙、云厂商的安全组没有放行这个端口":
    "This device never connected: the speed test page wasn't opened, or the host's firewall or the cloud provider's security group doesn't allow this port",
  "这台设备连上来了，但没有在限定时间里测完":
    "This device connected but didn't finish the test within the time limit",
};

export default dict;
