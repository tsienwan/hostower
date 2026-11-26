// ja 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "コマンドを実行するプロセスは権限昇格を禁止されています（サービスに NoNewPrivileges）。sudo が使えないため{a}。このマシンを SSH 経由で追加し直し（アドレス 127.0.0.1、sudo できるアカウント）、そちらで管理してください",
  "登录账号不是 root，也没有 sudo：{a}": "ログインアカウントは root ではなく sudo もありません：{a}",
  "改不了防火墙": "ファイアウォールを変更できません",
  "装不了 Caddy": "Caddy を入れられません",
  "改不了站点配置": "サイトの設定を変更できません",
  "装不了 Nginx": "Nginx をインストールできません",
  "续期不了证书": "証明書を更新できません",
  "管不了 Fail2ban": "Fail2ban を管理できません",
  "启动不了 Caddy": "Caddy を起動できません",
  "启动不了 Nginx": "Nginx を起動できません",
};

export default dict;
