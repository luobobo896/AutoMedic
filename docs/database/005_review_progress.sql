-- 审查任务过程可见：当前进度与过程日志。AutoMigrate 也会幂等补列。

ALTER TABLE review_jobs
  ADD COLUMN IF NOT EXISTS progress VARCHAR(512);

ALTER TABLE review_jobs
  ADD COLUMN IF NOT EXISTS logs TEXT;
