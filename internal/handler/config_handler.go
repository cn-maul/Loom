package handler

import (
	"net/http"
	"os"
	"strconv"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/models"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

// PrivacyInfo is the "what happens with my data" panel: the facts a user of a
// sensitive local app must be able to see without reading source code.
type PrivacyInfo struct {
	ListensOn          string `json:"listens_on"`
	AIEndpoint         string `json:"ai_endpoint"`
	AIEndpointExternal bool   `json:"ai_endpoint_external"`
	// SendsRawText is always true for extraction: the record's raw text is
	// part of every prompt. It is stated explicitly so the implication of a
	// cloud endpoint is unambiguous.
	SendsRawText    bool `json:"sends_raw_text"`
	AllowRemote     bool `json:"allow_remote"`
	AuthRequired    bool `json:"auth_required"`
	BackupEncrypted bool `json:"backup_encrypted"`
	ConfigHasSecret bool `json:"config_has_secret"`
}

type ConfigHandler struct {
	cfg *config.Config
	// llm is the same pointer the services hold. Update mutates through it —
	// replacing cfg.LLM wholesale would leave every service reading the stale
	// previous copy and "saved, takes effect immediately" would be a lie.
	llm        *config.LLMConfig
	configPath string
}

func NewConfigHandler(cfg *config.Config, configPath string) *ConfigHandler {
	return &ConfigHandler{cfg: cfg, llm: &cfg.LLM, configPath: configPath}
}

func (h *ConfigHandler) privacy() PrivacyInfo {
	return PrivacyInfo{
		ListensOn:          h.cfg.Server.Host + ":" + strconv.Itoa(h.cfg.Server.Port),
		AIEndpoint:         h.cfg.LLM.Endpoint,
		AIEndpointExternal: !ai.IsLoopbackEndpoint(h.cfg.LLM.Endpoint),
		SendsRawText:       true,
		AllowRemote:        h.cfg.LLM.AllowRemote,
		AuthRequired:       h.cfg.Server.AuthToken != "",
		BackupEncrypted:    h.cfg.Backup.Passphrase != "",
		ConfigHasSecret:    h.cfg.LLM.APIKey != "" || h.cfg.LLM.EmbedAPIKey != "",
	}
}

func (h *ConfigHandler) Get(c echo.Context) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"config":  h.cfg,
			"privacy": h.privacy(),
		},
	})
}

// Update rewrites the llm block in place through the pointer the services
// hold, so the new values take effect without a restart.
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
		req.EmbedDim = h.llm.EmbedDim
	}
	// async_extract and allow_remote are config.yaml-only switches: the
	// settings page does not send them, and a PUT that omits them must not
	// silently flip pipeline mode or reopen the data boundary. Edit
	// config.yaml to change them.
	req.AsyncExtract = h.llm.AsyncExtract
	req.AllowRemote = h.llm.AllowRemote

	// Stored vectors are only meaningful for the embedding model that produced them.
	reindexRequired := req.EmbedDim != h.llm.EmbedDim ||
		req.EmbedModel != h.llm.EmbedModel ||
		req.ResolvedEmbedEndpoint() != h.llm.ResolvedEmbedEndpoint()
	*h.llm = req

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
