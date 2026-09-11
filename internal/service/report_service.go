package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"relationship/internal/models"
	"relationship/internal/repository"

	"github.com/google/uuid"
)

// localReportAuthor marks a report whose text was rendered from the aggregation
// rather than written by a model. It is a real provenance value, not a failure.
const localReportAuthor = "local"

// reportEventLimit caps how many records one report cites. A period with more
// than this is no longer a report; truncating loudly is better than shipping an
// unbounded payload.
const reportEventLimit = 500

// ReportService builds a period report out of stored rows. It is deliberately
// not part of AIService: the facts a report asserts are read from the database
// and have to exist before any model is involved, so that a broken model costs
// the wording and nothing else.
type ReportService struct {
	reports   *repository.ReportRepo
	events    *repository.EventRepo
	followUps *repository.FollowUpRepo
	relations *repository.RelationshipRepo
	positions *repository.PositionRepo
	persons   *PersonService
	ai        *AIService
}

func NewReportService(
	reports *repository.ReportRepo,
	events *repository.EventRepo,
	followUps *repository.FollowUpRepo,
	relations *repository.RelationshipRepo,
	positions *repository.PositionRepo,
	persons *PersonService,
	ai *AIService,
) *ReportService {
	return &ReportService{
		reports: reports, events: events, followUps: followUps,
		relations: relations, positions: positions, persons: persons, ai: ai,
	}
}

// Generate builds a report for one window and stores it. The window is resolved
// first and written back onto the request, so what gets persisted describes the
// period that was actually reported on.
func (s *ReportService) Generate(ctx context.Context, req models.ReportRequest) (*models.Report, error) {
	now := time.Now()
	if err := req.Resolve(now); err != nil {
		return nil, err
	}

	report := &models.Report{
		ID:          uuid.New().String(),
		PersonID:    req.PersonID,
		Start:       req.Start,
		End:         req.End,
		Days:        req.DayCount(),
		Status:      models.ReportSucceeded,
		GeneratedBy: localReportAuthor,
		GeneratedAt: now.UTC().Truncate(time.Second),
		// Every list starts as an empty slice, not nil: a section with nothing
		// in it must render as [] in the API, not null.
		Overdue:     []models.ReportFollowUpRef{},
		DueSoon:     []models.ReportFollowUpRef{},
		Waiting:     []models.ReportFollowUpRef{},
		CarriedOver: []models.ReportFollowUpRef{},
		Upcoming:    []models.ReportFollowUpRef{},
		Completed:   []models.ReportFollowUpRef{},
		Changes:     []models.ReportChangeRef{},
		Events:      []models.ReportEventRef{},
		Promises:    []models.ReportPromise{},
		Persons:     []models.ReportPersonSummary{},
	}
	if req.PersonID != "" {
		person, err := s.persons.GetByID(req.PersonID)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return nil, models.NewError(models.ErrPersonNotFound, "person %s not found", req.PersonID)
			}
			return nil, err
		}
		report.PersonName = person.Name
	}

	if err := s.collect(ctx, report, now); err != nil {
		return nil, err
	}
	// The outline is the aggregation rendered as text. It doubles as the summary
	// when there is nothing to narrate and as the fallback when narrating fails.
	report.Summary = s.outline(report)

	// An empty period is answered locally. Asking a model to describe a stretch
	// of time with nothing in it is precisely how a report invents a busy week.
	if report.HasFindings() {
		narrative, err := s.ai.NarrateReport(ctx, report.Summary)
		switch {
		case err != nil:
			report.Status = models.ReportFailed
			report.FailureReason = err.Error()
		default:
			report.Summary = narrative
			report.GeneratedBy = s.ai.ReportModel()
		}
	}

	if err := s.reports.Create(report); err != nil {
		return nil, err
	}
	return report, nil
}

// Get returns one stored snapshot with its sections.
func (s *ReportService) Get(id string) (*models.Report, error) {
	return s.reports.Get(id)
}

