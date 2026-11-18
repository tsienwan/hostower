// en 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Commands on this host are run directly by the Hostower process, and the service sets NoNewPrivileges (a hardening that forbids privilege escalation), so sudo cannot work and the firewall and Caddy cannot be changed.",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Recommended: in Overview, add this machine again over SSH (address 127.0.0.1, with an account that has passwordless sudo) and manage the firewall and Caddy through it. You could also remove NoNewPrivileges=true from the service file, but that weakens the hardening.",
};

export default dict;
