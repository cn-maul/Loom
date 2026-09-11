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
	cfg        *config.Config
	configPath string
}

func NewConfigHandler(cfg *config.Config, configPath string) *ConfigHandler {
	return &ConfigHandler{cfg: cfg, configPath: configPath}
}

func (h *ConfigHandler) Get(c echo.Context) error {
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: h.cfg})
}

// Update replaces the llm block. Services hold a pointer into cfg.LLM, so the new
// values take effect without a restart.
func (h *ConfigHandler) Update(c echo.Context) error {
	var req config.LLMConfig
	if err := c.Bind(&req); err != nil {
		return badRequest(c, "INVALID_INPUT", err.Error())
	}
	if req.Endpoint == "" || req.ExtractModel == "" || req.AdviceModel == "" {
		// The remaining fields are legitimately optional (a provider may need no key,
		// an install may have no embedding model) but a body that binds to nothing
		// would otherwise overwrite a working configuration.
		return badRequest(c, "INVALID_INPUT", "endpoint, extract_model and advice_model are required")
	}
	if req.Protocol == "" {
		req.Protocol = "openai"
	}
	if req.EmbedDim == 0 {
		req.EmbedDim = h.cfg.LLM.EmbedDim
	}

	// Stored vectors are only meaningful for the embedding model that produced them.
	reindexRequired := req.EmbedDim != h.cfg.LLM.EmbedDim ||
		req.EmbedModel != h.cfg.LLM.EmbedModel ||
		req.ResolvedEmbedEndpoint() != h.cfg.LLM.ResolvedEmbedEndpoint()
	h.cfg.LLM = req

	data, err := yaml.Marshal(h.cfg)
	if err != nil {
		return serverError(c, "MARSHAL_FAILED", err.Error())
	}
	if err := os.WriteFile(h.configPath, data, 0644); err != nil {
		return serverError(c, "WRITE_FAILED", err.Error())
	}

	return c.JSON(http.StatusOK, models.APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"config":           h.cfg,
			"reindex_required": reindexRequired,
		},
	})
}
