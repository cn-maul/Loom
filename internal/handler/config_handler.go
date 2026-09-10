package handler

import (
	"net/http"
	"os"

	"relationship/internal/config"
	"relationship/internal/models"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

type ConfigHandler struct {
	cfg    *config.Config
	configPath string
}

func NewConfigHandler(cfg *config.Config, configPath string) *ConfigHandler {
	return &ConfigHandler{cfg: cfg, configPath: configPath}
}

func (h *ConfigHandler) Get(c echo.Context) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: h.cfg,
	})
}

func (h *ConfigHandler) Update(c echo.Context) error {
	var llmConfig config.LLMConfig
	if err := c.Bind(&llmConfig); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}

	// 更新内存中的配置
	h.cfg.LLM = llmConfig

	// 写入文件
	data, err := yaml.Marshal(h.cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "MARSHAL_FAILED", Message: err.Error()},
		})
	}

	if err := os.WriteFile(h.configPath, data, 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "WRITE_FAILED", Message: err.Error()},
		})
	}

	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: h.cfg,
	})
}