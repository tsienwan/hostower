// ja 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "このホストのコマンドは Hostower のプロセスが直接実行しており、サービスに NoNewPrivileges（権限昇格を禁止する強化設定）があるため sudo が使えず、ファイアウォールと Caddy を変更できません。",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "推奨：「概要」でこのマシンを SSH 経由でもう一度追加し（アドレス 127.0.0.1、パスワードなし sudo ができるアカウント）、そちらでファイアウォールと Caddy を管理してください。サービスファイルの NoNewPrivileges=true を外すこともできますが、強化が弱まります。",
};

export default dict;
