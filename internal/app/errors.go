package app

import (
	"errors"
	"net/http"
	"strings"

	"relationship/internal/models"

	"github.com/labstack/echo/v4"
)

// isAPIPath reports whether a request path belongs to the JSON API. The bare
// "/api" counts: it is a mistyped endpoint, never a page.
func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

// routeShapes is the set of URL shapes the API answers to. Echo reports 405
// both for a verb a known path does not implement and for a path that was
// never routed at all — the SPA catch-all claims "GET anything", so every path
// has at least one method registered and a typo looks like a verb problem.
// Telling those apart needs the route table, not the status code.
type routeShapes [][]string

func newRouteShapes(e *echo.Echo) routeShapes {
	var shapes routeShapes
	seen := make(map[string]bool)
	for _, r := range e.Routes() {
		if !isAPIPath(r.Path) || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		shapes = append(shapes, splitPath(r.Path))
	}
	return shapes
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// has reports whether any route could serve path. ":id" matches one segment,
// "*" matches whatever is left.
func (s routeShapes) has(path string) bool {
	segs := splitPath(path)
	for _, shape := range s {
		if matchesShape(shape, segs) {
			return true
		}
	}
	return false
}

func matchesShape(shape, segs []string) bool {
	for i, part := range shape {
		if part == "*" {
			return true
		}
		if i >= len(segs) {
			return false
		}
		if strings.HasPrefix(part, ":") {
			continue
		}
		if part != segs[i] {
			return false
		}
	}
	return len(shape) == len(segs)
}

// apiErrorHandler wraps Echo's error handler so that every failure under /api
// speaks the envelope {ok:false, error:{code,message}} — including the ones
// Echo raises itself: an unrouted path, a verb a route does not implement, a
// body the binder refused, a panic recovered into a 500. Handler failures
// already answer that way through handler.respondError; leaving Echo's own
// failures on its default {"message":...} made the documented contract only
// half true.
func apiErrorHandler(e *echo.Echo, shapes routeShapes) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		path := c.Request().URL.Path
		if !isAPIPath(path) {
			e.DefaultHTTPErrorHandler(err, c)
			return
		}

		status := http.StatusInternalServerError
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			status = httpErr.Code
		}
		// A verb the router refused on a path that does not exist is a wrong
		// URL, not a wrong verb: only the catch-all made it look routable.
		if status == http.StatusMethodNotAllowed && !shapes.has(path) {
			status = http.StatusNotFound
		}
		if status >= http.StatusInternalServerError {
			// What arrives here is a panic value or a database fault. It gets
			// logged, never echoed.
			c.Logger().Error(err)
		}
		_ = respondAPIError(c, status)
	}
}

// respondAPIError writes the envelope. A write failure is dropped on purpose:
// the status line is committed by then and there is nothing left to tell.
func respondAPIError(c echo.Context, status int) error {
	// A HEAD response carries no body; minting one would be a protocol lie.
	if c.Request().Method == http.MethodHead {
		return c.NoContent(status)
	}
	return c.JSON(status, models.APIResponse{
		OK: false,
		Error: &models.APIError{
			Code:    apiErrorCode(status),
			Message: apiErrorMessage(status, c),
		},
	})
}

// apiErrorCode keeps the same vocabulary as handler.classifyError, so a
// missing route and a missing row are spelled the same way for the client.
func apiErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_INPUT"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusUnsupportedMediaType:
		return "UNSUPPORTED_MEDIA_TYPE"
	}
	if status >= http.StatusInternalServerError {
		return "INTERNAL"
	}
	return "REQUEST_FAILED"
}

// apiErrorMessage names the problem without echoing internals. 5xx stays
// generic because what sits behind it is a panic or a driver error, and the
// caller has no use for either.
func apiErrorMessage(status int, c echo.Context) string {
	switch status {
	case http.StatusBadRequest:
		return "请求格式不正确"
	case http.StatusUnauthorized:
		return "需要访问令牌"
	case http.StatusNotFound:
		return "接口不存在：" + c.Request().URL.Path
	case http.StatusMethodNotAllowed:
		return "接口 " + c.Request().URL.Path + " 不支持 " + c.Request().Method + " 请求"
	case http.StatusRequestEntityTooLarge:
		return "请求体超出大小限制"
	}
	if status >= http.StatusInternalServerError {
		return "服务器内部错误"
	}
	return http.StatusText(status)
}
