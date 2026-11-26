// zh-TW 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以编辑这台主机，把连接方式改成 SSH（地址 127.0.0.1，选一条能 sudo 的账号的凭据），再来操作": "執行命令的行程被禁止提權（服務設定了 NoNewPrivileges），sudo 用不了，{a}。可以編輯這台主機，把連線方式改成 SSH（位址 127.0.0.1，選一條能 sudo 的帳號的憑證），再來操作",
  "登录账号不是 root，也没有 sudo：{a}": "登入帳號不是 root，也沒有 sudo：{a}",
  "改不了防火墙": "改不了防火牆",
  "装不了 Caddy": "裝不了 Caddy",
  "改不了站点配置": "改不了站點設定",
  "装不了 Nginx": "裝不了 Nginx",
  "续期不了证书": "續期不了憑證",
  "管不了 Fail2ban": "管不了 Fail2ban",
  "启动不了 Caddy": "啟動不了 Caddy",
  "启动不了 Nginx": "啟動不了 Nginx",
  "宿主机模式下本机上的命令已经以 root 身份执行，不需要改成 SSH": "宿主機模式下本機上的命令已經以 root 身分執行，不需要改成 SSH",
  "本机改成 SSH 连接时，地址只能是 127.0.0.1 或 localhost（可以带端口）": "本機改成 SSH 連線時，位址只能是 127.0.0.1 或 localhost（可以帶連接埠）",
};

export default dict;
