package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
)

type MarketplaceAliasHandler struct {
	aliasRepo   *repository.MarketplaceAliasRepo
	catalogRepo *repository.SMLCatalogRepo
	auditRepo   *repository.AuditLogRepo
	logger      *zap.Logger
}

func NewMarketplaceAliasHandler(
	aliasRepo *repository.MarketplaceAliasRepo,
	catalogRepo *repository.SMLCatalogRepo,
	auditRepo *repository.AuditLogRepo,
	logger *zap.Logger,
) *MarketplaceAliasHandler {
	return &MarketplaceAliasHandler{aliasRepo: aliasRepo, catalogRepo: catalogRepo, auditRepo: auditRepo, logger: logger}
}

func (h *MarketplaceAliasHandler) ReviewGroups(c *gin.Context) {
	billType := c.Query("bill_type")
	groups, err := h.aliasRepo.ReviewGroups(billType, 300)
	if err != nil {
		h.logger.Error("marketplace alias review groups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load review groups"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": groups, "total": len(groups)})
}

func (h *MarketplaceAliasHandler) Confirm(c *gin.Context) {
	var req struct {
		Source        string `json:"source" binding:"required"`
		BillType      string `json:"bill_type" binding:"required"`
		SourceSKU     string `json:"source_sku"`
		RawName       string `json:"raw_name"`
		NormalizedKey string `json:"normalized_key"`
		ItemCode      string `json:"item_code" binding:"required"`
		UnitCode      string `json:"unit_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isMarketplaceSource(req.Source) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported marketplace source"})
		return
	}

	unitCode := req.UnitCode
	if unitCode == "" && h.catalogRepo != nil {
		if cat, _ := h.catalogRepo.GetOne(req.ItemCode); cat != nil {
			unitCode = cat.UnitCode
		}
	}
	userID := c.GetString("user_id")
	alias, err := h.aliasRepo.Upsert(req.Source, req.SourceSKU, req.RawName, req.ItemCode, unitCode, userID)
	if err != nil {
		h.logger.Error("confirm marketplace alias", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save alias"})
		return
	}
	normalizedKey := req.NormalizedKey
	if alias != nil {
		normalizedKey = alias.NormalizedKey
	}
	applied, ready, err := h.aliasRepo.ApplyToOpenItems(req.Source, req.BillType, req.SourceSKU, normalizedKey, req.RawName, req.ItemCode, unitCode)
	if err != nil {
		h.logger.Error("apply marketplace alias", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply alias"})
		return
	}
	if h.auditRepo != nil && alias != nil {
		var auditUserID *string
		if userID != "" {
			auditUserID = &userID
		}
		_ = h.auditRepo.Log(models.AuditEntry{
			Action: "marketplace_alias_confirmed",
			UserID: auditUserID,
			Source: req.Source,
			Level:  "info",
			Detail: map[string]interface{}{
				"alias_id":       alias.ID,
				"source_sku":     alias.SourceSKU,
				"normalized_key": alias.NormalizedKey,
				"raw_name":       alias.RawName,
				"item_code":      alias.ItemCode,
				"unit_code":      alias.UnitCode,
				"applied_items":  applied,
				"ready_bills":    ready,
			},
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"alias":         alias,
		"applied_items": applied,
		"ready_bills":   ready,
	})
}