// List returns the report history, newest first.
func (s *ReportService) List(personID string, limit, offset int) ([]*models.ReportSnapshot, error) {
	limit, offset = normalizePage(limit, offset)
	return s.reports.List(personID, limit, offset)
}

func (s *ReportService) Delete(id string) error {
	if _, err := s.reports.Get(id); err != nil {
		return err
	}
	return s.reports.Delete(id)
}

// collect fills every section. Records, open work and graph changes are read
// independently and only meet here, because each answers a different question:
// what happened, what is owed, and what the relationship looks like now.
func (s *ReportService) collect(ctx context.Context, r *models.Report, now time.Time) error {
	names := &reportNames{persons: s.persons, cache: map[string]string{}}
	if r.PersonID != "" {
		names.cache[r.PersonID] = r.PersonName
	}

	events, _, err := s.events.ListFiltered(models.EventFilter{
		PersonID: r.PersonID, From: r.Start, To: r.End, Limit: reportEventLimit,
	})
	if err != nil {
		return fmt.Errorf("list records in period: %w", err)
	}
	// The list endpoint reads newest first; a period summary reads forwards.
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].EventDate != events[j].EventDate {
			return events[i].EventDate < events[j].EventDate
		}
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})

	agg := newPersonAggregate()
	for _, e := range events {
		summary := e.Summary
		if strings.TrimSpace(summary) == "" {
			// A record whose extraction never ran still has to say something.
			// The first line of the note is what the user typed, not a guess.
			summary = firstLine(e.RawText)
		}
		r.Events = append(r.Events, models.ReportEventRef{
			ID:               e.ID,
			PersonID:         e.PersonID,
			PersonName:       names.get(e.PersonID),
			EventDate:        e.EventDate,
			Summary:          summary,
			RecordType:       e.RecordType,
			ExtractionStatus: e.ExtractionStatus,
		})
		// A person-scoped report is about that person even when they merely
		// attended a record somebody else owns.
		owner := e.PersonID
		if r.PersonID != "" {
			owner = r.PersonID
		}
		agg.addEvent(owner, names.get(owner), e.EventDate)

		for _, p := range decodePromises(e.Promises) {
			r.Promises = append(r.Promises, models.ReportPromise{
				EventID:    e.ID,
				PersonID:   owner,
				PersonName: names.get(owner),
				EventDate:  e.EventDate,
				Who:        p.Who,
				What:       p.What,
				Deadline:   p.Deadline,
			})
		}
	}
	r.EventCount = len(r.Events)
	r.PromiseCount = len(r.Promises)

	items, err := s.followUps.List(r.PersonID, "", "", "")
	if err != nil {
		return fmt.Errorf("list follow-ups: %w", err)
	}
	s.bucketFollowUps(r, items, names, now, agg)

	changes, err := s.collectChanges(r, names)
	if err != nil {
		return err
	}
	r.Changes = changes

	r.Persons = agg.summary()
	return nil
}

