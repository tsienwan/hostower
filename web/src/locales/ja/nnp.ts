// ja 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "このホストのコマンドは Hostower のプロセスが直接実行しており、サービスに NoNewPrivileges（権限昇格を禁止する強化設定）があるため sudo が使えず、ファイアウォールと Caddy を変更できません。",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "推奨：右上の「編集」を押し、このホストの接続方式を SSH に変更します（アドレス 127.0.0.1、パスワードなし sudo ができるアカウントの認証情報）。ホストは同じ 1 台のままで、別のホストを追加する必要はありません。サービスファイルの NoNewPrivileges=true を外すこともできますが、強化が弱まります。",
};

export default dict;
