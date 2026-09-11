package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/models"
	"relationship/internal/repository"

	"github.com/google/uuid"
)

type AIService struct {
	cfg        *config.LLMConfig
	vecRepo    *repository.VecRepo
	traitRepo  *repository.TraitRepo
	eventRepo  *repository.EventRepo
	personRepo *repository.PersonRepo
	client     *ai.Client
}

func NewAIService(
	cfg *config.LLMConfig,
	vecRepo *repository.VecRepo,
	traitRepo *repository.TraitRepo,
	eventRepo *repository.EventRepo,
	personRepo *repository.PersonRepo,
	client *ai.Client,
) *AIService {
	return &AIService{
		cfg:        cfg,
		vecRepo:    vecRepo,
		traitRepo:  traitRepo,
		eventRepo:  eventRepo,
		personRepo: personRepo,
		client:     client,
	}
}

// IngestEvent runs the record pipeline: extract, persist, embed, refresh the person
// profile. A failing LLM never discards the user's record — the event is still stored
// and the shortfall comes back as a warning.
func (s *AIService) IngestEvent(ctx context.Context, event *models.Event) (models.IngestReport, error) {
	report := models.IngestReport{Warnings: []string{}}

	extraction, err := s.ExtractEvent(ctx, event.RawText)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("AI 提取失败，已保留原文：%v", err))
	} else {
		event.Summary = extraction.Summary
		event.MyFeeling = extraction.MyFeeling
		event.TheirReaction = extraction.TheirReaction
		promises, err := json.Marshal(extraction.Promises)
		if err != nil {
			promises = []byte("[]")
		}
		event.Promises = string(promises)
		report.Extracted = true
	}
	if event.Promises == "" {
		event.Promises = "[]"
	}

	if err := s.eventRepo.Create(event); err != nil {
		return report, fmt.Errorf("create event: %w", err)
	}

	if err := s.indexEvent(ctx, event); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("向量索引跳过：%v", err))
	} else {
		report.Vectorized = true
	}

	updated, err := s.RefreshTraits(ctx, event.PersonID)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("画像更新失败：%v", err))
	}
	report.TraitsUpdated = updated

	return report, nil
}

// ExtractEvent tolerates a gateway failure mode seen in the wild: the reply parses as
// JSON but carries no fields (`null`, `{}`, or a renamed schema). Reporting that as
// success would store an empty event, so retry once and then return an error.
func (s *AIService) ExtractEvent(ctx context.Context, raw string) (*models.EventExtraction, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := s.client.Chat(ctx, s.cfg.ExtractModel, []ai.Message{
			ai.System(extractPrompt),
			ai.User(raw),
		}, ai.ChatOptions{JSONMode: true, Temperature: 0.2, MaxTokens: 1200})
		if err != nil {
			lastErr = fmt.Errorf("chat: %w", err)
			continue
		}
		var extraction models.EventExtraction
		if err := parseJSON(resp, &extraction); err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(extraction.Summary) == "" {
			lastErr = fmt.Errorf("extraction returned no summary: %s", truncate(resp, 200))
			log.Printf("warning: %v (model=%s attempt=%d)", lastErr, s.cfg.ExtractModel, attempt+1)
			continue
		}
		return &extraction, nil
	}
	return nil, lastErr
}

func (s *AIService) indexEvent(ctx context.Context, event *models.Event) error {
	embedding, err := s.client.Embed(ctx, eventEmbedSource(event))
	if err != nil {
		return err
	}
	return s.vecRepo.Insert(eventVectorID(event.ID), embedding, event.PersonID, "event_summary", event.ID)
}

func eventVectorID(eventID string) string { return "event:" + eventID }
func traitVectorID(traitID string) string { return "trait:" + traitID }

type traitUpdate struct {
	Key            string   `json:"key"`
	Value          string   `json:"value"`
	Confidence     float64  `json:"confidence"`
	SourceEventIDs []string `json:"source_event_ids"`
}

