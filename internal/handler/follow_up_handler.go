package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

type FollowUpHandler struct{ service *service.FollowUpService }

func NewFollowUpHandler(s *service.FollowUpService) *FollowUpHandler {
	return &FollowUpHandler{service: s}
}

func (h *FollowUpHandler) List(c echo.Context) error {
	items, err := h.service.List(c.QueryParam("person_id"), c.QueryParam("status"), c.QueryParam("from"), c.QueryParam("to"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if items == nil {
		items = []*models.FollowUp{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: items})
}

// ListByPerson serves /persons/:id/follow-ups. The route already carries the
// person id as a path parameter, so the parameter is read directly instead of
// rewriting the request path to reuse the collection handler.
func (h *FollowUpHandler) ListByPerson(c echo.Context) error {
	items, err := h.service.List(c.Param("id"), c.QueryParam("status"), c.QueryParam("from"), c.QueryParam("to"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if items == nil {
		items = []*models.FollowUp{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: items})
}

// ListPostponements serves the deadline history of one item.
func (h *FollowUpHandler) ListPostponements(c echo.Context) error {
	history, err := h.service.ListPostponements(c.Param("id"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: history})
}

func (h *FollowUpHandler) Get(c echo.Context) error {
	f, err := h.service.Get(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: f})
}

func (h *FollowUpHandler) Create(c echo.Context) error {
	var f models.FollowUp
	if err := bindJSON(c, &f); err != nil {
		return err
	}
	if err := h.service.Create(&f); err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: f})
}

func (h *FollowUpHandler) Update(c echo.Context) error {
	var f models.FollowUp
	if err := bindJSON(c, &f); err != nil {
		return err
	}
	f.ID = c.Param("id")
	if err := h.service.Update(&f); err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	updated, err := h.service.Get(f.ID)
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: updated})
}

// Postpone moves the deadline. The reason is optional; the body itself is not,
// because a postpone without a new date changes nothing.
func (h *FollowUpHandler) Postpone(c echo.Context) error {
	var req struct {
		DueDate string `json:"due_date"`
		Reason  string `json:"reason"`
	}
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if req.DueDate == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK: false, Error: &models.APIError{Code: "INVALID_INPUT", Message: "due_date is required"},
		})
	}
	if err := h.service.Postpone(c.Param("id"), req.DueDate, req.Reason); err != nil {
		return respondError(c, err, "POSTPONE_FAILED")
	}
	item, err := h.service.Get(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: item})
}

// Complete closes an item and may record the outcome in the same call: what
// happened, and the record written afterwards. Both are optional, so the plain
// one-click completion still works.
func (h *FollowUpHandler) Complete(c echo.Context) error {
	var req struct {
		CompletionNote   string `json:"completion_note"`
		CompletedEventID string `json:"completed_event_id"`
	}
	if c.Request().ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			return err
		}
	}
	if err := h.service.Complete(c.Param("id"), req.CompletionNote, req.CompletedEventID); err != nil {
		return respondError(c, err, "ACTION_FAILED")
	}
	item, err := h.service.Get(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: item})
}

func (h *FollowUpHandler) Action(status string) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := h.service.Action(c.Param("id"), status); err != nil {
			return respondError(c, err, "ACTION_FAILED")
		}
		item, err := h.service.Get(c.Param("id"))
		if err != nil {
			return respondError(c, err, "READ_FAILED")
		}
		return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: item})
	}
}
