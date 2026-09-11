package handler

import (
	"errors"
	"net/http"

	"relationship/internal/models"

	"github.com/labstack/echo/v4"
)

// respondError maps a domain error to the HTTP status that actually describes
// it. Previously every handler failure came back as 400, which hid the
// difference between a bad payload, a missing row and a database fault.
func respondError(c echo.Context, err error, fallbackCode string) error {
	status, code := classifyError(err, fallbackCode)
	return c.JSON(status, models.APIResponse{
		OK:    false,
		Error: &models.APIError{Code: code, Message: err.Error()},
	})
}

func classifyError(err error, fallbackCode string) (int, string) {
	switch {
	case errors.Is(err, models.ErrInvalidInput):
		return http.StatusBadRequest, "INVALID_INPUT"
	case errors.Is(err, models.ErrPersonNotFound):
		return http.StatusNotFound, "PERSON_NOT_FOUND"
	case errors.Is(err, models.ErrOrgNotFound):
		return http.StatusNotFound, "ORG_NOT_FOUND"
	case errors.Is(err, models.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, models.ErrConflict):
		return http.StatusConflict, "CONFLICT"
	default:
		return http.StatusInternalServerError, fallbackCode
	}
}
