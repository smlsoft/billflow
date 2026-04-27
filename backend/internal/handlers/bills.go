package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/sml"
)

type BillHandler struct {
	billRepo  *repository.BillRepo
	mapperSvc *mapper.Service
	smlClient *sml.Client
	lineSvc   *lineservice.Service
	auditRepo *repository.AuditLogRepo
	log       *zap.Logger
}

func NewBillHandler(
	billRepo *repository.BillRepo,
	mapperSvc *mapper.Service,
	smlClient *sml.Client,
	lineSvc *lineservice.Service,
	auditRepo *repository.AuditLogRepo,
	log *zap.Logger,
) *BillHandler {
	return &BillHandler{
		billRepo:  billRepo,
		mapperSvc: mapperSvc,
		smlClient: smlClient,
		lineSvc:   lineSvc,
		auditRepo: auditRepo,
		log:       log,
	}
}

// GET /api/bills
func (h *BillHandler) List(c *gin.Context) {
	var f models.BillListFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bills, total, err := h.billRepo.List(f)
	if err != nil {
		h.log.Error("List bills", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      bills,
		"total":     total,
		"page":      f.Page,
		"page_size": f.PageSize,
	})
}

// GET /api/bills/:id
func (h *BillHandler) Get(c *gin.Context) {
	id := c.Param("id")
	bill, err := h.billRepo.FindByID(id)
	if err != nil {
		h.log.Error("FindByID", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	c.JSON(http.StatusOK, bill)
}

// POST /api/bills/:id/retry
func (h *BillHandler) Retry(c *gin.Context) {
	id := c.Param("id")
	bill, err := h.billRepo.FindByID(id)
	if err != nil || bill == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bill not found"})
		return
	}
	if bill.Status != "failed" && bill.Status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only failed or pending bills can be retried"})
		return
	}

	// Re-run mapper on each item to pick up any newly added mappings
	var smlItems []sml.SMLItem
	allMapped := true
	for _, item := range bill.Items {
		match := h.mapperSvc.Match(item.RawName)

		itemCode := item.RawName
		unitCode := ""
		price := 0.0
		if item.Price != nil {
			price = *item.Price
		}

		if match.Mapping != nil && !match.NeedsReview {
			itemCode = match.Mapping.ItemCode
			unitCode = match.Mapping.UnitCode
			// Update item in DB with resolved mapping
			_ = h.billRepo.UpdateBillItem(item.ID, match.Mapping.ItemCode, match.Mapping.UnitCode, match.Mapping.ID, true)
		} else {
			allMapped = false
		}

		smlItems = append(smlItems, sml.SMLItem{
			ItemCode: itemCode,
			Qty:      item.Qty,
			UnitCode: unitCode,
			Price:    price,
		})
	}

	if !allMapped {
		_ = h.billRepo.UpdateStatus(id, "pending", nil, nil, nil)
		c.JSON(http.StatusAccepted, gin.H{"message": "some items still unmapped — bill set to pending"})
		return
	}

	// Extract customer info from raw_data
	var rawData struct {
		CustomerName  string  `json:"customer_name"`
		CustomerPhone *string `json:"customer_phone"`
	}
	if bill.RawData != nil {
		_ = json.Unmarshal(bill.RawData, &rawData)
	}

	req := sml.SaleOrderRequest{
		ContactName: rawData.CustomerName,
		Items:       smlItems,
	}
	if rawData.CustomerPhone != nil {
		req.ContactPhone = *rawData.CustomerPhone
	}

	reqJSON, _ := json.Marshal(req)
	retryStart := time.Now()
	result, err := h.smlClient.CreateSaleReserve(req)
	if err != nil {
		errMsg := err.Error()
		respJSON, _ := json.Marshal(map[string]string{"error": errMsg})
		_ = h.billRepo.UpdateStatus(id, "failed", nil, respJSON, &errMsg)
		h.log.Error("Retry: SML failed", zap.String("bill", id), zap.Error(err))
		if h.auditRepo != nil {
			billID := id
			durMs := int(time.Since(retryStart).Milliseconds())
			_ = h.auditRepo.Log(models.AuditEntry{
				Action:     "sml_failed",
				TargetID:   &billID,
				Source:     bill.Source,
				Level:      "error",
				TraceID:    c.GetString("trace_id"),
				DurationMs: &durMs,
				Detail: map[string]interface{}{
					"sml_payload": json.RawMessage(reqJSON),
					"error":       errMsg,
					"via":         "retry",
				},
			})
		}
		if h.lineSvc != nil {
			_ = h.lineSvc.PushAdmin("⚠️ Bill retry SML failed\nBill: " + id + "\nError: " + errMsg)
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "SML send failed: " + errMsg})
		return
	}

	respJSON, _ := json.Marshal(result)
	_ = h.billRepo.UpdateStatus(id, "sent", &result.DocNo, respJSON, nil)
	_ = h.billRepo.UpdateSMLPayload(id, reqJSON)
	if h.auditRepo != nil {
		billID := id
		durMs := int(time.Since(retryStart).Milliseconds())
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:     "sml_sent",
			TargetID:   &billID,
			Source:     bill.Source,
			Level:      "info",
			TraceID:    c.GetString("trace_id"),
			DurationMs: &durMs,
			Detail: map[string]interface{}{
				"doc_no":       result.DocNo,
				"sml_payload":  json.RawMessage(reqJSON),
				"sml_response": json.RawMessage(respJSON),
				"via":          "retry",
			},
		})
	}
	if b, err := h.billRepo.FindByID(id); err == nil && b != nil {
		_ = h.billRepo.UpdatePriceHistory(b.Items)
	}

	h.log.Info("Retry: bill sent", zap.String("bill", id), zap.String("doc", result.DocNo))
	c.JSON(http.StatusOK, gin.H{
		"message": "bill sent to SML",
		"doc_no":  result.DocNo,
	})
}
