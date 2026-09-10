package handler

import (
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

type AIHandler struct {
	aiService *service.AIService
}

func NewAIHandler(aiService *service.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

func (h *AIHandler) GetAdvice(c echo.Context) error {
	var req models.AdviceRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}

	if req.PersonID == "" || req.Question == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: "person_id and question are required"},
		})
	}

	advice, err := h.aiService.GenerateAdvice(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "ADVICE_FAILED", Message: err.Error()},
		})
	}

	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: advice})
}

func (h *AIHandler) GetWeeklyReport(c echo.Context) error {
	// TODO: Implement weekly report generation
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]string{"message": "周报功能待实现"},
	})
}