package app

import (
	"log"
	"time"

	"github.com/labstack/echo/v4"
)

// APIVersion marks the current contract generation. Every /api response
// carries it, so a frontend can pin what it was built against and a future
// breaking change can go to /api/v2 while v1 keeps its header — the header is
// the promise, the URL is the escape hatch.
const APIVersion = "1"

// slowRequestThreshold is where a request stops being "the LLM is thinking"
// and becomes worth a log line. Sync AI endpoints legitimately take tens of
// seconds; the warning is about the surprise case, not the expected one.
const slowRequestThreshold = 15 * time.Second

// ObserveMiddleware stamps the API version on every response and warns about
// slow requests. One line of context per slow call — method, path, status,
// duration — is the difference between "the app felt slow" and a diagnosis.
func ObserveMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-API-Version", APIVersion)
			start := time.Now()
			err := next(c)
			if d := time.Since(start); d >= slowRequestThreshold {
				log.Printf("warning: slow request %s %s -> %d in %s",
					c.Request().Method, c.Request().URL.Path, c.Response().Status, d.Round(time.Millisecond))
			}
			return err
		}
	}
}
