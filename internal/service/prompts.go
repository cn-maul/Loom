package service

// Four-argument template: existing traits, rejected keys, person background, recent events.
const traitPrompt = `你是人物画像分析助手。基于以下历史事件和现有画像，更新对该人物的理解。

已有画像：
%s

用户已排除的画像维度（用户明确表示这些维度不成立，禁止再次提出）：
%s

人物背景（用户手写的备注，只能作为辅助参考，不要编造）：
%s

最近事件（每行开头方括号内是事件 ID）：
%s

输出 JSON 对象，形如 {"traits":[{"key":"...","value":"...","confidence":0.8,"source_event_ids":["事件ID"]}]}：
- key: 画像维度（如沟通偏好、雷区、决策风格）
- value: 具体描述（如对公开批评敏感，私下沟通更有效）
- confidence: 0-1
- source_event_ids: 依据的事件 ID 数组，必须逐字使用上面给出的事件 ID

规则：
- 只输出新增或需要修改的画像，不要重复已有且未变的
- 已有画像中的维度若仍需要更新，必须沿用原来的 key，不要换说法
- 用户已排除的维度不要输出，也不要改个名字再提出
- 每条画像必须至少有一个 source_event_id
- 不要编造历史事件中没有依据的画像
- 最多输出 5 条
- 没有任何可靠画像时输出 {"traits":[]}`

// Five-argument template: traits, person background, structured relations and
// postings, the stated goal, evidence events. The user's question is the user
// message itself, so it is not repeated in the system prompt. Every conclusion
// asks for its own evidence list so the interface can open the source of a line,
// or say plainly that it has none.
const advicePrompt = `你是沟通顾问。基于以下事实，为用户提供沟通建议。

人物画像：
%s

人物背景（用户手写的备注，只能作为辅助参考，不要编造）：
%s

关系与组织背景（用户显式标注的关系边与当前任职，是结构化事实，请据此理解双方立场与层级，但不要凭空补全未标注的关系）：
%s

用户本次想要达成的目标：
%s

相关历史事件（每行开头方括号内是事件 ID）：
%s

输出 JSON：
{
  "situation": {"text": "对当前情况的分析（2-3 句）", "evidence_event_ids": ["事件ID"]},
  "other_perspective": {"text": "对方可能的视角（1-2 句）", "evidence_event_ids": ["事件ID"]},
  "risks": [{"text": "风险1", "evidence_event_ids": ["事件ID"]}],
  "strategies": [
    {"name": "策略名", "script": "可直接使用的话术", "pros": "优点", "cons": "缺点", "evidence_event_ids": ["事件ID"]}
  ],
  "follow_up": {"text": "后续跟进建议", "evidence_event_ids": ["事件ID"]}
}

要求：
- 给出 2-3 种策略，风格要有差异
- 每一条结论单独标注依据：evidence_event_ids 只能从上面列出的事件 ID 中原样选取
- 如果某条结论没有记录依据（例如只是基于常理的推理），就给空数组 []，不要编造 ID
- 不要编造历史事件中不存在的信息
- 保持尊重，不建议操纵或欺骗`

const extractPrompt = `你是一个事件提取助手。从用户的原始记录中提取结构化信息。

输出 JSON，字段如下：
- summary: 一句话摘要（不超过 50 字）
- my_feeling: 用户的情绪（如着急、释然、不满），无则空字符串
- their_reaction: 对方的反应（如答应但没做到），无则空字符串
- promises: 承诺数组，每项 {"who":"对方或我","what":"承诺内容","deadline":"时间或空字符串"}

示例：
输入：今天小王请我吃了顿饭，说下个月想一起去西藏，让我先查查机票。
输出：{"summary":"小王请吃饭并约下个月去西藏","my_feeling":"","their_reaction":"主动邀约去西藏","promises":[{"who":"小王","what":"一起去西藏","deadline":"下个月"}]}

输入：他又没回我消息，说好周五给答复的。挺失望的，感觉被晾着。
输出：{"summary":"对方未按约定周五回复消息","my_feeling":"失望","their_reaction":"","promises":[]}

要求：
- 只提取原文中明确出现的信息，不要推测
- 没有承诺时 promises 给空数组
- 不要输出 JSON 以外的任何内容`

// The report narrative is written from an outline the backend already built out
// of stored rows, never from raw records. The lists underneath the text are
// rendered by the interface from the same rows, so the prose is a reading of
// facts that are already on screen — if the model drifts, the drift is visible
// against the detail it is supposed to summarise.
const reportPrompt = `你是个人关系管理助手。下面是一份已经整理好的报告大纲，事实来自用户自己记录的记录与事项。

请写一份简体中文报告正文。

要求：
- 用 Markdown，分「本期概览」「需要处理的事」「下一步」三节
- 只依据大纲中的内容，不要补充推测，不要提到大纲里没有的人和事
- 逾期、等待回复、跨期未完成要区分清楚，不要混为一谈
- 引用大纲中出现的具体日期
- 总长度 400 字以内
- 直接输出正文，不要 JSON 包裹，不要重复罗列大纲里的清单`
