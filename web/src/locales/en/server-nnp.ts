// en 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以编辑这台主机，把连接方式改成 SSH（地址 127.0.0.1，选一条能 sudo 的账号的凭据），再来操作": "The process running commands may not escalate privileges (the service sets NoNewPrivileges), so sudo cannot work: {a}. Edit this host and change its connection to SSH (address 127.0.0.1, with a credential for an account that can sudo), then try again",
  "登录账号不是 root，也没有 sudo：{a}": "The login account is not root and has no sudo: {a}",
  "改不了防火墙": "the firewall cannot be changed",
  "装不了 Caddy": "Caddy cannot be installed",
  "改不了站点配置": "the site configuration cannot be changed",
  "装不了 Nginx": "Nginx cannot be installed",
  "续期不了证书": "certificates cannot be renewed",
  "管不了 Fail2ban": "Fail2ban cannot be managed",
  "启动不了 Caddy": "Caddy cannot be started",
  "启动不了 Nginx": "Nginx cannot be started",
  "宿主机模式下本机上的命令已经以 root 身份执行，不需要改成 SSH": "In host mode, commands on the local host already run as root; switching to SSH is not needed",
  "本机改成 SSH 连接时，地址只能是 127.0.0.1 或 localhost（可以带端口）": "When the local host connects over SSH, the address must be 127.0.0.1 or localhost (a port is allowed)",
};

export default dict;
