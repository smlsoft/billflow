package handlers

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

// AttentionCounts reports global active, unsent purchase email work only.
func (h *BillHandler) AttentionCounts(c *gin.Context) {
	counts, err := h.billRepo.AttentionCounts()
	if err != nil {
		h.log.Error("count bill attention", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดจำนวนงานที่ต้องตรวจไม่สำเร็จ กรุณาลองใหม่"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, counts)
}
