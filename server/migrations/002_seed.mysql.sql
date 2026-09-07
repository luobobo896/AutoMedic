-- ============================================================
-- AutoMedic 初始化数据（可选）
-- 说明：程序首次启动会通过 SeedDefault() 自动写入下列厂家与默认模型，
--       本脚本供需要"用 SQL 预置数据"或"锁死初始数据"的场景使用，
--       幂等：已存在同 key / 同 slug 的记录会被忽略。
-- 用法：mysql -u root -p automedic < 002_seed.mysql.sql
-- ============================================================

SET NAMES utf8mb4;

INSERT IGNORE INTO `providers`
  (`created_at`, `updated_at`, `name`, `key`, `kind`, `base_url`,
   `max_input_context`, `max_output_context`, `enabled`, `remark`)
VALUES
  (NOW(3), NOW(3), 'DeepSeek 官方',        'deepseek-official', 'deepseek', NULL, 1048576, 131072, 1, 'dsh 内置 provider'),
  (NOW(3), NOW(3), 'OpenAI',               'openai',            'openai',   NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), 'Anthropic Claude',     'anthropic',         'anthropic',NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), '通义千问（阿里云）',    'qwen',              'qwen',     NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), '智谱 GLM',             'zhipu',             'zhipu',    NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), '月之暗面 Kimi',        'moonshot',          'moonshot', NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), '豆包（火山引擎）',      'doubao',            'doubao',   NULL, 1000000,  65536, 1, NULL),
  (NOW(3), NOW(3), 'Google Gemini',        'gemini',            'gemini',   NULL, 2000000,  65536, 1, NULL),
  (NOW(3), NOW(3), '自定义 OpenAI 兼容',   'openai-compatible', 'custom',   NULL, 1000000,  65536, 1, NULL);

-- 默认模型（挂在 deepseek-official 下）
INSERT IGNORE INTO `llm_models`
  (`created_at`, `updated_at`, `provider_id`, `name`, `slug`,
   `input_context`, `output_context`, `max_turns`, `enabled`, `is_default`, `remark`)
SELECT NOW(3), NOW(3), p.`id`, 'DeepSeek V4 Flash', 'deepseek-v4-flash',
       1048576, 131072, 128, 1, 1, '默认模型'
  FROM `providers` p WHERE p.`key` = 'deepseek-official'
  AND NOT EXISTS (SELECT 1 FROM `llm_models` m WHERE m.`slug` = 'deepseek-v4-flash');

INSERT IGNORE INTO `llm_models`
  (`created_at`, `updated_at`, `provider_id`, `name`, `slug`,
   `input_context`, `output_context`, `max_turns`, `enabled`, `is_default`, `remark`)
SELECT NOW(3), NOW(3), p.`id`, 'DeepSeek V4 Pro', 'deepseek-v4-pro',
       1048576, 131072, 256, 1, 0, NULL
  FROM `providers` p WHERE p.`key` = 'deepseek-official'
  AND NOT EXISTS (SELECT 1 FROM `llm_models` m WHERE m.`slug` = 'deepseek-v4-pro');
