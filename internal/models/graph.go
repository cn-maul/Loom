package models

// GraphData is the ready-to-render payload behind /relationships: every node,
// every structured edge and the derived co-attendance pairs, so the page needs
// a single request instead of stitching four lists together.
type GraphData struct {
	Nodes        []*GraphNode   `json:"nodes"`
	Orgs         []*GraphOrg    `json:"orgs"`
	Edges        []*GraphEdge   `json:"edges"`
	CoAttendance []*GraphCoLink `json:"co_attendance"`
}

// GraphNode is one person on the canvas. OrgID/OrgName and EventCount exist so
// the layout can cluster by affiliation and size by activity without extra calls.
type GraphNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsSelf     int    `json:"is_self"`
	OrgID      string `json:"org_id"`
	OrgName    string `json:"org_name"`
	Importance int    `json:"importance"`
	EventCount int    `json:"event_count"`
}

// GraphOrg is an organisation node. Only active organisations appear; archived
// ones stay reachable from the organisation page, not from the graph.
type GraphOrg struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	MemberCount int    `json:"member_count"`
}

// GraphEdge is one edge on the canvas. Kind tells the two edge families apart:
// "relationship" joins two persons, "position" joins a person to an organisation
// (current postings only — a history table is not a graph). Dates travel so the
// client can filter by time window without a second request.
type GraphEdge struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`
	Direction string `json:"direction"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Confirmed int    `json:"confirmed"`
	// SourceEventID is the evidence behind the edge when one was recorded.
	SourceEventID string `json:"source_event_id"`
	Notes         string `json:"notes"`
}

const (
	GraphEdgeRelationship = "relationship"
	GraphEdgePosition     = "position"
)

// GraphCoLink is the derived "shared experience" pair for the canvas: two
// people who attended the same record, anchors included. It is never a typed
// relationship edge and the UI must present it as such.
type GraphCoLink struct {
	A         string `json:"a"`
	B         string `json:"b"`
	AName     string `json:"a_name"`
	BName     string `json:"b_name"`
	SharedCnt int    `json:"shared_count"`
}
