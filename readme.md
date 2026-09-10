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
GET    apipersons                 列出人物（含最近互动时间）
GET    apipersonsid             人物详情（含 traits）
PUT    apipersonsid             更新人物
DELETE apipersonsid             删除人物（级联）

GET    apipersonsidevents      该人物的事件时间线
POST   apievents                  创建事件（触发 AI 管线）
GET    apieventsid              事件详情
DELETE apieventsid              删除事件

GET    apipersonsidtraits      人物画像列表
PUT    apitraitsidverify       标记画像准确不准确

POST   apiaiadvice               请求建议
POST   apiaiadvicestream        流式建议（SSE）
GET    apiaireportweekly        周报

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
│   │   ├── TraitList.tsx      # 画像列表（可标记准确不准确）
│   │   └── AdvicePanel.tsx    # 建议展示
│   ├── routes
│   │   ├── Home.tsx           # 人物列表 + 快速记录
│   │   ├── PersonDetail.tsx   # 人物详情
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

相关历史事件：
{retrieved_events}

用户的问题：
{question}

输出 JSON：
{
  situation 对当前情况的分析（2-3 句）,
  other_perspective 对方可能的视角（1-2 句）,
  risks [风险1, 风险2],
  strategies [
    {
      name 策略名,
      script 可直接使用的话术,
      pros 优点,
      cons 缺点
    }
  ],
  evidence_event_ids [事件ID],
  follow_up 后续跟进建议
}

要求：
- 提供 2-3 种策略，风格要有差异
- evidence_event_ids 必须来自上面提供的历史事件
- 不要编造历史事件中不存在的信息
- 保持尊重，不建议操纵或欺骗
```

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