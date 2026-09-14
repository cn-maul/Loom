# API 清单（当前真实路由）

> 以 `internal/app/router.go` 为唯一权威来源，本文为快照。改路由必须同步本文。

- **版本策略**：当前为 v1。所有 `/api` 响应带 `X-API-Version: 1` 头；不兼容变更将
  迁往 `/api/v2/...` 并保留 v1 一个弃用期。路由表面由
  `internal/app/routes.golden.txt` 固定（`internal/app` 契约测试自动校验，
  `LOOM_UPDATE_GOLDEN=1` 重新生成）。
- 认证：`server.auth_token` 非空时，所有 `/api` 路由要求 `X-Auth-Token` / `Authorization: Bearer <token>` / `?token=`；未携带返回 401。SPA 静态资源不设防。
- 分页约定：`limit` / `offset` 查询参数；匹配总数放 `X-Total-Count` 响应头；`limit<=0`
  表示不分页（全量返回）。显式 `limit` 超过 500 会被钳制到 500（`MaxPageLimit`）。
- 错误约定：统一 JSON `{ok, data, error:{code,message}}`；错误码见 `internal/models/errors.go`。
- 慢请求：任一请求超过 15 秒写一条 warning 日志（method/path/状态/耗时）——同步 AI
  端点慢是预期，日志抓的是意外。

## 人物与组织

```
POST   /api/persons                    创建人物
GET    /api/persons                    列表（q/org_id[|none]/relation/sort=recent|name|importance|created/limit/offset）
GET    /api/persons/:id                详情（含 traits）
PUT    /api/persons/:id                更新
DELETE /api/persons/:id                删除（级联）
POST   /api/organizations              创建组织
GET    /api/organizations              组织列表
PUT    /api/organizations/:id          更新
POST   /api/organizations/:id/archive  归档
POST   /api/organizations/:id/restore  恢复
DELETE /api/organizations/:id          删除
```

## 记录（事件）

```
POST   /api/events                              创建（落库后触发 AI 提取）
GET    /api/events                              检索（q/person_id[锚定或参与]/from/to/status/limit/offset）
GET    /api/events/:id                          详情
PUT    /api/events/:id                          人工修订（置 manually_edited；promises 需原样回传）
DELETE /api/events/:id                          删除
GET    /api/events/:id/participants             参与人列表
PUT    /api/events/:id/participants             整体替换参与人
DELETE /api/events/:id/participants/:personID   移除单个参与人（最后一位不可移除）
POST   /api/events/:id/extract                  重试提取（人工修订过需 {"force":true}，否则 409）
GET    /api/persons/:id/events                  该人物时间线
```

## 关系、任职与图谱

```
POST   /api/relationships                       创建关系边
GET    /api/relationships/types                 已用关系类型
PUT    /api/relationships/:id                   更新
DELETE /api/relationships/:id                   删除
GET    /api/persons/:id/relationships           某人的关系
GET    /api/graph                               全图（?limit= 人物节点上限，默认1000、硬上限5000；
                                                超出时 nodes_total/truncated 明示截断；含共同经历派生对）
POST   /api/persons/:id/positions               新增任职
GET    /api/persons/:id/positions               任职历史
GET    /api/organizations/:id/members           成员（current=true 现任筛选）
PUT    /api/positions/:id                       更新（写 end_date 即离任）
DELETE /api/positions/:id                       删除
```

## 画像与 AI 工具

```
GET    /api/persons/:id/traits                  画像列表
PUT    /api/traits/:id/verify                   标记准确/不准确
POST   /api/ai/reindex                          重建向量索引
POST   /api/ai/infer-hierarchy                  按职位 AI 推断上下级（body {org_id} 可选；生成 confirmed=0 的「上级」边，
                                                不覆盖已存在的有效关系；返回 {created, links}）
GET    /api/ai/embeddings/status                向量索引状态
GET    /api/ai/models                           对话端点的模型目录（string[]，经 rosetta 探测 /models，用当前已保存配置）
GET    /api/config                              配置 + privacy 面板（{config, privacy}）
PUT    /api/config                              更新 llm 块（async_extract/allow_remote 不受影响）
GET    /api/audit                               审计记录（?limit=，新→旧）
```

