package handler

import (
	"context"
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"
	"time"

	"github.com/labstack/echo/v4"
)

type AIHandler struct {
	aiService *service.AIService
}

func NewAIHandler(aiService *service.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

// Reindex rebuilds every vector. It keeps running if the client disconnects, since
// the table has already been cleared and a half-built index would silently degrade
// retrieval.
func (h *AIHandler) Reindex(c echo.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request().Context()), 30*time.Minute)
	defer cancel()

	result, err := h.aiService.Reindex(ctx)
	if err != nil {
		return serverError(c, "REINDEX_FAILED", err.Error())
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: result})
}

// EmbeddingStatus lets the UI explain why advice skipped semantic search.
func (h *AIHandler) EmbeddingStatus(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	if err := h.aiService.CheckEmbedding(ctx); err != nil {
		return c.JSON(http.StatusOK, models.APIResponse{
			OK:   true,
			Data: map[string]interface{}{"available": false, "detail": err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"available": true},
	})
}

// ListModels proxies the configured chat endpoint's /models catalog so the
// settings page can offer a model picker without storing credentials in the
// browser. Uses the current config, so save before fetching.
func (h *AIHandler) ListModels(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	ids, err := h.aiService.ListModels(ctx)
	if err != nil {
		return serverError(c, "LIST_MODELS_FAILED", err.Error())
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: ids})
}

func badRequest(c echo.Context, code, message string) error {
	return c.JSON(http.StatusBadRequest, models.APIResponse{
		OK:    false,
		Error: &models.APIError{Code: code, Message: message},
	})
}

func serverError(c echo.Context, code, message string) error {
	return c.JSON(http.StatusInternalServerError, models.APIResponse{
		OK:    false,
		Error: &models.APIError{Code: code, Message: message},
	})
}
