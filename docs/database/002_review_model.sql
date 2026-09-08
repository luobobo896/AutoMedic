-- 项目/仓库可单独指定审查模型；留空则与修复模型同一套（大模型配置中心）。
-- AutoMigrate 启动时也会幂等补列；本脚本供预建库或手工升级。

ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS default_review_model_id BIGINT;

ALTER TABLE repositories
  ADD COLUMN IF NOT EXISTS review_model_id BIGINT;
