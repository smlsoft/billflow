package emailservice

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	lineservice "billflow/internal/services/line"
)

// AccountPoller wraps one IMAP account with its own ticker goroutine.
// The coordinator owns N of these (one per enabled account).
type AccountPoller struct {
	accountID  string
	repo       *repository.ImapAccountRepo
	processors *Processors
	lineSvc    *lineservice.Service
	logger     *zap.Logger

	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	running bool
}

// alertThrottle — minimum gap between LINE admin notifications per account
// when consecutive_failures stays ≥ 3. Prevents spamming during long outages.
const alertThrottle = 1 * time.Hour

// alertThreshold — number of consecutive failed polls before paging admin.
const alertThreshold = 3

func NewAccountPoller(
	accountID string,
	repo *repository.ImapAccountRepo,
	processors *Processors,
	lineSvc *lineservice.Service,
	logger *zap.Logger,
) *AccountPoller {
	return &AccountPoller{
		accountID:  accountID,
		repo:       repo,
		processors: processors,
		lineSvc:    lineSvc,
		logger:     logger.With(zap.String("account_id", accountID)),
	}
}

// Start spawns the poll loop. Calling Start a second time is a no-op.
func (p *AccountPoller) Start(parent context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.running = true
	p.mu.Unlock()

	go p.run(ctx)
}

// Stop cancels the poll loop and waits for the goroutine to exit.
// Safe to call multiple times.
func (p *AccountPoller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	done := p.done
	p.running = false
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		// Bound the wait so a stuck IMAP connection can't hang shutdown.
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			p.logger.Warn("imap_poller_stop_timeout")
		}
	}
}

func (p *AccountPoller) run(ctx context.Context) {
	defer close(p.done)

	// Always poll once on start so admins see immediate feedback after
	// adding or editing an account.
	p.pollCycle(ctx)
	if ctx.Err() != nil {
		return
	}

	// Initial interval comes from the first DB read; re-read each tick so
	// admin can change the cadence without restarting the poller.
	for {
		account, err := p.repo.GetByID(p.accountID)
		if err != nil || account == nil || !account.Enabled {
			return
		}
		interval := time.Duration(account.PollIntervalSeconds) * time.Second
		if interval < 5*time.Minute {
			interval = 5 * time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
		p.pollCycle(ctx)
	}
}

// PollNow runs one cycle immediately, ignoring the interval. Used by the
// "poll-now" admin button. Returns the result for the caller to surface.
func (p *AccountPoller) PollNow(ctx context.Context) PollResult {
	return p.pollCycle(ctx)
}

func (p *AccountPoller) pollCycle(ctx context.Context) PollResult {
	account, err := p.repo.GetByID(p.accountID)
	if err != nil {
		p.logger.Error("imap_poller_load_failed", zap.Error(err))
		return PollResult{Err: err}
	}
	if account == nil {
		// Row was deleted — stop the goroutine.
		p.logger.Info("imap_poller_account_gone")
		if p.cancel != nil {
			p.cancel()
		}
		return PollResult{}
	}
	if !account.Enabled {
		// Disabled — caller will Stop() us shortly via coordinator.
		return PollResult{}
	}

	cfg := pollConfigFromAccount(account)
	res := PollOnce(ctx, cfg, p.processors, p.logger)

	errMsg := ""
	if res.Err != nil {
		errMsg = res.Err.Error()
	}
	if updateErr := p.repo.UpdatePollStatus(account.ID, res.Status(), errMsg, res.Processed); updateErr != nil {
		p.logger.Warn("imap_poller_status_update_failed", zap.Error(updateErr))
	}

	if res.Err != nil {
		p.maybeAlertAdmin(account, res)
	}

	return res
}

// maybeAlertAdmin pushes a LINE message to the admin if this account has
// failed ≥ alertThreshold times in a row AND we haven't alerted within
// alertThrottle. Re-reads the row after UpdatePollStatus so consecutive_failures
// is current.
func (p *AccountPoller) maybeAlertAdmin(_ *models.IMAPAccount, res PollResult) {
	if p.lineSvc == nil {
		return
	}
	fresh, err := p.repo.GetByID(p.accountID)
	if err != nil || fresh == nil {
		return
	}
	if fresh.ConsecutiveFailures < alertThreshold {
		return
	}
	if fresh.LastAdminAlertAt != nil && time.Since(*fresh.LastAdminAlertAt) < alertThrottle {
		return
	}

	msg := fmt.Sprintf(
		"⚠️ BillFlow IMAP fail\nInbox: %s (%s)\nFails: %d ครั้งติด\nError: %s",
		fresh.Name, fresh.Username, fresh.ConsecutiveFailures, truncate(res.Err.Error(), 200),
	)
	if err := p.lineSvc.PushAdmin(msg); err != nil {
		p.logger.Warn("imap_admin_alert_failed", zap.Error(err))
		return
	}
	if err := p.repo.MarkAlertSent(fresh.ID); err != nil {
		p.logger.Warn("imap_admin_alert_stamp_failed", zap.Error(err))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// pollConfigFromAccount snapshots the DB row into a value struct so the
// goroutine isn't holding the *IMAPAccount across a long-running poll.
func pollConfigFromAccount(a *models.IMAPAccount) PollConfig {
	return PollConfig{
		AccountID:      a.ID,
		AccountName:    a.Name,
		Host:           a.Host,
		Port:           a.Port,
		Username:       a.Username,
		Password:       a.Password,
		Mailbox:        a.Mailbox,
		FilterFrom:     a.FilterFrom,
		FilterSubjects: parseCSV(a.FilterSubjects, true),
		LookbackDays:   a.LookbackDays,
		Channel:        a.Channel,
		ShopeeDomains:  parseCSV(a.ShopeeDomains, true),
	}
}

// parseCSV splits a comma-separated string and trims whitespace + drops
// empty entries. lower=true normalizes to lowercase for case-insensitive matching.
func parseCSV(s string, lower bool) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if lower {
			p = strings.ToLower(p)
		}
		out = append(out, p)
	}
	return out
}