// bucketFollowUps sorts open work into five disjoint buckets covering every
// unclosed item. Overlap would make the reader reconcile two lists that disagree
// about the same row; a gap would let an item disappear from a report, which is
// the failure mode this whole feature exists to prevent.
func (s *ReportService) bucketFollowUps(r *models.Report, items []*models.FollowUp, names *reportNames, now time.Time, agg *personAggregate) {
	today := now.Format(models.DateLayout)

	for _, f := range items {
		name := names.get(f.PersonID)
		switch f.Status {
		case models.FollowUpCancelled:
			// Cancelled work is not work. Keeping it out of every bucket is the
			// point of cancelling rather than deleting.
			continue
		case models.FollowUpCompleted:
			if completed := localDate(f.CompletedAt, f.UpdatedAt); within(completed, r.Start, r.End) {
				r.Completed = append(r.Completed, followUpRef(f, name, today))
			}
			continue
		}

		ref := followUpRef(f, name, today)
		switch {
		case f.Status == models.FollowUpWaiting:
			// The ball is on the other side: it is neither this period's work
			// nor properly late, and mixing the two hides who owes an answer.
			r.Waiting = append(r.Waiting, ref)
		case f.DueDate != "" && f.DueDate < today:
			r.Overdue = append(r.Overdue, ref)
		case f.DueDate != "" && f.DueDate <= r.End:
			r.DueSoon = append(r.DueSoon, ref)
		case localDate(&f.CreatedAt, f.CreatedAt) < r.Start:
			// Open since before this period began, and not yet late: exactly the
			// item that quietly falls off a weekly review.
			r.CarriedOver = append(r.CarriedOver, ref)
		default:
			// Not yet due, or not yet given a date: next period's plan.
			r.Upcoming = append(r.Upcoming, ref)
		}
		agg.addOpen(f.PersonID, name)
	}

	r.OpenCount = len(r.Overdue) + len(r.DueSoon) + len(r.Waiting) + len(r.CarriedOver) + len(r.Upcoming)
}

// collectChanges reports what moved on the relationship graph inside the period.
// Dated events are used rather than row creation time: an edge entered today
// about a posting that ended in March is history, not news.
func (s *ReportService) collectChanges(r *models.Report, names *reportNames) ([]models.ReportChangeRef, error) {
	links, err := s.relations.List(models.RelationshipFilter{PersonID: r.PersonID})
	if err != nil {
		return nil, fmt.Errorf("list relationships: %w", err)
	}
	positions, err := s.positions.ListAll()
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	changes := []models.ReportChangeRef{}
	for _, link := range links {
		fromID, fromName := link.FromPersonID, link.FromPersonName
		toID, toName := link.ToPersonID, link.ToPersonName
		// A scoped report phrases every edge from the person it is about.
		if r.PersonID != "" && r.PersonID == toID {
			fromID, fromName, toID, toName = toID, toName, fromID, fromName
		}
		for _, edge := range []struct {
			kind string
			date string
		}{
			{models.ReportRelationshipStarted, link.StartDate},
			{models.ReportRelationshipEnded, link.EndDate},
		} {
			if !within(edge.date, r.Start, r.End) {
				continue
			}
			changes = append(changes, models.ReportChangeRef{
				Kind: edge.kind, ID: link.ID,
				PersonID: fromID, PersonName: displayName(fromName, names, fromID),
				CounterpartID: toID, CounterpartName: displayName(toName, names, toID),
				Description: link.RelationType, Date: edge.date,
			})
		}
	}

	for _, pos := range positions {
		if r.PersonID != "" && pos.PersonID != r.PersonID {
			continue
		}
		for _, edge := range []struct {
			kind string
			date string
		}{
			{models.ReportPositionStarted, pos.StartDate},
			{models.ReportPositionEnded, pos.EndDate},
		} {
			if !within(edge.date, r.Start, r.End) {
				continue
			}
			changes = append(changes, models.ReportChangeRef{
				Kind: edge.kind, ID: pos.ID,
				PersonID: pos.PersonID, PersonName: displayName(pos.PersonName, names, pos.PersonID),
				CounterpartID: pos.OrgID, CounterpartName: pos.OrgName,
				Description: pos.Role, Date: edge.date,
			})
		}
	}

	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Date < changes[j].Date })
	return changes, nil
}

