package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/mapper"
)

type MappingHandler struct {
	mappingRepo *repository.MappingRepo
	mapperSvc   *mapper.Service
	log         *zap.Logger
}

func NewMappingHandler(mappingRepo *repository.MappingRepo, mapperSvc *mapper.Service, log *zap.Logger) *MappingHandler {
	return &MappingHandler{mappingRepo: mappingRepo, mapperSvc: mapperSvc, log: log}
}

// GET /api/mappings
func (h *MappingHandler) List(c *gin.Context) {
	mappings, err := h.mappingRepo.ListAll()
	if err != nil {
		h.log.Error("ListAll mappings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": mappings})
}

// POST /api/mappings
func (h *MappingHandler) Create(c *gin.Context) {
	var req models.CreateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	mapping, err := h.mappingRepo.Create(req.RawName, req.ItemCode, req.UnitCode, userID.(string))
	if err != nil {
		h.log.Error("Create mapping", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, mapping)
}

// PUT /api/mappings/:id
func (h *MappingHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req models.CreateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	if err := h.mappingRepo.Upsert(req.RawName, req.ItemCode, req.UnitCode, "manual", nil); err != nil {
		h.log.Error("Update mapping", zap.String("id", id), zap.String("user", userID.(string)), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DELETE /api/mappings/:id
func (h *MappingHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.mappingRepo.Delete(id); err != nil {
		h.log.Error("Delete mapping", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// GET /api/mappings/stats
func (h *MappingHandler) Stats(c *gin.Context) {
	stats, err := h.mappingRepo.Stats()
	if err != nil {
		h.log.Error("Mapping stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// POST /api/mappings/feedback — F1 human correction
func (h *MappingHandler) Feedback(c *gin.Context) {
	var req struct {
		BillItemID    string `json:"bill_item_id"`
		RawName       string `json:"raw_name" binding:"required"`
		CorrectedCode string `json:"corrected_item_code" binding:"required"`
		CorrectedUnit string `json:"corrected_unit_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var billItemID *string
	if req.BillItemID != "" {
		billItemID = &req.BillItemID
	}

	if err := h.mapperSvc.LearnFromFeedback(req.RawName, req.CorrectedCode, req.CorrectedUnit, billItemID); err != nil {
		h.log.Error("Feedback: LearnFromFeedback", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "feedback saved"})
}
