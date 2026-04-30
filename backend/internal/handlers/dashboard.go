package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/insight"
)

type DashboardHandler struct {
	billRepo             *repository.BillRepo
	insightRepo          *repository.InsightRepo
	convRepo             *repository.ChatConversationRepo
	imapRepo             *repository.ImapAccountRepo
	insightSvc           *insight.Service
	lineConfigured       bool
	imapConfigured       bool
	smlConfigured        bool
	aiConfigured         bool
	autoConfirmThreshold float64
	log                  *zap.Logger
}

func NewDashboardHandler(
	billRepo *repository.BillRepo,
	insightRepo *repository.InsightRepo,
	convRepo *repository.ChatConversationRepo,
	imapRepo *repository.ImapAccountRepo,
	insightSvc *insight.Service,
	log *zap.Logger,
) *DashboardHandler {
	return &DashboardHandler{
		billRepo:    billRepo,
		insightRepo: insightRepo,
		convRepo:    convRepo,
		imapRepo:    imapRepo,
		insightSvc:  insightSvc,
		log:         log,
	}
}

// SetConfigStatus sets config flags for the settings status endpoint
func (h *DashboardHandler) SetConfigStatus(line, imap, sml, ai bool, threshold float64) {
	h.lineConfigured = line
	h.imapConfigured = imap
	h.smlConfigured = sml
	h.aiConfigured = ai
	h.autoConfirmThreshold = threshold
}

// GET /api/dashboard/stats
//
// Returns the existing bill stats plus `unread_messages` so the Sidebar can
// power both the /bills pending badge and the /messages unread badge with a
// single poll instead of two.
func (h *DashboardHandler) Stats(c *gin.Context) {
	stats, err := h.billRepo.DashboardStats()
	if err != nil {
		h.log.Error("DashboardStats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	// Wrap the stats struct as a map so we can attach extra fields without
	// changing the existing DashboardStats type signature.
	out := map[string]interface{}{}
	statsBytes, _ := json.Marshal(stats)
	_ = json.Unmarshal(statsBytes, &out)

	if h.convRepo != nil {
		if unread, err := h.convRepo.UnreadCount(); err == nil {
			out["unread_messages"] = unread
		}
	}
	// Email-inbox health — surfaces "1 inbox มีปัญหา" on the dashboard
	// without admin needing to open /settings/email to check status.
	if h.imapRepo != nil {
		if failing, err := h.imapRepo.CountFailing(); err == nil {
			out["email_inbox_errors"] = failing
		}
	}
	c.JSON(http.StatusOK, out)
}

// GET /api/dashboard/insights — returns last 7 daily insights
func (h *DashboardHandler) Insights(c *gin.Context) {
	if h.insightRepo == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}
	items, err := h.insightRepo.List(7)
	if err != nil {
		h.log.Error("Insights", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if items == nil {
		items = []models.DailyInsight{}
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// POST /api/dashboard/insights/generate — on-demand F4 insight generation
func (h *DashboardHandler) GenerateInsight(c *gin.Context) {
	if h.insightSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not configured"})
		return
	}

	stats, err := h.billRepo.DashboardStats()
	if err != nil {
		h.log.Error("GenerateInsight: get stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	statsBytes, _ := json.Marshal(stats)
	text, err := h.insightSvc.Generate(string(statsBytes))
	if err != nil {
		h.log.Error("GenerateInsight: AI", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI generation failed"})
		return
	}

	if h.insightRepo != nil {
		_ = h.insightRepo.Save(string(statsBytes), text)
	}

	c.JSON(http.StatusOK, gin.H{"insight": text})
}

// GET /api/settings/status — config status for frontend settings page
func (h *DashboardHandler) SettingsStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"line_configured":        h.lineConfigured,
		"imap_configured":        h.imapConfigured,
		"sml_configured":         h.smlConfigured,
		"ai_configured":          h.aiConfigured,
		"auto_confirm_threshold": h.autoConfirmThreshold,
	})
}
