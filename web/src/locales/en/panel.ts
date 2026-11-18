// en 词典：设置页“面板”：环境变量接管时的说明。
const dict: Record<string, string> = {
  "现在生效的是环境变量 ": "Currently in effect: the value of environment variable ",
  " 的值": "",
  "。这里保存的值要等去掉这个环境变量、重启后才生效。": ". A value saved here takes effect only after that variable is removed and Hostower restarts.",
  "有 {n} 项设置由环境变量决定：": "{n} settings are set by environment variables: ",
  "。环境变量优先于这里保存的值。": ". Environment variables take precedence over values saved here.",
  "想改成在这里管理：先在下面填好并保存（可以点“带入环境变量的值”），再去掉这些环境变量、重启 Hostower。这样顺序做，不会断开访问。": "To manage them here instead: fill in and save the values below first (you can click \"Use environment values\"), then remove those variables and restart Hostower. Doing it in this order keeps you from losing access.",
  "systemd 部署：运行 ": "systemd: run ",
  " 删掉对应的 Environment= 行（或检查 /etc/systemd/system/hostower.service），然后 ": " and delete the matching Environment= lines (or check /etc/systemd/system/hostower.service), then ",
  "。Docker 部署：删掉 compose 文件 environment 里对应的行，再 ": ". Docker: delete the matching lines under environment in the compose file, then ",
  "带入环境变量的值": "Use environment values",
  "谁来提供 HTTPS。对外监听时必须二选一，否则塔台拒绝启动。选“塔台自己提供”后填证书和私钥文件的路径。": "Who provides HTTPS. When listening publicly you must pick one, or the tower refuses to start. Choose \"The tower itself\" to enter the certificate and private key file paths.",
};

export default dict;
