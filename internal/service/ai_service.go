package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

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

// ListModels fetches the chat endpoint's model catalog for the settings page.
// A fresh client is built per call so the just-saved endpoint/key is what gets
// probed, not the one the process started with.
func (s *AIService) ListModels(ctx context.Context) ([]string, error) {
	return ai.NewClient(s.cfg).ListModels(ctx)
}

// IngestEvent runs the record pipeline: extract, persist, embed, refresh the person
// profile. A failing LLM never discards the user's record — the event is still stored
// and the shortfall comes back as a warning, recorded on the row so the UI can offer
// a retry instead of showing an empty summary as if it were a result.
func (s *AIService) IngestEvent(ctx context.Context, event *models.Event) (models.IngestReport, error) {
	report := models.IngestReport{Warnings: []string{}}
	_, attemptedAt := models.NowUTC()

	extraction, err := s.ExtractEvent(ctx, event.RawText)
	if err != nil {
		event.ExtractionStatus = models.ExtractionFailed
		event.ExtractionError = err.Error()
		event.ExtractedAt = &attemptedAt
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
		event.ExtractionStatus = models.ExtractionSucceeded
		event.ExtractionError = ""
		event.ExtractedAt = &attemptedAt
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

	// "Succeeded with warnings" must be visible from the list, not only in the
	// ephemeral response: the warnings ride on the row.
	if err := s.eventRepo.SetPipelineWarnings(event.ID, report.Warnings); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("警告未写入记录：%v", err))
	}

	return report, nil
}

// CreatePendingEvent stores the fact row without calling the model: raw text,
// date and participants land immediately with extraction_status='pending'. The
// queued worker later turns it into a full record.
func (s *AIService) CreatePendingEvent(event *models.Event) error {
	event.ExtractionStatus = models.ExtractionPending
	if event.Promises == "" {
		event.Promises = "[]"
	}
	return s.eventRepo.Create(event)
}

// ListPendingEvents returns records stored but never extracted — the recovery
// set the async queue re-enqueues on startup.
func (s *AIService) ListPendingEvents() ([]*models.Event, error) {
	all, err := s.eventRepo.ListAll()
	if err != nil {
		return nil, err
	}
	var pending []*models.Event
	for _, e := range all {
		if e.ExtractionStatus == models.ExtractionPending {
			pending = append(pending, e)
		}
	}
	return pending, nil
}

// RetryExtraction re-runs the extraction over a record already in the timeline.
// A record a human has curated is not overwritten unless the caller explicitly
// forces it — that is the whole point of tracking manual edits. A failed retry
// keeps the previous content and only records the attempt.
func (s *AIService) RetryExtraction(ctx context.Context, eventID string, force bool) (*models.Event, models.IngestReport, error) {
	report := models.IngestReport{Warnings: []string{}}
	event, err := s.eventRepo.GetByID(eventID)
	if err != nil {
		return nil, report, err
	}
	if event.ManuallyEdited == 1 && !force {
		return nil, report, models.NewError(models.ErrConflict,
			"record %s was edited by hand; force is required to overwrite it", eventID)
	}

	extraction, err := s.ExtractEvent(ctx, event.RawText)
	if err != nil {
		if failErr := s.eventRepo.RecordExtractionFailure(eventID, err.Error()); failErr != nil {
			return nil, report, failErr
		}
		report.Warnings = append(report.Warnings, fmt.Sprintf("AI 提取失败，已保留原有内容：%v", err))
		if event.ManuallyEdited == 1 {
			report.Warnings = append(report.Warnings, "该记录的提取结果由人工修订，未被本次重试覆盖")
		}
		updated, readErr := s.eventRepo.GetByID(eventID)
		if readErr != nil {
			return nil, report, readErr
		}
		return updated, report, nil
	}

	event.Summary = extraction.Summary
	event.MyFeeling = extraction.MyFeeling
	event.TheirReaction = extraction.TheirReaction
	promises, err := json.Marshal(extraction.Promises)
	if err != nil {
		promises = []byte("[]")
	}
	event.Promises = string(promises)
	if err := s.eventRepo.UpdateExtraction(event); err != nil {
		return nil, report, err
	}
	report.Extracted = true

	if err := s.indexEvent(ctx, event); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("向量索引跳过：%v", err))
	} else {
		report.Vectorized = true
	}

	// A retry is the user's answer to "this profile note lost its evidence", so
	// the profile has to be re-derived here as well: rewriting the record is
	// what flagged the note, and only a re-derivation clears the flag.
	if updated, err := s.RefreshTraits(ctx, event.PersonID); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("画像更新失败：%v", err))
	} else {
		report.TraitsUpdated = updated
	}

	refreshed, err := s.eventRepo.GetByID(eventID)
	if err != nil {
		return nil, report, err
	}
	// Same honesty rule as the inline pipeline: warnings outlive the response.
	if err := s.eventRepo.SetPipelineWarnings(eventID, report.Warnings); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("警告未写入记录：%v", err))
	}
	return refreshed, report, nil
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

