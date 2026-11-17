package handlers

import (
	"billflow/internal/services/shippingcheck"
	"go.uber.org/zap"
)

func (h *EmailHandler) SetShippingCheckStore(s *shippingcheck.Store) { h.shippingChecks = s }
func (h *EmailHandler) enrollShippingCheck(raw map[string]interface{}, account string) {
	if h.shippingChecks == nil {
		return
	}
	if err := h.shippingChecks.Enroll(raw, account); err != nil {
		raw["shipping_check_summary"] = map[string]any{"status": "unverifiable", "attention": true, "reason": "โหลดกติกาตรวจจัดส่งไม่สำเร็จ"}
		h.logger.Warn("shipping_check_enrollment_failed", zap.Error(err))
	}
}
func (h *EmailHandler) recordShippingCheck(billID, messageID, bodyHTML string) {
	if h.shippingChecks == nil {
		return
	}
	artifactID, err := h.shippingChecks.ArtifactID(billID, messageID)
	if err == nil {
		err = h.shippingChecks.Record(billID, messageID, bodyHTML, artifactID)
	}
	if err != nil {
		h.logger.Warn("shipping_check_record_failed", zap.String("bill_id", billID), zap.Error(err))
		// Best effort fallback: no intake/SML state is changed and old warnings survive.
		_, fallbackErr := h.billRepo.DB().Exec(`UPDATE bills SET raw_data=jsonb_set(COALESCE(raw_data,'{}'::jsonb),'{shipping_check_summary}',COALESCE(raw_data->'shipping_check_summary','{}'::jsonb)||'{"status":"unverifiable","attention":true,"has_warning":true,"reason":"บันทึกผลตรวจจัดส่งไม่สำเร็จ"}'::jsonb,true) WHERE id=$1 AND raw_data ? 'shipping_check_policy'`, billID)
		if fallbackErr != nil {
			h.logger.Warn("shipping_check_fallback_failed", zap.String("bill_id", billID), zap.Error(fallbackErr))
		}
	}
}
