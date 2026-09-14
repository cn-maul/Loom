# 数据库 Schema（当前真实状态）

> 以 `internal/db/migrations.sql`（基线）与 `internal/db/migrate.go`（v2–v8 迁移）为准。本文是快照，改动 schema 时必须同步更新。

迁移版本表：`schema_migrations(version, name, applied_at)`，每个版本只执行一次；`runMigrations` 最后执行 `PRAGMA foreign_key_check` 验证引用完整性。

## 表清单（v8 后）

| 表 | 作用 | 关键列 |
|---|---|---|
| `organizations` | 组织 | name；v8 增 kind / description / archived_at（软归档） |
| `persons` | 人物档案 | name、relation（自由文本）、importance、notes、org_id、position；`is_self`（部分唯一索引：最多一人） |
| `events` | **事实来源**：互动记录 | raw_text（原文）、person_id（锚定人，可空，ON DELETE SET NULL）、event_date、summary、my_feeling、their_reaction、promises(JSON)、record_type、channel；v5 增 extraction_status / extraction_error / extracted_at / manually_edited / edited_at |
| `traits` | AI 画像（派生） | trait_key/value、confidence、source_event_ids(JSON)、verified(-1/0/1)；v5 增 source_stale / source_stale_reason |
| `person_relationships` | **已废弃**：人物关系边。建表保留、数据保留，无代码读写（ADR-011） | from/to_person_id、relation_type、direction(directed/undirected)、start/end_date、source_event_id、confirmed |
| `person_org_positions` | **已废弃**：结构化任职历史。建表保留、数据保留，无代码读写；职位见 `persons.position`（ADR-011） | person_id、org_id、role、start/end_date（end_date 非空即离任）、source |
| `event_participants` | 记录参与人 | (event_id, person_id) 主键、role（primary=锚定人回填） |
| `follow_ups` | 跟进事项 | title、due_date、due_text（期限原文）、status、owner、source_event_id、completion_note、completed_event_id；v6 增 source_advice_id / source_advice_stale(_reason) |
| `follow_up_postponements` | 延期历史 | follow_up_id、old/new_due_date、reason |
| `advice_sessions` | 建议会话（派生） | question、goal、answer(JSON 逐条证据)、used_event_ids、used_trait_ids、evidence_version(内容指纹)、retrieval_status、model、adopted_strategy_* |
| `report_snapshots` | 报告快照 | person_id/person_name(冗余)、start/end_date、status、generated_by、failure_reason、summary、event_count、payload(JSON 全部分段) |
| `vec_memory` | sqlite-vec 虚拟表 | embedding FLOAT[dim 可配置]、person_id、chunk_type、source_id |

## 核心约定

1. **事实 vs 派生**：`events.raw_text` 唯一事实来源；summary/traits/advice/report 均可丢弃重算。
2. **删除语义**：events→persons 为 `SET NULL`（删人不删共同记录）；participants/traits/follow_ups 对 persons 为 `CASCADE`；`source_event_id`/`source_advice_id` 类引用一律 `SET NULL` 并配 stale 标记。
3. **失效检测是读时指纹比对**：advice 的 `evidence_version` 与 traits 的 `source_stale` 以内容指纹/标记实现，改回原文可自动恢复「未失效」。
4. **提取状态机**：`extraction_status ∈ pending | succeeded | failed`；失败不清空原内容；`manually_edited` 保护人工修订（force 重试成功才清零）。
5. **ID 全 TEXT UUID**；时间 TEXT ISO8601；未知日期存 NULL，不推测填充。

## 向量索引

- 维度由配置 `llm.embed_dim` 决定，建表后不可改，切换模型需重建（`POST /api/ai/reindex`）。
- 初始化失败仅告警，检索退化为「最近事件 + 画像上下文」，`retrieval_status=vector_disabled`。
