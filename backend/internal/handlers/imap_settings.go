package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	emailservice "billflow/internal/services/email"
)

// IMAPSettingsHandler exposes CRUD + test/poll for imap_accounts. Admin only.
type IMAPSettingsHandler struct {
	repo        *repository.ImapAccountRepo
	coordinator *emailservice.Coordinator
	logger      *zap.Logger
}

func NewIMAPSettingsHandler(
	repo *repository.ImapAccountRepo,
	coordinator *emailservice.Coordinator,
	logger *zap.Logger,
) *IMAPSettingsHandler {
	return &IMAPSettingsHandler{repo: repo, coordinator: coordinator, logger: logger}
}

// List returns all accounts. Passwords are scrubbed before sending to the client
// so admins editing a row never see the existing password (re-enter to change).
func (h *IMAPSettingsHandler) List(c *gin.Context) {
	accounts, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, a := range accounts {
		a.Password = ""
	}
	c.JSON(http.StatusOK, gin.H{"data": accounts})
}

// Get returns a single account, password scrubbed.
func (h *IMAPSettingsHandler) Get(c *gin.Context) {
	id := c.Param("id")
	a, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	a.Password = ""
	c.JSON(http.StatusOK, a)
}

// Create inserts a new account and starts its poller.
func (h *IMAPSettingsHandler) Create(c *gin.Context) {
	var in models.IMAPAccountUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required for new account"})
		return
	}
	a := upsertToModel(in)
	if err := h.repo.Create(a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.coordinator.ReloadAccount(a.ID); err != nil {
		h.logger.Warn("imap_create_reload_failed", zap.String("id", a.ID), zap.Error(err))
	}
	a.Password = ""
	c.JSON(http.StatusCreated, a)
}

// Update overwrites a row. Empty password means "keep existing".
func (h *IMAPSettingsHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var in models.IMAPAccountUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a := upsertToModel(in)
	if err := h.repo.Update(id, a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.coordinator.ReloadAccount(id); err != nil {
		h.logger.Warn("imap_update_reload_failed", zap.String("id", id), zap.Error(err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Delete removes the row and stops its poller.
func (h *IMAPSettingsHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.coordinator.RemoveAccount(id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PollNow triggers an immediate poll for the account. Useful after edits or
// when admin wants to verify the connection without waiting for the next tick.
func (h *IMAPSettingsHandler) PollNow(c *gin.Context) {
	id := c.Param("id")
	res, err := h.coordinator.PollNow(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{
		"trace_id":       res.TraceID,
		"messages_found": res.MessagesFound,
		"processed":      res.Processed,
		"skipped":        res.Skipped,
		"duration_ms":    res.Duration.Milliseconds(),
		"status":         res.Status(),
	}
	if res.Err != nil {
		resp["error"] = res.Err.Error()
	}
	c.JSON(http.StatusOK, resp)
}

// TestConnection runs a dry connect+auth+select cycle WITHOUT saving anything.
// Body is the same shape as Create. Used by the "ทดสอบการเชื่อมต่อ" button.
func (h *IMAPSettingsHandler) TestConnection(c *gin.Context) {
	var in models.IMAPAccountUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a := upsertToModel(in)
	if a.Password == "" {
		// Editing an existing row without re-typing password — pull from DB.
		if id := c.Query("id"); id != "" {
			if existing, _ := h.repo.GetByID(id); existing != nil {
				a.Password = existing.Password
			}
		}
	}
	if a.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()

	start := time.Now()
	if err := h.coordinator.TestConnection(ctx, a); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"ok":          false,
			"error":       err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":          true,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// ListFolders returns the IMAP mailbox names for the supplied account creds.
// Body shape = same as Create. Used to populate the folder dropdown.
func (h *IMAPSettingsHandler) ListFolders(c *gin.Context) {
	var in models.IMAPAccountUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a := upsertToModel(in)
	if a.Password == "" {
		if id := c.Query("id"); id != "" {
			if existing, _ := h.repo.GetByID(id); existing != nil {
				a.Password = existing.Password
			}
		}
	}
	if a.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()

	folders, err := emailservice.ListMailboxes(ctx, a)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"folders": folders, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"folders": folders})
}

func upsertToModel(in models.IMAPAccountUpsert) *models.IMAPAccount {
	mailbox := in.Mailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	return &models.IMAPAccount{
		Name:                in.Name,
		Host:                in.Host,
		Port:                in.Port,
		Username:            in.Username,
		Password:            in.Password,
		Mailbox:             mailbox,
		FilterFrom:          in.FilterFrom,
		FilterSubjects:      in.FilterSubjects,
		Channel:             in.Channel,
		ShopeeDomains:       in.ShopeeDomains,
		LookbackDays:        in.LookbackDays,
		PollIntervalSeconds: in.PollIntervalSeconds,
		Enabled:             in.Enabled,
	}
}
