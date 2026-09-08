-- AutoMedic PostgreSQL：建库与授权（表结构由程序启动时 AutoMigrate 幂等补齐）
-- 以 postgres 超级用户执行：
--   sudo -u postgres psql -f docs/database/001_create_database.sql

CREATE ROLE automedic WITH LOGIN;
-- 若角色已存在会失败；可忽略后继续执行下面语句。

CREATE DATABASE automedic OWNER automedic
  ENCODING 'UTF8'
  TEMPLATE template1;

GRANT ALL PRIVILEGES ON DATABASE automedic TO automedic;

\c automedic
GRANT ALL ON SCHEMA public TO automedic;
ALTER SCHEMA public OWNER TO automedic;
