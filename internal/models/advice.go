package models

import (
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"time"
)

// Retrieval outcomes for the semantic branch behind an answer. The three
// non-success states are deliberately distinct, because they ask the user to do
// different things: turn the embedding service on, fix it, or accept that the
// timeline genuinely holds nothing about this question.
const (
	// RetrievalVectorUsed means semantic search returned context that was used.
	RetrievalVectorUsed = "vector_used"
	// RetrievalNoEvidence means search ran and matched nothing relevant.
	RetrievalNoEvidence = "no_relevant_evidence"
	// RetrievalFailed means search errored; the answer fell back to recency.
	RetrievalFailed = "retrieval_failed"
	// RetrievalVectorDisabled means no vector index was available at all.
	RetrievalVectorDisabled = "vector_disabled"
)

// RetrievalStatuses is the closed set the advice pipeline reports.
var RetrievalStatuses = map[string]bool{
	RetrievalVectorUsed:     true,
	RetrievalNoEvidence:     true,
	RetrievalFailed:         true,
	RetrievalVectorDisabled: true,
}

// AdviceRequest is what the user asks for. Goal is kept apart from Question
// because the same question with a different goal leads to different advice:
// "how do I tell him" means something else when the goal is salvaging the
// relationship than when it is a clean exit.
type AdviceRequest struct {
	PersonID string `json:"person_id"`
	Question string `json:"question"`
	Goal     string `json:"goal"`
}

func (r *AdviceRequest) Validate() error {
	if strings.TrimSpace(r.PersonID) == "" {
		return NewError(ErrInvalidInput, "person_id is required")
	}
	if strings.TrimSpace(r.Question) == "" {
		return NewError(ErrInvalidInput, "question is required")
	}
	r.Question = strings.TrimSpace(r.Question)
	r.Goal = strings.TrimSpace(r.Goal)
	return nil
}

// AdvicePoint is one conclusion together with the records it rests on. Evidence
// is attached per conclusion rather than once for the whole answer, so a line
// drawn from the timeline can be opened while a line of pure reasoning is not
// dressed up as a fact. An empty list means "no direct evidence", which the UI
// is expected to say out loud.
type AdvicePoint struct {
	Text             string   `json:"text"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
}

// Strategy is one recommended course of action. EvidenceEventIDs names the
// records the recommendation rests on; empty means it is advice, not a fact.
type Strategy struct {
	Name             string   `json:"name"`
	Script           string   `json:"script"`
	Pros             string   `json:"pros"`
	Cons             string   `json:"cons"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
}

// AdviceResponse is the generated answer. Every conclusion carries its own
// evidence so the interface can make "each line opens its source, or shows that
// it has none" true rather than aspirational.
type AdviceResponse struct {
	Situation        AdvicePoint   `json:"situation"`
	OtherPerspective AdvicePoint   `json:"other_perspective"`
	Risks            []AdvicePoint `json:"risks"`
	Strategies       []Strategy    `json:"strategies"`
	FollowUp         AdvicePoint   `json:"follow_up"`
	// EvidenceEventIDs is the union of every cited record, kept so the evidence
	// list does not have to walk the sections.
	EvidenceEventIDs []string `json:"evidence_event_ids"`
	// VectorUsed reports whether the semantic branch contributed context.
	VectorUsed      bool   `json:"vector_used"`
	RetrievalStatus string `json:"retrieval_status"`
}

// EvidenceVersion records what the evidence looked like when the answer was
// drawn. Staleness is then a comparison against the current rows, not a guess:
// a record edited after the fact makes the old conclusion visibly out of date
// instead of silently wrong.
type EvidenceVersion struct {
	EventID    string `json:"event_id"`
	ContentRev string `json:"content_rev"`
}

