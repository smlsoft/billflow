package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/sml"
)

// ChannelDefaultsHandler exposes CRUD + quick-setup for channel_defaults. Admin only.
type ChannelDefaultsHandler struct {
	repo       *repository.ChannelDefaultRepo
	auditRepo  *repository.AuditLogRepo
	partyCache *sml.PartyCache
	logger     *zap.Logger
}

func NewChannelDefaultsHandler(
	repo *repository.ChannelDefaultRepo,
	auditRepo *repository.AuditLogRepo,
	partyCache *sml.PartyCache,
	logger *zap.Logger,
) *ChannelDefaultsHandler {
	return &ChannelDefaultsHandler{
		repo:       repo,
		auditRepo:  auditRepo,
		partyCache: partyCache,
		logger:     logger,
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
		"channel":    in.Channel,
		"bill_type":  in.BillType,
		"party_code": in.PartyCode,
		"party_name": in.PartyName,
	})
	c.JSON(http.StatusOK, d)
}

// DELETE /api/settings/channel-defaults/:channel/:bill_type
func (h *ChannelDefaultsHandler) Delete(c *gin.Context) {
	channel := c.Param("channel")
	billType := c.Param("bill_type")
	if err := h.repo.Delete(channel, billType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "channel_default_deleted", map[string]interface{}{
		"channel":   channel,
		"bill_type": billType,
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// quickSetupMapping pairs each channel/bill_type with the placeholder name to
// look up in the customer master ("ลูกค้า จาก AI/Line/Email/Shopee" rows that
// production SML 248 already has) plus the SML doc_format_code that channel
// posts with.
type quickSetupMapping struct {
	Channel          string
	BillType         string
	PlaceholderName  string
	DocFormatCode    string // SML doc_format_code (empty = endpoint doesn't take one)
	DocPrefix        string // BillFlow doc_no prefix
	DocRunningFormat string // running counter format (YYMM####, etc.)
}

var quickSetupMappings = []quickSetupMapping{
	// SML 213 sale_reserve auto-generates its own doc_no — prefix/format unused
	{"line", "sale", "ลูกค้า จาก Line", "", "", ""},
	{"email", "sale", "ลูกค้า จาก Email", "", "", ""},
	// SML 248 saleorder — BF-SO + YYMM + 4-digit running counter (resets monthly)
	{"shopee", "sale", "ลูกค้า จาก Shopee", "SR", "BF-SO", "YYMM####"},
	{"shopee_email", "sale", "ลูกค้า จาก Shopee", "SR", "BF-SO", "YYMM####"},
}

// QuickSetupResult describes what happened in one row of POST /quick-setup.
type QuickSetupResult struct {
	Channel  string `json:"channel"`
	BillType string `json:"bill_type"`
	Applied  bool   `json:"applied"`
	PartyCode string `json:"party_code,omitempty"`
	PartyName string `json:"party_name,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// POST /api/settings/channel-defaults/quick-setup
// One-click pairing of LINE/Email/Shopee with the AR00001-04 placeholder
// customers. Skips channels whose placeholder is missing or already set.
func (h *ChannelDefaultsHandler) QuickSetup(c *gin.Context) {
	if h.partyCache == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "party cache not configured"})
		return
	}
	userID := c.GetString("user_id")
	results := make([]QuickSetupResult, 0, len(quickSetupMappings))
	appliedCount := 0
	for _, m := range quickSetupMappings {
		party := h.partyCache.FindByExactName("sale", m.PlaceholderName)
		if party == nil {
			results = append(results, QuickSetupResult{
				Channel:  m.Channel,
				BillType: m.BillType,
				Applied:  false,
				Reason:   "ไม่พบลูกค้า placeholder ชื่อ \"" + m.PlaceholderName + "\" ใน SML",
			})
			continue
		}
		d := &models.ChannelDefault{
			Channel:          m.Channel,
			BillType:         m.BillType,
			PartyCode:        party.Code,
			PartyName:        party.Name,
			PartyPhone:       party.Telephone,
			PartyAddress:     party.Address,
			PartyTaxID:       party.TaxID,
			DocFormatCode:    m.DocFormatCode,
			DocPrefix:        m.DocPrefix,
			DocRunningFormat: m.DocRunningFormat,
			// Sentinel: -1 / '' = "use server .env default" — quick-setup
			// never wants to lock a channel to a specific WH/VAT.
			VATType: -1,
			VATRate: -1,
		}
		if err := h.repo.Upsert(d, userID); err != nil {
			results = append(results, QuickSetupResult{
				Channel:  m.Channel,
				BillType: m.BillType,
				Applied:  false,
				Reason:   "บันทึกล้มเหลว: " + err.Error(),
			})
			continue
		}
		appliedCount++
		results = append(results, QuickSetupResult{
			Channel:   m.Channel,
			BillType:  m.BillType,
			Applied:   true,
			PartyCode: party.Code,
			PartyName: party.Name,
		})
	}
	h.audit(c, "channel_default_quick_setup", map[string]interface{}{
		"applied_count": appliedCount,
		"results":       results,
	})
	c.JSON(http.StatusOK, gin.H{
		"applied": appliedCount,
		"results": results,
	})
}

// validChannelBillTypeCombo enforces UI-level rules so admins can't save
// nonsensical pairs (shopee_shipped is purchase-only, etc.).
func validChannelBillTypeCombo(channel, billType string) bool {
	switch channel {
	case "shopee_shipped":
		return billType == "purchase"
	case "shopee", "shopee_email", "line", "email", "manual":
		return billType == "sale"
	case "lazada":
		return billType == "sale" || billType == "purchase"
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
