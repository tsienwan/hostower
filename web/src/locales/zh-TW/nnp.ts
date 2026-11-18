// zh-TW 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "這台主機上的命令由 Hostower 行程直接執行，而服務設定了 NoNewPrivileges（禁止提權的安全加固），所以 sudo 用不了，改不了防火牆和 Caddy。",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "推薦：在「總覽」裡把這台機器以 SSH 方式再新增一次（位址 127.0.0.1，用一個能免密 sudo 的帳號），透過它來管防火牆和 Caddy。也可以去掉服務檔裡的 NoNewPrivileges=true，但那會削弱加固。",
};

export default dict;
