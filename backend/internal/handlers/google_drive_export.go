package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"billflow/internal/services/googledrive"
)

// GoogleDriveExportHandler is intentionally admin-only at the route level:
// its controls can create external copies of customer source emails.
type GoogleDriveExportHandler struct {
	service *googledrive.Service
}

func NewGoogleDriveExportHandler(service *googledrive.Service) *GoogleDriveExportHandler {
	return &GoogleDriveExportHandler{service: service}
}

type googleDriveExportSettingsRequest struct {
	Enabled    bool   `json:"enabled"`
	RootFolder string `json:"root_folder"`
	StartDate  string `json:"start_date"`
}

type googleDriveBackfillRequest struct {
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
}

func (h *GoogleDriveExportHandler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Status())
}

func (h *GoogleDriveExportHandler) UpdateSettings(c *gin.Context) {
	var req googleDriveExportSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลตั้งค่าไม่ถูกต้อง"})
		return
	}
	status, err := h.service.UpdateSettings(req.Enabled, req.RootFolder, req.StartDate, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *GoogleDriveExportHandler) TestConnection(c *gin.Context) {
	var req googleDriveExportSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลตั้งค่าไม่ถูกต้อง"})
		return
	}
	if err := h.service.TestConnection(req.RootFolder); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "detail": "เชื่อมต่อ Google Drive และ PDF renderer สำเร็จ พร้อมทดสอบสิทธิ์สร้าง/ลบโฟลเดอร์"})
}

func (h *GoogleDriveExportHandler) ListJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "50")))
	jobs, counts, err := h.service.ListJobs(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดประวัติอัปโหลดไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs, "counts": counts})
}

func (h *GoogleDriveExportHandler) PreviewBackfill(c *gin.Context) {
	var req googleDriveBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุช่วงวันที่"})
		return
	}
	preview, err := h.service.PreviewBackfill(req.DateFrom, req.DateTo)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (h *GoogleDriveExportHandler) EnqueueBackfill(c *gin.Context) {
	var req googleDriveBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาระบุช่วงวันที่"})
		return
	}
	result, err := h.service.EnqueueBackfill(req.DateFrom, req.DateTo, c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (h *GoogleDriveExportHandler) RetryJob(c *gin.Context) {
	ok, err := h.service.Retry(c.Param("id"), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สั่งลองอัปโหลดใหม่ไม่สำเร็จ"})
		return
	}
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"error": "งานนี้ยังไม่อยู่ในสถานะที่ลองใหม่ได้"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *GoogleDriveExportHandler) RequeueAsPDF(c *gin.Context) {
	ok, err := h.service.RequeueAsPDF(c.Param("id"), c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"error": "งานนี้ไม่ใช่ไฟล์ HTML ที่อัปโหลดสำเร็จแล้ว"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}
