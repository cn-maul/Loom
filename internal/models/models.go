package models

import "time"

type Person struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Relation   string    `json:"relation"`
	Importance int       `json:"importance"`
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Event struct {
	ID             string    `json:"id"`
	PersonID       string    `json:"person_id"`
	RawText        string    `json:"raw_text"`
	EventDate      string    `json:"event_date"`
	Summary        string    `json:"summary"`
	MyFeeling      string    `json:"my_feeling"`
	TheirReaction  string    `json:"their_reaction"`
	Promises       string    `json:"promises"`
	CreatedAt      time.Time `json:"created_at"`
}

type Trait struct {
	ID              string    `json:"id"`
	PersonID        string    `json:"person_id"`
	TraitKey        string    `json:"trait_key"`
	TraitValue      string    `json:"trait_value"`
	Confidence      float64   `json:"confidence"`
	SourceEventIDs  string    `json:"source_event_ids"`
	Verified        int       `json:"verified"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type EventExtraction struct {
	Summary        string   `json:"summary"`
	MyFeeling      string   `json:"my_feeling"`
	TheirReaction  string   `json:"their_reaction"`
	Promises       []Promise `json:"promises"`
}

type Promise struct {
	Who      string `json:"who"`
	What     string `json:"what"`
	Deadline string `json:"deadline"`
}

type AdviceRequest struct {
	PersonID string `json:"person_id"`
	Question string `json:"question"`
}

type AdviceResponse struct {
	Situation        string   `json:"situation"`
	OtherPerspective string   `json:"other_perspective"`
	Risks            []string `json:"risks"`
	Strategies       []Strategy `json:"strategies"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
	FollowUp         string   `json:"follow_up"`
}

type Strategy struct {
	Name  string `json:"name"`
	Script string `json:"script"`
	Pros  string `json:"pros"`
	Cons  string `json:"cons"`
}

type APIResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data"`
	Error *APIError   `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}