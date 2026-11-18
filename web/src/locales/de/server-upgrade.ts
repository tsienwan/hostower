// de 词典：应用升级说明的服务端文字。
const dict: Record<string, string> = {
  "{a} 不能从 {b} 直接升级到 {c}：{d!}": "{a} lässt sich nicht direkt von {b} auf {c} aktualisieren: {d!}",
  "从 1.x 升级到 2.x 时，第一次启动会自动迁移数据库，数据多时要几分钟，期间页面打不开；升级前建议先备份实例目录下的 data。": "Beim Upgrade von 1.x auf 2.x wird die Datenbank beim ersten Start automatisch migriert; bei vielen Daten dauert das einige Minuten, in denen die Seite nicht erreichbar ist. Vorher data/ im Instanzverzeichnis sichern.",
  "Redis 8 直接读取 7.x 的数据文件；升级前建议先备份实例目录下的 data。": "Redis 8 liest 7.x-Datendateien direkt; vorher data/ im Instanzverzeichnis sichern.",
  "升级到 11.8 后，第一次启动会自动升级数据（MARIADB_AUTO_UPGRADE）；升级前建议先备份实例目录下的 data。": "Nach dem Upgrade auf 11.8 werden die Daten beim ersten Start automatisch aktualisiert (MARIADB_AUTO_UPGRADE); vorher data/ sichern.",
  "PostgreSQL 的大版本之间数据目录不兼容，17 不能直接换成 18：要用 pg_upgrade，或者先导出（pg_dumpall）再导入新装的 18。Hostower 不会自动做这一步。": "PostgreSQL-Datenverzeichnisse sind zwischen Hauptversionen nicht kompatibel; 17 lässt sich nicht einfach gegen 18 tauschen. pg_upgrade nutzen oder exportieren (pg_dumpall) und in ein neues 18 importieren. Hostower macht das nicht automatisch.",
};

export default dict;
