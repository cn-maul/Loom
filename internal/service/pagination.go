package service

// Paging defaults shared by every list endpoint: a caller that asks for nothing
// gets a usable page, and a caller that asks for the whole table gets a bounded
// one.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// normalizePage clamps the requested window so a caller cannot ask for the whole
// table in one request.
func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
