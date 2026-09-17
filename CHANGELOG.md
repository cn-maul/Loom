# 变更日志

本文件记录 Loom 的显著变更。格式参考 Keep a Changelog；数据库 schema 版本与 API 版本
在对应条目中注明。

## [1.3.0] - 2026-09-17

**纯前端视觉层重构。** API 路由、请求/响应形状、数据库 schema 与 Go 代码零变化：
路由仍为 54 条，`routes.golden.txt` 未重新生成，`go test ./...` 不受影响。

### 变更

- **前端视觉语言整体切换为 Apple Liquid Glass**。`web/src/index.css` 重写为两层
  token：第一层 `--al-*` 是唯一真源（页面地面 `#f5f5f7`、白色面板、hairline 分隔、
  灰度正文配单一强调色 `#0071e3`）；第二层把 shadcn 风格的 `--background` /
  `--card` / `--muted` / `--border` 等经 `@theme inline` 重新指向 `--al-*`，因此既有
  标记里的 `bg-card` / `border-border` / `text-muted-foreground` 不改一行就渲染成新
  语言。amber / red / green / emerald 四族重声明为半透明系统色，明暗自动适配，
  **组件中所有 `dark:` 配对随之删除**。
- 九个路由页与十六个组件按新语言重排（`App.tsx`、`components/layout.tsx`、
  Home / Events / Organizations / Advice / Report / PersonDetail / EventDetail /
  Settings，以及 `ui.tsx` 与 `ui/` 基础件、`AdvicePanel`、`PersonPicker`、
  `QuickRecord`、`QuickRecordModal`、`CommandPalette`、`EventTimeline`、
  `EventStatus`、`TraitList`）。核心手法是**消灭碎片化卡片**：同级条目（报告分桶、
  AI 策略、历史快照、参与人、时间线）原本各带边框与底色，现在统一为「一个白色
  面板 + hairline 分隔行」。
- **间距与排版对齐参照实现**：区块间距 `space-y-5`(20px) → `space-y-9`(36px)；
  列表行内距改由 `.panel-rows` 自带（`13px 18px`）；eyebrow 小标题字距
  `0.08em` → `0.14em`，缩进与行内头像对齐。
- 玻璃效果收敛到真正发生层叠的位置：sticky 顶栏、弹窗遮罩与面板。普通内容块是
  纯白加双层柔和阴影，不使用模糊。
- 选中态由「彩色底 + ring」改为「安静填充 + 字重」，与 `App.tsx` 的 `NavItem` 统一。

### 新增

- `web/src/lib/overlay.ts`：`useEnterState` —— 条件挂载的弹窗需要一个过渡起点，
  否则首次渲染即为 `open`、没有起始状态可插值，出场会硬闪；常驻挂载的层直接把
  `data-state` 绑到 prop 上，不走这个 hook。
- 语义 token 工具类：`text-ink-2/3/4`、`text-faint`、`bg-fill`、`bg-hover`、
  `bg-track`、`border-hairline`、`bg-heat-bg`/`text-heat-text`、
  `bg-live-bg`/`text-live-text`。状态用底纹片表达，不用彩色描边 chip。
- 组件层模式类：`.panel`（内容面）、`.panel-rows`（行列表，自带行内距与分隔线）、
  `.glass-nav`（hairline 仅在下滚后出现）、`.al-scrim` / `.al-sheet`（叠层弹窗）。
- 三套降级查询：`prefers-reduced-motion`、`prefers-reduced-transparency`、
  `prefers-contrast`。

### 修复

- **列表文字贴着面板边缘**：`.panel-rows` 原先只加分隔线、不设内距，行内距全靠每个
  `<li>` 自觉写，漏一个文字就贴到 22px 圆角内侧——看起来像从圆角里溢出来。行内距
  改为模式自身的属性（`.panel-rows > *`）。
