package jobs

import (
	"time"

	emailservice "billflow/internal/services/email"
	lineservice "billflow/internal/services/line"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// AttachmentProcessor matches emailservice.AttachmentProcessor to avoid circular imports
type AttachmentProcessor interface {
	ProcessAttachment(data []byte, mimeType, filename, messageID string) error
}

type EmailPoller struct {
	imapSvc       *emailservice.IMAPService
	lineSvc       *lineservice.Service
	interval      time.Duration
	log           *zap.Logger
	stop          chan struct{}
	lastErrNotify time.Time // throttle LINE notify to once per hour
}

func NewEmailPoller(
	imapSvc *emailservice.IMAPService,
	lineSvc *lineservice.Service,
	processor AttachmentProcessor,
	interval time.Duration,
	log *zap.Logger,
) *EmailPoller {
	if processor != nil {
		imapSvc.SetProcessor(processor.ProcessAttachment)
	}
	return &EmailPoller{
		imapSvc:  imapSvc,
		lineSvc:  lineSvc,
		interval: interval,
		log:      log,
		stop:     make(chan struct{}),
	}
}

// Register starts the polling goroutine (uses ticker, not cron, for flexible intervals)
func (p *EmailPoller) Register(_ *cron.Cron) {
	go p.run()
}

func (p *EmailPoller) run() {
	// Poll immediately on start, then every interval
	p.poll()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.stop:
			return
		}
	}
}

func (p *EmailPoller) poll() {
	if err := p.imapSvc.Poll(); err != nil {
		p.log.Error("email poller: IMAP poll failed", zap.Error(err))
		// Notify LINE at most once per hour, and only if IMAP is actually configured
		if p.lineSvc != nil && p.imapSvc.IsConfigured() && time.Since(p.lastErrNotify) >= time.Hour {
			_ = p.lineSvc.PushAdmin("⚠️ BillFlow: IMAP connection failed\n" + err.Error())
			p.lastErrNotify = time.Now()
		}
	}
}

func (p *EmailPoller) Stop() {
	close(p.stop)
}
