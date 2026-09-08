-- 事件闭环：同一指纹合并计数、最后出现时间。
-- AutoMigrate 会幂等补列；本脚本供手工预建。

ALTER TABLE events
  ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ;

ALTER TABLE events
  ADD COLUMN IF NOT EXISTS occurrence_n INTEGER NOT NULL DEFAULT 1;
