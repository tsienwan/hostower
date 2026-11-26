// en 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Commands on this host are run directly by the Hostower process, and the service sets NoNewPrivileges (a hardening that forbids privilege escalation), so sudo cannot work and the firewall and Caddy cannot be changed.",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Recommended: click Edit at the top right and change this host's connection to SSH (address 127.0.0.1, with a credential for an account that has passwordless sudo). It stays the same host; there is no need to add another one. You could also remove NoNewPrivileges=true from the service file, but that weakens the hardening.",
};

export default dict;
