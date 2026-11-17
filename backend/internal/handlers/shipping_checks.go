package handlers

import (
	"errors"
	"net/http"
	"regexp"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/artifact"
	"billflow/internal/services/shippingcheck"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var shippingUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type ShippingCheckHandler struct {
	store     *shippingcheck.Store
	artifacts *repository.BillArtifactRepo
	files     *artifact.Service
	log       *zap.Logger
}

func NewShippingCheckHandler(store *shippingcheck.Store, artifacts *repository.BillArtifactRepo, files *artifact.Service, log *zap.Logger) *ShippingCheckHandler {
	return &ShippingCheckHandler{store, artifacts, files, log}
}
func (h *ShippingCheckHandler) Settings(c *gin.Context) {
	v, err := h.store.Load()
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, v)
}
func (h *ShippingCheckHandler) Save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var v shippingcheck.Settings
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(400, gin.H{"error": "รูปแบบการตั้งค่าไม่ถูกต้อง"})
		return
	}
	if err := v.Validate(); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	for _, p := range v.Policies {
		if !shippingUUID.MatchString(p.AccountID) {
			c.JSON(400, gin.H{"error": "รหัสกล่องอีเมลไม่ถูกต้อง"})
			return
		}
	}
	saved, err := h.store.Save(v, c.GetString("user_id"))
	if errors.Is(err, shippingcheck.ErrConflict) {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, saved)
}
func (h *ShippingCheckHandler) Preview(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var in struct {
		AccountID string                 `json:"account_id"`
		BillID    string                 `json:"bill_id"`
		Config    shippingcheck.Settings `json:"config"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": "ข้อมูลทดลองตรวจไม่ถูกต้อง"})
		return
	}
	if !shippingUUID.MatchString(in.BillID) {
		c.JSON(400, gin.H{"error": "รหัสบิลไม่ถูกต้อง"})
		return
	}
	if err := in.Config.Validate(); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	bill, err := h.store.Sample(in.BillID)
	if err != nil {
		h.failure(c, err)
		return
	}
	if bill == nil {
		c.JSON(404, gin.H{"error": "ไม่พบบิล"})
		return
	}
	if bill.AccountID != in.AccountID || bill.BillType != "purchase" || (bill.Source != "shopee_shipped" && bill.Source != "lazada_email") {
		c.JSON(400, gin.H{"error": "เลือกบิลซื้อจากกล่องอีเมลเดียวกับกติกาที่ทดลอง"})
		return
	}
	var profile *shippingcheck.Profile
	var names []string
	for _, policy := range in.Config.Policies {
		if policy.AccountID == in.AccountID {
			names = policy.Recipients
			for i := range in.Config.Profiles {
				if in.Config.Profiles[i].ID == policy.ProfileID {
					profile = &in.Config.Profiles[i]
				}
			}
		}
	}
	if profile == nil || len(names) == 0 {
		c.JSON(400, gin.H{"error": "เลือกโปรไฟล์และระบุชื่อผู้รับก่อนทดลองตรวจ"})
		return
	}
	artifactID, err := h.store.ArtifactID(bill.ID, bill.MessageID)
	if err != nil {
		h.failure(c, err)
		return
	}
	var a *models.BillArtifact
	if artifactID != "" {
		a, err = h.artifacts.GetOne(artifactID)
		if err != nil {
			h.failure(c, err)
			return
		}
	}
	if a == nil || a.Kind != "email_html" {
		c.JSON(200, gin.H{"result": shippingcheck.Result{Status: "unverifiable", Version: shippingcheck.Version, Reason: "ไม่พบหลักฐาน HTML ต้นฉบับ"}})
		return
	}
	body, _, err := h.files.Read(a.ID)
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, gin.H{"result": shippingcheck.Check(string(body), *profile, names), "artifact_id": a.ID, "artifact_bill_id": a.BillID})
}
func (h *ShippingCheckHandler) History(c *gin.Context) {
	if !shippingUUID.MatchString(c.Param("id")) {
		c.JSON(400, gin.H{"error": "รหัสบิลไม่ถูกต้อง"})
		return
	}
	entries, err := h.store.History(c.Param("id"))
	if err != nil {
		h.failure(c, err)
		return
	}
	c.JSON(200, gin.H{"data": entries})
}
func (h *ShippingCheckHandler) failure(c *gin.Context, err error) {
	h.log.Error("shipping_check_request_failed", zap.Error(err))
	c.JSON(500, gin.H{"error": "ตรวจสอบข้อมูลจัดส่งไม่สำเร็จ กรุณาลองใหม่"})
}
