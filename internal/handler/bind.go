package handler

import (
	"net/http"
	"strconv"

	"relationship/internal/models"

	"github.com/labstack/echo/v4"
)

// bindJSON binds the request body, turning a malformed payload into the 400 the
// caller needs instead of a 500 further down.
func bindJSON(c echo.Context, target any) error {
	if err := c.Bind(target); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	return nil
}

// queryInt reads an optional integer query parameter, falling back to fallback
// when it is absent or unparsable.
func queryInt(c echo.Context, name string, fallback int) int {
	raw := c.QueryParam(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// queryBool reads an optional tri-state boolean: nil when the parameter is absent.
func queryBool(c echo.Context, name string) (*bool, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, models.NewError(models.ErrInvalidInput, "%s must be true or false", name)
	}
	return &value, nil
}
