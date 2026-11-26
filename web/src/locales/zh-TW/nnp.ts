// zh-TW 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "這台主機上的命令由 Hostower 行程直接執行，而服務設定了 NoNewPrivileges（禁止提權的安全加固），所以 sudo 用不了，改不了防火牆和 Caddy。",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "推薦：點右上角的「編輯」，把這台主機的連線方式改成 SSH（位址 127.0.0.1，選一條能免密 sudo 的帳號的憑證）。它還是這一台，不用另外新增主機。也可以去掉服務檔裡的 NoNewPrivileges=true，但那會削弱加固。",
};

export default dict;