// RefreshTraits re-derives a person's profile from the ten most recent events and
// upserts whatever the model proposes. User verification flags survive the update.
func (s *AIService) RefreshTraits(ctx context.Context, personID string) (int, error) {
	events, err := s.eventRepo.ListByPerson(personID, 10)
	if err != nil {
		return 0, fmt.Errorf("list events: %w", err)
	}
	existing, err := s.traitRepo.ListByPerson(personID)
	if err != nil {
		return 0, fmt.Errorf("list traits: %w", err)
	}
	excluded, err := s.traitRepo.ListRejectedKeys(personID)
	if err != nil {
		return 0, fmt.Errorf("list rejected traits: %w", err)
	}
	var notes string
	if person, err := s.personRepo.GetByID(personID); err == nil {
		notes = person.Notes
	}

	var summaryLines []string
	for _, e := range events {
		summaryLines = append(summaryLines, formatEvent(e))
	}
	var existingLines []string
	for _, t := range existing {
		existingLines = append(existingLines, fmt.Sprintf("%s: %s (置信度 %.2f)", t.TraitKey, t.TraitValue, t.Confidence))
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := s.client.Chat(ctx, s.cfg.ExtractModel, []ai.Message{
			ai.System(fmt.Sprintf(traitPrompt, joinOr(existingLines, "无"), joinOr(excluded, "无"), notes, joinOr(summaryLines, "无"))),
			ai.User("请分析并更新人物画像"),
		}, ai.ChatOptions{JSONMode: true, Temperature: 0.2, MaxTokens: 2000})
		if err != nil {
			lastErr = fmt.Errorf("chat: %w", err)
			continue
		}

		var updates []traitUpdate
		var payload struct {
			Traits []traitUpdate `json:"traits"`
		}
		switch {
		case parseJSON(resp, &payload) == nil:
			updates = payload.Traits
		case parseJSON(resp, &updates) == nil:
			// Some models ignore the wrapper and return a bare array.
		default:
			lastErr = fmt.Errorf("parse trait updates from: %s", truncate(resp, 200))
			continue
		}
		return s.applyTraitUpdates(ctx, personID, events, updates)
	}
	return 0, lastErr
}

// applyTraitUpdates persists the proposed profile rows that cite known events and
// re-indexes their vectors.
func (s *AIService) applyTraitUpdates(ctx context.Context, personID string, events []*models.Event, updates []traitUpdate) (int, error) {
	count := 0
	for _, u := range updates {
		if strings.TrimSpace(u.Key) == "" || strings.TrimSpace(u.Value) == "" {
			continue
		}
		if !allKnownEvents(u.SourceEventIDs, events) {
			// A profile note with no traceable event is not evidence.
			continue
		}
		trait := &models.Trait{
			ID:             uuid.New().String(),
			PersonID:       personID,
			TraitKey:       u.Key,
			TraitValue:     u.Value,
			Confidence:     clampConfidence(u.Confidence),
			SourceEventIDs: models.EncodeIDList(u.SourceEventIDs),
		}
		if err := s.traitRepo.Upsert(trait); err != nil {
			return count, fmt.Errorf("upsert trait %s: %w", u.Key, err)
		}
		stored, err := s.traitRepo.GetByKey(personID, u.Key)
		if err != nil {
			return count, fmt.Errorf("read back trait %s: %w", u.Key, err)
		}
		if err := s.indexTrait(ctx, stored); err != nil {
			return count, fmt.Errorf("index trait %s: %w", u.Key, err)
		}
		count++
	}
	return count, nil
}

func (s *AIService) indexTrait(ctx context.Context, trait *models.Trait) error {
	embedding, err := s.client.Embed(ctx, trait.TraitKey+": "+trait.TraitValue)
	if err != nil {
		return err
	}
	return s.vecRepo.Insert(traitVectorID(trait.ID), embedding, trait.PersonID, "trait", trait.ID)
}

