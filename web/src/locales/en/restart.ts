// en 词典：设置页的“重启 Hostower”。
const dict: Record<string, string> = {
  "立即重启": "Restart now",
  "重启 Hostower": "Restart Hostower",
  "重启": "Restart",
  "刷新页面": "Reload page",
  "重启会让面板断开几秒钟，保存过的面板设置（监听地址、访问地址、证书、会话时长）在重启后生效。需要当场验证一次身份。": "Restarting disconnects the panel for a few seconds; saved panel settings (listen address, access URL, certificate, session length) take effect after the restart. You will need to verify your identity once.",
  "有 {n} 个请求正在执行，重启会中断它们（之后标成“中断”，不会自动重跑）。": "{n} requests are running; restarting interrupts them (they are marked \"interrupted\" and not rerun).",
  "访问地址会变成 ": "The access URL will change to ",
  "。重启后请到新地址打开；如果前面有反向代理，记得一起改它转发的端口。": ". Open it at the new address after the restart; if there is a reverse proxy in front, update the port it forwards to as well.",
  "正在重启。稍等几秒后到新地址打开：": "Restarting. Wait a few seconds, then open the new address: ",
  "正在重启，回来后会自动刷新页面…": "Restarting; the page reloads when it is back…",
  "一分钟了还没连上。可能是新的面板设置有问题，或者反向代理转发的端口没跟着改。到主机上看日志：": "Still not reachable after a minute. The new panel settings may be wrong, or the reverse proxy still forwards to the old port. Check the log on the host: ",
  "；面板设置写错时，可以临时设置环境变量 HOSTOWER_IGNORE_PANEL=1 跳过它启动。": "; if the panel settings are wrong, temporarily set HOSTOWER_IGNORE_PANEL=1 to start without them.",
};

export default dict;
