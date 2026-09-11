# AI 人际关系助手 · 最终设计书

版本：v1.0  
目标：单人自用，纯文字输入，单二进制部署，3 周跑通核心闭环  
原则：够用就好，不做企业级架构，能实现 80% 效果即可

---

## 一、产品定位

一个个人关系记忆助手。用户手动输入与他人互动的文字记录，AI 从中提取事件、画像、承诺，形成跨会话的长期记忆。当用户需要沟通建议时，AI 基于历史记录给出可追溯的判断和话术。

核心闭环：

```
记录 → AI 提取 → 结构化存储 → 向量索引 → 检索 → 建议 → 用户纠正
```

不做的事：
- 不接入聊天软件，不自动导入，不 OCR
- 不做多用户，不做权限系统
- 不做加密，不做云端同步（本地优先）
- 不做移动端，只做 Web（本地浏览器访问）

---

## 二、技术栈

 层级  选型  说明 
---------
 语言  Go 1.27  泛型方法、`uuid` 标准库、`encodingjsonv2` 
 Web 框架  Echo v4  绑定顺手，中间件成熟，文档好 
 数据库  SQLite（modernc.orgsqlite）  纯 Go，零 CGO，单文件 
 向量扩展  sqlite-vec（通过 `modernc.orgsqlitevec` 加载）  库内相似度搜索，无需外部向量库 
 LLM 接入  Rosetta（你自己的库）  统一 OpenAI  Anthropic  兼容协议 
 前端  Vite + React 19 + TypeScript  React 19 严格模式 
 样式  Tailwind CSS v4  零配置 
 组件  shadcnui  按需复制，Tailwind v4 兼容 
 打包  goembed  前端产物嵌入二进制 

最终交付物：一个 `relationship` 二进制文件 + 一个 `data` 目录（存放 SQLite 文件）。

---

## 三、整体架构

```
┌──────────────────────────────────────────────────┐
│                单个 Go 二进制                      │
│                                                   │
│  ┌────────────────────────────────────────────┐  │
│  │            Echo HTTP Server                 │  │
│  │  api  → JSON API                         │  │
│  │        → 前端静态文件 (goembed)           │  │
│  └────────────────────────────────────────────┘  │
│                                                   │
│  ┌────────────────────────────────────────────┐  │
│  │         Service Layer                       │  │
│  │  PersonService  EventService  AIService   │  │
│  └────────────────────────────────────────────┘  │
│                                                   │
│  ┌────────────────────────────────────────────┐  │
│  │      AI Pipeline (自实现编排)                │  │
│  │  Extractor → Embedder → TraitUpdater        │  │
│  │  Retriever → AdviceGenerator                │  │
│  └────────────────────────────────────────────┘  │
│                                                   │
│  ┌──────────────────────┐  ┌─────────────────┐  │
│  │  Rosetta (LLM 接入)   │  │  SQLite + vec   │  │
│  │  Chat  Embed        │  │  relationship.db│  │
│  └──────────────────────┘  └─────────────────┘  │
└──────────────────────────────────────────────────┘
```

---

## 四、数据库设计

SQLite 单文件，4 张核心表 + 1 张向量虚拟表。所有 ID 用 `TEXT` 存 UUID 字符串，时间用 `TEXT` 存 ISO8601。

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- 人物档案
CREATE TABLE persons (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    relation    TEXT,               -- 同事  家人  朋友  伴侣
    importance  INTEGER DEFAULT 3,  -- 1-5
    notes       TEXT,
    created_at  TEXT DEFAULT (datetime('now')),
    updated_at  TEXT DEFAULT (datetime('now'))
);

-- 事件记录（原始记录层，唯一事实来源）
CREATE TABLE events (
    id              TEXT PRIMARY KEY,
    person_id       TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    raw_text        TEXT NOT NULL,      -- 用户原始输入，永不修改
    event_date      TEXT NOT NULL,      -- YYYY-MM-DD
    summary         TEXT,               -- AI 生成摘要
    my_feeling      TEXT,               -- AI 提取：我的感受
    their_reaction  TEXT,               -- AI 提取：对方反应
    promises        TEXT DEFAULT '[]',  -- JSON 数组：承诺待办
    created_at      TEXT DEFAULT (datetime('now'))
);
CREATE INDEX idx_events_person_date ON events(person_id, event_date DESC);

-- 人物画像（AI 维护，用户可纠正）
CREATE TABLE traits (
    id                TEXT PRIMARY KEY,
    person_id         TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    trait_key         TEXT NOT NULL,     -- 沟通偏好
    trait_value       TEXT NOT NULL,     -- 私下沟通更有效
    confidence        REAL DEFAULT 0.5,  -- 0-1
    source_event_ids  TEXT DEFAULT '[]', -- JSON 数组，可追溯
    verified          INTEGER DEFAULT 0, -- 0 未确认  1 用户确认  -1 用户否认
    updated_at        TEXT DEFAULT (datetime('now')),
    UNIQUE(person_id, trait_key)
);
CREATE INDEX idx_traits_person ON traits(person_id);

-- 向量记忆表（sqlite-vec 虚拟表）
CREATE VIRTUAL TABLE vec_memory USING vec0(
    id          TEXT PRIMARY KEY,
    embedding   FLOAT[1536],     -- 维度可配置，见 §6.2
    person_id   TEXT,
    chunk_type  TEXT,            -- event_summary  trait  advice
    source_id   TEXT             -- 对应 events.id 或 traits.id
);
```

核心原则：

- `events.raw_text` 是唯一事实来源，AI 产出的所有内容都是可丢弃、可重生成的派生数据。
- 如果 AI 画像错误，用户删掉对应 trait 或标记 `verified = -1`，下次更新时重新评估。
- 删除事件时级联删除对应的 `vec_memory` 条目，并触发该人物画像重新评估（可选，手动触发）。

---

## 五、AI 管线

### 5.1 记录事件时（同步执行）

```
用户提交 raw_text + person_id + event_date
    │
    ▼
