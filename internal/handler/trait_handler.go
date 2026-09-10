package handler

import (
	"net/http"
	"relationship/internal/models"
	"relationship/internal/repository"

	"github.com/labstack/echo/v4"
)

type TraitHandler struct {
	repo *repository.TraitRepo
}

func NewTraitHandler(repo *repository.TraitRepo) *TraitHandler {
	return &TraitHandler{repo: repo}
}

func (h *TraitHandler) ListByPerson(c echo.Context) error {
	personID := c.Param("id")
	traits, err := h.repo.ListByPerson(personID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: traits})
}

func (h *TraitHandler) Verify(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Verified int `json:"verified"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}

	if err := h.repo.UpdateVerified(id, req.Verified); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "UPDATE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}