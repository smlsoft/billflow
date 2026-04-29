package emailservice

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
	"go.uber.org/zap"
)

// AttachmentProcessor is called once per qualifying attachment found in email.
type AttachmentProcessor func(data []byte, mimeType, filename, messageID, subject, fromAddr string) error

// ShopeeBodyProcessor is called when an email from a Shopee domain is detected.
type ShopeeBodyProcessor func(subject, from, bodyText, messageID string) error

// PollConfig holds everything one IMAP poll cycle needs.
// Re-built per cycle from the account's current DB state, so admin edits
// take effect on the next poll without restarting the goroutine.
type PollConfig struct {
	AccountID      string
	AccountName    string
	Host           string
	Port           int
	Username       string
	Password       string
	Mailbox        string // default "INBOX" if empty
	FilterFrom     string
	FilterSubjects []string // lower-cased keywords; empty = match all
	LookbackDays   int      // ≥1
	Channel        string   // "general" | "shopee" | "lazada"
	ShopeeDomains  []string // lower-cased; only consulted for channel="shopee"
}

// PollResult summarises one poll cycle. Either Err is non-nil or the counts
// describe what happened.
type PollResult struct {
	TraceID        string
	MessagesFound  int
	Processed      int
	Skipped        int
	Duration       time.Duration
	Err            error
	FailureStage   string // "connect" | "authenticate" | "select" | "search" | "" if ok
}

// Status returns a short tag suitable for `imap_accounts.last_poll_status`.
func (r *PollResult) Status() string {
	if r.Err == nil {
		return "ok"
	}
	switch r.FailureStage {
	case "":
		return "error"
	default:
		return r.FailureStage + "_failed"
	}
}

// PollOnce runs one search-and-process cycle against the supplied account.
// It does not own any goroutine — the caller (AccountPoller) loops on a ticker.
func PollOnce(ctx context.Context, cfg PollConfig, p *Processors, logger *zap.Logger) PollResult {
	res := PollResult{TraceID: imapNewTraceID()}
	pollStart := time.Now()
	defer func() { res.Duration = time.Since(pollStart) }()

	mailbox := cfg.Mailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	lookback := cfg.LookbackDays
	if lookback <= 0 {
		lookback = 30
	}

	logger.Info("imap_poll_start",
		zap.String("trace_id", res.TraceID),
		zap.String("account_id", cfg.AccountID),
		zap.String("account_name", cfg.AccountName),
		zap.String("host", cfg.Host),
		zap.String("user", cfg.Username),
		zap.String("mailbox", mailbox),
	)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		res.Err = fmt.Errorf("IMAP connect %s: %w", addr, err)
		res.FailureStage = "connect"
		logger.Error("imap_poll_failed", zap.String("trace_id", res.TraceID), zap.String("stage", "connect"), zap.Error(err))
		return res
	}
	defer c.Close()

	if err := c.Authenticate(sasl.NewPlainClient("", cfg.Username, cfg.Password)); err != nil {
		res.Err = fmt.Errorf("IMAP authenticate: %w", err)
		res.FailureStage = "auth"
		logger.Error("imap_poll_failed", zap.String("trace_id", res.TraceID), zap.String("stage", "authenticate"), zap.Error(err))
		return res
	}

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		res.Err = fmt.Errorf("IMAP select %s: %w", mailbox, err)
		res.FailureStage = "select"
		logger.Error("imap_poll_failed", zap.String("trace_id", res.TraceID), zap.String("stage", "select"), zap.Error(err))
		return res
	}

	since := time.Now().AddDate(0, 0, -lookback)
	criteria := &imap.SearchCriteria{Since: since}
	if cfg.FilterFrom != "" {
		criteria.Header = []imap.SearchCriteriaHeaderField{{Key: "From", Value: cfg.FilterFrom}}
	}

	searchData, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		res.Err = fmt.Errorf("IMAP search: %w", err)
		res.FailureStage = "search"
		logger.Error("imap_poll_failed", zap.String("trace_id", res.TraceID), zap.String("stage", "search"), zap.Error(err))
		return res
	}

	uids := searchData.AllUIDs()
	res.MessagesFound = len(uids)
	if len(uids) == 0 {
		logger.Info("imap_poll_done",
			zap.String("trace_id", res.TraceID),
			zap.Int("messages_found", 0),
		)
		return res
	}

	logger.Info("imap_messages_found", zap.String("trace_id", res.TraceID), zap.Int("count", len(uids)))

	var uidSet imap.UIDSet
	for _, u := range uids {
		uidSet.AddNum(u)
	}

	bodySection := &imap.FetchItemBodySection{}
	fetchOptions := &imap.FetchOptions{
		UID:         true,
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}
	fetchCmd := c.Fetch(uidSet, fetchOptions)
	defer fetchCmd.Close()

	var processedUIDs imap.UIDSet

	for {
		// Cancel mid-fetch if context was cancelled (e.g. account removed).
		select {
		case <-ctx.Done():
			res.Err = ctx.Err()
			return res
		default:
		}

		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		var msgUID imap.UID
		var envelope *imap.Envelope
		var bodyBytes []byte
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			switch v := item.(type) {
			case imapclient.FetchItemDataUID:
				msgUID = v.UID
			case imapclient.FetchItemDataEnvelope:
				envelope = v.Envelope
			case imapclient.FetchItemDataBodySection:
				bodyBytes, _ = io.ReadAll(v.Literal)
			}
		}

		if envelope == nil || len(bodyBytes) == 0 {
			res.Skipped++
			continue
		}

		if !matchesSubject(envelope.Subject, cfg.FilterSubjects) {
			logger.Info("imap_message_skipped",
				zap.String("trace_id", res.TraceID),
				zap.String("subject", envelope.Subject),
				zap.String("reason", "subject_filter_mismatch"),
			)
			res.Skipped++
			continue
		}

		messageID := envelope.MessageID
		fromAddr := ""
		if len(envelope.From) > 0 {
			fromAddr = envelope.From[0].Addr()
		}

		logger.Info("imap_message_received",
			zap.String("trace_id", res.TraceID),
			zap.String("message_id", messageID),
			zap.String("subject", envelope.Subject),
		)

		ok := dispatch(cfg, p, envelope, fromAddr, bodyBytes, messageID, logger, res.TraceID)
		if ok && msgUID != 0 {
			processedUIDs.AddNum(msgUID)
			res.Processed++
		} else {
			res.Skipped++
		}
	}

	if err := fetchCmd.Close(); err != nil {
		res.Err = fmt.Errorf("IMAP fetch close: %w", err)
		res.FailureStage = "fetch"
		return res
	}

	if len(processedUIDs) > 0 {
		c.Store(processedUIDs, &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Flags:  []imap.Flag{imap.FlagSeen},
			Silent: true,
		}, nil).Close() //nolint:errcheck
		logger.Info("imap_mark_read", zap.String("trace_id", res.TraceID), zap.Int("count", res.Processed))
	}

	logger.Info("imap_poll_done",
		zap.String("trace_id", res.TraceID),
		zap.String("account_id", cfg.AccountID),
		zap.Int("messages_found", res.MessagesFound),
		zap.Int("processed", res.Processed),
		zap.Int("skipped", res.Skipped),
		zap.Int64("duration_ms", time.Since(pollStart).Milliseconds()),
	)

	return res
}

