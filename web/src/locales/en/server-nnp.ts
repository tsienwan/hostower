// en 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "The process running commands may not escalate privileges (the service sets NoNewPrivileges), so sudo cannot work: {a}. Add this machine again over SSH (address 127.0.0.1, with an account that can sudo) and manage it through that",
  "登录账号不是 root，也没有 sudo：{a}": "The login account is not root and has no sudo: {a}",
  "改不了防火墙": "the firewall cannot be changed",
  "改不了 Caddy 的配置": "Caddy's configuration cannot be changed",
  "装不了 Caddy": "Caddy cannot be installed",
};

export default dict;
