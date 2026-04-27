package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
)

type LogHandler struct {
	auditRepo *repository.AuditLogRepo
	log       *zap.Logger
}

func NewLogHandler(auditRepo *repository.AuditLogRepo, log *zap.Logger) *LogHandler {
	return &LogHandler{auditRepo: auditRepo, log: log}
}

// GET /api/logs
func (h *LogHandler) List(c *gin.Context) {
	var f models.AuditLogFilter
	if err := c.ShouldBindQuery(&f); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logs, total, err := h.auditRepo.List(f)
	if err != nil {
		h.log.Error("list audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if logs == nil {
		logs = []models.AuditLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      logs,
		"total":     total,
		"page":      f.Page,
		"page_size": f.PageSize,
	})
}