// AdviceSession is a stored answer plus the context it was drawn from. Persisting
// the context is what makes an old answer reviewable: the question, the goal,
// the model and the exact records that were in front of it are all recoverable.
type AdviceSession struct {
	ID       string `json:"id"`
	PersonID string `json:"person_id"`
	Question string `json:"question"`
	Goal     string `json:"goal"`
	// AdviceResponse is inlined so the generated answer reads as one flat object.
	AdviceResponse
	// UsedEventIDs and UsedTraitIDs are everything that went into the prompt,
	// which is a superset of what the answer ended up citing.
	UsedEventIDs    []string          `json:"used_event_ids"`
	UsedTraitIDs    []string          `json:"used_trait_ids"`
	EvidenceVersion []EvidenceVersion `json:"evidence_version"`
	Model           string            `json:"model"`
	// AdoptedStrategyIndex is nil until the user picks a strategy to act on.
	AdoptedStrategyIndex *int   `json:"adopted_strategy_index,omitempty"`
	AdoptedStrategyName  string `json:"adopted_strategy_name"`
	// FollowUpIDs are the follow-ups created from this advice, resolved on read.
	FollowUpIDs []string `json:"follow_up_ids"`
	// SourceStale is computed on read by comparing EvidenceVersion with the
	// records as they are now; it is never trusted from storage.
	SourceStale       int       `json:"source_stale"`
	SourceStaleReason string    `json:"source_stale_reason"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AdoptRequest turns one strategy into a concrete commitment. Title and DueText
// are optional: without them the follow-up is seeded from the strategy name.
type AdoptRequest struct {
	StrategyIndex int    `json:"strategy_index"`
	Title         string `json:"title"`
	Owner         string `json:"owner"`
	DueText       string `json:"due_text"`
	DueDate       string `json:"due_date"`
}

func (r *AdoptRequest) Validate() error {
	if r.StrategyIndex < 0 {
		return NewError(ErrInvalidInput, "strategy_index must not be negative")
	}
	if r.DueDate != "" {
		if _, err := time.Parse("2006-01-02", r.DueDate); err != nil {
			return NewError(ErrInvalidInput, "due_date must be YYYY-MM-DD")
		}
	}
	return nil
}

// EventContentRev fingerprints the evidence-bearing fields of a record. It is
// derived from content rather than a timestamp, because records carry no
// updated_at and a rewrite that changes nothing must not invalidate a
// conclusion that is still exactly right.
func EventContentRev(e *Event) string {
	h := fnv.New64a()
	for _, part := range []string{e.EventDate, e.RawText, e.Summary, e.MyFeeling, e.TheirReaction, e.Promises} {
		_, _ = io.WriteString(h, part)
		_, _ = h.Write([]byte{0})
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// EvidenceIDs lists the records whose versions were recorded. Staleness is
// evaluated over all of them rather than only over the ones the answer ended up
// citing: a record that was in front of the model shaped the answer even when no
// single line points at it.
func (s *AdviceSession) EvidenceIDs() []string {
	ids := make([]string, 0, len(s.EvidenceVersion))
	for _, v := range s.EvidenceVersion {
		ids = append(ids, v.EventID)
	}
	return ids
}

// EvaluateEvidence compares the stored snapshot with the records as they are now
// and annotates the session. A missing record and a rewritten one are the same
// conclusion for the reader — this answer no longer rests on what it cites — so
// both set the marker, with a reason that says which happened.
func (s *AdviceSession) EvaluateEvidence(current map[string]string) {
	stale := []string{}
	for _, v := range s.EvidenceVersion {
		rev, ok := current[v.EventID]
		if !ok {
			stale = append(stale, "来源记录已删除")
			continue
		}
		if rev != v.ContentRev {
			stale = append(stale, "来源记录已修改")
		}
	}
	if len(stale) == 0 {
		s.SourceStale = 0
		s.SourceStaleReason = ""
		return
	}
	s.SourceStale = 1
	s.SourceStaleReason = strings.Join(uniqueStrings(stale), "、") + "，建议需要重新生成"
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
