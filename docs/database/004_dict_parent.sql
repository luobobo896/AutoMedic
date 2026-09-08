-- 字典跨组父子：厂家类型(provider_kind) → 模型标识(model_slug)。
-- AutoMigrate 启动时也会幂等补列；本脚本供预建库或手工升级。
-- 回填由 SeedDicts.linkDictParents 按 extra.kind 补 parent_id（只填空，不覆盖已改关系）。

ALTER TABLE dicts
  ADD COLUMN IF NOT EXISTS parent_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_dict_parent ON dicts (parent_id);
