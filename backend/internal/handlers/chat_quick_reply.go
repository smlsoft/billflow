package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"billflow/internal/models"
	"billflow/internal/repository"
)

// ChatQuickReplyHandler exposes /api/admin/quick-replies CRUD.
// Templates are global (not per-OA / per-admin) — keeps Phase 4 simple.
type ChatQuickReplyHandler struct {
	repo *repository.ChatQuickReplyRepo
}

func NewChatQuickReplyHandler(repo *repository.ChatQuickReplyRepo) *ChatQuickReplyHandler {
	return &ChatQuickReplyHandler{repo: repo}
}

// GET /api/admin/quick-replies
func (h *ChatQuickReplyHandler) List(c *gin.Context) {
	rows, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// POST /api/admin/quick-replies
func (h *ChatQuickReplyHandler) Create(c *gin.Context) {
	var in models.ChatQuickReplyUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q := &models.ChatQuickReply{
		Label:     in.Label,
		Body:      in.Body,
		SortOrder: in.SortOrder,
	}
	if uid := c.GetString("user_id"); uid != "" {
		q.CreatedBy = &uid
	}
	if err := h.repo.Create(q); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, q)
}

// PUT /api/admin/quick-replies/:id
func (h *ChatQuickReplyHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var in models.ChatQuickReplyUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q, err := h.repo.Update(id, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, q)
}

// DELETE /api/admin/quick-replies/:id
func (h *ChatQuickReplyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
