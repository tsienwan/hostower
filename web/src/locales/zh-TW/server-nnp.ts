// zh-TW 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "執行命令的行程被禁止提權（服務設定了 NoNewPrivileges），sudo 用不了，{a}。可以把這台機器以 SSH 方式再新增一次（位址 127.0.0.1，用一個能 sudo 的帳號），透過它來管",
  "登录账号不是 root，也没有 sudo：{a}": "登入帳號不是 root，也沒有 sudo：{a}",
  "改不了防火墙": "改不了防火牆",
  "改不了 Caddy 的配置": "改不了 Caddy 的設定",
  "装不了 Caddy": "裝不了 Caddy",
};

export default dict;
