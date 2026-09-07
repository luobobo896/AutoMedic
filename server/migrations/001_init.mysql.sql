-- ============================================================
-- AutoMedic MySQL 初始化迁移脚本
-- 适用于 MySQL 8.0+ / MariaDB 10.6+
-- 用法：mysql -u root -p automedic < 001_init.mysql.sql
-- 说明：
--   1. 本脚本与 GORM AutoMigrate 结果等价，生产环境推荐用本脚本建表，
--      并在 config.yaml 中关闭自动迁移（默认即为关闭 AutoMigrate 之外的额外 DDL 风险）。
--   2. 若已用 AutoMigrate 建过表，可直接从 002 开始执行。
-- ============================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ---------- 项目 ----------
CREATE TABLE IF NOT EXISTS `projects` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`      DATETIME(3) NULL,
  `updated_at`      DATETIME(3) NULL,
  `name`            VARCHAR(128) NOT NULL,
  `key`             VARCHAR(64)  NOT NULL COMMENT '项目唯一标识',
  `description`     VARCHAR(512) DEFAULT NULL,
  `fix_mode`        VARCHAR(16)  NOT NULL DEFAULT 'semi' COMMENT 'auto=全自动 / semi=半自动',
  `default_model_id` BIGINT UNSIGNED DEFAULT NULL,
  `release_hook`    VARCHAR(1024) DEFAULT NULL COMMENT '修复成功后执行的钩子命令',
  `enabled`         TINYINT(1)   NOT NULL DEFAULT 1,
  `context`         TEXT         COMMENT '项目背景，写入 dsh 指令文件',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_projects_key` (`key`),
  KEY `idx_projects_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='项目';

-- ---------- 仓库 ----------
CREATE TABLE IF NOT EXISTS `repositories` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`    DATETIME(3) NULL,
  `updated_at`    DATETIME(3) NULL,
  `project_id`    BIGINT UNSIGNED NOT NULL,
  `name`          VARCHAR(128) NOT NULL,
  `url`           VARCHAR(512) NOT NULL,
  `branch`        VARCHAR(128) NOT NULL DEFAULT 'main',
  `code_paths`    VARCHAR(1024) DEFAULT NULL COMMENT '关注的代码路径前缀，逗号分隔',
  `language`      VARCHAR(64)  DEFAULT NULL,
  `credential_id` BIGINT UNSIGNED DEFAULT NULL,
  `model_id`      BIGINT UNSIGNED DEFAULT NULL,
  `auto_push`     TINYINT(1) NOT NULL DEFAULT 1,
  `enabled`       TINYINT(1) NOT NULL DEFAULT 1,
  `last_commit`   VARCHAR(64) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_repositories_project_id` (`project_id`),
  KEY `idx_repositories_credential_id` (`credential_id`),
  KEY `idx_repositories_model_id` (`model_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库';

-- ---------- 凭证（密文 AES-256-GCM 存储） ----------
CREATE TABLE IF NOT EXISTS `credentials` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`     DATETIME(3) NULL,
  `updated_at`     DATETIME(3) NULL,
  `name`           VARCHAR(128) NOT NULL,
  `type`           VARCHAR(32)  NOT NULL COMMENT 'ssh_key / http_auth / http_token',
  `username`       VARCHAR(128) DEFAULT NULL,
  `secret_enc`     TEXT         COMMENT '加密后的密钥/密码/Token',
  `passphrase_enc` TEXT         COMMENT '加密后的私钥口令',
  `description`    VARCHAR(512) DEFAULT NULL,
  `enabled`        TINYINT(1) NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='git 凭证';

CREATE TABLE IF NOT EXISTS `credential_usages` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `credential_id` BIGINT UNSIGNED NOT NULL,
  `ref_type`      VARCHAR(32) DEFAULT NULL COMMENT 'repo / task',
  `ref_id`        BIGINT UNSIGNED DEFAULT NULL,
  `action`        VARCHAR(32) DEFAULT NULL COMMENT 'clone / fetch / push',
  `result`        VARCHAR(16) DEFAULT NULL COMMENT 'ok / fail',
  `message`       VARCHAR(512) DEFAULT NULL,
  `created_at`    DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_credential_usages_credential_id` (`credential_id`),
  KEY `idx_credential_usages_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='凭证使用记录';

-- ---------- 大模型厂家 ----------
CREATE TABLE IF NOT EXISTS `providers` (
  `id`                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`         DATETIME(3) NULL,
  `updated_at`         DATETIME(3) NULL,
  `name`               VARCHAR(128) NOT NULL,
  `key`                VARCHAR(64)  NOT NULL COMMENT 'dsh provider 标识，如 deepseek-official',
  `kind`               VARCHAR(64)  DEFAULT NULL,
  `base_url`           VARCHAR(512) DEFAULT NULL,
  `api_key_enc`        TEXT         COMMENT '加密后的 API Key',
  `max_input_context`  BIGINT NOT NULL DEFAULT 0,
  `max_output_context` BIGINT NOT NULL DEFAULT 0,
  `enabled`            TINYINT(1) NOT NULL DEFAULT 1,
  `remark`             VARCHAR(512) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_providers_key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='大模型厂家';