- **记录页表格的操作列被裁掉**：表格直接放进 `.panel`，宽度溢出被 `overflow:hidden`
  切平，且首列 `px-4`(16px) 小于面板圆角。改为 `table-fixed` 让列宽受控，并把首末列
  内距对齐面板内距。
- 全仓散落的 `px-2.5`(10px) / `px-3`(12px) / `px-[18px]` 一类任意值统一到面板内距，
  消除同一页面内不同缩进并存。
- `components/ui.tsx` 中 `Spinner` 标签漏改的 `text-sm` 清理。

## [1.2.0] - 2026-09-15

### 移除

- **关系图谱功能整体下线**：前端 `/relationships` 页面与 `web/src/routes/Relationships.tsx`
  一并删除（力导向画布、关系增删改、AI 推断上下级入口）。
- **`GET /api/graph`**：端点及其 `GraphHandler`、`GraphService`、`GraphRepo`、
  `models/graph.go` 与 `graph_handler_test.go` 全部移除；`routes.golden.txt` 与
  `docs/architecture/api.md` 同步更新；ADR-009 的「图谱护栏」条目废止存档。
- 前端 `graphApi` 封装与 `GraphData`/`GraphNode`/`GraphOrg`/`GraphEdge`/`GraphCoLink`
  类型随之清理。
- **`POST /api/ai/infer-hierarchy`**：端点及其 `AIHandler.InferHierarchy`、
  `AIService.InferHierarchy`、`hierarchyPrompt`、`activeEdgeExists` 一并移除。
  唯一入口是已下线的图谱页，删除后成为孤儿。前端 `aiApi.inferHierarchy` 同步清理。
- **结构化关系边整体移除**：10 个端点中的 5 个——`POST /api/relationships`、
  `GET /api/relationships/types`、`PUT/DELETE /api/relationships/:id`、
  `GET /api/persons/:id/relationships`——连同 `RelationshipHandler`、`RelationshipService`、
  `RelationshipRepo`、`models.Relationship`/`RelationshipLink`/`RelationshipFilter` 删除。
  连带清理：AI 建议 prompt 的「关系与组织背景」整段与 `AIService.structureContext`、
  报告里的「关系与任职变化」段与 `collectChanges`（含 `Report.Changes` 字段、
  `ReportChangeRef` 与四个 change-kind 常量）、前端 `relationshipApi`、Advice 页
  「AI 会读到的关系与组织」面板与 `Share2` 图标。路由总数 62 → 54。
- **结构化任职整体移除**：`POST/GET /api/persons/:id/positions`、
  `GET /api/organizations/:id/members`、`PUT/DELETE /api/positions/:id` 连同
  `PositionHandler`、`PositionService`、`PositionRepo`、`models.OrgPosition`/`OrgPositionLink`
  删除；前端 `positionApi`、`organizationApi` 的 members/addMember/endMembership/
  removeMembership 四个方法同步清理。
- 前端零引用文件：`components/ui/card.tsx`、`App.css`、`assets/hero.png`、
  `assets/react.svg`、`assets/vite.svg`；以及 `EventTimeline.tsx` 的 `EventTimelineDense`、
  `EventStatus.tsx` 的 `statusLabel`。

### 新增

- **组织归档/恢复端点回来，并且这次有前端入口**：`POST /api/organizations/:id/archive`
  与 `/restore` 重新实现（上一版把它们当成无人调用的孤儿删掉了），组织页行操作区新增
  归档/恢复按钮。归档是软删：组织不再出现在任何选择框，成员归属保留，随时可恢复；
  真正删除仍走 `DELETE`（会把成员解除归属）。

### 修复

