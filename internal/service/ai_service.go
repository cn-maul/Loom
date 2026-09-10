package service

import (
	"context"
	"encoding/json"
	"fmt"
	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/models"
	"relationship/internal/repository"
	"strings"

	"github.com/google/uuid"
)

type AIService struct {
	cfg       *config.LLMConfig
	vecRepo   *repository.VecRepo
	traitRepo *repository.TraitRepo
	eventRepo *repository.EventRepo
	client    *ai.Client
}

func NewAIService(cfg *config.LLMConfig, vecRepo *repository.VecRepo, traitRepo *repository.TraitRepo, eventRepo *repository.EventRepo, client *ai.Client) *AIService {
	return &AIService{
		cfg:       cfg,
		vecRepo:   vecRepo,
		traitRepo: traitRepo,
		eventRepo: eventRepo,
		client:    client,
	}
}

func (s *AIService) ExtractEvent(ctx context.Context, raw string) (*models.EventExtraction, error) {
	prompt := `你是一个事件提取助手。从用户的原始记录中提取结构化信息。

输出 JSON，字段如下：
- summary: 一句话摘要（不超过 50 字）
- my_feeling: 用户的情绪（如着急、释然、不满），无则空字符串
- their_reaction: 对方的反应（如答应但没做到），无则空字符串
- promises: 承诺数组，每项 { who: "对方/我", what: "...", deadline: "..." 或 null }

要求：
- 只提取原文中明确出现的信息，不要推测
- 不要输出 JSON 以外的任何内容`

	messages := []ai.Message{
		ai.System(prompt),
		ai.User(raw),
	}

	resp, err := s.client.Chat(s.cfg.ExtractModel, messages)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	var extraction models.EventExtraction
	if err := parseJSON(resp, &extraction); err != nil {
		return nil, fmt.Errorf("parse extraction: %w", err)
	}

	return &extraction, nil
}

func (s *AIService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return s.client.Embed(s.cfg.EmbedModel, text)
}

func (s *AIService) UpdateTraits(ctx context.Context, personID string, recentSummaries []string, existingTraits []*models.Trait) ([]*models.Trait, error) {
	existingTraitsStr := "无"
	if len(existingTraits) > 0 {
		var traits []string
		for _, t := range existingTraits {
			traits = append(traits, fmt.Sprintf("%s: %s (置信度: %.2f)", t.TraitKey, t.TraitValue, t.Confidence))
		}
		existingTraitsStr = strings.Join(traits, "\n")
	}

	summariesStr := strings.Join(recentSummaries, "\n")

	prompt := fmt.Sprintf(`你是人物画像分析助手。基于以下历史事件和现有画像，更新对该人物的理解。

已有画像：
%s

最近事件：
%s

输出 JSON 数组，每项：
- key: 画像维度（如沟通偏好、雷区、决策风格）
- value: 具体描述（如对公开批评敏感，私下沟通更有效）
- confidence: 0-1
- source_event_ids: 依据的事件 ID 数组

规则：
- 只输出新增或需要修改的画像，不要重复已有且未变的
- 每条画像必须至少有一个 source_event_id
- 不要编造历史事件中没有依据的画像
- 最多输出 5 条`, existingTraitsStr, summariesStr)

	messages := []ai.Message{
		ai.System(prompt),
		ai.User("请分析并更新人物画像"),
	}

	resp, err := s.client.Chat(s.cfg.ExtractModel, messages)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	type TraitUpdate struct {
		Key             string   `json:"key"`
		Value           string   `json:"value"`
		Confidence      float64  `json:"confidence"`
		SourceEventIDs  []string `json:"source_event_ids"`
	}

	var updates []TraitUpdate
	if err := parseJSON(resp, &updates); err != nil {
		return nil, fmt.Errorf("parse updates: %w", err)
	}

	var traits []*models.Trait
	for _, update := range updates {
		trait := &models.Trait{
			ID:             uuid.New().String(),
			PersonID:       personID,
			TraitKey:       update.Key,
			TraitValue:     update.Value,
			Confidence:     update.Confidence,
			SourceEventIDs: strings.Join(update.SourceEventIDs, ","),
			Verified:       0,
		}
		traits = append(traits, trait)
	}

	return traits, nil
}

func (s *AIService) GenerateAdvice(ctx context.Context, req models.AdviceRequest) (*models.AdviceResponse, error) {
	// Get traits
	traits, err := s.traitRepo.ListByPerson(req.PersonID)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}

	// Get recent events
	events, err := s.eventRepo.ListByPerson(req.PersonID, 5)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	// Build context
	traitsStr := "无"
	if len(traits) > 0 {
		var traitStrs []string
		for _, t := range traits {
			traitStrs = append(traitStrs, fmt.Sprintf("%s: %s", t.TraitKey, t.TraitValue))
		}
		traitsStr = strings.Join(traitStrs, "\n")
	}

	eventsStr := "无"
	if len(events) > 0 {
		var eventStrs []string
		for _, e := range events {
			eventStrs = append(eventStrs, fmt.Sprintf("[%s] %s", e.EventDate, e.Summary))
		}
		eventsStr = strings.Join(eventStrs, "\n")
	}

	prompt := fmt.Sprintf(`你是沟通顾问。基于以下事实，为用户提供沟通建议。

人物画像：
%s

相关历史事件：
%s

用户的问题：
%s

输出 JSON：
{
  "situation": "对当前情况的分析（2-3 句）",
  "other_perspective": "对方可能的视角（1-2 句）",
  "risks": ["风险1", "风险2"],
  "strategies": [
    {
      "name": "策略名",
      "script": "可直接使用的话术",
      "pros": "优点",
      "cons": "缺点"
    }
  ],
  "evidence_event_ids": ["事件ID"],
  "follow_up": "后续跟进建议"
}

要求：
- 提供 2-3 种策略，风格要有差异
- evidence_event_ids 必须来自上面提供的历史事件
- 不要编造历史事件中不存在的信息
- 保持尊重，不建议操纵或欺骗`, traitsStr, eventsStr, req.Question)

	messages := []ai.Message{
		ai.System(prompt),
		ai.User(req.Question),
	}

	resp, err := s.client.Chat(s.cfg.AdviceModel, messages)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	var advice models.AdviceResponse
	if err := parseJSON(resp, &advice); err != nil {
		return nil, fmt.Errorf("parse advice: %w", err)
	}

	return &advice, nil
}

func parseJSON[T any](raw string, out *T) error {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	return nil
}