// GenerateAdvice answers from three context sources: profile traits, the recent
// timeline, and person-scoped semantic hits.
func (s *AIService) GenerateAdvice(ctx context.Context, req models.AdviceRequest) (*models.AdviceResponse, error) {
	traits, err := s.traitRepo.ListByPerson(req.PersonID)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	recent, err := s.eventRepo.ListByPerson(req.PersonID, 5)
	if err != nil {
		return nil, fmt.Errorf("list recent events: %w", err)
	}
	var notes string
	if person, err := s.personRepo.GetByID(req.PersonID); err == nil {
		notes = person.Notes
	}

	related, relatedTraits, vectorUsed := s.retrieveRelated(ctx, req.PersonID, req.Question)

	seen := make(map[string]bool, len(recent)+len(related))
	var evidence []*models.Event
	for _, e := range append(append([]*models.Event{}, related...), recent...) {
		if e == nil || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		evidence = append(evidence, e)
	}

	// Traits that answered the question go first so the profile section is
	// focused on what matters, not a flat confidence-ordered list.
	relatedTraitIDs := make(map[string]bool, len(relatedTraits))
	for _, t := range relatedTraits {
		if t != nil {
			relatedTraitIDs[t.ID] = true
		}
	}
	traitOrder := make([]*models.Trait, 0, len(traits))
	seenTrait := make(map[string]bool, len(relatedTraits))
	for _, t := range relatedTraits {
		if t != nil && !seenTrait[t.ID] {
			traitOrder = append(traitOrder, t)
			seenTrait[t.ID] = true
		}
	}
	for _, t := range traits {
		if !seenTrait[t.ID] {
			traitOrder = append(traitOrder, t)
			seenTrait[t.ID] = true
		}
	}

	var traitLines []string
	for _, t := range traitOrder {
		mark := ""
		if relatedTraitIDs[t.ID] {
			mark = "（与当前问题相关）"
		}
		if t.Verified == 1 {
			traitLines = append(traitLines, fmt.Sprintf("%s: %s（用户已确认）%s", t.TraitKey, t.TraitValue, mark))
		} else {
			traitLines = append(traitLines, fmt.Sprintf("%s: %s（置信度 %.2f）%s", t.TraitKey, t.TraitValue, t.Confidence, mark))
		}
	}
	var eventLines []string
	for _, e := range evidence {
		eventLines = append(eventLines, formatEventForAdvice(e))
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := s.client.Chat(ctx, s.cfg.AdviceModel, []ai.Message{
			ai.System(fmt.Sprintf(advicePrompt, joinOr(traitLines, "无"), notes, joinOr(eventLines, "无"))),
			ai.User(req.Question),
		}, ai.ChatOptions{JSONMode: true, Temperature: 0.7, MaxTokens: 8000})
		if err != nil {
			lastErr = fmt.Errorf("chat: %w", err)
			continue
		}

		var advice models.AdviceResponse
		if err := parseJSON(resp, &advice); err != nil {
			lastErr = fmt.Errorf("parse advice: %w", err)
			continue
		}
		if strings.TrimSpace(advice.Situation) == "" && strings.TrimSpace(advice.FollowUp) == "" && len(advice.Strategies) == 0 {
			lastErr = fmt.Errorf("advice returned no content: %s", truncate(resp, 200))
			log.Printf("warning: %v (model=%s attempt=%d)", lastErr, s.cfg.AdviceModel, attempt+1)
			continue
		}
		// Evidence the model invents would break the traceability this product promises.
		advice.EvidenceEventIDs = filterKnown(advice.EvidenceEventIDs, seen)
		advice.VectorUsed = vectorUsed
		return &advice, nil
	}
	return nil, lastErr
}