- **未匹配的 `/api` 路径曾以 200 + HTML 应答**：SPA 兜底路由 `GET /*` 会吃下任何 GET，
  于是 `GET /api/persons-typo` 返回 `200 text/html` 和一份 index.html —— 调用方拿到的是
  解析失败的 JSON，而不是「这个接口不存在」。同时 405 与 500 走的是 Echo 默认的
  `{"message":...}`，只有 handler 自己产生的错误才符合 `docs/architecture/api.md` 承诺的包络。
  现在 `internal/app/errors.go` 接管 `/api` 前缀下的所有失败：未路由 404 `NOT_FOUND`、
  已知路径的错误方法 405 `METHOD_NOT_ALLOWED`（`Allow` 头保留）、请求体被拒 400
  `INVALID_INPUT`、panic 500 `INTERNAL`；5xx 只回固定文案，不回显 panic 内容与驱动错误。
  404/405 的判定不靠状态码而靠路由表（`:id` 段参与匹配），因为兜底路由让每个路径都
  「有」一个方法。SPA 页面路径与静态资源的兜底行为不变，`OPTIONS` 预检仍由 CORS 中间件
  以 204 短路。`routes.golden.txt` 未变——本次不动路由，只改失败路径的应答形状；
  `internal/app/errors_test.go` 覆盖以上全部情形。
- `scripts/release.sh` / `release.ps1` 的冒烟注释与断言同步更新：那两处「不要用未知路由，
  因为会被 SPA 兜底吃掉」的绕行说明已失效，并各补一条未匹配 `/api` 路径必须 404 + 包络的断言。
- **发布管线冒烟检查早已失效**：`scripts/release.ps1` 与 `release.sh` 断言
  `POST /api/maintenance/cleanup` 返回 200，但该路由在阶段 6 改造中已不存在 —— 发布会在
  冒烟这一步 exit 1。两处检查删除；`release.sh` 另有一处同类问题：`GET /api/nonexistent`
  会被 SPA catch-all 命中并返回 `200 text/html`，永远过不了「404 包络」断言，改为
  `GET /api/persons/does-not-exist`（与 release.ps1 一致）——该绕行随后被本次的
  `/api` 错误包络修复彻底解决（见上一条），脚本现在两条路径都断言。
- `docs/RELEASE.md`、`readme.md`、`docs/architecture/decisions.md`（ADR-010 第 1、4 条）
  中残留的 `POST /api/maintenance/cleanup` 说法改为「只在启动时执行，无手动入口」。
- **陈旧注释**：`docs/architecture/decisions.md`（ADR-006、ADR-009 第 5 条）与
  `internal/models/models.go` 里仍在指向已删除的 `GET /api/tasks`，改为「记录自身的
  extraction_status + 启动日志」；`models.Person.IsSelf` 与 `Organization.ArchivedAt`
  的注释不再提关系边与任职行。

### 影响

- **关系边与任职的记录从此没有代码路径**：读写端点全部删除，也没有替代端点。
  后端测试同步收缩（`structure_handler_test.go` 只保留参与人部分、
  `structure_test.go` 只保留记录/人物部分、`report_service_test.go` 去掉 changes 断言、
  `report_repo_test.go` 同理），`TestAPIRouteContract` 金样重新生成。
- **数据零损失**：本地库实测 `person_relationships` 与 `person_org_positions` 均为 0 行，
  所以这次移除没有丢任何用户数据。
- **表结构保留**：两张表、索引、迁移 v3/v4（含回填步骤）与备份导出清单都不动——
  删表是破坏性操作，收益只是少两张空表。`docs/architecture/api.md` 与 `readme.md` §7.1
  已写明「表在，但没有任何代码读写」，避免以后被重新接上。
- **职位仍在**：`persons.position` 自由文本照旧由人物表单读写、组织页展示。它和已删除的
  结构化任职表从来不是一回事——ADR-011 专门记录这一点。
- **唯一的实质能力损失**：AI 建议不再能读到用户手工标注的结构化关系，只能从画像与记录
  原文推断立场。已确认无数据可读，且该面板在前端零使用。
- 路由总数 62 → 54；`routes.golden.txt` 与 `docs/architecture/api.md` 同步；
  新增 ADR-011 记录本次下线决策。

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
