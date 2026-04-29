package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"billflow/internal/models"
	"billflow/internal/repository"
)

// ChatNotesHandler exposes /api/admin/conversations/:lineUserId/notes
// (Phase 4.8 — internal admin annotations on a conversation).
type ChatNotesHandler struct {
	repo *repository.ChatNoteRepo
}

func NewChatNotesHandler(repo *repository.ChatNoteRepo) *ChatNotesHandler {
	return &ChatNotesHandler{repo: repo}
}

// GET /api/admin/conversations/:lineUserId/notes
func (h *ChatNotesHandler) List(c *gin.Context) {
	lineUserID := c.Param("lineUserId")
	rows, err := h.repo.ListByUser(lineUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// POST /api/admin/conversations/:lineUserId/notes
func (h *ChatNotesHandler) Create(c *gin.Context) {
	lineUserID := c.Param("lineUserId")
	var in models.ChatNoteUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n := &models.ChatNote{LineUserID: lineUserID, Body: in.Body}
	if uid := c.GetString("user_id"); uid != "" {
		n.CreatedBy = &uid
	}
	if err := h.repo.Create(n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, n)
}

// PUT /api/admin/conversations/:lineUserId/notes/:noteId
func (h *ChatNotesHandler) Update(c *gin.Context) {
	noteID := c.Param("noteId")
	var in models.ChatNoteUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	n, err := h.repo.Update(noteID, in.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, n)
}

// DELETE /api/admin/conversations/:lineUserId/notes/:noteId
func (h *ChatNotesHandler) Delete(c *gin.Context) {
	noteID := c.Param("noteId")
	if err := h.repo.Delete(noteID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