// outline renders the aggregation as plain text. It carries names and dates but
// deliberately no ids: the narrative should read the facts, not echo keys that
// the interface already renders as links.
func (s *ReportService) outline(r *models.Report) string {
	if !r.HasFindings() {
		return fmt.Sprintf("%s 至 %s 没有新的记录，也没有待处理的事项。", r.Start, r.End)
	}

	var b strings.Builder
	scope := "全部人物"
	if r.PersonName != "" {
		scope = r.PersonName
	}
	fmt.Fprintf(&b, "时间范围：%s ~ %s（%d 天）\n范围：%s\n本期记录：%d 条\n",
		r.Start, r.End, r.Days, scope, r.EventCount)

	writeSection(&b, "逾期事项", describeFollowUps(r.Overdue, func(ref models.ReportFollowUpRef) string {
		return fmt.Sprintf("%s：%s（原定 %s，已逾期 %d 天）", ref.PersonName, ref.Title, ref.DueDate, ref.DaysOverdue)
	}))
	writeSection(&b, "本期到期", describeFollowUps(r.DueSoon, func(ref models.ReportFollowUpRef) string {
		return fmt.Sprintf("%s：%s（%s 到期）", ref.PersonName, ref.Title, ref.DueDate)
	}))
	writeSection(&b, "等待对方回复", describeFollowUps(r.Waiting, func(ref models.ReportFollowUpRef) string {
		return fmt.Sprintf("%s：%s（我方已发出，等待回应）", ref.PersonName, ref.Title)
	}))
	writeSection(&b, "跨期未完成", describeFollowUps(r.CarriedOver, func(ref models.ReportFollowUpRef) string {
		return fmt.Sprintf("%s：%s（本期内一直未完成）", ref.PersonName, ref.Title)
	}))
	writeSection(&b, "本期完成", describeFollowUps(r.Completed, func(ref models.ReportFollowUpRef) string {
		return fmt.Sprintf("%s：%s", ref.PersonName, ref.Title)
	}))
	writeSection(&b, "关系与任职变化", changeLines(r.Changes))
	writeSection(&b, "本期记录", eventLines(r.Events))
	writeSection(&b, "本期承诺", promiseLines(r.Promises))
	writeSection(&b, "下一步可执行", describeFollowUps(r.Upcoming, func(ref models.ReportFollowUpRef) string {
		if ref.DueDate != "" {
			return fmt.Sprintf("%s：%s（%s 前）", ref.PersonName, ref.Title, ref.DueDate)
		}
		return fmt.Sprintf("%s：%s（未定日期）", ref.PersonName, ref.Title)
	}))
	return strings.TrimRight(b.String(), "\n")
}

func writeSection(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s：\n", title)
	for _, line := range lines {
		fmt.Fprintf(b, "- %s\n", line)
	}
}

func describeFollowUps(refs []models.ReportFollowUpRef, render func(models.ReportFollowUpRef) string) []string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		lines = append(lines, render(ref))
	}
	return lines
}

func changeLines(changes []models.ReportChangeRef) []string {
	lines := make([]string, 0, len(changes))
	for _, c := range changes {
		verb := map[string]string{
			models.ReportRelationshipStarted: "建立关系",
			models.ReportRelationshipEnded:   "结束关系",
			models.ReportPositionStarted:     "开始任职",
			models.ReportPositionEnded:       "结束任职",
		}[c.Kind]
		who := c.PersonName
		if who == "" {
			who = "某人"
		}
		what := c.Description
		if what == "" {
			what = "身份"
		}
		line := fmt.Sprintf("%s 于 %s %s：%s", who, c.Date, verb, what)
		if c.CounterpartName != "" {
			line += fmt.Sprintf("（对方：%s）", c.CounterpartName)
		}
		lines = append(lines, line)
	}
	return lines
}

func eventLines(events []models.ReportEventRef) []string {
	lines := make([]string, 0, len(events))
	for _, e := range events {
		lines = append(lines, fmt.Sprintf("%s %s：%s", e.EventDate, e.PersonName, e.Summary))
	}
	return lines
}

func promiseLines(promises []models.ReportPromise) []string {
	lines := make([]string, 0, len(promises))
	for _, p := range promises {
		what := p.What
		if p.Deadline != "" {
			what += "（" + p.Deadline + "）"
		}
		lines = append(lines, fmt.Sprintf("%s %s：%s %s", p.EventDate, p.PersonName, p.Who, what))
	}
	return lines
}