[1] Extractor：调用 LLM 提取结构化字段
    输出 JSON { summary, my_feeling, their_reaction, promises[] }
    │
    ▼
[2] 写入 events 表（raw_text + 提取结果）
    │
    ▼
[3] Embedder：对 summary 生成 embedding
    写入 vec_memory (chunk_type = 'event_summary')
    │
    ▼
[4] TraitUpdater：
    检索该 person 最近 10 条 summary + 现有 traits
    调用 LLM 输出新增修改的 traits
    写入 traits 表（标注 source_event_ids）
    对新 trait 也生成 embedding 写入 vec_memory
    │
    ▼
返回 201 + 完整事件对象
```

个人使用，单次请求耗时 3-15 秒可接受。不做异步队列，前端显示 loading 即可。

### 5.2 请求建议时

```
用户提交 person_id + question
    │
    ▼
[1] 混合检索：
    a) 问题 embedding → vec_memory 相似度 Top-5（限定 person_id）
    b) 该 person 全部 traits（verified != -1）
    c) 该 person 最近 5 条事件原文
    │
    ▼
[2] 组装 Prompt：
    System 你是沟通顾问，基于以下事实给建议，不要编造。
    Context traits + 检索到的事件摘要 + 最近事件
    User question
    │
    ▼
[3] 调用 LLM，要求结构化 JSON 输出：
    {
      situation ...,
      other_perspective ...,
      risks [...],
      strategies [
        { name 直接沟通, script ..., pros ..., cons ... }
      ],
      evidence_event_ids [..., ...],
      follow_up ...
    }
    │
    ▼
[4] 返回建议，evidence 关联的事件可在前端点击跳转
```

### 5.3 Rosetta 调用方式

Rosetta 负责所有 LLM 通信，业务层只依赖其统一接口。

```go
 初始化（main.go）
client, err = rosetta.New(
    rosetta.WithEndpoint(cfg.LLMEndpoint),    如 httplocalhost11434 或 httpsapi.deepseek.com
    rosetta.WithAPIKey(cfg.LLMAPIKey),
    rosetta.WithProtocol(cfg.LLMProtocol),    openai  anthropic  responses
)

 事件提取（示意）
func (s AIService) ExtractEvent(ctx context.Context, raw string) (EventExtraction, error) {
    resp, err = s.client.Chat(ctx, &rosetta.ChatRequest{
        Model s.cfg.ExtractModel,            如 qwen38b 或 deepseek-chat
        Messages []rosetta.Message{
            rosetta.System(extractPrompt),
            rosetta.User(raw),
        },
        ResponseFormat &rosetta.ResponseFormat{Type json_object},
    })
    if err != nil { return nil, err }
    var out EventExtraction
    if err = json.Unmarshal([]byte(resp.Text()), &out); err != nil {
        return nil, fmt.Errorf(parse extraction %w, err)
    }
    return &out, nil
}

 流式生成建议（可选，用于前端展示思考过程）
func (s AIService) StreamAdvice(ctx context.Context, req AdviceRequest) (-chan rosetta.Event, error) {
    return s.client.ChatStream(ctx, &rosetta.ChatRequest{ ... })
}
```

关于 Embedding：Rosetta 是否已封装 embedding 接口我不确定。如果尚未支持，可以在 `AIService` 内单独实现一个薄封装，调用 OpenAI 兼容的 `v1embeddings` 或 Ollama 的 `apiembeddings`。这部分代码量约 50 行。

### 5.4 模型配置建议

```yaml
# config.yaml
llm
  endpoint httplocalhost11434    # Ollama
  protocol openai                     # Ollama 兼容 OpenAI 协议
  extract_model qwen38b              # 事件提取用本地小模型
  advice_model deepseek-chat          # 建议生成可切云端
  embed_model nomic-embed-text        # 768 维
  embed_dim 768
```

如果一开始没有本地 Ollama，可以先全用云端 API（DeepSeek 性价比高），后续再切换。

`api_key` 可以省略：本地端点（Ollama、LM Studio）不需要凭证，但底层的 chat 客户端
拒绝空密钥，所以未配置时会自动使用一个占位值，否则每次提取都会在发出请求前就以
`api key is required` 失败。云端服务照常在配置里或设置页填写真实密钥。

---

## 六、关键技术细节

### 6.1 SQLite + sqlite-vec 初始化

```go
import (
    databasesql
    _ modernc.orgsqlite
    _ modernc.orgsqlitevec    空导入自动注册 sqlite-vec 扩展
)

func OpenDB(path string) (sql.DB, error) {
    db, err = sql.Open(sqlite, path+_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON))
    if err != nil { return nil, err }
    db.SetMaxOpenConns(1)   SQLite 单写，避免锁竞争
    return db, nil
}
```

注意：`SetMaxOpenConns(1)` 是 SQLite 的关键。个人使用场景吞吐量极低，单连接完全够用，还能避免 `database is locked` 错误。

### 6.2 向量维度可配置

`vec_memory` 表的 `FLOAT[N]` 维度必须在建表时确定。为了支持不同的 embedding 模型，有两个方案：

- 方案 A（推荐）：默认 1536（OpenAI text-embedding-3-small），在配置里固定。切换模型时需要重建表。
- 方案 B：如果频繁切换，改为多张表 `vec_memory_768`  `vec_memory_1536`，按配置选表。

个人使用选 A，简单直接。

### 6.3 向量检索

```sql
-- 查询某人相关的 Top-5 记忆
SELECT vm.source_id, vm.chunk_type, vm.distance
FROM vec_memory vm
WHERE vm.embedding MATCH 
  AND vm.person_id = 
  AND k = 5
