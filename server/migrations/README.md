# server/migrations

表结构由程序启动时 GORM AutoMigrate 幂等创建，本目录不再存放 SQL。

建库与破坏性/手工迁移脚本按团队约定放在 [`docs/database/`](../../docs/database/)：

- `001_create_database.sql` 建库与授权（裸机首次安装用）
- `002`…`00N` 增量脚本，发版说明里会点名对应文件

各部署方式的落地位置：

| 方式 | SQL 位置 |
| --- | --- |
| Docker | 构建时 `COPY docs/database /app/migrations`，容器内 `/app/migrations` |
| 裸机 | `sudo cp docs/database/*.sql /opt/automedic/migrations/` |
| HK | `deploy/hk/remote.sh` 从 `docs/database/` 同步到 `/opt/automedic/migrations/` |

执行任何脚本前先备份：`pg_dump -Fc automedic > automedic-$(date +%F).dump`。