// followUpRef projects one stored item into the report's reference form.
func followUpRef(f *models.FollowUp, personName, today string) models.ReportFollowUpRef {
	ref := models.ReportFollowUpRef{
		ID: f.ID, PersonID: f.PersonID, PersonName: personName,
		Title: f.Title, Description: f.Description, Status: f.Status, Owner: f.Owner,
		DueDate: f.DueDate, DueText: f.DueText,
		SourceEventID: f.SourceEventID, SourceAdviceID: f.SourceAdviceID,
	}
	// Lateness is reported wherever it exists, not only in the overdue bucket:
	// an item waiting on the other side past its date is late too, and hiding
	// that would make the waiting list look calm.
	if f.DueDate != "" && f.DueDate < today {
		ref.DaysOverdue = daysBetween(f.DueDate, today)
	}
	return ref
}

// reportNames resolves display names with a cache. A report over a busy period
// touches the same handful of people hundreds of times.
type reportNames struct {
	persons *PersonService
	cache   map[string]string
}

func (n *reportNames) get(id string) string {
	if id == "" {
		return ""
	}
	if name, ok := n.cache[id]; ok {
		return name
	}
	name := ""
	if person, err := n.persons.GetByID(id); err == nil {
		name = person.Name
	}
	n.cache[id] = name
	return name
}

// displayName prefers a name that came from a join and falls back to a lookup,
// so the report never shows a raw id where a person is expected.
func displayName(joined string, names *reportNames, id string) string {
	if strings.TrimSpace(joined) != "" {
		return joined
	}
	return names.get(id)
}

// personAggregate accumulates the per-person view while the sections are built,
// so it costs no extra query.
type personAggregate struct {
	order     []string
	names     map[string]string
	events    map[string]int
	lastEvent map[string]string
	open      map[string]int
}

func newPersonAggregate() *personAggregate {
	return &personAggregate{
		names: map[string]string{}, events: map[string]int{},
		lastEvent: map[string]string{}, open: map[string]int{},
	}
}

func (a *personAggregate) addEvent(id, name, date string) {
	if id == "" {
		// A record whose primary person was deleted keeps its place in the
		// timeline but cannot be attributed to anyone.
		return
	}
	a.touch(id, name)
	a.events[id]++
	if date > a.lastEvent[id] {
		a.lastEvent[id] = date
	}
}

func (a *personAggregate) addOpen(id, name string) {
	if id == "" {
		return
	}
	a.touch(id, name)
	a.open[id]++
}

func (a *personAggregate) touch(id, name string) {
	if _, ok := a.names[id]; !ok {
		a.order = append(a.order, id)
	}
	if name != "" {
		a.names[id] = name
	}
}

func (a *personAggregate) summary() []models.ReportPersonSummary {
	out := make([]models.ReportPersonSummary, 0, len(a.order))
	for _, id := range a.order {
		out = append(out, models.ReportPersonSummary{
			PersonID:      id,
			PersonName:    a.names[id],
			EventCount:    a.events[id],
			LastEventDate: a.lastEvent[id],
			OpenFollowUps: a.open[id],
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastEventDate != out[j].LastEventDate {
			return out[i].LastEventDate > out[j].LastEventDate
		}
		return out[i].PersonName < out[j].PersonName
	})
	return out
}

// localDate renders a stored UTC timestamp as the local calendar date, which is
// what every user-facing date in this app is. Comparing the raw UTC text would
// put something created at 07:00 in Beijing on the wrong side of midnight.
func localDate(primary *time.Time, fallback time.Time) string {
	if primary != nil && !primary.IsZero() {
		return primary.Local().Format(models.DateLayout)
	}
	if fallback.IsZero() {
		return ""
	}
	return fallback.Local().Format(models.DateLayout)
}

func within(date, start, end string) bool {
	return date != "" && date >= start && date <= end
}

func daysBetween(from, to string) int {
	a, err := time.Parse(models.DateLayout, from)
	if err != nil {
		return 0
	}
	b, err := time.Parse(models.DateLayout, to)
	if err != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}
