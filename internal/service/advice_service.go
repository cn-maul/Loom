package service

import (
	"context"
	"errors"
	"strings"

	"relationship/internal/models"
	"relationship/internal/repository"

	"github.com/google/uuid"
)

// AdviceService owns the life of an answer: it is produced by the AI pipeline,
// stored with the context it came from, reviewed later against the records as
// they are now, and finally turned into an action. Keeping that in one place is
// what makes "why did I do this?" answerable months later.
type AdviceService struct {
	repo      *repository.AdviceRepo
	events    *repository.EventRepo
	followUps *repository.FollowUpRepo
	persons   *PersonService
	ai        *AIService
}

func NewAdviceService(
	repo *repository.AdviceRepo,
	events *repository.EventRepo,
	followUps *repository.FollowUpRepo,
	persons *PersonService,
	ai *AIService,
) *AdviceService {
	return &AdviceService{repo: repo, events: events, followUps: followUps, persons: persons, ai: ai}
}

// Generate answers a question and stores the session. Persisting is not a side
// effect here but the point: an answer the user cannot find again cannot be
// relied on, and the evidence snapshot is only meaningful if it is taken while
// the context is still exactly what the model saw.
func (s *AdviceService) Generate(ctx context.Context, req models.AdviceRequest) (*models.AdviceSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.persons.GetByID(req.PersonID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return nil, models.NewError(models.ErrPersonNotFound, "person %s not found", req.PersonID)
		}
		return nil, err
	}

	draft, err := s.ai.GenerateAdvice(ctx, req)
	if err != nil {
		return nil, err
	}
	versions, err := s.snapshotEvidence(draft.UsedEventIDs)
	if err != nil {
		return nil, err
	}

	session := &models.AdviceSession{
		ID:              uuid.New().String(),
		PersonID:        req.PersonID,
		Question:        req.Question,
		Goal:            req.Goal,
		AdviceResponse:  *draft.Answer,
		UsedEventIDs:    draft.UsedEventIDs,
		UsedTraitIDs:    draft.UsedTraitIDs,
		EvidenceVersion: versions,
		Model:           draft.Model,
		FollowUpIDs:     []string{},
	}
	if err := s.repo.Create(session); err != nil {
		return nil, err
	}
	return s.Decorate(session)
}

// Get returns one session with its staleness evaluated against the records as
// they are now, and the follow-ups it produced.
func (s *AdviceService) Get(id string) (*models.AdviceSession, error) {
	session, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	return s.Decorate(session)
}

// List returns the advice history, newest first.
func (s *AdviceService) List(personID string, limit, offset int) ([]*models.AdviceSession, error) {
	limit, offset = normalizePage(limit, offset)
	sessions, err := s.repo.List(personID, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return sessions, nil
	}

	adviceIDs := make([]string, 0, len(sessions))
	cited := []string{}
	for _, session := range sessions {
		adviceIDs = append(adviceIDs, session.ID)
		cited = append(cited, session.EvidenceIDs()...)
	}
	// One query for every record in the page, and one for every follow-up link,
	// so a history of N answers still costs two reads.
	current, err := s.events.ContentRevs(cited)
	if err != nil {
		return nil, err
	}
	linked, err := s.followUps.ListIDsByAdvice(adviceIDs)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		session.EvaluateEvidence(current)
		session.FollowUpIDs = linked[session.ID]
	}
	return sessions, nil
}

// Delete removes an answer. Follow-ups that came out of it are flagged first,
// because the foreign key detaches the link and would otherwise erase the only
// trace of why the item exists.
func (s *AdviceService) Delete(id string) error {
	if _, err := s.repo.Get(id); err != nil {
		return err
	}
	if _, err := s.followUps.MarkSourceAdviceStale(id, "来源建议已删除，依据需要重新评估"); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

// Adopt turns one recommended strategy into a commitment. The strategy's script
// becomes the item's description, so the words that were going to be used are
// still there when the moment comes.
func (s *AdviceService) Adopt(id string, req models.AdoptRequest) (*models.AdviceSession, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	session, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if req.StrategyIndex >= len(session.Strategies) {
		return nil, models.NewError(models.ErrInvalidInput,
			"strategy_index %d is out of range, this advice has %d strategies", req.StrategyIndex, len(session.Strategies))
	}
	strategy := session.Strategies[req.StrategyIndex]

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSpace(strategy.Name)
	}
	if title == "" {
		title = session.Question
	}

	followUp := &models.FollowUp{
		ID:             uuid.New().String(),
		PersonID:       session.PersonID,
		Title:          title,
		Description:    strings.TrimSpace(strategy.Script),
		Owner:          strings.TrimSpace(req.Owner),
		DueText:        strings.TrimSpace(req.DueText),
		DueDate:        strings.TrimSpace(req.DueDate),
		Status:         models.FollowUpPending,
		SourceAdviceID: session.ID,
	}
	if err := followUp.Validate(); err != nil {
		return nil, err
	}
	if err := s.followUps.Create(followUp); err != nil {
		return nil, err
	}
	if err := s.repo.SetAdopted(session.ID, req.StrategyIndex, strategy.Name); err != nil {
		return nil, err
	}

	session.AdoptedStrategyIndex = &req.StrategyIndex
	session.AdoptedStrategyName = strategy.Name
	return s.Decorate(session)
}

// Decorate fills the derived fields: staleness against the current records and
// the follow-ups this advice produced. Both are computed rather than stored, so
// they cannot drift out of date.
func (s *AdviceService) Decorate(session *models.AdviceSession) (*models.AdviceSession, error) {
	current, err := s.events.ContentRevs(session.EvidenceIDs())
	if err != nil {
		return nil, err
	}
	session.EvaluateEvidence(current)

	linked, err := s.followUps.ListIDsByAdvice([]string{session.ID})
	if err != nil {
		return nil, err
	}
	session.FollowUpIDs = linked[session.ID]
	return session, nil
}

// snapshotEvidence fingerprints the records that were actually put in front of
// the model. A record that vanished between retrieval and persistence is still
// recorded — with an empty fingerprint, which reads as "deleted" — because it
// genuinely was part of the context.
func (s *AdviceService) snapshotEvidence(eventIDs []string) ([]models.EvidenceVersion, error) {
	if len(eventIDs) == 0 {
		return []models.EvidenceVersion{}, nil
	}
	revs, err := s.events.ContentRevs(eventIDs)
	if err != nil {
		return nil, err
	}
	versions := make([]models.EvidenceVersion, 0, len(eventIDs))
	for _, id := range eventIDs {
		versions = append(versions, models.EvidenceVersion{EventID: id, ContentRev: revs[id]})
	}
	return versions, nil
}