// dispatch routes one fetched message to the right Processor based on
// account channel + Shopee subject heuristics. Returns true if any
// processor accepted the message (so it can be marked Seen).
func dispatch(
	cfg PollConfig,
	p *Processors,
	envelope *imap.Envelope,
	fromAddr string,
	bodyBytes []byte,
	messageID string,
	logger *zap.Logger,
	traceID string,
) bool {
	if p == nil {
		return false
	}

	switch cfg.Channel {
	case "shopee":
		// Only honor Shopee handlers when the From address is on the
		// configured Shopee domain list — guards against test imports
		// from non-Shopee senders polluting the bill stream.
		if !isShopeeFrom(fromAddr, cfg.ShopeeDomains) {
			logger.Info("imap_message_skipped",
				zap.String("trace_id", traceID), zap.String("from", fromAddr),
				zap.String("reason", "shopee_channel_non_shopee_from"),
			)
			return false
		}
		bodyText := extractBodyText(bodyBytes)
		if isShippedSubject(envelope.Subject) && p.ShopeeShipped != nil {
			if err := p.ShopeeShipped(envelope.Subject, fromAddr, bodyText, messageID); err != nil {
				logger.Warn("imap_shopee_shipped_failed",
					zap.String("trace_id", traceID), zap.String("message_id", messageID), zap.Error(err))
				return false
			}
			return true
		}
		if p.ShopeeOrder != nil {
			if err := p.ShopeeOrder(envelope.Subject, fromAddr, bodyText, messageID); err != nil {
				logger.Warn("imap_shopee_order_failed",
					zap.String("trace_id", traceID), zap.String("message_id", messageID), zap.Error(err))
				return false
			}
			return true
		}
		return false

	default:
		// general / lazada → attachment pipeline
		if p.Attachment == nil {
			return false
		}
		return parseAndProcess(bodyBytes, messageID, envelope.Subject, fromAddr, p.Attachment, logger, traceID)
	}
}

