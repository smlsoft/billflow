package handlers

import (
	"net/http"

	"billflow/internal/models"
	"github.com/gin-gonic/gin"
)

type paymentMethodFilterPolicy struct {
	PaymentMethods []string `json:"payment_methods"`
}

type paymentMethodFilterRow struct {
	Channel     string                    `json:"channel"`
	BillType    string                    `json:"bill_type"`
	PrintPolicy paymentMethodFilterPolicy `json:"print_policy"`
}

// ListPaymentMethodOptions exposes only purchase payment choices to bill readers.
// The full channel-default settings endpoint remains admin-only.
func (h *ChannelDefaultsHandler) ListPaymentMethodOptions(c *gin.Context) {
	rows, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรายการวิธีชำระเงินไม่สำเร็จ"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": paymentMethodFilterRows(rows)})
}

func paymentMethodFilterRows(rows []*models.ChannelDefault) []paymentMethodFilterRow {
	options := make([]paymentMethodFilterRow, 0, 2)
	for _, channel := range []string{"shopee_shipped", "lazada_email"} {
		policy := models.DefaultMarketplacePrintPolicy(channel, "purchase")
		for _, row := range rows {
			if row.Channel == channel && row.BillType == "purchase" && len(row.PrintPolicy.PaymentMethods) > 0 {
				policy = row.PrintPolicy
				break
			}
		}
		options = append(options, paymentMethodFilterRow{Channel: channel, BillType: "purchase", PrintPolicy: paymentMethodFilterPolicy{PaymentMethods: policy.PaymentMethods}})
	}
	return options
}
