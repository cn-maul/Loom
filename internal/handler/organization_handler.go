package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type OrganizationHandler struct {
	service *service.OrganizationService
}

func NewOrganizationHandler(service *service.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

func (h *OrganizationHandler) Create(c echo.Context) error {
	var org models.Organization
	if err := bindJSON(c, &org); err != nil {
		return err
	}
	org.ID = uuid.New().String()
	if err := h.service.Create(&org); err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) List(c echo.Context) error {
	includeArchived, err := queryBool(c, "include_archived")
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	orgs, err := h.service.List(includeArchived != nil && *includeArchived)
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if orgs == nil {
		orgs = []*models.Organization{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: orgs})
}

// Archive is soft: the row stays so historical postings keep their meaning,
// and it merely disappears from assignment pickers.
func (h *OrganizationHandler) Archive(c echo.Context) error {
	org, err := h.service.Archive(c.Param("id"))
	if err != nil {
		return respondError(c, err, "ACTION_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) Restore(c echo.Context) error {
	org, err := h.service.Restore(c.Param("id"))
	if err != nil {
		return respondError(c, err, "ACTION_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) Update(c echo.Context) error {
	var org models.Organization
	if err := bindJSON(c, &org); err != nil {
		return err
	}
	org.ID = c.Param("id")
	if err := h.service.Update(&org); err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