// matchesSubject is true if any keyword is contained in the subject (case-insensitive).
// Empty filter list = match everything.
func matchesSubject(subject string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	lower := strings.ToLower(subject)
	for _, kw := range filters {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isShippedSubject returns true if the subject indicates a Shopee
// payment-or-shipping confirmation — both should produce a purchase-order
// bill in SML. The two channels Shopee uses are:
//   - เก็บเงินปลายทาง (cash on delivery): subject contains "ถูกจัดส่งแล้ว"
//     when the package ships
//   - ชำระเงินทันที (pay now): subject contains "ยืนยันการชำระเงิน"
//     when the buyer pays — this is the equivalent trigger for COD-shipped
func isShippedSubject(subject string) bool {
	return strings.Contains(subject, "ถูกจัดส่งแล้ว") ||
		strings.Contains(subject, "ยืนยันการชำระเงิน")
}

// isShopeeFrom returns true if the from address matches any of the configured
// entries. Each entry may be either:
//   - a domain like "shopee.co.th" → matches if from ends with "@shopee.co.th"
//   - a full email like "user@example.com" → matches the exact address (used
//     for forwarded mail where a single forwarder relays Shopee notifications
//     into the bot's inbox under their own gmail address)
func isShopeeFrom(from string, domains []string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" {
		return false
	}
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		// Full email entry → exact match
		if strings.Contains(d, "@") {
			if from == d {
				return true
			}
			continue
		}
		// Domain entry → suffix match against @<domain>
		if strings.HasSuffix(from, "@"+d) {
			return true
		}
	}
	return false
}

// parseAndProcess extracts qualifying attachments from raw email bytes and
// fans them out to the AttachmentProcessor. Returns true if at least one
// attachment was processed successfully.
func parseAndProcess(
	rawMsg []byte,
	messageID, subject, fromAddr string,
	processor AttachmentProcessor,
	logger *zap.Logger,
	traceID string,
) bool {
	mr, err := gomail.CreateReader(bytes.NewReader(rawMsg))
	if err != nil {
		logger.Warn("imap_message_parse_failed",
			zap.String("trace_id", traceID), zap.String("message_id", messageID), zap.Error(err))
		return false
	}

	processed := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		var filename, mimeType string
		switch h := part.Header.(type) {
		case *gomail.AttachmentHeader:
			filename, _ = h.Filename()
			mimeType, _, _ = h.ContentType()
		case *gomail.InlineHeader:
			mimeType, _, _ = h.ContentType()
		default:
			continue
		}

		mimeType = strings.ToLower(strings.Split(mimeType, ";")[0])
		if !isSupportedAttachment(mimeType, filename) {
			continue
		}

		data, err := io.ReadAll(part.Body)
		if err != nil || len(data) == 0 {
			continue
		}

		logger.Info("imap_attachment_parsed",
			zap.String("trace_id", traceID),
			zap.String("message_id", messageID),
			zap.String("filename", filename),
			zap.String("mime_type", mimeType),
			zap.Int("size_bytes", len(data)),
		)

		if err := processor(data, mimeType, filename, messageID, subject, fromAddr); err == nil {
			processed = true
		} else {
			logger.Warn("imap_attachment_process_failed",
				zap.String("trace_id", traceID), zap.String("message_id", messageID), zap.Error(err))
		}
	}

	return processed
}

func isSupportedAttachment(mimeType, filename string) bool {
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	if mimeType == "application/pdf" {
		return true
	}
	lower := strings.ToLower(filename)
	for _, ext := range []string{".pdf", ".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// extractBodyText pulls the best text content from a raw RFC 2822 message.
// Tries text/html first (richer content), falls back to text/plain.
func extractBodyText(rawMsg []byte) string {
	mr, err := gomail.CreateReader(bytes.NewReader(rawMsg))
	if err != nil {
		return ""
	}

	var htmlBody, plainBody string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if h, ok := part.Header.(*gomail.InlineHeader); ok {
			mimeType, _, _ := h.ContentType()
			mimeType = strings.ToLower(strings.Split(mimeType, ";")[0])
			data, err := io.ReadAll(part.Body)
			if err != nil || len(data) == 0 {
				continue
			}
			switch mimeType {
			case "text/html":
				if htmlBody == "" {
					htmlBody = string(data)
				}
			case "text/plain":
				if plainBody == "" {
					plainBody = string(data)
				}
			}
		}
	}

	if htmlBody != "" {
		return htmlBody
	}
	return plainBody
}

// imapNewTraceID generates a random 12-char hex trace ID for poll cycles.
func imapNewTraceID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
