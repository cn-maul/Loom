package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type PositionHandler struct {
	service *service.PositionService
}

func NewPositionHandler(service *service.PositionService) *PositionHandler {
	return &PositionHandler{service: service}
}

// ListByPerson returns a person's full stint history, current postings first.
func (h *PositionHandler) ListByPerson(c echo.Context) error {
	links, err := h.service.ListByPerson(c.Param("id"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if links == nil {
		links = []*models.OrgPositionLink{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: links})
}

// ListByOrg returns an organisation's members. current=true keeps only the people
// whose posting has not ended, which is the "current member" list.
func (h *PositionHandler) ListByOrg(c echo.Context) error {
	currentOnly, err := queryBool(c, "current")
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	links, err := h.service.ListByOrg(c.Param("id"), currentOnly != nil && *currentOnly)
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if links == nil {
		links = []*models.OrgPositionLink{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: links})
}

func (h *PositionHandler) Create(c echo.Context) error {
	var pos models.OrgPosition
	if err := bindJSON(c, &pos); err != nil {
		return err
	}
	// A posting is normally created from a person's page; the path parameter wins
	// so the body cannot point at a different person.
	if personID := c.Param("id"); personID != "" {
		if pos.PersonID != "" && pos.PersonID != personID {
			return respondError(c, models.NewError(models.ErrInvalidInput, "person_id in the body must match the path"), "CREATE_FAILED")
		}
		pos.PersonID = personID
	}
	pos.ID = uuid.New().String()
	link, err := h.service.Create(&pos)
	if err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: link})
}

func (h *PositionHandler) Get(c echo.Context) error {
	link, err := h.service.GetByID(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: link})
}

func (h *PositionHandler) Update(c echo.Context) error {
	var pos models.OrgPosition
	if err := bindJSON(c, &pos); err != nil {
		return err
	}
	pos.ID = c.Param("id")
	link, err := h.service.Update(&pos)
	if err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: link})
}

func (h *PositionHandler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