// AdviceDraft is a generated answer together with the exact context it was drawn
// from, so the caller can persist a session that is reviewable later instead of
// a response body that disappears with the page.
type AdviceDraft struct {
	Answer       *models.AdviceResponse
	UsedEventIDs []string
	UsedTraitIDs []string
	Model        string
}

// GenerateAdvice answers from three context sources: profile traits, the recent
// timeline, and person-scoped semantic hits.
func (s *AIService) GenerateAdvice(ctx context.Context, req models.AdviceRequest) (*AdviceDraft, error) {
	traits, err := s.traitRepo.ListByPerson(req.PersonID)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	recent, err := s.eventRepo.ListByPerson(req.PersonID, 5)
	if err != nil {
		return nil, fmt.Errorf("list recent events: %w", err)
	}
	var notes string
	if p, err := s.personRepo.GetByID(req.PersonID); err == nil {
		notes = p.Notes
	}

	related, relatedTraits, retrievalStatus := s.retrieveRelated(ctx, req.PersonID, req.Question)

	seen := make(map[string]bool, len(recent)+len(related))
	var evidence []*models.Event
	for _, e := range append(append([]*models.Event{}, related...), recent...) {
		if e == nil || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		evidence = append(evidence, e)
	}
	usedEventIDs := make([]string, 0, len(evidence))
	for _, e := range evidence {
		usedEventIDs = append(usedEventIDs, e.ID)
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
	usedTraitIDs := make([]string, 0, len(traitOrder))
	for _, t := range traitOrder {
		usedTraitIDs = append(usedTraitIDs, t.ID)
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
	goalLine := strings.TrimSpace(req.Goal)
	if goalLine == "" {
		goalLine = "未说明"
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := s.client.Chat(ctx, s.cfg.AdviceModel, []ai.Message{
			ai.System(fmt.Sprintf(advicePrompt, joinOr(traitLines, "无"), notes, goalLine, joinOr(eventLines, "无"))),
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
		if strings.TrimSpace(advice.Situation.Text) == "" &&
			strings.TrimSpace(advice.FollowUp.Text) == "" &&
			len(advice.Strategies) == 0 {
			lastErr = fmt.Errorf("advice returned no content: %s", truncate(resp, 200))
			log.Printf("warning: %v (model=%s attempt=%d)", lastErr, s.cfg.AdviceModel, attempt+1)
			continue
		}
		// Evidence the model invents would break the traceability this product
		// promises, so every conclusion is filtered against what it was given.
		normalizeAdviceEvidence(&advice, seen)
		advice.VectorUsed = retrievalStatus == models.RetrievalVectorUsed
		advice.RetrievalStatus = retrievalStatus
		return &AdviceDraft{
			Answer:       &advice,
			UsedEventIDs: usedEventIDs,
			UsedTraitIDs: usedTraitIDs,
			Model:        s.cfg.AdviceModel,
		}, nil
	}
	return nil, lastErr
}

// Retrieval candidate depths. The vector KNN over-fetches, a configured
// rerank pass (llm.rerank_model) rescores the candidates, and the keep counts
// are what reach the prompt — the same limits the plain vector path always used.
const (
	retrievalEventCandidates = 24
	retrievalEventKeep       = 5
	retrievalTraitCandidates = 8
	retrievalTraitKeep       = 3
)

// retrieveRelated returns the records and profile notes nearest to the question
// and reports how the semantic branch fared. The three non-success outcomes are
// kept apart on purpose: "no index is set up", "the search broke" and "nothing
// matched" call for three different next steps from the user.
func (s *AIService) retrieveRelated(ctx context.Context, personID, question string) ([]*models.Event, []*models.Trait, string) {
	if !s.vecRepo.Available() {
		return nil, nil, models.RetrievalVectorDisabled
	}
	embedding, err := s.client.Embed(ctx, question)
	if err != nil {
		return nil, nil, models.RetrievalFailed
	}
	eventResults, err := s.vecRepo.SearchByPerson(embedding, personID, "event_summary", retrievalEventCandidates)
	if err != nil {
		return nil, nil, models.RetrievalFailed
	}
	var events []*models.Event
	for _, r := range eventResults {
		event, err := s.eventRepo.GetByID(r.SourceID)
		if err != nil {
			continue
		}
		events = append(events, event)
	}
	events, err = s.rerankEvents(ctx, question, events)
	if err != nil {
		return nil, nil, models.RetrievalFailed
	}

	traitResults, err := s.vecRepo.SearchByPerson(embedding, personID, "trait", retrievalTraitCandidates)
	if err != nil {
		return events, nil, models.RetrievalFailed
	}
	var traits []*models.Trait
	for _, r := range traitResults {
		trait, err := s.traitRepo.GetByID(r.SourceID)
		if err != nil || trait.Verified == -1 {
			continue
		}
		traits = append(traits, trait)
	}
	traits, err = s.rerankTraits(ctx, question, traits)
	if err != nil {
		return events, nil, models.RetrievalFailed
	}
	if len(events) == 0 && len(traits) == 0 {
		return nil, nil, models.RetrievalNoEvidence
	}
	return events, traits, models.RetrievalVectorUsed
}

// rerankEvents rescores the vector candidates with the configured rerank model
// and keeps the top few. A rerank failure is a hard retrieval failure: a
// configured-but-broken pass must not silently degrade to cosine order and
// misorder evidence without telling anyone.
func (s *AIService) rerankEvents(ctx context.Context, question string, events []*models.Event) ([]*models.Event, error) {
	if len(events) < 2 {
		return events, nil
	}
	if s.cfg.RerankModel == "" {
		return events[:min(len(events), retrievalEventKeep)], nil
	}
	docs := make([]string, len(events))
	for i, e := range events {
		docs[i] = eventEmbedSource(e)
	}
	order, err := s.client.Rerank(ctx, question, docs, retrievalEventKeep)
	if err != nil {
		return nil, err
	}
	ranked := applyRerankOrder(events, order)
	return ranked[:min(len(ranked), retrievalEventKeep)], nil
}

// rerankTraits is the trait-side twin of rerankEvents; the document text is
// the same "key: value" string the trait was indexed with.
func (s *AIService) rerankTraits(ctx context.Context, question string, traits []*models.Trait) ([]*models.Trait, error) {
	if len(traits) < 2 {
		return traits, nil
	}
	if s.cfg.RerankModel == "" {
		return traits[:min(len(traits), retrievalTraitKeep)], nil
	}
	docs := make([]string, len(traits))
	for i, t := range traits {
		docs[i] = t.TraitKey + ": " + t.TraitValue
	}
	order, err := s.client.Rerank(ctx, question, docs, retrievalTraitKeep)
	if err != nil {
		return nil, err
	}
	ranked := applyRerankOrder(traits, order)
	return ranked[:min(len(ranked), retrievalTraitKeep)], nil
}

// applyRerankOrder maps rerank result indexes back onto the candidates, ranked
// first, then any candidate the reranker did not report keeps its cosine order
// (the SDK rejects out-of-range and duplicate indexes, so the residue is only
// candidates beyond the reranker's TopN). Callers truncate to the keep count.
func applyRerankOrder[T any](items []T, order []int) []T {
	ranked := make([]T, 0, len(items))
	seen := make([]bool, len(items))
	for _, idx := range order {
		if idx < 0 || idx >= len(items) || seen[idx] {
			continue
		}
		seen[idx] = true
		ranked = append(ranked, items[idx])
	}
	for i, item := range items {
		if !seen[i] {
			ranked = append(ranked, item)
		}
	}
	return ranked
}

// normalizeAdviceEvidence keeps only citations of records the model was actually
// handed, then rebuilds the flat union from the filtered per-conclusion lists so
// the two can never disagree.
func normalizeAdviceEvidence(a *models.AdviceResponse, known map[string]bool) {
	union := []string{}
	seen := make(map[string]bool)
	collect := func(ids []string) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				union = append(union, id)
			}
		}
	}

	a.Situation.EvidenceEventIDs = filterKnown(a.Situation.EvidenceEventIDs, known)
	collect(a.Situation.EvidenceEventIDs)
	a.OtherPerspective.EvidenceEventIDs = filterKnown(a.OtherPerspective.EvidenceEventIDs, known)
	collect(a.OtherPerspective.EvidenceEventIDs)

	if a.Risks == nil {
		a.Risks = []models.AdvicePoint{}
	}
	for i := range a.Risks {
		a.Risks[i].EvidenceEventIDs = filterKnown(a.Risks[i].EvidenceEventIDs, known)
		collect(a.Risks[i].EvidenceEventIDs)
	}

	if a.Strategies == nil {
		a.Strategies = []models.Strategy{}
	}
	for i := range a.Strategies {
		a.Strategies[i].EvidenceEventIDs = filterKnown(a.Strategies[i].EvidenceEventIDs, known)
		collect(a.Strategies[i].EvidenceEventIDs)
	}

	a.FollowUp.EvidenceEventIDs = filterKnown(a.FollowUp.EvidenceEventIDs, known)
	collect(a.FollowUp.EvidenceEventIDs)

	a.EvidenceEventIDs = union
}

// NarrateReport turns an already-rendered outline of one period into prose. The
// outline is built by the report service from stored rows, so the model is only
// ever rewriting facts the report has already established — it never gets the
// chance to remember a week it did not see. A failure here costs the wording,
// not the report: the caller keeps the structured aggregation and records that
// the narrative is missing.
func (s *AIService) NarrateReport(ctx context.Context, outline string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := s.client.Chat(ctx, s.cfg.AdviceModel, []ai.Message{
			ai.System(reportPrompt),
			ai.User(outline),
		}, ai.ChatOptions{Temperature: 0.5, MaxTokens: 1500})
		if err != nil {
			lastErr = fmt.Errorf("chat: %w", err)
			continue
		}
		if text := strings.TrimSpace(resp); text != "" {
			return text, nil
		}
		lastErr = fmt.Errorf("report narrative came back empty")
		log.Printf("warning: %v (model=%s attempt=%d)", lastErr, s.cfg.AdviceModel, attempt+1)
	}
	return "", lastErr
}

// ReportModel names the model that writes report narratives, so a snapshot can
// record what produced its wording alongside the facts it was given.
func (s *AIService) ReportModel() string { return s.cfg.AdviceModel }

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

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
