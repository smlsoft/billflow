package handlers

import (
	"net/http"

	"billflow/internal/models"
	"billflow/internal/services/shippingcheck"
	"github.com/gin-gonic/gin"
)

type shippingSourceDetails struct {
	Recipient            string `json:"recipient"`
	Address              string `json:"address"`
	Reason               string `json:"reason,omitempty"`
	ArtifactID           string `json:"artifact_id,omitempty"`
	ArtifactBillID       string `json:"artifact_bill_id,omitempty"`
	ManualCheckAvailable bool   `json:"manual_check_available"`
	ManualCheckReason    string `json:"manual_check_reason,omitempty"`
}

func (h *ShippingCheckHandler) shippingSource(c *gin.Context) (*shippingcheck.Sample, string, *models.BillArtifact, bool) {
	id := c.Param("id")
	if !shippingUUID.MatchString(id) {
		c.JSON(400, gin.H{"error": "รหัสบิลไม่ถูกต้อง"})
		return nil, "", nil, false
	}
	bill, err := h.store.Sample(id)
	if err != nil {
		h.failure(c, err)
		return nil, "", nil, false
	}
	if bill == nil {
		c.JSON(404, gin.H{"error": "ไม่พบบิล"})
		return nil, "", nil, false
	}
	if bill.BillType != "purchase" || (bill.Source != "shopee_shipped" && bill.Source != "lazada_email") {
		c.JSON(400, gin.H{"error": "รองรับเฉพาะบิลซื้อจากอีเมล Shopee/Lazada"})
		return nil, "", nil, false
	}
	artifactID, err := h.store.ArtifactID(id, bill.MessageID)
	if err != nil {
		h.failure(c, err)
		return nil, "", nil, false
	}
	// Legacy bills with no message id can use their own sole HTML artifact only.
	if artifactID == "" && bill.MessageID == "" && h.artifacts != nil {
		list, err := h.artifacts.ListByBill(id)
		if err != nil {
			h.failure(c, err)
			return nil, "", nil, false
		}
		count := 0
		for _, a := range list {
			if a.Kind == "email_html" {
				count++
				artifactID = a.ID
			}
		}
		if count != 1 {
			artifactID = ""
		}
	}
	if artifactID == "" {
		return bill, "", nil, true
	}
	body, a, err := h.files.Read(artifactID)
	if err != nil {
		h.failure(c, err)
		return nil, "", nil, false
	}
	if a == nil || a.Kind != "email_html" {
		return bill, "", nil, true
	}
	return bill, string(body), a, true
}

// SourceDetails displays evidence without creating a result or reading credentials.
func (h *ShippingCheckHandler) SourceDetails(c *gin.Context) {
	bill, body, a, ok := h.shippingSource(c)
	if !ok {
		return
	}
	parsed := shippingcheck.Extract(body)
	out := shippingSourceDetails{Recipient: parsed.Recipient, Address: parsed.Address, Reason: parsed.Reason}
	if a != nil {
		out.ArtifactID = a.ID
		out.ArtifactBillID = a.BillID
	} else {
		out.Reason = "ไม่พบหลักฐาน HTML ต้นฉบับที่ระบุได้ชัดเจน"
	}
	policy, err := h.store.CurrentPolicy(bill.AccountID)
	if err != nil {
		h.failure(c, err)
		return
	}
	out.ManualCheckAvailable = policy != nil
	if policy == nil {
		out.ManualCheckReason = "ยังไม่มีกติกาพร้อมใช้สำหรับกล่องอีเมลของบิลนี้ ให้ผู้ดูแลตั้งค่าโปรไฟล์และชื่อผู้รับก่อน"
	}
	c.JSON(200, out)
}

// ManualCheck evaluates this bill's original email against current mailbox rules.
// Historical bills are not enrolled for automatic follow-up checks by this action.
func (h *ShippingCheckHandler) ManualCheck(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in struct {
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || !shippingUUID.MatchString(in.RequestID) {
		c.JSON(400, gin.H{"error": "รหัสคำขอตรวจไม่ถูกต้อง"})
		return
	}
	userID := c.GetString("user_id")
	if !shippingUUID.MatchString(userID) {
		c.JSON(401, gin.H{"error": "กรุณาเข้าสู่ระบบใหม่"})
		return
	}
	bill, body, a, ok := h.shippingSource(c)
	if !ok {
		return
	}
	policy, err := h.store.CurrentPolicy(bill.AccountID)
	if err != nil {
		h.failure(c, err)
		return
	}
	if policy == nil {
		c.JSON(400, gin.H{"error": "ยังไม่มีกติกาพร้อมใช้สำหรับกล่องอีเมลของบิลนี้ ให้ผู้ดูแลตั้งค่าโปรไฟล์และชื่อผู้รับก่อน"})
		return
	}
	result := shippingcheck.Check(body, policy.Profile, policy.Recipients)
	result.CheckedBy = c.GetString("user_email")
	if result.CheckedBy == "" {
		result.CheckedBy = userID
	}
	artifactID := ""
	if a != nil {
		artifactID = a.ID
	} else {
		result.Reason = "ไม่พบหลักฐาน HTML ต้นฉบับที่ระบุได้ชัดเจน"
	}
	if err = h.store.RecordManual(bill.ID, in.RequestID, bill.MessageID, artifactID, *policy, result, userID); err != nil {
		h.failure(c, err)
		return
	}
	// Reload history after success. A retry may have reused an already-saved request
	// id, so do not return a newly computed result as if it replaced that record.
	c.JSON(200, gin.H{"status": "recorded"})
}
