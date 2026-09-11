package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

// graphAPI seeds a throwaway database with the whole picture: three persons,
// one organisation, anchored and participant records, one active and one ended
// relationship edge, one current and one ended posting.
func graphAPI(t *testing.T) *echo.Echo {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "graph_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO organizations (id, name, kind) VALUES ('o1', '腾讯', '公司')`,
		`INSERT INTO persons (id, name, is_self, org_id) VALUES ('p1', '我', 1, 'o1')`,
		`INSERT INTO persons (id, name, org_id) VALUES ('p2', '张总', 'o1')`,
		`INSERT INTO persons (id, name) VALUES ('p3', '李工')`,
		// e1 has only an anchored person (no participant rows): the graph's
		// co-attendance must still count 我×张总 through the union.
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e1', 'p1', '2026-09-01', '我和张总开会', datetime('now'))`,
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e2', 'p2', '2026-09-02', '张总和李工吃饭', datetime('now'))`,
		`INSERT INTO event_participants (event_id, person_id, role) VALUES ('e1', 'p2', '')`,
		`INSERT INTO event_participants (event_id, person_id, role) VALUES ('e2', 'p2', 'primary')`,
		`INSERT INTO event_participants (event_id, person_id, role) VALUES ('e2', 'p3', '')`,
		`INSERT INTO person_relationships (id, from_person_id, to_person_id, relation_type, direction, confirmed, created_at, updated_at)
		 VALUES ('r1', 'p2', 'p3', '同事', 'undirected', 1, datetime('now'), datetime('now'))`,
		`INSERT INTO person_relationships (id, from_person_id, to_person_id, relation_type, direction, confirmed, end_date, created_at, updated_at)
		 VALUES ('r2', 'p1', 'p2', '朋友', 'directed', 1, '2026-01-01', datetime('now'), datetime('now'))`,
		`INSERT INTO person_org_positions (id, person_id, org_id, role) VALUES ('pos1', 'p1', 'o1', '工程师')`,
		// Ended posting: history, not a living edge, so it must not appear.
		`INSERT INTO person_org_positions (id, person_id, org_id, role, end_date) VALUES ('pos2', 'p3', 'o1', '实习', '2026-02-01')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	graphService := service.NewGraphService(
		repository.NewPersonRepo(database),
		repository.NewGraphRepo(database),
	)
	e := echo.New()
	e.GET("/api/graph", NewGraphHandler(graphService).Get)
	return e
}

func decodeGraph(t *testing.T, resp models.APIResponse) models.GraphData {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data models.GraphData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	return data
}

func TestGraphPayload(t *testing.T) {
	e := graphAPI(t)

	status, resp := doJSON(t, e, http.MethodGet, "/api/graph", "")
	if status != http.StatusOK {
		t.Fatalf("graph returned %d: %+v", status, resp.Error)
	}
	data := decodeGraph(t, resp)

	// Nodes: every person, with self flag, org name and activity counts.
	if len(data.Nodes) != 3 {
		t.Fatalf("graph holds %d person nodes, want 3", len(data.Nodes))
	}
	var self *models.GraphNode
	for _, node := range data.Nodes {
		if node.ID == "p1" {
			self = node
		}
	}
	if self == nil {
		t.Fatalf("self node missing: %+v", data.Nodes)
	}
	if self.IsSelf != 1 || self.OrgName != "腾讯" || self.EventCount != 1 {
		t.Fatalf("self node = %+v", self)
	}

	// Organisations: active only, with current member counts. pos2 has ended,
	// so 腾讯 currently counts exactly one member.
	if len(data.Orgs) != 1 || data.Orgs[0].ID != "o1" || data.Orgs[0].MemberCount != 1 {
		t.Fatalf("orgs = %+v", data.Orgs)
	}

	// Edges: both relationship edges (history stays readable) plus only the
	// current posting. The ended posting must not become an edge.
	if len(data.Edges) != 3 {
		t.Fatalf("edges = %d, want 3 (%+v)", len(data.Edges), data.Edges)
	}
	kinds := map[string]int{}
	for _, edge := range data.Edges {
		kinds[edge.Kind]++
	}
	if kinds[models.GraphEdgeRelationship] != 2 || kinds[models.GraphEdgePosition] != 1 {
		t.Fatalf("edge kinds = %+v", kinds)
	}
	var endedFound bool
	for _, edge := range data.Edges {
		if edge.ID == "r2" {
			endedFound = edge.EndDate == "2026-01-01"
		}
		if edge.ID == "pos2" {
			t.Fatal("ended posting must not appear as a graph edge")
		}
	}
	if !endedFound {
		t.Fatal("ended relationship r2 must stay in the payload with its end_date")
	}

	// Co-attendance: the anchored person joins via the union, so 我×张总 counts
	// even though e1 has no participant row for 我.
	if len(data.CoAttendance) != 2 {
		t.Fatalf("co-attendance pairs = %+v, want 2", data.CoAttendance)
	}
	pairs := map[string]int{}
	for _, link := range data.CoAttendance {
		pairs[link.A+"<"+link.B] = link.SharedCnt
		if link.SharedCnt != 1 {
			t.Fatalf("co-attendance counts wrong: %+v", data.CoAttendance)
		}
	}
	if _, ok := pairs["p1<p2"]; !ok {
		t.Fatalf("anchor-only record must produce 我×张总 pair, got %+v", pairs)
	}
	if _, ok := pairs["p2<p3"]; !ok {
		t.Fatalf("participant pair missing: %+v", pairs)
	}
}
