# 变更日志

本文件记录 Loom 的显著变更。格式参考 Keep a Changelog；数据库 schema 版本与 API 版本
在对应条目中注明。

## [1.1.0] - 2026-09-12（架构演进改造：阶段 0–6）

按《Loom架构演进与改造计划书》完成七个阶段改造。目标：不重写、不拆微服务，
补齐可靠性、数据安全与模块边界，把 Loom 变成可长期使用的本地产品。

### 新增

- **阶段 0 基线**：`docs/architecture/`（decisions/schema/api），README 与实现同步，
  契约金样与冒烟固化为基线检查。
- **阶段 1 装配抽离**：`internal/app`（App 装配 + NewRouter），`cmd/server` 降为薄启动器
  并支持优雅关闭；`WarnIfExposed` 对非本机绑定给出安全告警（ADR-001/005）。
- **阶段 2 AI 异步化**：`internal/tasks` 单 worker 队列（退避重试、目标去重、优雅停机、
  `GET /api/tasks` 任务与统计视图）；`POST /api/events` 先落库立即返回 201，
  提取转后台；启动时 `RecoverPending` 崩溃自愈；`llm.async_extract` 配置（ADR-006）。
- **阶段 3 备份恢复**：`internal/backup`——`VACUUM INTO` 原子快照、调度备份、四项校验
  （integrity/核心表/迁移版本/外键）、两阶段恢复（`.restore-pending` 启动换入）、
  JSON 导出（full/redacted）；API `POST/GET /api/backups`、`/status|validate|restore`、
  `GET /api/export`；`backup:` 配置段（ADR-007）。
- **阶段 4 隐私安全**：`server.auth_token` 访问控制（常量时间比较、前端令牌门）、
  `llm.allow_remote` 数据边界（非本机端点 fail closed）、备份加密（AES-256-GCM +
  PBKDF2，210k 迭代）、审计日志（迁移 **v9**，`GET /api/audit`）、
  `GET /api/config` 附 privacy 面板、日志脱敏（ADR-008）。
- **阶段 5 契约与性能**：`routes.golden.txt` 契约金样（71+ 路由，`LOOM_UPDATE_GOLDEN=1`
  再生成）、错误包络 `{ok:false, error:{code,message}}` 断言、`X-API-Version: 1` 版本头、
  分页 `limit` 钳制（≤500）、图谱 `truncated` 截断标记（默认 1000/硬上限 5000）、
  队列 stats 指标、慢请求（>15s）告警、大数据量回归测试（2000 人/6000 记录/3000 跟进，
  1 秒预算）（ADR-009）。
- **阶段 6 运营**：`scripts/release.ps1|sh` 发布管线（vet → 全量测试 → 前端/后端构建 →
  临时库迁移冒烟：版本头/404 包络/维护端点）；`scripts/drill.ps1|sh` 月度备份恢复演练
  （建数据→快照→校验→删除→两阶段恢复→重启验证，全自动、不触真实数据）；
  `POST /api/maintenance/cleanup` 数据治理（孤儿向量清理 + 审计日志保留期
  `maintenance.audit_retention_days`，启动时自动执行）；`docs/RELEASE.md` 发布检查清单、
  迁移检查与回滚方案（ADR-010）。

### 变更

- `main.go` 238 行 → 约 75 行（装配抽离）。
- `GET /api/graph` 响应增加 `nodes_total` / `truncated` 字段。
- `GET /api/tasks` 响应增加 `stats` 对象。
- README 新增 5.5 备份恢复、5.6 隐私安全、5.7 运维与发布章节。

### 不变的承诺

- 单体单二进制、SQLite 单连接、事实/派生分离、个人规模优先（ADR-001~004）。
- 未引入微服务、消息中间件、复杂权限系统。