ORDER BY vm.distance;
```

`` 传入问题文本的 embedding（用 `[]byte` 或 `[]float32` 序列化，取决于驱动）。

### 6.4 结构化输出容错

LLM 返回的 JSON 可能包含 markdown 代码块标记。统一用一个辅助函数清洗：

```go
func parseJSON[T any](raw string) (T, error) {
    raw = strings.TrimSpace(raw)
    raw = strings.TrimPrefix(raw, ```json)
    raw = strings.TrimPrefix(raw, ```)
    raw = strings.TrimSuffix(raw, ```)
    raw = strings.TrimSpace(raw)
    var out T
    if err = json.Unmarshal([]byte(raw), &out); err != nil {
        return nil, err
    }
    return &out, nil
}
```

---

## 七、API 设计

```
POST   apipersons                 创建人物
GET    apipersons                 列出人物（含最近互动时间；支持 q/org_id/
                                   relation/sort/limit/offset 检索分页，
                                   匹配总数在 X-Total-Count 头）
GET    apipersonsid             人物详情（含 traits）
PUT    apipersonsid             更新人物
DELETE apipersonsid             删除人物（级联）

GET    apipersonsidevents      该人物的事件时间线
POST   apievents                  创建事件（触发 AI 管线）
GET    apievents                  记录检索：person_id/from/to/q/status/limit/offset，
                                   匹配总数在 X-Total-Count 头
GET    apieventsid              事件详情
PUT    apieventsid              人工修订（置 manually_edited，承诺原样回传否则会清空）
DELETE apieventsid              删除事件
POST   apieventsidextract       重新提取（人工修订过的需 {"force":true}，否则 409）

GET    apipersonsidtraits      人物画像列表
PUT    apitraitsidverify       标记画像准确不准确

POST   apiadvice                 生成建议并保存
GET    apiadvice                 建议历史
GET    apiadviceid              单条建议（含来源失效状态）
DELETE apiadviceid              删除建议
POST   apiadviceidadopt         采纳策略，转为跟进事项

POST   apireports                 生成报告并保存快照（默认最近七天，支持
                                   start/end 或 week_of 指定自然周）
GET    apireports                 报告历史（headline，不带分段）
GET    apireportsid              单份报告（含全部分段）
DELETE apireportsid              删除快照

GET    apiconfig                  读取当前 LLM 配置
PUT    apiconfig                  更新配置（切换模型endpoint）
```

所有响应统一格式：

```json
{
  ok true,
  data { ... },
  error null
}
```

```json
{
  ok false,
  data null,
  error { code LLM_ERROR, message ... }
}
```

---

### 7.1 结构化关系、任职与多人记录（已实现）

除人物档案上的自由文本 `relation` 外，另有三张结构化表。它们与人物档案的旧字段并存，
旧字段只作为档案描述，不再用来推断关系边。

```sql
-- 人物—人物关系（有向/无向、起止时间、来源记录、确认状态）
CREATE TABLE person_relationships (
    id              TEXT PRIMARY KEY,
    from_person_id  TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    to_person_id    TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    relation_type   TEXT NOT NULL,                     -- 上级 / 同事 / 客户 …
    direction       TEXT NOT NULL DEFAULT 'directed',  -- directed | undirected
    start_date      TEXT,
    end_date        TEXT,                              -- 非空即已结束，保留历史
    source_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
    confirmed       INTEGER NOT NULL DEFAULT 0,
    notes           TEXT,
    created_at      TEXT DEFAULT (datetime('now')),
    updated_at      TEXT DEFAULT (datetime('now'))
);

-- 人物—组织任职（同一人可多组织、多段历史）
CREATE TABLE person_org_positions (
    id          TEXT PRIMARY KEY,
    person_id   TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role        TEXT,
    start_date  TEXT,
    end_date    TEXT,        -- 非空即已离任；NULL 表示现任
    source      TEXT,
    notes       TEXT,
    created_at  TEXT DEFAULT (datetime('now')),
    updated_at  TEXT DEFAULT (datetime('now'))
);

-- 记录参与人（一条记录只存一次，从每位参与人都能看到）
CREATE TABLE event_participants (
    event_id   TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    person_id  TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    role       TEXT,      -- primary 为锚定人，其余为参与方式
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (event_id, person_id)
);
```

关键约定：

- `events.person_id` 是可空的「锚定人」，外键为 `ON DELETE SET NULL`。删除某人不会连带删掉
  其他人也参与的记录；参与人行会随之级联清理。
- 「共同经历」由 `event_participants` 自连接派生（`GET /api/relationships/co-attendance`），
  只作为图上的一种关联展示，绝不自动写成朋友、同事或上下级关系。
- `persons.is_self` 标记「我」，全局最多一人（部分唯一索引保证）；关系边因此可以说清主体。
- 未知时间一律存 NULL，不用推测日期填充。旧 `org_id` 会迁移成一段起止时间未知的任职。

对应接口：

```
POST/GET/PUT/DELETE api/relationships            关系边（List 支持 person_id/type/confirmed/active/q/limit/offset）
GET                 api/relationships/types      已用过的关系类型
GET                 api/relationships/co-attendance  派生的共同经历
GET                 api/persons/:id/relationships

POST                api/persons/:id/positions    新增任职
GET                 api/persons/:id/positions    某人的任职历史
GET                 api/organizations/:id/members?current=true   现任/历史成员
PUT/DELETE          api/positions/:id            改期或结束任职

GET/PUT             api/events/:id/participants  参与人列表 / 整体替换
DELETE              api/events/:id/participants/:personID
GET                 api/events?person_id=&from=&to=&q=&limit=&offset=   记录检索与分页
```

迁移采用版本号表 `schema_migrations`，每个版本只执行一次，可重复启动；
详细定义见 `internal/db/migrate.go`，基线表结构见 `internal/db/migrations.sql`。

---

### 7.2 跟进事项闭环与记录提取状态（已实现）

**跟进事项的闭环字段。** 事项不再只有「标题 + 期限 + 状态」，而是记住它从哪来、谁的事、
原话怎么说、最后出了什么结果：

```sql
ALTER TABLE follow_ups ADD COLUMN source_event_id     TEXT REFERENCES events(id) ON DELETE SET NULL;
ALTER TABLE follow_ups ADD COLUMN owner               TEXT;  -- 谁的球：我 / 对方
ALTER TABLE follow_ups ADD COLUMN due_text            TEXT;  -- 期限原文，如「下周三前」
ALTER TABLE follow_ups ADD COLUMN completion_note     TEXT;  -- 完成结果
ALTER TABLE follow_ups ADD COLUMN completed_event_id  TEXT REFERENCES events(id) ON DELETE SET NULL;

-- 延期历史。事项本身只存最新期限，没有这张表，被推迟掉的原期限就永久丢失。
CREATE TABLE follow_up_postponements (
    id            TEXT PRIMARY KEY,
    follow_up_id  TEXT NOT NULL REFERENCES follow_ups(id) ON DELETE CASCADE,
    old_due_date  TEXT,        -- 首次设定期限时为 NULL
    new_due_date  TEXT NOT NULL,
    reason        TEXT,
    created_at    TEXT DEFAULT (datetime('now'))
);
```

约定：

- `due_text` 与 `due_date` 并存：前者保留记录里的原话（「下周三前」），后者是确认后的日期，
  排序与筛选只用后者。含糊的承诺因此既能存下来，也能被查到。
- 完成时可以同时记下结果与「完成时写的那条新记录」（`completed_event_id`），
  这样完成的不只是状态，还有产出。
- 重复完成不会刷新 `completed_at`，但允许补记后到的结果；一旦事项被改回未完成，
  `completion_note` / `completed_event_id` / `completed_at` 会被一并清掉，避免「未完成却有结果」。
- `source_event_id` 与 `completed_event_id` 都是 `ON DELETE SET NULL`：删掉记录不会连带删掉事项，
  但引用会断掉。若希望「来源已删除」也保留标记，需要给 `follow_ups` 再加一列 source_stale。
- 创建时校验被引用的记录确实存在（不存在返回 404），所以不会写入悬空引用；创建时传入的结果字段会被忽略。

```
POST   api/follow-ups                          body 可含 owner / due_text / source_event_id
PUT    api/follow-ups/:id
POST   api/follow-ups/:id/complete             body {completion_note?, completed_event_id?}
POST   api/follow-ups/:id/postpone             body {due_date, reason?}  写入延期历史
POST   api/follow-ups/:id/wait                 等待对方（新增，此前只能靠 PUT）
POST   api/follow-ups/:id/cancel
GET    api/follow-ups/:id                      详情，附带 postponements 历史
GET    api/follow-ups/:id/postponements        单独的延期历史
```

**记录的提取状态与人工修订保护。** 记录先落库、再交给模型提取，所以「提取失败」必须是一种
可见状态，而不是一个空的摘要：

```sql
ALTER TABLE events ADD COLUMN extraction_status TEXT;                        -- pending | succeeded | failed
ALTER TABLE events ADD COLUMN extraction_error  TEXT;
ALTER TABLE events ADD COLUMN extracted_at      TEXT;
ALTER TABLE events ADD COLUMN manually_edited   INTEGER NOT NULL DEFAULT 0;  -- 提取结果是否由人手改写
ALTER TABLE events ADD COLUMN edited_at         TEXT;

ALTER TABLE traits ADD COLUMN source_stale        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE traits ADD COLUMN source_stale_reason TEXT;
```

约定：

- 记录落库即为 `pending`；提取成功写 `succeeded` 并记时间，失败写 `failed` 与错误信息，
  但**不会清空原有内容**——失败的重试只报告这次尝试。
- `PUT /api/events/:id` 是人工修订，会置 `manually_edited = 1`。此后
  `POST /api/events/:id/extract` 会被拒绝（409），除非显式 `{"force": true}`。
- 强制重试成功时 `manually_edited` 会被清零：此时库存内容已经是机器产出，
  这个标记描述的是「内容由谁写的」，而不是「谁点过按钮」。强制重试若失败，人工内容与标记都保留。
- 来源记录被改写原文或删除时，引用它的画像会被标为 `source_stale`（附原因），
  而不是被悄悄丢弃——「这条结论的依据已经变了」本身就是要给用户看的信息。
  画像重新推导成功后标记自动清除。
- 记录列表支持 `GET /api/events?status=pending|succeeded|failed`，用于挑出需要重试的记录。

---

### 7.3 建议持久化与证据映射（已实现）

建议此前只是一个响应体：页面一刷新，问过什么、用哪个模型、依据了哪些记录，全都没了，
「当时为什么这么判断」无从回答。现在一次生成会落库成一条建议会话（`advice_sessions`），
每一条结论各自带着自己的依据。

```sql
CREATE TABLE advice_sessions (
  id                     TEXT PRIMARY KEY,
  person_id              TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
  question               TEXT NOT NULL,
  goal                   TEXT,               -- 用户想达成的目标，和问题分开存
  answer                 TEXT NOT NULL,      -- 完整回答（含逐条依据）的 JSON
  used_event_ids         TEXT NOT NULL,      -- 实际放进提示词的记录
  used_trait_ids         TEXT NOT NULL,      -- 实际放进提示词的画像
  evidence_version       TEXT NOT NULL,      -- 每条依据记录在生成时的内容指纹
  retrieval_status       TEXT,               -- 见下
  model                  TEXT,
  adopted_strategy_index INTEGER,            -- 采纳了哪一条策略
  adopted_strategy_name  TEXT,
  created_at TEXT, updated_at TEXT
);

ALTER TABLE follow_ups ADD COLUMN source_advice_id             TEXT REFERENCES advice_sessions(id) ON DELETE SET NULL;
ALTER TABLE follow_ups ADD COLUMN source_advice_stale          INTEGER NOT NULL DEFAULT 0;
ALTER TABLE follow_ups ADD COLUMN source_advice_stale_reason   TEXT;
```

```json
// 回答的形状：逐条结论 + 各自的依据
{
  "situation":         {"text": "...", "evidence_event_ids": ["事件ID"]},
  "other_perspective": {"text": "...", "evidence_event_ids": []},
  "risks":             [{"text": "...", "evidence_event_ids": ["事件ID"]}],
  "strategies":        [{"name": "...", "script": "...", "pros": "...", "cons": "...",
                         "evidence_event_ids": ["事件ID"]}],
  "follow_up":         {"text": "...", "evidence_event_ids": []}
}
```

约定：

- **逐条结论各自带依据**，而不是整篇一个 `evidence_event_ids`。空数组表示「没有直接记录依据」，
  界面必须明说这是推断，不能把推断包装成事实。模型编造的 ID 在入库前被过滤掉。
- **`evidence_version` 是内容指纹，不是时间戳**：对「实际放进提示词的每条记录」按
  `raw_text / summary / my_feeling / their_reaction / promises / event_date` 生成 FNV 指纹。
  读的时候拿它和当前行比对——对不上就是「来源记录已修改」，查不到就是「来源记录已删除」。
  所以 `source_stale` 是**读时算出来的**，永远不会和记录的真实状态脱节；
  改写后又改回原样，也会自动恢复成「未失效」。
- **检索结果分四态**：`vector_used` / `no_relevant_evidence` / `retrieval_failed` / `vector_disabled`。
  「没启用向量检索」「检索坏了」「确实没有相关记录」是三件不同的事，用户要做的事完全不同，
  不能塌缩成一个 `vector_used: false`。
- **采纳策略 = 转成跟进事项**：`POST /api/advice/:id/adopt` 把选定策略转成一条事项，
  策略话术写进事项描述（`description`），事项带 `source_advice_id` 指回建议。
- **删建议不删行动**：删除时先给引用它的事项写 `source_advice_stale` 与原因，再删会话
  （外键把 `source_advice_id` 置空）。这与 `source_event_id` 的处理相反是有意的：
  采纳的策略是行动的前提，「前提没了」这件事值得留在事项上。

```
POST   api/advice                   生成并保存，返回整条会话（含 id）
GET    api/advice?person_id=&limit=&offset=   建议历史，新的在前
GET    api/advice/:id               单条会话（含算好的 source_stale 与 follow_up_ids）
DELETE api/advice/:id               删除；引用它的事项被标记来源已删除
POST   api/advice/:id/adopt         body {strategy_index, title?, owner?, due_text?, due_date?}
```

### 7.4 周报重做：结构化报告与快照（已实现）

旧周报只有一段模型文字和两个计数，没有明确的时间边界，明细也不能跳转。现在报告分两层：
**先从数据库聚合出结构化事实，再让模型只负责叙述**——模型坏了只损失措辞，不损失事实。

```sql
-- 迁移 v7：报告快照。payload 存完整结构化报告；列表只读 headline 列，
-- 回看一年报告不会带出一年的分段数据。
CREATE TABLE report_snapshots (
  id TEXT PRIMARY KEY,
  person_id TEXT REFERENCES persons(id) ON DELETE SET NULL,
  person_name TEXT,               -- 冗余保存，人物删除后报告仍可读
  start_date TEXT NOT NULL,       -- 报告实际覆盖的窗口（解析后写回）
  end_date   TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'succeeded',   -- succeeded / failed（叙述失败）
  generated_by TEXT,              -- 写叙述的模型，或 local
  failure_reason TEXT,
  summary TEXT,                   -- 叙述；失败时是大纲本身
  event_count INTEGER NOT NULL DEFAULT 0,
  payload TEXT NOT NULL DEFAULT '{}',
  generated_at TEXT DEFAULT (datetime('now'))
);
```

**窗口规则**（都在后端解析，写入快照的是实际覆盖的窗口）：

- 默认最近七天（含今天）；`week_of` 展开为该日期所在的自然周（周一至周日）；
  `start`/`end` 必须成对出现、跨度上限 366 天。
- 一个请求只做一个窗口。「上周」这种口语只在后端有一份定义，前端只是调用方。

**未完成事项分五桶**，互斥且穷尽，事项不会从报告里悄悄消失：
`overdue`（逾期）/ `due_soon`（本期内到期）/ `waiting`（等对方回复）/ `carried_over`
（期前已开、尚未到期）/ `upcoming`（未到期或未定日期）。已完成且完成时间在窗口内的进
`completed`；`cancelled` 不进任何桶。每条引用都带可跳转的 id（事项、来源记录、来源建议）。

**叙述失败是降级不是失败**：聚合先于模型完成；叙述调用失败时报告仍落库，
`status=failed`、`failure_reason` 写明原因、`summary` 退回大纲文本。空窗口直接本地作答
（`generated_by=local`），不请模型为空白期编故事。

```
POST   api/reports                  body {person_id?, start?, end?, week_of?} → 201 完整报告
GET    api/reports?person_id=       历史（快照 headline），新的在前
GET    api/reports/:id              单份报告（payload 反序列化，含全部分段）
DELETE api/reports/:id              删除快照
```

---

### 7.5 人物检索与聚合（已实现）

人物列表是最后一个支持服务端检索的列表。`GET /api/persons` 现在接受：

| 参数 | 行为 |
|---|---|
| `q` | 关键字，命中姓名 / 关系 / 职位 / 备注（LIKE 模糊） |
| `org_id` | 组织过滤；哨兵值 `none` 表示未归属任何组织 |
| `relation` | 关系精确匹配（与 `q` 的模糊命中相对） |
| `sort` | `recent`（默认，最近记录倒序，无记录回退创建时间）/ `name`（NOCASE 码点序）/ `importance` / `created` |
| `limit` / `offset` | 页窗口；`limit<=0` 表示不分页 |

- **总数走响应头**：匹配总数放 `X-Total-Count`，响应体仍是数组——与记录、关系列表的
  返回形状保持一致，前端用「请求 limit+1 条」判断是否还有下一页。
- **排序词汇表是封闭的**：未知 `sort` 返回 400 `INVALID_INPUT`，不静默回退到默认排序。
- **过滤条件只引用 persons 列**，计数查询不必拖着活动统计的 JOIN；列表查询仍然
  LEFT JOIN 事件与参与人（锚定或参与都算一次活动），一次查询带出 `last_event_date` /
  `event_count` / `org_name`。
- 前端首页人物列表改为服务端搜索（防抖 300ms）、组织筛选、四种排序与「加载更多」
  分页；无记录的人物明示「尚无记录」，不再拿创建时间冒充最近联系。

---

### 7.6 关系图谱（已实现）

`GET /api/graph` 一次返回整张画布，前端不再自行拼接四个列表：

| 分区 | 内容 |
|---|---|
| `nodes` | 全部人物：id、姓名、is_self、组织与归属、重要度、记录数（复用人物列表投影，与首页口径一致） |
| `orgs` | 活跃组织节点，带现任成员数；已归档组织不出现在图谱 |
| `edges` | 两族边：`kind=relationship`（person_relationships 全量，含已结束——历史置灰不隐藏）与 `kind=position`（现任任职，人物→组织；已离任的任职属于组织页历史，不画进图谱） |
| `co_attendance` | 共同经历对，`participants UNION events.person_id` 后自连接派生 |

- **锚定人计入共同经历**：只用参与人表自连接会漏掉「只有锚定人、没填参与人」的记录，
  UNION 把 `events.person_id` 纳入后，一条只写给自己的记录也能把我和对方连起来。
- **共同经历永远不是关系边**：它是「同场出现」的派生事实，图谱上用点线呈现且明确提示，
  不自动推断为朋友/同事/上下级。
- **过滤在前端**：数据量是个人 CRM 量级，接口整体返回；关系类型、组织、时间窗
  （边的起止与窗口相交）、含/不含已结束均在客户端筛选。
- 前端 `/relationships` 页：无依赖的轻量力导向布局（人物圆节点按记录数定大小，
  组织为方块节点），点边看详情（方向/起止/确认状态/备注/来源记录跳转），
  点节点跳人物或组织页；支持添加、编辑、结束、删除关系，双击关系边直接编辑。

---

### 7.7 记录列表与详情（已实现）

记录此前只能从人物页的时间线进入，提取失败的记录也没有集中的入口。现在 `/events`
是独立页面，`GET /api/events` 与人物列表口径一致：

| 参数 | 行为 |
|---|---|
| `q` | 关键字，命中原文与摘要 |
| `person_id` | 锚定人**或参与人**，共同记录在每个出席者的视图里都能查到 |
| `from` / `to` | 记录日期窗口（闭区间，非法格式或倒置返回 400） |
| `status` | `pending` / `succeeded` / `failed`，用于挑出需要重试的记录；未知值返回 400 |
| `limit` / `offset` | 页窗口，匹配总数同样在 `X-Total-Count` 头 |

- **计数与列表共用同一段 WHERE**（`eventFilterClause`）：两边不可能各写一套条件，
  也就不会出现「总数 3、翻页只有 2 条」这类错位。
- 前端 `/events`：关键词防抖 300ms、人物/状态/时间窗筛选、「加载更多」并显示剩余条数；
  每行带提取状态徽章（已提取 / 待提取 / 提取失败），失败行显示错误原因并可直接重试。
- 前端记录详情 `/events/:id`：
  - 参与人维护——列表带姓名与参与方式、可增删；最后一位不可移除（否则这条记录在
    任何人的时间线里都找不到）；重复添加会被去重。
  - 编辑——日期、形式（见面/通话/消息…）、渠道（当面/微信…）、摘要、我的感受、
    对方反应、原文。保存会置 `manually_edited`，此后重试需二次确认（force）。
  - 承诺转跟进——一条承诺一键转成跟进事项，带上 `source_event_id`，已转的不再重复提供；
    页面底部列出由这条记录转出的全部事项。
- **编辑必须原样回传 `promises`**：`PUT /api/events/:id` 只写它收到的字段，省略
  `promises` 会被当成「没有承诺」而清空提取结果，所以前端总是把当前承诺带回去。

---

### 7.8 人物详情：任职、关系与画像失效（已实现）

后端接口此前已全部就绪，这一轮把人物页补成规格书里的「概览 / 画像 / 任职 / 关系 / 记录」：

- **组织与任职卡片**：`GET /persons/:id/positions` 给出完整任职历史（现任在前），
  可新增、**结束只写 `end_date` 保留历史行**、删除；卡片下半部显示当前组织的
  类型/描述与同组织现任同事。
- **关系卡片**：`GET /persons/:id/relationships` 带出双方姓名，展示方向、类型、
  双向标记、未确认标记与起止；空态写明「同场出现只算共同经历，不会自动推断成关系」。
- **画像失效提示**：`source_stale` 条目显示「依据已变」徽章、原因，并给出跳到来源
  记录重新提取的入口——这是清除标记的唯一正确动作。
- **切人清场**：切换人物时清空提问框、编辑态与任职表单，避免上一个人的草稿留在该页面。

配套修掉的缺陷：`POST /api/events/:id/extract` 成功后会重新派生画像（此前只重写
提取结果，导致「依据已变」的画像按提示重试后仍然挂着标记）。

---

## 八、前端设计

### 8.1 目录结构

```
web
├── src
│   ├── api
│   │   ├── client.ts          # fetch 封装
│   │   └── types.ts           # 与后端共享类型
│   ├── components
│   │   ├── ui                # shadcnui
│   │   ├── PersonPicker.tsx   # 人物选择器
│   │   ├── QuickRecord.tsx    # 快速记录输入框
│   │   ├── EventTimeline.tsx  # 事件时间线
│   │   ├── TraitList.tsx      # 画像列表（可标记准确不准确，失效条目提示重算）
│   │   └── AdvicePanel.tsx    # 建议展示
│   ├── routes
│   │   ├── Home.tsx           # 人物列表 + 快速记录
│   │   ├── PersonDetail.tsx   # 人物详情（任职历史 / 组织 / 关系 / 画像 / 记录）
│   │   ├── Events.tsx         # 记录列表（筛选 + 提取状态 + 重试）
│   │   ├── EventDetail.tsx    # 记录详情（参与人 / 编辑 / 重试 / 承诺转事项）
│   │   ├── FollowUps.tsx      # 跟进事项（筛选/操作/延期历史/手动新建）
│   │   ├── Organizations.tsx  # 组织与成员任职
│   │   ├── Relationships.tsx  # 关系图谱（力导向画布 + 增改关系）
│   │   ├── Report.tsx         # 周报（窗口选择 + 五段式 + 历史快照）
│   │   └── Advice.tsx         # 建议对话
│   ├── App.tsx
│   └── main.tsx
├── index.css                  # @import tailwindcss;
└── vite.config.ts
```

### 8.2 三个核心页面

首页：左侧人物列表（按最近互动排序），右侧顶部一个快速记录框。选人 → 打字 → 回车提交。提交后显示AI 正在处理…，完成后在下方时间线插入新事件。

```
┌─────────────────┬─────────────────────────────────┐
│ 人物            │  [张总 ▼]  2026-09-10           │
│ ─────────────── │  ┌───────────────────────────┐  │
│ 张总    2天前   │  │ 今天和张总开会...          │  │
│ 李工    5天前   │  └───────────────────────────┘  │
│ 妈妈    1周前   │              [记录] [记录并提问] │
│ 老王    1月前   │                                  │
│                 │  ── 时间线 ──                    │
│ [+ 新建人物]    │  ● 09-08 张总答应周五给数据      │
│                 │  ● 08-30 项目周会，张总提到...   │
└─────────────────┴─────────────────────────────────┘
```

人物详情页：上半部分画像卡片（每条右侧有 ✓  ✗ 按钮），下半部分事件时间线。

建议页：选人 → 输入问题 → 显示结构化建议。每条建议下方显示依据事件链接，点击跳到事件详情。

### 8.3 Tailwind v4

`srcindex.css`：

```css
@import tailwindcss;

@theme {
  --color-primary oklch(0.6 0.18 260);
  --color-surface oklch(0.99 0.005 260);
  --color-border oklch(0.92 0.01 260);
  --font-sans Inter, system-ui, sans-serif;
}
```

无需 `tailwind.config.js`。

### 8.4 统一版式（已实现）

- `components/layout.tsx` 提供三个通用件：PageHeader（标题 + 说明 + 操作区）、
  StatCard（统计卡）、SectionCard（分节卡），统一间距 / 圆角 / 描边 / 字号层级。
- 侧边栏按「概览 / 人物 / 记录 / 洞察 / 系统」分组，带活跃高亮；顶栏常驻「记一笔」抽屉。
- 首页为统计条 + 人物卡片网格；记录页为工具栏筛选 + 卡片流；跟进页按紧急度分桶；
  人物详情为头图 + 两栏；记录详情分节展示概要 / 画像 / 承诺 / 事项。
- 时间线组件 `EventTimeline` 按日期分组；空态统一卡片化说明。

---

## 九、项目结构

```
relationship
├── cmd
│   └── server
│       └── main.go              # 入口，初始化 DB、Rosetta、Echo
├── internal
│   ├── config
│   │   └── config.go            # 读取 config.yaml
│   ├── db
│   │   ├── db.go                # OpenDB、迁移
│   │   └── migrations.sql       # 建表 SQL
│   ├── models
│   │   └── models.go            # Person  Event  Trait 结构体
│   ├── repository
│   │   ├── person_repo.go
│   │   ├── event_repo.go
│   │   ├── trait_repo.go
│   │   └── vec_repo.go          # 向量写入与检索
│   ├── service
│   │   ├── person_service.go
│   │   ├── event_service.go
│   │   └── ai_service.go        # 调用 Rosetta
│   ├── ai
│   │   ├── extractor.go         # 事件提取
│   │   ├── embedder.go          # embedding 生成
│   │   ├── trait_updater.go     # 画像更新
│   │   ├── retriever.go         # 混合检索
│   │   ├── advice.go            # 建议生成
│   │   └── prompts.go           # 所有 Prompt 常量
│   └── handler
│       ├── person_handler.go
│       ├── event_handler.go
│       ├── trait_handler.go
│       └── ai_handler.go
├── web                         # 前端（见 §8）
├── config.yaml
├── go.mod
└── README.md
```

前端嵌入：在 `main.go` 中：

```go
goembed allwebdist
var frontendFS embed.FS

func main() {
     ...
    sub, _ = fs.Sub(frontendFS, webdist)
    e.StaticFS(, sub)
     API 路由注册在 api 下
}
```

构建顺序：先 `cd web && npm run build`，再 `go build`。

---

## 十、开发计划

 阶段  内容  验收标准 
---------
 Day 1-2  项目脚手架：Go module、Echo、SQLite 打开、迁移执行、config.yaml 读取  `go run .cmdserver` 能启动，DB 文件生成 
 Day 3-4  人物 CRUD：repository + service + handler + 前端首页人物列表  浏览器能增删改查人物 
 Day 5-6  事件 CRUD（不含 AI）：手动填写 summary 字段，前端时间线展示  能记录事件并看到时间线 
 Day 7-9  Rosetta 接入：封装 `AIService`，实现 `ExtractEvent`、`EmbedText`，打通事件提取 + embedding 写入  输入一段文字，DB 里出现 summary 和 vec_memory 记录 
 Day 10-12  画像更新：`TraitUpdater`，traits 表读写，前端画像列表 + 准确性标记  记录 3 条事件后，能看到合理的 traits 
 Day 13-15  建议生成：混合检索 + Prompt 组装 + 结构化输出 + 前端建议页  提问怎么催 A，返回带依据的建议 
 Day 16-18  周报 + 打磨：周报接口、UI 细节、错误处理、Loading 状态  能生成一份可读的周报 
 Day 19-21  自用验证：录入 10 人、30 事件，连续使用 3 天  记录摩擦足够低，建议有实际参考价值 

关键验证点（Day 12 结束）：用 10 个真实人物、30 条事件跑一遍。如果 AI 提取的 summary 和 traits 准确率达到 60% 以上（肉眼可接受），继续；否则先调 Prompt。

---

## 十一、Prompt 设计要点

### 11.1 事件提取 Prompt

```
你是一个事件提取助手。从用户的原始记录中提取结构化信息。

输出 JSON，字段如下：
- summary 一句话摘要（不超过 50 字）
- my_feeling 用户的情绪（如着急、释然、不满），无则空字符串
- their_reaction 对方的反应（如答应但没做到），无则空字符串
- promises 承诺数组，每项 { who 对方我, what ..., deadline ... 或 null }

要求：
- 只提取原文中明确出现的信息，不要推测
- 不要输出 JSON 以外的任何内容
```

### 11.2 画像更新 Prompt

```
你是人物画像分析助手。基于以下历史事件和现有画像，更新对该人物的理解。

已有画像：
{existing_traits}

最近事件：
{recent_summaries}

输出 JSON 数组，每项：
- key 画像维度（如沟通偏好、雷区、决策风格）
- value 具体描述（如对公开批评敏感，私下沟通更有效）
- confidence 0-1
- source_event_ids 依据的事件 ID 数组

规则：
- 只输出新增或需要修改的画像，不要重复已有且未变的
- 每条画像必须至少有一个 source_event_id
- 不要编造历史事件中没有依据的画像
- 最多输出 5 条
```

### 11.3 建议生成 Prompt

```
你是沟通顾问。基于以下事实，为用户提供沟通建议。

人物画像：
{traits}

人物背景（用户手写的备注，只能作为辅助参考，不要编造）：
{notes}

用户本次想要达成的目标：
{goal}

相关历史事件（每行开头方括号内是事件 ID）：
{retrieved_events}

输出 JSON：
{
  situation:         {text: 对当前情况的分析（2-3 句）, evidence_event_ids: [事件ID]},
  other_perspective: {text: 对方可能的视角（1-2 句）, evidence_event_ids: [事件ID]},
  risks:             [{text: 风险1, evidence_event_ids: [事件ID]}],
  strategies: [
    {
      name: 策略名,
      script: 可直接使用的话术,
      pros: 优点,
      cons: 缺点,
      evidence_event_ids: [事件ID]
    }
  ],
  follow_up: {text: 后续跟进建议, evidence_event_ids: [事件ID]}
}

要求：
- 给出 2-3 种策略，风格要有差异
- 每一条结论单独标注依据：evidence_event_ids 只能从上面列出的事件 ID 中原样选取
- 如果某条结论没有记录依据（例如只是基于常理的推理），就给空数组 []，不要编造 ID
- 不要编造历史事件中不存在的信息
- 保持尊重，不建议操纵或欺骗
```

要点：问题放在 user 消息里，目标放进 system 模板——同一个问题在不同目标下该说的话不一样。
每条结论单独要依据，是为了让界面能「打开这一行的来源」，或者干脆承认它没有来源；
模型返回的整篇 `evidence_event_ids` 没有被采用，依据由各条结论的列表合并而来。

---

## 十二、风险与应对

 风险  应对 
------
 记录坚持不了  快速记录框极简，只有人物选择 + 输入框。不做表单。提供补记日期选择。 
 AI 画像不准  每条 trait 显示来源事件，用户可标记 ✗。标记后 `verified = -1`，不再参与建议。 
 LLM 幻觉  Prompt 明确要求只提取原文信息、必须有 source_event_id。结构化输出 + 容错解析。 
 SQLite 锁  `SetMaxOpenConns(1)` + WAL 模式。 
 本地模型质量差  提取用本地，建议切云端。配置里可随时切换。 
 API 成本  提取和画像更新用廉价模型。建议生成才用强模型。 

---

## 十三、明确不做的事

- ❌ 不做用户系统  登录  权限
- ❌ 不做数据加密（本地文件，自己电脑）
- ❌ 不做异步任务队列（同步执行，loading 等待）
- ❌ 不做 WebSocket  实时推送（SSE 可选用于流式建议）
- ❌ 不做移动端适配（桌面浏览器优先）
- ❌ 不做数据导入导出（除非后续需要）
- ❌ 不做 LangChainGo（Rosetta + 自己写编排更简单）
- ❌ 不做 Docker  K8s（`go build` 单二进制）

---

## 十四、一句话总结

Echo + SQLite（sqlite-vec）+ Rosetta + React 19 + Tailwind 4，`goembed` 打成单二进制。事件提取用本地小模型，建议生成可切云端。3 周跑通记录 → 提取 → 建议闭环，先用自己的真实数据验证效果，再迭代。