// retrieveRelated returns the events and traits nearest to the question and reports
// whether the vector branch contributed anything at all.
func (s *AIService) retrieveRelated(ctx context.Context, personID, question string) ([]*models.Event, []*models.Trait, bool) {
	embedding, err := s.client.Embed(ctx, question)
	if err != nil {
		return nil, nil, false
	}
	eventResults, err := s.vecRepo.SearchByPerson(embedding, personID, "event_summary", 5)
	if err != nil {
		return nil, nil, false
	}
	var events []*models.Event
	for _, r := range eventResults {
		event, err := s.eventRepo.GetByID(r.SourceID)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	traitResults, err := s.vecRepo.SearchByPerson(embedding, personID, "trait", 3)
	if err != nil {
		return events, nil, len(events) > 0
	}
	var traits []*models.Trait
	for _, r := range traitResults {
		trait, err := s.traitRepo.GetByID(r.SourceID)
		if err != nil || trait.Verified == -1 {
			continue
		}
		traits = append(traits, trait)
	}
	return events, traits, len(events) > 0 || len(traits) > 0
}

func (s *AIService) WeeklyReport(ctx context.Context, personID string) (*models.WeeklyReport, error) {
	start := time.Now().AddDate(0, 0, -6)
	events, err := s.eventRepo.ListSince(start.Format("2006-01-02"), personID)
	if err != nil {
		return nil, fmt.Errorf("list events since: %w", err)
	}

	report := &models.WeeklyReport{
		Start:      start.Format("2006-01-02"),
		End:        time.Now().Format("2006-01-02"),
		EventCount: len(events),
		Persons:    []models.WeeklySlice{},
	}

	byPerson := make(map[string]*models.WeeklySlice)
	var order []string
	for _, e := range events {
		slice, ok := byPerson[e.PersonID]
		if !ok {
			name := e.PersonID
			if person, err := s.personRepo.GetByID(e.PersonID); err == nil {
				name = person.Name
			}
			slice = &models.WeeklySlice{PersonID: e.PersonID, PersonName: name, Events: []string{}, Promises: []string{}}
			byPerson[e.PersonID] = slice
			order = append(order, e.PersonID)
		}
		text := e.Summary
		if text == "" {
			text = firstLine(e.RawText)
		}
		slice.Events = append(slice.Events, fmt.Sprintf("%s %s", e.EventDate, text))
		for _, p := range decodePromises(e.Promises) {
			slice.Promises = append(slice.Promises, fmt.Sprintf("%s：%s%s", p.Who, p.What, deadlineSuffix(p.Deadline)))
		}
	}
	for _, id := range order {
		report.Persons = append(report.Persons, *byPerson[id])
	}

	if len(events) == 0 {
		report.Summary = "本周没有新的记录。"
		report.GeneratedBy = "local"
		return report, nil
	}

	var sections []string
	for _, id := range order {
		slice := byPerson[id]
		sections = append(sections, fmt.Sprintf("## %s\n事件：\n%s\n承诺：\n%s",
			slice.PersonName,
			joinOr(slice.Events, "无"),
			joinOr(slice.Promises, "无")))
	}

	resp, err := s.client.Chat(ctx, s.cfg.AdviceModel, []ai.Message{
		ai.System(weeklyReportPrompt),
		ai.User(fmt.Sprintf("时间范围：%s ~ %s\n\n%s", report.Start, report.End, strings.Join(sections, "\n\n"))),
	}, ai.ChatOptions{Temperature: 0.7, MaxTokens: 1500})
	if err != nil {
		// The structured aggregation is still a readable report without the LLM.
		report.Summary = strings.Join(sections, "\n\n")
		report.GeneratedBy = "local"
		return report, nil
	}
	report.Summary = strings.TrimSpace(resp)
	report.GeneratedBy = s.cfg.AdviceModel
	return report, nil
}

// Reindex rebuilds vec_memory from stored events and traits, which is what changing
// embed_dim or the embedding model requires.
func (s *AIService) Reindex(ctx context.Context) (*models.ReindexResult, error) {
	if _, err := s.client.Embed(ctx, "loom embedding probe"); err != nil {
		return nil, fmt.Errorf("embedding 不可用，已中止重建：%w", err)
	}
	if err := s.vecRepo.Reset(s.cfg.EmbedDim); err != nil {
		return nil, err
	}

	result := &models.ReindexResult{}
	events, err := s.eventRepo.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	for _, e := range events {
		if err := s.indexEvent(ctx, e); err != nil {
			result.Failed++
			continue
		}
		result.EventsIndexed++
	}

	traits, err := s.traitRepo.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	sort.Slice(traits, func(i, j int) bool { return traits[i].PersonID < traits[j].PersonID })
	for _, t := range traits {
		if err := s.indexTrait(ctx, t); err != nil {
			result.Failed++
			continue
		}
		result.TraitsIndexed++
	}
	return result, nil
}

// CheckEmbedding reports whether the configured embedding service answers.
func (s *AIService) CheckEmbedding(ctx context.Context) error {
	_, err := s.client.Embed(ctx, "ping")
	return err
}

func decodePromises(raw string) []models.EventPromise {
	var promises []models.EventPromise
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &promises)
	}
	return promises
}

