package handlers

import (
	"fmt"
	"net/http"

	"billflow/internal/models"
	"billflow/internal/services/shippingcheck"

	"go.uber.org/zap"
)

func (h *BillHandler) shippingSendStates(bills []models.Bill) (map[string]models.ShippingSendCheck, error) {
	// Ordinary bills do not depend on the shipping-check subsystem.
	relevant := false
	for i := range bills {
		if isMarketplacePurchaseEmailBill(&bills[i]) {
			relevant = true
			break
		}
	}
	if !relevant {
		return map[string]models.ShippingSendCheck{}, nil
	}
	if h.billRepo == nil {
		return nil, fmt.Errorf("shipping check database unavailable")
	}
	return shippingcheck.NewStore(h.billRepo.DB()).SendReadiness(bills)
}

func (h *BillHandler) attachShippingSendReadiness(bills []models.Bill) {
	states, err := h.shippingSendStates(bills)
	if err != nil && h.log != nil {
		h.log.Error("load shipping send readiness", zap.Error(err))
	}
	for i := range bills {
		if !isMarketplacePurchaseEmailBill(&bills[i]) {
			continue
		}
		state := states[bills[i].ID]
		if err != nil {
			state = models.ShippingSendCheck{Required: true, Reason: "อ่านผลตรวจจัดส่งไม่สำเร็จ กรุณาลองใหม่ก่อนส่ง SML"}
		}
		bills[i].ShippingSendCheck = &state
	}
}

// Called at the shared single/bulk send boundary before any SML request.
// Read the ledger again here: UI state and raw_data summaries are not authority.
func (h *BillHandler) guardShippingManualPass(bill *models.Bill) *retrySendResult {
	states, err := h.shippingSendStates([]models.Bill{*bill})
	if err != nil {
		if h.log != nil {
			h.log.Error("shipping send guard failed", zap.String("bill_id", bill.ID), zap.Error(err))
		}
		return &retrySendResult{HTTPStatus: http.StatusServiceUnavailable, Error: "อ่านผลตรวจจัดส่งไม่สำเร็จ กรุณาลองใหม่ก่อนส่ง SML", Skipped: true}
	}
	state, ok := states[bill.ID]
	if ok && state.Required && !state.CanSend {
		return &retrySendResult{HTTPStatus: http.StatusConflict, Error: state.Reason, Skipped: true}
	}
	return nil
}
