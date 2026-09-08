-- 表单选项字典。程序启动 SeedDicts 会按 (group, value) 幂等补缺，不覆盖已改项。

CREATE TABLE IF NOT EXISTS dicts (
  id         BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  "group"    VARCHAR(64)  NOT NULL,
  value      VARCHAR(256) NOT NULL,
  label      VARCHAR(256),
  extra      TEXT,
  sort       INTEGER      NOT NULL DEFAULT 0,
  enabled    BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_dict_group_value ON dicts ("group", value);
