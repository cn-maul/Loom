package handler

import (
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type PersonHandler struct {
	service *service.PersonService
}

func NewPersonHandler(service *service.PersonService) *PersonHandler {
	return &PersonHandler{service: service}
}

func (h *PersonHandler) Create(c echo.Context) error {
	var person models.Person
	if err := c.Bind(&person); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}

	person.ID = uuid.New().String()
	if err := h.service.Create(&person); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "CREATE_FAILED", Message: err.Error()},
		})
	}

	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: person})
}

func (h *PersonHandler) GetByID(c echo.Context) error {
	id := c.Param("id")
	person, err := h.service.GetByID(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "NOT_FOUND", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: person})
}

func (h *PersonHandler) List(c echo.Context) error {
	persons, err := h.service.List()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()},
		})
	}
	if persons == nil {
		persons = []*models.PersonWithActivity{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: persons})
}

func (h *PersonHandler) Update(c echo.Context) error {
	id := c.Param("id")
	var person models.Person
	if err := c.Bind(&person); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	person.ID = id

	if err := h.service.Update(&person); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "UPDATE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: person})
}

func (h *PersonHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if err := h.service.Delete(id); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "DELETE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
