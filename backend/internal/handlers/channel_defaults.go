package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
)

// ChannelDefaultsHandler exposes route/document defaults for channel_defaults.
type ChannelDefaultsHandler struct {
	repo      *repository.ChannelDefaultRepo
	auditRepo *repository.AuditLogRepo
	logger    *zap.Logger
}

func NewChannelDefaultsHandler(
	repo *repository.ChannelDefaultRepo,
	auditRepo *repository.AuditLogRepo,
	logger *zap.Logger,
) *ChannelDefaultsHandler {
	return &ChannelDefaultsHandler{
		repo:      repo,
		auditRepo: auditRepo,
		logger:    logger,
	}
}

// GET /api/settings/channel-defaults
func (h *ChannelDefaultsHandler) List(c *gin.Context) {
	rows, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// PUT /api/settings/channel-defaults — upsert by (channel, bill_type)
func (h *ChannelDefaultsHandler) Upsert(c *gin.Context) {
	var in models.ChannelDefaultUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validChannelBillTypeCombo(in.Channel, in.BillType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid channel/bill_type combo (e.g. shopee_shipped must be purchase)",
		})
		return
	}

	userID := c.GetString("user_id")
	d := &models.ChannelDefault{
		Channel:          in.Channel,
		BillType:         in.BillType,
		PartyCode:        in.PartyCode,
		PartyName:        in.PartyName,
		PartyPhone:       in.PartyPhone,
		PartyAddress:     in.PartyAddress,
		PartyTaxID:       in.PartyTaxID,
		DocFormatCode:    in.DocFormatCode,
		Endpoint:         in.Endpoint,
		DocPrefix:        in.DocPrefix,
		DocRunningFormat: in.DocRunningFormat,
		BranchCode:       in.BranchCode,
		SaleCode:         in.SaleCode,
		UnitCode:         in.UnitCode,
		DocTime:          in.DocTime,
		WHCode:           in.WHCode,
		ShelfCode:        in.ShelfCode,
		VATType:          in.VATType,
		VATRate:          in.VATRate,
	}
	if err := h.repo.Upsert(d, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.audit(c, "channel_default_updated", map[string]interface{}{
		"channel":            in.Channel,
		"bill_type":          in.BillType,
		"endpoint":           in.Endpoint,
		"doc_format_code":    in.DocFormatCode,
		"doc_prefix":         in.DocPrefix,
		"doc_running_format": in.DocRunningFormat,
	})
	c.JSON(http.StatusOK, d)
}

// validChannelBillTypeCombo enforces UI-level rules so admins can't save
// nonsensical pairs (shopee_shipped is purchase-only, etc.).
func validChannelBillTypeCombo(channel, billType string) bool {
	switch channel {
	case "shopee_shipped":
		return billType == "purchase"
	case "email":
		return billType == "sale" || billType == "purchase"
	case "shopee", "shopee_email", "line", "manual":
		return billType == "sale"
	case "lazada":
		return billType == "sale" || billType == "purchase"
	case "tiktok":
		return billType == "sale"
	}
	return false
}

func (h *ChannelDefaultsHandler) audit(c *gin.Context, action string, detail map[string]interface{}) {
	if h.auditRepo == nil {
		return
	}
	var userID *string
	if uid := c.GetString("user_id"); uid != "" {
		userID = &uid
	}
	_ = h.auditRepo.Log(models.AuditEntry{
		Action:  action,
		UserID:  userID,
		Source:  "channel_defaults",
		Level:   "info",
		TraceID: c.GetString("trace_id"),
		Detail:  detail,
	})
}
