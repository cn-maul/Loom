package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newAPIRouter(t *testing.T) *echo.Echo {
	t.Helper()
	a := newTestApp(t)
	e, err := a.NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	e.Logger.SetOutput(io.Discard) // the error handler logs 5xx; tests need quiet
	return e
}

func doRequest(t *testing.T, e *echo.Echo, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// apiEnvelope is the part of the error contract a client actually depends on.
type apiEnvelope struct {
	OK    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// TestUnroutedAPIPathAnswersJSONError is the regression guard for the worst
// answer this server ever gave: an API typo answered with the SPA shell and a
// 200. The caller sees a parse error and learns nothing about the URL.
func TestUnroutedAPIPathAnswersJSONError(t *testing.T) {
	e := newAPIRouter(t)

	cases := []struct {
		method, path string
		status       int
		code         string
		why          string
	}{
		{http.MethodGet, "/api/typo", 404, "NOT_FOUND", "an unrouted GET must not serve the app shell"},
		{http.MethodGet, "/api/typo/deeper", 404, "NOT_FOUND", "depth does not change the rule"},
		{http.MethodGet, "/api", 404, "NOT_FOUND", "the bare group root is not a page either"},
		{http.MethodGet, "/api/", 404, "NOT_FOUND", "trailing slash included"},
		{http.MethodPost, "/api/typo", 404, "NOT_FOUND", "a write to a nonexistent path is a wrong URL"},
		{http.MethodDelete, "/api/typo", 404, "NOT_FOUND", "same for DELETE"},
		{http.MethodPatch, "/api/persons", 405, "METHOD_NOT_ALLOWED", "a known path with an unsupported verb stays 405"},
		{http.MethodGet, "/api/persons", 200, "", "the happy path is untouched"},
	}

	for _, tc := range cases {
		rec := doRequest(t, e, tc.method, tc.path)
		if rec.Code != tc.status {
			t.Errorf("%s %s = %d, want %d (%s)", tc.method, tc.path, rec.Code, tc.status, tc.why)
			continue
		}
		if v := rec.Header().Get("X-API-Version"); v != APIVersion {
			t.Errorf("%s %s: X-API-Version = %q, want %q", tc.method, tc.path, v, APIVersion)
		}
		if tc.status == http.StatusOK {
			continue
		}

		body := rec.Body.String()
		if strings.Contains(strings.ToLower(body), "<!doctype") {
			t.Errorf("%s %s answered with the app shell: %s", tc.method, tc.path, body)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s %s: Content-Type = %q, want application/json", tc.method, tc.path, ct)
		}
		var env apiEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Errorf("%s %s: body is not JSON (%v): %s", tc.method, tc.path, err, body)
			continue
		}
		if env.OK || env.Error == nil {
			t.Errorf("%s %s: want ok:false with an error object, got %s", tc.method, tc.path, body)
			continue
		}
		if env.Error.Code != tc.code {
			t.Errorf("%s %s: code = %q, want %q", tc.method, tc.path, env.Error.Code, tc.code)
		}
		if env.Error.Message == "" {
			t.Errorf("%s %s: error message is empty", tc.method, tc.path)
		}
	}

	// A 405 still owes the caller the list of verbs that would work.
	rec := doRequest(t, e, http.MethodPatch, "/api/persons")
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodPost) {
		t.Errorf("405 lost its Allow header: Allow = %q", allow)
	}
}

// TestSPAPathsStillServeTheShell keeps the fix narrow: page paths and asset
// paths keep the index.html fallback.
func TestSPAPathsStillServeTheShell(t *testing.T) {
	e := newAPIRouter(t)
	for _, path := range []string{"/", "/persons/6f1c", "/settings/deep/page"} {
		rec := doRequest(t, e, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s: Content-Type = %q, want text/html", path, ct)
		}
		if !strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html") {
			t.Errorf("GET %s did not serve the app shell", path)
		}
	}
}

// TestAPIErrorHandlerHidesServerDetail pins two things at once: a recovered
// panic reads as INTERNAL rather than Echo's default shape, and the panic
// value itself never reaches the client.
func TestAPIErrorHandlerHidesServerDetail(t *testing.T) {
	e := newAPIRouter(t)

	cases := []struct {
		name   string
		err    error
		status int
		code   string
		leak   string
	}{
		{
			name:   "recovered panic",
			err:    echo.NewHTTPError(http.StatusInternalServerError, "panic: bearer token abc123"),
			status: http.StatusInternalServerError,
			code:   "INTERNAL",
			leak:   "abc123",
		},
		{
			name:   "binder refusal",
			err:    echo.NewHTTPError(http.StatusBadRequest, "unexpected end of JSON input"),
			status: http.StatusBadRequest,
			code:   "INVALID_INPUT",
		},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/persons", nil), rec)
		e.HTTPErrorHandler(tc.err, c)

		if rec.Code != tc.status {
			t.Fatalf("%s: status = %d, want %d", tc.name, rec.Code, tc.status)
		}
		body := rec.Body.String()
		if tc.leak != "" && strings.Contains(body, tc.leak) {
			t.Errorf("%s: internal detail leaked to the client: %s", tc.name, body)
		}
		var env apiEnvelope
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatalf("%s: body is not JSON (%v): %s", tc.name, err, body)
		}
		if env.Error == nil || env.Error.Code != tc.code {
			t.Errorf("%s: code = %+v, want %s (body %s)", tc.name, env.Error, tc.code, body)
		}
	}
}

// TestPreflightStillAnswers204 guards the CORS path: OPTIONS is answered by
// middleware before routing, so the /api guard must not have moved in front.
func TestPreflightStillAnswers204(t *testing.T) {
	e := newAPIRouter(t)
	if rec := doRequest(t, e, http.MethodOptions, "/api/persons"); rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /api/persons = %d, want 204", rec.Code)
	}
}

// TestRouteShapeMatching covers the 404-vs-405 decision rule on its own: the
// guard is only as good as this matcher.
func TestRouteShapeMatching(t *testing.T) {
	shapes := routeShapes{
		splitPath("/api/persons"),
		splitPath("/api/persons/:id"),
		splitPath("/api/events/:id/participants"),
	}

	for _, path := range []string{
		"/api/persons",
		"/api/persons/",
		"/api/persons/6f1c",
		"/api/events/9/participants",
	} {
		if !shapes.has(path) {
			t.Errorf("has(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		"/api",
		"/api/person",
		"/api/persons/6f1c/extra",
		"/api/event/9/participants",
		"/api/persons/6f1c/events",
	} {
		if shapes.has(path) {
			t.Errorf("has(%q) = true, want false", path)
		}
	}
}
