package handler

import (
	"net/http"
	"strconv"

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
	if err := bindJSON(c, &person); err != nil {
		return err
	}
	person.ID = uuid.New().String()
	if err := h.service.Create(&person); err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: person})
}

func (h *PersonHandler) GetByID(c echo.Context) error {
	person, err := h.service.GetByID(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: person})
}

// List serves the person list with keyword, affiliation, relation, ordering
// and page window. The match count travels in X-Total-Count so a paginated
// caller can size the rest of the list without a second query.
func (h *PersonHandler) List(c echo.Context) error {
	persons, total, err := h.service.List(models.PersonFilter{
		Q:        c.QueryParam("q"),
		OrgID:    c.QueryParam("org_id"),
		Relation: c.QueryParam("relation"),
		Sort:     c.QueryParam("sort"),
		Limit:    queryLimit(c, "limit", 0, MaxPageLimit),
		Offset:   queryInt(c, "offset", 0),
	})
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if persons == nil {
		persons = []*models.PersonWithActivity{}
	}
	c.Response().Header().Set("X-Total-Count", strconv.Itoa(total))
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: persons})
}

func (h *PersonHandler) Update(c echo.Context) error {
	var person models.Person
	if err := bindJSON(c, &person); err != nil {
		return err
	}
	person.ID = c.Param("id")
	if err := h.service.Update(&person); err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: person})
}

func (h *PersonHandler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
