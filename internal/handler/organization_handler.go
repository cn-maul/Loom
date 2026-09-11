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
	if err := c.Bind(&org); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	org.ID = uuid.New().String()
	if err := h.service.Create(&org); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "CREATE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) List(c echo.Context) error {
	orgs, err := h.service.List()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()},
		})
	}
	if orgs == nil {
		orgs = []*models.Organization{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: orgs})
}

func (h *OrganizationHandler) Update(c echo.Context) error {
	id := c.Param("id")
	var org models.Organization
	if err := c.Bind(&org); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	org.ID = id
	if err := h.service.Update(&org); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "UPDATE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: org})
}

func (h *OrganizationHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if err := h.service.Delete(id); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "DELETE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
