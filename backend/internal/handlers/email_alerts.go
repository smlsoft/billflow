package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"billflow/internal/services/emailalert"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type EmailAlertHandler struct {
	store *emailalert.Store
	log   *zap.Logger
}

func NewEmailAlertHandler(store *emailalert.Store, log *zap.Logger) *EmailAlertHandler {
	return &EmailAlertHandler{store, log}
}
func (h *EmailAlertHandler) fail(c *gin.Context, err error) {
	h.log.Error("email_alert_api_failed", zap.Error(err))
	c.JSON(500, gin.H{"error": "ดำเนินการแจ้งเตือนอีเมลไม่สำเร็จ กรุณาลองใหม่หรือตรวจ log ของระบบ"})
}
func (h *EmailAlertHandler) Settings(c *gin.Context) {
	v, err := h.store.Load(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}
func (h *EmailAlertHandler) Save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var v emailalert.Config
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(400, gin.H{"error": "รูปแบบการตั้งค่าไม่ถูกต้อง"})
		return
	}
	old, err := h.store.Load(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	v.Normalize()
	v.HasPassword = old.HasPassword || v.Password != ""
	if err = v.Validate(false); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	saved, err := h.store.Save(c.Request.Context(), v, c.GetString("user_id"))
	if errors.Is(err, emailalert.ErrConflict) {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		var requestError emailalert.RequestError
		if errors.As(err, &requestError) {
			c.JSON(400, gin.H{"error": requestError.Error()})
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(200, saved)
}
func (h *EmailAlertHandler) Test(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in struct {
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || !shippingUUID.MatchString(in.RequestID) {
		c.JSON(400, gin.H{"error": "รหัสคำขอทดสอบไม่ถูกต้อง"})
		return
	}
	config, err := h.store.Load(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	if err = config.Validate(true); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err = h.store.QueueTest(c.Request.Context(), in.RequestID, c.GetString("user_id")); err != nil {
		if errors.Is(err, emailalert.ErrRateLimited) {
			c.JSON(429, gin.H{"error": err.Error()})
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"status": "queued"})
}
func (h *EmailAlertHandler) History(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000 {
		c.JSON(400, gin.H{"error": "เลขหน้าไม่ถูกต้อง"})
		return
	}
	v, err := h.store.History(c.Request.Context(), page)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}
func (h *EmailAlertHandler) Retry(c *gin.Context) {
	if !shippingUUID.MatchString(c.Param("id")) {
		c.JSON(400, gin.H{"error": "รหัสประวัติไม่ถูกต้อง"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var in struct {
		Confirm bool `json:"confirm_duplicate_risk"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": "ข้อมูลคำขอไม่ถูกต้อง"})
		return
	}
	if err := h.store.Retry(c.Request.Context(), c.Param("id"), c.GetString("user_id"), in.Confirm); err != nil {
		var requestError emailalert.RequestError
		if errors.As(err, &requestError) {
			c.JSON(409, gin.H{"error": requestError.Error()})
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"status": "queued"})
}