func formatEvent(e *models.Event) string {
	text := e.Summary
	if text == "" {
		text = firstLine(e.RawText)
	}
	return fmt.Sprintf("[%s] %s %s", e.ID, e.EventDate, text)
}

// formatEventForAdvice is the advice-prompt variant: it appends the feeling, the
// other side's reaction and any promises. Those are the fields a relationship
// question is usually about, and a summary-only line hides them from the model.
func formatEventForAdvice(e *models.Event) string {
	var b strings.Builder
	b.WriteString(formatEvent(e))
	if e.MyFeeling != "" {
		fmt.Fprintf(&b, " 我的感受：%s", e.MyFeeling)
	}
	if e.TheirReaction != "" {
		fmt.Fprintf(&b, " 对方反应：%s", e.TheirReaction)
	}
	if promises := decodePromises(e.Promises); len(promises) > 0 {
		b.WriteString(" 承诺：")
		for i, p := range promises {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(p.Who)
			b.WriteString(" ")
			b.WriteString(p.What)
			if p.Deadline != "" {
				fmt.Fprintf(&b, "（%s）", p.Deadline)
			}
		}
	}
	return b.String()
}

// eventEmbedSource includes feelings, the other side's reaction and promises:
// those carry the relational signals a user's question is usually about, and a
// summary-only blob cannot retrieve them.
func eventEmbedSource(event *models.Event) string {
	var b strings.Builder
	if event.Summary != "" {
		b.WriteString(event.Summary)
	} else {
		b.WriteString(firstLine(event.RawText))
	}
	if event.MyFeeling != "" {
		fmt.Fprintf(&b, " 我的感受：%s", event.MyFeeling)
	}
	if event.TheirReaction != "" {
		fmt.Fprintf(&b, " 对方反应：%s", event.TheirReaction)
	}
	if promises := decodePromises(event.Promises); len(promises) > 0 {
		b.WriteString(" 承诺：")
		for i, p := range promises {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(p.Who)
			b.WriteString(" ")
			b.WriteString(p.What)
			if p.Deadline != "" {
				fmt.Fprintf(&b, "（%s）", p.Deadline)
			}
		}
	}
	return b.String()
}

func allKnownEvents(ids []string, events []*models.Event) bool {
	if len(ids) == 0 {
		return false
	}
	known := make(map[string]bool, len(events))
	for _, e := range events {
		known[e.ID] = true
	}
	for _, id := range ids {
		if !known[id] {
			return false
		}
	}
	return true
}

func filterKnown(ids []string, known map[string]bool) []string {
	out := []string{}
	for _, id := range ids {
		if known[id] {
			out = append(out, id)
		}
	}
	return out
}

func parseJSON[T any](raw string, out *T) error {
	candidates := []string{stripFences(raw)}
	if start, end, ok := jsonRegion(candidates[0]); ok {
		if trimmed := strings.TrimSpace(candidates[0][start : end+1]); trimmed != candidates[0] {
			candidates = append(candidates, trimmed)
		}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if err := json.Unmarshal([]byte(candidate), out); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no JSON document found in: %s", truncate(raw, 200))
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// jsonRegion locates the outermost JSON array or object so prose around it is ignored.
func jsonRegion(s string) (int, int, bool) {
	arrayStart := strings.IndexByte(s, '[')
	objectStart := strings.IndexByte(s, '{')
	open := -1
	closer := byte(0)
	switch {
	case arrayStart >= 0 && (objectStart < 0 || arrayStart < objectStart):
		open, closer = arrayStart, ']'
	case objectStart >= 0:
		open, closer = objectStart, '}'
	default:
		return 0, 0, false
	}
	if idx := strings.LastIndexByte(s, closer); idx > open {
		return open, idx, true
	}
	return 0, 0, false
}

func clampConfidence(c float64) float64 {
	switch {
	case c < 0:
		return 0
	case c > 1:
		return 1
	default:
		return c
	}
}

func joinOr(lines []string, fallback string) string {
	if len(lines) == 0 {
		return fallback
	}
	return strings.Join(lines, "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func deadlineSuffix(deadline string) string {
	if deadline == "" {
		return ""
	}
	return "（" + deadline + "）"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
