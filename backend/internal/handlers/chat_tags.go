package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"billflow/internal/models"
	"billflow/internal/repository"
)

// ChatTagsHandler exposes:
//   /api/settings/chat-tags          — global tag CRUD
//   /api/admin/conversations/:user/tags — m2m attach/detach
type ChatTagsHandler struct {
	repo *repository.ChatTagRepo
}

func NewChatTagsHandler(repo *repository.ChatTagRepo) *ChatTagsHandler {
	return &ChatTagsHandler{repo: repo}
}

// ── Global CRUD (admin only) ─────────────────────────────────────────────────

// GET /api/settings/chat-tags
func (h *ChatTagsHandler) ListAll(c *gin.Context) {
	rows, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// POST /api/settings/chat-tags
func (h *ChatTagsHandler) Create(c *gin.Context) {
	var in models.ChatTagUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t := &models.ChatTag{Label: in.Label, Color: in.Color}
	if err := h.repo.Create(t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, t)
}

// PUT /api/settings/chat-tags/:id
func (h *ChatTagsHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var in models.ChatTagUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.repo.Update(id, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// DELETE /api/settings/chat-tags/:id
func (h *ChatTagsHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Per-conversation tag attach ──────────────────────────────────────────────

// GET /api/admin/conversations/:lineUserId/tags
func (h *ChatTagsHandler) TagsForConversation(c *gin.Context) {
	lineUserID := c.Param("lineUserId")
	rows, err := h.repo.TagsForConversation(lineUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// PUT /api/admin/conversations/:lineUserId/tags
// Body: {tag_ids: ["uuid", "uuid", …]}  — replaces the full set
func (h *ChatTagsHandler) SetTagsForConversation(c *gin.Context) {
	lineUserID := c.Param("lineUserId")
	var in struct {
		TagIDs []string `json:"tag_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.SetTagsForConversation(lineUserID, in.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows, _ := h.repo.TagsForConversation(lineUserID)
	c.JSON(http.StatusOK, gin.H{"data": rows})
}
