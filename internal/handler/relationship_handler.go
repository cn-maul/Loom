package handler

import (
	"net/http"
	"strconv"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type RelationshipHandler struct {
	service *service.RelationshipService
}

func NewRelationshipHandler(service *service.RelationshipService) *RelationshipHandler {
	return &RelationshipHandler{service: service}
}

// List serves the graph query. person_id matches either end of an edge, so a
// person's page shows both the edges they own and the ones pointing at them.
func (h *RelationshipHandler) List(c echo.Context) error {
	filter, err := relationshipFilter(c, "")
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	links, err := h.service.List(filter)
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if links == nil {
		links = []*models.RelationshipLink{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: links})
}

func (h *RelationshipHandler) ListByPerson(c echo.Context) error {
	filter, err := relationshipFilter(c, c.Param("id"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	links, err := h.service.List(filter)
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if links == nil {
		links = []*models.RelationshipLink{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: links})
}

func (h *RelationshipHandler) ListTypes(c echo.Context) error {
	types, err := h.service.ListTypes()
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: types})
}

// CoAttendance exposes derived "shared experience" pairs. They are computed from
// records and deliberately not stored as relationship edges.
func (h *RelationshipHandler) CoAttendance(c echo.Context) error {
	pairs, err := h.service.CoAttendance(queryLimit(c, "limit", 0, MaxPageLimit))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: pairs})
}

func (h *RelationshipHandler) Get(c echo.Context) error {
	link, err := h.service.GetByID(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: link})
}

func (h *RelationshipHandler) Create(c echo.Context) error {
	var rel models.Relationship
	if err := bindJSON(c, &rel); err != nil {
		return err
	}
	rel.ID = uuid.New().String()
	link, err := h.service.Create(&rel)
	if err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: link})
}

func (h *RelationshipHandler) Update(c echo.Context) error {
	var rel models.Relationship
	if err := bindJSON(c, &rel); err != nil {
		return err
	}
	rel.ID = c.Param("id")
	link, err := h.service.Update(&rel)
	if err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: link})
}

func (h *RelationshipHandler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}

func relationshipFilter(c echo.Context, personID string) (models.RelationshipFilter, error) {
	filter := models.RelationshipFilter{
		PersonID: personID,
		Type:     c.QueryParam("type"),
		Query:    c.QueryParam("q"),
		Limit:    queryLimit(c, "limit", 0, MaxPageLimit),
		Offset:   queryInt(c, "offset", 0),
	}
	if raw := c.QueryParam("confirmed"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || (value != 0 && value != 1) {
			return filter, models.NewError(models.ErrInvalidInput, "confirmed must be 0 or 1")
		}
		filter.Confirmed = &value
	}
	active, err := queryBool(c, "active")
	if err != nil {
		return filter, err
	}
	filter.Active = active
	return filter, nil
}
