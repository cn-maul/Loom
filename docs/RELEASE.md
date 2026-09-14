# 发布与运营手册

> 适用范围：Loom 本地单机部署。配套脚本：`scripts/release.ps1`（发布管线）、`scripts/drill.ps1`（恢复演练）。相关决策：`docs/architecture/decisions.md` ADR-010。

## 一、发布检查清单

每次发布（无论是新版本还是小改动重新部署）按以下顺序执行。**前 3 项由 `scripts/release.ps1` 一条命令完成。**

| # | 检查项 | 方式 | 通过标准 |
|---|---|---|---|
| 1 | 静态检查 | `release.ps1`（内部 `go vet ./...`） | 无输出、退出码 0 |
| 2 | 全量测试 | `release.ps1`（内部 `go test ./...`） | 全部 ok；**含契约金样**：改过路由必须先 `LOOM_UPDATE_GOLDEN=1 go test ./internal/app/` 重新生成 `routes.golden.txt` 并同步 `docs/architecture/api.md` |
| 3 | 构建产物 | `release.ps1`（前端 build → 拷贝 dist → `go build`） | `relationship.exe` 生成，`web/dist` 已内嵌 |
| 4 | 迁移冒烟 | `release.ps1` 第 5 步（临时库真实启动） | 服务就绪、`X-API-Version: 1`、404 包络正确、维护端点 200 |
| 5 | 迁移版本核对 | 升级后的库执行 `select max(version) from schema_migrations;`（有 sqlite3 CLI 时），或看启动日志无迁移 WARNING | 与 `docs/architecture/schema.md` 记载的版本一致 |
| 6 | 恢复演练 | `pwsh scripts/drill.ps1` | 最后一行 `DRILL PASSED` |
| 7 | 变更日志 | 更新 `CHANGELOG.md`（日期 + 变更条目 + 是否涉及 schema/API） | 已记录 |
| 8 | 配置样例 | 若新增/变更配置项，同步 `config.yaml` 注释与 README | 已同步 |

## 二、数据库迁移检查

- 迁移只前进不后退：`schema_migrations` 记录每个版本，启动时自动补齐缺口，最后跑 `PRAGMA foreign_key_check`。
- **升级前**：确认最近一次自动备份存在且时间合理（`GET /api/backups`，或看 `backups/` 目录）。升级前手动 `POST /api/backups` 打一个快照最稳妥。
- **升级后**：核对 schema 版本（见清单第 5 项）；启动日志无 `WARNING:` 前缀的迁移报错。
- 新 schema 版本合入时必须同步更新 `docs/architecture/schema.md` 与 `internal/db/migrate_test.go` 的版本断言。

## 三、回滚方案

**不做 schema 降级。** 回滚 = 用快照恢复数据 + 换回旧二进制：

1. 停止 Loom。
2. `POST /api/backups` 打当前快照（防止回滚后丢新数据），或直接使用升级前快照。
3. 换回旧版 `relationship.exe`。
4. 启动前把快照放入暂存：调旧版没有该端点时，手动把备份文件复制为 `data/relationship.db.restore-pending`（两阶段恢复约定），启动即换入。
5. 启动后抽验核心数据（人物、记录、跟进）。

## 四、月度备份恢复演练

**频率：每月至少一次；改动备份/恢复/迁移路径后加跑一次。**

```powershell
pwsh scripts/drill.ps1
```

脚本在临时目录全链路执行（不接触真实数据）：

```
建数据（人物+记录）→ POST /api/backups 快照 → validate 四项校验
→ 删除记录（模拟灾难；注意删人是 SET NULL，记录会保留，故删记录本身）
→ restore 暂存（202）→ 重启服务（.restore-pending 换入）→ 验证人物与记录恢复
```

通过标准：最后一行 `DRILL PASSED`。演练失败时脚本保留现场目录供排查，修复后重跑。

人工抽查（建议每季度）：对**真实备份目录**里最新快照跑一次 `POST /api/backups/validate`，确认自动备份本身没有静默坏掉（调度器状态另见 `GET /api/backups/status`）。

## 五、数据治理

| 对象 | 机制 | 说明 |
|---|---|---|
| 孤儿向量 | 每次启动自动清理 | 删除 person/event/trait 已不存在的 `vec_memory` 行；未知 chunk 类型不动 |
| 审计日志 | `maintenance.audit_retention_days`（默认 0 = 永久保留） | 显式配置后按窗口修剪；安全证据默认不清 |
| 失败提取任务 | 无需清理 | 队列历史自剪 500 条上限；失败记录保留原文——原文是事实 |
| 备份堆积 | `backup.keep` 保留个数 | 调度器自动滚动删除最旧快照 |

清理没有手动入口：只在启动时执行一遍，报告写进启动日志（`startup maintenance: removed N orphan vector(s), M expired audit row(s)`）。需要立刻清理就重启进程。

## 六、演进边界（重申）

没有数据证明之前不引入：微服务、消息中间件、复杂权限系统。SSE、全文检索、更强本地模型按真实使用情况再评估。新功能的硬约束是不绕过「事实 / 派生物 / 证据」的设计边界（ADR-003）。
