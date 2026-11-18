// zh-TW 词典：应用升级说明的服务端文字。
const dict: Record<string, string> = {
  "{a} 不能从 {b} 直接升级到 {c}：{d!}": "{a} 不能從 {b} 直接升級到 {c}：{d!}",
  "从 1.x 升级到 2.x 时，第一次启动会自动迁移数据库，数据多时要几分钟，期间页面打不开；升级前建议先备份实例目录下的 data。": "從 1.x 升級到 2.x 時，第一次啟動會自動遷移資料庫，資料多時要幾分鐘，期間頁面打不開；升級前建議先備份實例目錄下的 data。",
  "Redis 8 直接读取 7.x 的数据文件；升级前建议先备份实例目录下的 data。": "Redis 8 直接讀取 7.x 的資料檔；升級前建議先備份實例目錄下的 data。",
  "升级到 11.8 后，第一次启动会自动升级数据（MARIADB_AUTO_UPGRADE）；升级前建议先备份实例目录下的 data。": "升級到 11.8 後，第一次啟動會自動升級資料（MARIADB_AUTO_UPGRADE）；升級前建議先備份實例目錄下的 data。",
  "PostgreSQL 的大版本之间数据目录不兼容，17 不能直接换成 18：要用 pg_upgrade，或者先导出（pg_dumpall）再导入新装的 18。Hostower 不会自动做这一步。": "PostgreSQL 的大版本之間資料目錄不相容，17 不能直接換成 18：要用 pg_upgrade，或者先匯出（pg_dumpall）再匯入新裝的 18。Hostower 不會自動做這一步。",
};

export default dict;