-- ---------- 模型（每个模型独立配置输入/输出上下文） ----------
CREATE TABLE IF NOT EXISTS `llm_models` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`     DATETIME(3) NULL,
  `updated_at`     DATETIME(3) NULL,
  `provider_id`    BIGINT UNSIGNED NOT NULL,
  `name`           VARCHAR(128) NOT NULL COMMENT '展示名',
  `slug`           VARCHAR(128) NOT NULL COMMENT '传给 dsh 的模型标识',
  `input_context`  BIGINT NOT NULL DEFAULT 0 COMMENT '输入上下文 token 上限',
  `output_context` BIGINT NOT NULL DEFAULT 0 COMMENT '输出上下文 token 上限',
  `max_turns`      INT    NOT NULL DEFAULT 0,
  `temperature`    VARCHAR(16) DEFAULT NULL,
  `extra_params`   TEXT    COMMENT '透传到 dsh patch 的额外参数（JSON）',
  `enabled`        TINYINT(1) NOT NULL DEFAULT 1,
  `is_default`     TINYINT(1) NOT NULL DEFAULT 0,
  `remark`         VARCHAR(512) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_llm_models_provider_id` (`provider_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='模型';

-- ---------- 投递令牌 ----------
CREATE TABLE IF NOT EXISTS `ingest_tokens` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`   DATETIME(3) NULL,
  `updated_at`   DATETIME(3) NULL,
  `project_id`   BIGINT UNSIGNED NOT NULL,
  `name`         VARCHAR(128) NOT NULL,
  `prefix`       VARCHAR(16)  DEFAULT NULL COMMENT '明文前缀，便于识别',
  `token_hash`   VARCHAR(128) NOT NULL COMMENT 'sha256(明文令牌)',
  `enabled`      TINYINT(1) NOT NULL DEFAULT 1,
  `expires_at`   DATETIME(3) DEFAULT NULL,
  `last_used_at` DATETIME(3) DEFAULT NULL,
  `allow_cidr`   VARCHAR(512) DEFAULT NULL COMMENT '来源 IP 白名单，逗号分隔',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_ingest_tokens_token_hash` (`token_hash`),
  KEY `idx_ingest_tokens_project_id` (`project_id`),
  KEY `idx_ingest_tokens_prefix` (`prefix`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='事件投递令牌';

-- ---------- 事件 ----------
CREATE TABLE IF NOT EXISTS `events` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`   DATETIME(3) NULL,
  `updated_at`   DATETIME(3) NULL,
  `project_id`   BIGINT UNSIGNED NOT NULL,
  `token_id`     BIGINT UNSIGNED DEFAULT NULL,
  `source`       VARCHAR(64)  DEFAULT NULL COMMENT 'sentry / loki / k8s / custom',
  `level`        VARCHAR(32)  DEFAULT NULL COMMENT 'fatal / error / warn / info',
  `title`        VARCHAR(512) DEFAULT NULL,
  `message`      TEXT,
  `stack`        TEXT,
  `fingerprint`  VARCHAR(128) DEFAULT NULL,
  `payload`      TEXT,
  `status`       VARCHAR(16) NOT NULL DEFAULT 'received' COMMENT 'received/matched/ignored/dropped',
  `rule_id`      BIGINT UNSIGNED DEFAULT NULL,
  `dispose_msg`  VARCHAR(512) DEFAULT NULL,
  `occurred_at`  DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_events_project_id` (`project_id`),
  KEY `idx_events_source` (`source`),
  KEY `idx_events_level` (`level`),
  KEY `idx_events_fingerprint` (`fingerprint`),
  KEY `idx_events_status` (`status`),
  KEY `idx_events_occurred_at` (`occurred_at`),
  KEY `idx_events_project_occurred` (`project_id`, `occurred_at`),
  KEY `idx_events_fp_occurred` (`fingerprint`, `occurred_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='生产事件';

-- ---------- 规则 ----------
CREATE TABLE IF NOT EXISTS `rules` (
  `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`       DATETIME(3) NULL,
  `updated_at`       DATETIME(3) NULL,
  `project_id`       BIGINT UNSIGNED NOT NULL,
  `name`             VARCHAR(128) NOT NULL,
  `enabled`          TINYINT(1) NOT NULL DEFAULT 1,
  `priority`         INT NOT NULL DEFAULT 0 COMMENT '越大越优先',
  `levels`           VARCHAR(256)  DEFAULT NULL,
  `sources`          VARCHAR(256)  DEFAULT NULL,
  `keywords`         VARCHAR(1024) DEFAULT NULL COMMENT '任一命中即可',
  `all_keywords`     VARCHAR(1024) DEFAULT NULL COMMENT '必须全部命中',
  `exclude_keywords` VARCHAR(1024) DEFAULT NULL COMMENT '命中则排除（业务拒绝/第三方故障）',
  `exclude_sources`  VARCHAR(256)  DEFAULT NULL,
  `pattern`          VARCHAR(512)  DEFAULT NULL COMMENT '正则',
  `min_count`        INT NOT NULL DEFAULT 0 COMMENT '时间窗口内最小出现次数',
  `window_sec`       INT NOT NULL DEFAULT 0,
  `cooldown_sec`     INT NOT NULL DEFAULT 0 COMMENT '同一指纹冷却秒数',
  `action`           VARCHAR(16) NOT NULL DEFAULT 'fix' COMMENT 'fix / ignore',
  `repo_ids`         VARCHAR(256)  DEFAULT NULL,
  `model_id`         BIGINT UNSIGNED DEFAULT NULL,
  `fix_mode`         VARCHAR(16)  DEFAULT NULL COMMENT '为空继承项目配置',
  `max_retries`      INT NOT NULL DEFAULT 0,
  `prompt_template`  TEXT,
  `description`      VARCHAR(512) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_rules_project_id` (`project_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='过滤与修复规则';

-- ---------- 修复任务 ----------
CREATE TABLE IF NOT EXISTS `tasks` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at`     DATETIME(3) NULL,
  `updated_at`     DATETIME(3) NULL,
  `event_id`       BIGINT UNSIGNED DEFAULT NULL,
  `project_id`     BIGINT UNSIGNED NOT NULL,
  `repo_id`        BIGINT UNSIGNED NOT NULL,
  `rule_id`        BIGINT UNSIGNED DEFAULT NULL,
  `model_id`       BIGINT UNSIGNED DEFAULT NULL,
  `status`         VARCHAR(16) NOT NULL DEFAULT 'pending',
  `mode`           VARCHAR(16) NOT NULL DEFAULT 'semi',
  `retry`          INT NOT NULL DEFAULT 0,
  `branch`         VARCHAR(256) DEFAULT NULL,
  `base_commit`    VARCHAR(64)  DEFAULT NULL,
  `fix_commit`     VARCHAR(64)  DEFAULT NULL,
  `workspace`      VARCHAR(512) DEFAULT NULL,
  `pr_url`         VARCHAR(512) DEFAULT NULL,
  `summary`        TEXT,
  `diagnosis`      TEXT,
  `changed_files`  TEXT,
  `diff_stat`      VARCHAR(512) DEFAULT NULL,
  `patch`          TEXT,
  `error_msg`      TEXT,
  `dsh_model`      VARCHAR(128) DEFAULT NULL,
  `dsh_provider`   VARCHAR(128) DEFAULT NULL,
  `dsh_exit_code`  INT NOT NULL DEFAULT 0,
  `dsh_cmd`        TEXT,
  `input_context`  BIGINT NOT NULL DEFAULT 0,
  `output_context` BIGINT NOT NULL DEFAULT 0,
  `confirmed_by`   VARCHAR(64) DEFAULT NULL,
  `confirmed_at`   DATETIME(3) DEFAULT NULL,
  `confirm_note`   VARCHAR(512) DEFAULT NULL,
  `started_at`     DATETIME(3) DEFAULT NULL,
  `finished_at`    DATETIME(3) DEFAULT NULL,
  `duration_ms`    BIGINT NOT NULL DEFAULT 0,
  `stage`          VARCHAR(32) DEFAULT NULL COMMENT 'prepare/dsh/verify/commit/push/release',
  PRIMARY KEY (`id`),
  KEY `idx_tasks_event_id` (`event_id`),
  KEY `idx_tasks_project_id` (`project_id`),
  KEY `idx_tasks_repo_id` (`repo_id`),
  KEY `idx_tasks_rule_id` (`rule_id`),
  KEY `idx_tasks_status` (`status`),
  KEY `idx_tasks_created_at` (`created_at`),
  KEY `idx_tasks_status_created` (`status`, `created_at`),
  KEY `idx_tasks_project_created` (`project_id`, `created_at`),
  KEY `idx_tasks_repo_status` (`repo_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='修复任务';

-- ---------- 任务终端日志 ----------
CREATE TABLE IF NOT EXISTS `task_logs` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `task_id`    BIGINT UNSIGNED NOT NULL,
  `seq`        BIGINT NOT NULL DEFAULT 0,
  `stream`     VARCHAR(8) DEFAULT NULL COMMENT 'stdout / stderr / sys',
  `content`    TEXT,
  `created_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task_logs_task_id` (`task_id`),
  KEY `idx_task_logs_created_at` (`created_at`),
  KEY `idx_task_logs_task_seq` (`task_id`, `seq`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务终端日志';

-- ---------- 运行时配置 ----------
CREATE TABLE IF NOT EXISTS `settings` (
  `key`        VARCHAR(64) NOT NULL,
  `value`      TEXT,
  `updated_at` DATETIME(3) NULL,
  PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='键值配置';

SET FOREIGN_KEY_CHECKS = 1;