> 数据治理（清理孤儿向量 + 按保留期修剪审计日志）在每次启动时自动执行，
> 没有独立的手动触发端点；提取队列状态同样只存在于进程内部。

### 隐私与安全

- `GET /api/config` 的 `privacy` 面板：监听地址、AI 端点是否外部、是否发送记录原文、
  `allow_remote`、令牌/备份加密是否开启——只报事实，不回显密钥与令牌。
- `llm.allow_remote: false`（config.yaml）：拒绝把记录内容发送到非本机端点，chat、
  embeddings 与 rerank 三路 fail closed；错误信息给出改法。设置页不修改该开关。
- 备份加密：`backup.passphrase` 非空时快照为 `.db.enc`（AES-256-GCM + PBKDF2），
  `validate`/`restore` 透明解密；口令错误报「口令错误或文件已损坏」。暂存的恢复文件
  始终为明文 SQLite。
- 审计：全部变更请求与整库导出写入 `audit_log`（迁移 v9），含 method/path/状态码；
  审计写失败只告警。

### 异步提取（默认开启）

`POST /api/events` 先落库（`extraction_status=pending`）并立即返回 201，
`report.async=true`；提取、向量索引、画像刷新由进程内队列执行。记录本身的
提取结果照常从事件接口读取。启动时自动把遗留的 pending
记录重新入队（崩溃自愈）。同步模式可在 `config.yaml` 设 `llm.async_extract: false`；
`PUT /api/config` 不改变该开关。

## 建议

```
POST   /api/advice                              生成并持久化（含逐条证据与指纹）
GET    /api/advice                              历史（person_id/limit/offset，新的在前）
GET    /api/advice/:id                          详情（含算好的 source_stale、follow_up_ids）
DELETE /api/advice/:id                          删除（引用它的事项先写 stale 标记）
POST   /api/advice/:id/adopt                    采纳策略 → 转跟进事项
```

## 备份、导出与恢复

```
POST   /api/backups                             立即创建快照（VACUUM INTO，原子改名）
GET    /api/backups                             备份目录内快照列表（新→旧）
GET    /api/backups/status                      调度状态（last_run/last_error/next_run/backup_count）
POST   /api/backups/validate                    {"file": 名称或绝对路径} → 校验报告（202/422）
POST   /api/backups/restore                     {"file": ...} 校验并暂存，重启后生效（202）
GET    /api/export?mode=full|redacted           导出 JSON（redacted 脱敏；attachment 下载）
```

### 校验报告

`validate`/`restore` 返回 `{ok, integrity, schema_version, foreign_key_violations,
vec_memory_rows, tables, problems}`。校验失败（文件损坏、缺核心表、外键冲突）
restore 返回 422 且不落任何文件。`vec_memory` 缺失只是警告：索引可重建，
不阻断恢复事实数据。

### 恢复是两阶段的

暂存为 `<数据库文件>.restore-pending`，**下次启动时**在打开任何连接之前
原子换入（并清理旧 `-wal`/`-shm`）。运行中的进程永远不会换库。自动备份由
`config.yaml` 的 `backup:` 段控制（enabled/dir/interval_hours/keep），
首次快照在启动约 15 秒后执行。

## 跟进事项

```
GET    /api/follow-ups                          列表
POST   /api/follow-ups                          创建（owner/due_text/source_event_id）
GET    /api/follow-ups/:id                      详情（附延期历史）
PUT    /api/follow-ups/:id                      更新
GET    /api/follow-ups/:id/postponements        延期历史
POST   /api/follow-ups/:id/complete             完成（completion_note/completed_event_id）
POST   /api/follow-ups/:id/postpone             延期（写延期历史）
POST   /api/follow-ups/:id/wait                 等待对方
POST   /api/follow-ups/:id/cancel               取消
GET    /api/persons/:id/follow-ups              某人的事项
```

## 报告

```
POST   /api/reports                             生成快照（start/end 成对 或 week_of；默认最近七天）
GET    /api/reports                             历史 headline
GET    /api/reports/:id                         单份（payload 反序列化）
DELETE /api/reports/:id                         删除快照
```

## 静态资源

`GET /*` 命中嵌入的前端构建产物；未命中文件时回退 `index.html`（SPA 路由）。
