// en 词典：应用升级说明的服务端文字。
const dict: Record<string, string> = {
  "{a} 不能从 {b} 直接升级到 {c}：{d!}": "{a} cannot be upgraded directly from {b} to {c}: {d!}",
  "从 1.x 升级到 2.x 时，第一次启动会自动迁移数据库，数据多时要几分钟，期间页面打不开；升级前建议先备份实例目录下的 data。": "When upgrading from 1.x to 2.x, the database is migrated automatically on first start; with lots of data this takes a few minutes, during which the page is unavailable. Back up data/ in the instance directory first.",
  "Redis 8 直接读取 7.x 的数据文件；升级前建议先备份实例目录下的 data。": "Redis 8 reads 7.x data files directly; back up data/ in the instance directory first.",
  "升级到 11.8 后，第一次启动会自动升级数据（MARIADB_AUTO_UPGRADE）；升级前建议先备份实例目录下的 data。": "After upgrading to 11.8 the data is upgraded automatically on first start (MARIADB_AUTO_UPGRADE); back up data/ in the instance directory first.",
  "PostgreSQL 的大版本之间数据目录不兼容，17 不能直接换成 18：要用 pg_upgrade，或者先导出（pg_dumpall）再导入新装的 18。Hostower 不会自动做这一步。": "PostgreSQL data directories are not compatible across major versions, so 17 cannot simply be swapped for 18: use pg_upgrade, or export (pg_dumpall) and import into a fresh 18. Hostower does not do this automatically.",
};

export default dict;
