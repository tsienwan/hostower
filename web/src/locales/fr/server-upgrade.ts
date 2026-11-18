// fr 词典：应用升级说明的服务端文字。
const dict: Record<string, string> = {
  "{a} 不能从 {b} 直接升级到 {c}：{d!}": "{a} ne peut pas passer directement de {b} à {c} : {d!}",
  "从 1.x 升级到 2.x 时，第一次启动会自动迁移数据库，数据多时要几分钟，期间页面打不开；升级前建议先备份实例目录下的 data。": "Lors du passage de 1.x à 2.x, la base est migrée automatiquement au premier démarrage ; avec beaucoup de données cela prend quelques minutes, page indisponible. Sauvegardez d’abord data/ dans le répertoire de l’instance.",
  "Redis 8 直接读取 7.x 的数据文件；升级前建议先备份实例目录下的 data。": "Redis 8 lit directement les fichiers de données 7.x ; sauvegardez d’abord data/ dans le répertoire de l’instance.",
  "升级到 11.8 后，第一次启动会自动升级数据（MARIADB_AUTO_UPGRADE）；升级前建议先备份实例目录下的 data。": "Après passage à 11.8, les données sont mises à niveau au premier démarrage (MARIADB_AUTO_UPGRADE) ; sauvegardez d’abord data/.",
  "PostgreSQL 的大版本之间数据目录不兼容，17 不能直接换成 18：要用 pg_upgrade，或者先导出（pg_dumpall）再导入新装的 18。Hostower 不会自动做这一步。": "Les répertoires de données PostgreSQL ne sont pas compatibles entre versions majeures : impossible de remplacer 17 par 18 tel quel. Utilisez pg_upgrade, ou exportez (pg_dumpall) puis importez dans un 18 neuf. Hostower ne le fait pas automatiquement.",
};

export default dict;
