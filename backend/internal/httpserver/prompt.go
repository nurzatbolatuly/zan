package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/prompt"
)

// agentPromptResponse — AgentPrompt (zan-backend-tz-v2.md §2.9/§3.7).
type agentPromptResponse struct {
	AgentType  string `json:"agent_type"`
	PromptText string `json:"prompt_text"`
	UpdatedAt  string `json:"updated_at"`
}

func newAgentPromptResponse(p domain.AgentPrompt) agentPromptResponse {
	return agentPromptResponse{
		AgentType:  string(p.AgentType),
		PromptText: p.PromptText,
		UpdatedAt:  formatTime(p.UpdatedAt),
	}
}

// adminListPromptsHandler — GET /admin/prompts: обе записи (qa/document).
func adminListPromptsHandler(svc *prompt.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.List(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("admin_list_prompts_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]agentPromptResponse, 0, len(items))
		for _, p := range items {
			out = append(out, newAgentPromptResponse(p))
		}
		c.JSON(http.StatusOK, out)
	}
}

// updatePromptRequest — PUT /admin/prompts/{agent_type} (zan-backend-tz-v2.md
// §3.7: "{prompt_text}").
type updatePromptRequest struct {
	PromptText string `json:"prompt_text" binding:"required"`
}

// adminUpdatePromptHandler — PUT /admin/prompts/{agent_type}.
func adminUpdatePromptHandler(svc *prompt.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentType, err := domain.ParseAgentType(c.Param("agent_type"))
		if err != nil {
			writeError(c, notFoundError("agent_type_not_found", "Unknown agent_type"))
			return
		}

		var req updatePromptRequest
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			writeError(c, invalidRequestError("prompt_text is required"))
			return
		}

		updated, err := svc.Update(c.Request.Context(), agentType, req.PromptText)
		if err != nil {
			switch {
			case errors.Is(err, prompt.ErrEmptyText):
				writeError(c, invalidRequestError("prompt_text must not be empty"))
			case errors.Is(err, prompt.ErrNotFound):
				writeError(c, notFoundError("agent_type_not_found", "Unknown agent_type"))
			default:
				logger.FromContext(c.Request.Context()).Error("admin_update_prompt_failed",
					slog.Group("context", slog.String("error", err.Error())))
				writeError(c, apierror.Internal())
			}
			return
		}
		c.JSON(http.StatusOK, newAgentPromptResponse(updated))
	}
}
