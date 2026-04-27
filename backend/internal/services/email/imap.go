package emailservice

import (
	"bytes"
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
// messageID is the RFC 2822 Message-ID header, used for deduplication.
// subject and fromAddr come from the envelope so the handler can persist
// the email context in bills.raw_data for audit.
type AttachmentProcessor func(data []byte, mimeType, filename, messageID, subject, fromAddr string) error

// ShopeeBodyProcessor is called when an email from a Shopee domain is detected.
// subject, from = raw envelope values; bodyText = best available text from the email body.
type ShopeeBodyProcessor func(subject, from, bodyText, messageID string) error

// IMAPService polls email and processes attachments
type IMAPService struct {
	host               string
	port               int
	user               string
	password           string
	filterFrom         string
	filterSubjects     []string // lower-cased, parsed from comma-separated config
	processor          AttachmentProcessor
	shopeeProcessor    ShopeeBodyProcessor
	shippedProcessor   ShopeeBodyProcessor // Shopee shipping confirmation emails
	shopeeEmailDomains []string            // e.g. ["shopee.co.th","mail.shopee.co.th"]
	logger             *zap.Logger
}

func New(host string, port int, user, password, filterFrom, filterSubject string, logger *zap.Logger) *IMAPService {
	var subjects []string
	for _, s := range strings.Split(filterSubject, ",") {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			subjects = append(subjects, s)
		}
	}
	return &IMAPService{
		host:           host,
		port:           port,
		user:           user,
		password:       password,
		filterFrom:     filterFrom,
		filterSubjects: subjects,
		logger:         logger,
	}
}

// SetProcessor sets the callback invoked for every qualifying attachment
func (s *IMAPService) SetProcessor(fn AttachmentProcessor) {
	s.processor = fn
}

// SetShopeeProcessor sets the callback for Shopee order emails (HTML body)
func (s *IMAPService) SetShopeeProcessor(fn ShopeeBodyProcessor, domains []string) {
	s.shopeeProcessor = fn
	s.shopeeEmailDomains = domains
}

// SetShippedProcessor sets the callback for Shopee shipping confirmation emails
// (subject contains "ถูกจัดส่งแล้ว"). Routed before shopeeProcessor for Shopee-domain emails.
func (s *IMAPService) SetShippedProcessor(fn ShopeeBodyProcessor) {
	s.shippedProcessor = fn
}

// IsConfigured returns true if IMAP host and credentials are set
func (s *IMAPService) IsConfigured() bool {
	return s.host != "" && s.user != "" && s.password != ""
}

// Poll connects to IMAP, fetches matching emails, processes attachments, marks as read
func (s *IMAPService) Poll() error {
	if s.host == "" {
		return nil
	}

	traceID := imapNewTraceID()
	pollStart := time.Now()

	s.logger.Info("imap_poll_start",
		zap.String("trace_id", traceID),
		zap.String("account", s.user),
		zap.String("host", s.host),
	)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		s.logger.Error("imap_poll_failed",
			zap.String("trace_id", traceID),
			zap.String("stage", "connect"),
			zap.Error(err),
		)
		return fmt.Errorf("IMAP connect %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.Authenticate(sasl.NewPlainClient("", s.user, s.password)); err != nil {
		s.logger.Error("imap_poll_failed",
			zap.String("trace_id", traceID),
			zap.String("stage", "authenticate"),
			zap.Error(err),
		)
		return fmt.Errorf("IMAP authenticate: %w", err)
	}

	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		s.logger.Error("imap_poll_failed",
			zap.String("trace_id", traceID),
			zap.String("stage", "select_inbox"),
			zap.Error(err),
		)
		return fmt.Errorf("IMAP select INBOX: %w", err)
	}

	// Build search criteria: emails from last 30 days + optional From header filter.
	// Deduplication is handled by Message-ID stored in the bills table.
	since := time.Now().AddDate(0, 0, -30)
	criteria := &imap.SearchCriteria{
		Since: since,
	}
	if s.filterFrom != "" {
		criteria.Header = []imap.SearchCriteriaHeaderField{
			{Key: "From", Value: s.filterFrom},
		}
	}

	searchData, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		s.logger.Error("imap_poll_failed",
			zap.String("trace_id", traceID),
			zap.String("stage", "search"),
			zap.Error(err),
		)
		return fmt.Errorf("IMAP search: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		s.logger.Info("imap_poll_done",
			zap.String("trace_id", traceID),
			zap.Int("messages_found", 0),
			zap.Int("processed", 0),
			zap.Int64("duration_ms", time.Since(pollStart).Milliseconds()),
		)
		return nil
	}

	s.logger.Info("imap_messages_found",
		zap.String("trace_id", traceID),
		zap.Int("count", len(uids)),
	)

	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(uid)
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
	processedCount := 0
	skippedCount := 0

	for {
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
			skippedCount++
			continue
		}

		// Post-filter by subject
		if !s.matchesSubject(envelope.Subject) {
			s.logger.Info("imap_message_skipped",
				zap.String("trace_id", traceID),
				zap.String("subject", envelope.Subject),
				zap.String("reason", "subject_filter_mismatch"),
			)
			skippedCount++
			continue
		}

		messageID := ""
		if envelope != nil {
			messageID = envelope.MessageID
		}

		s.logger.Info("imap_message_received",
			zap.String("trace_id", traceID),
			zap.String("message_id", messageID),
			zap.String("subject", envelope.Subject),
		)

		fromAddr := ""
		if envelope != nil && len(envelope.From) > 0 {
			fromAddr = envelope.From[0].Addr()
		}

		// Shopee-domain emails route to one of two body processors based on subject
		if s.isShopeeEmail(fromAddr) {
			bodyText := extractBodyText(bodyBytes)

			// Shipping confirmation (purchase order flow) — match BEFORE order flow
			if s.shippedProcessor != nil && isShippedSubject(envelope.Subject) {
				if err := s.shippedProcessor(envelope.Subject, fromAddr, bodyText, messageID); err == nil && msgUID != 0 {
					processedUIDs.AddNum(msgUID)
					processedCount++
				} else if err != nil {
					s.logger.Warn("imap_shopee_shipped_failed",
						zap.String("trace_id", traceID),
						zap.String("message_id", messageID),
						zap.Error(err),
					)
					skippedCount++
				}
				continue
			}

			// Regular order email (saleinvoice flow)
			if s.shopeeProcessor != nil {
				if err := s.shopeeProcessor(envelope.Subject, fromAddr, bodyText, messageID); err == nil && msgUID != 0 {
					processedUIDs.AddNum(msgUID)
					processedCount++
				} else if err != nil {
					s.logger.Warn("imap_shopee_email_failed",
						zap.String("trace_id", traceID),
						zap.String("message_id", messageID),
						zap.Error(err),
					)
					skippedCount++
				}
				continue
			}
		}

		if err := s.parseAndProcess(bodyBytes, messageID, envelope.Subject, fromAddr); err == nil && msgUID != 0 {
			processedUIDs.AddNum(msgUID)
			processedCount++
		} else if err != nil {
			s.logger.Warn("imap_message_process_failed",
				zap.String("trace_id", traceID),
				zap.String("message_id", messageID),
				zap.Error(err),
			)
			skippedCount++
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return fmt.Errorf("IMAP fetch close: %w", err)
	}

	// Mark successfully processed messages as Seen to avoid re-processing
	if len(processedUIDs) > 0 {
		c.Store(processedUIDs, &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Flags:  []imap.Flag{imap.FlagSeen},
			Silent: true,
		}, nil).Close() //nolint:errcheck
		s.logger.Info("imap_mark_read",
			zap.String("trace_id", traceID),
			zap.Int("count", processedCount),
		)
	}

	s.logger.Info("imap_poll_done",
		zap.String("trace_id", traceID),
		zap.Int("messages_found", len(uids)),
		zap.Int("processed", processedCount),
		zap.Int("skipped", skippedCount),
		zap.Int64("duration_ms", time.Since(pollStart).Milliseconds()),
	)

	return nil
}

// isShippedSubject returns true if the subject indicates a Shopee shipping
// confirmation ("...ถูกจัดส่งแล้ว..."). Used to route between order vs shipped flows.
func isShippedSubject(subject string) bool {
	return strings.Contains(subject, "ถูกจัดส่งแล้ว")
}

// matchesSubject returns true if subject matches any configured keyword (or no filter set)
func (s *IMAPService) matchesSubject(subject string) bool {
	if len(s.filterSubjects) == 0 {
		return true
	}
	lower := strings.ToLower(subject)
	for _, kw := range s.filterSubjects {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// parseAndProcess extracts attachments from raw email bytes and calls s.processor.
// subject and fromAddr come from the envelope and are forwarded so the
// processor can persist email context for audit.
func (s *IMAPService) parseAndProcess(rawMsg []byte, messageID, subject, fromAddr string) error {
	mr, err := gomail.CreateReader(bytes.NewReader(rawMsg))
	if err != nil {
		return fmt.Errorf("parse email: %w", err)
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
			// PDF inline — treat as processable attachment
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

		if s.logger != nil {
			s.logger.Info("imap_attachment_parsed",
				zap.String("message_id", messageID),
				zap.String("filename", filename),
				zap.String("mime_type", mimeType),
				zap.Int("size_bytes", len(data)),
			)
		}

		if s.processor != nil {
			if err := s.processor(data, mimeType, filename, messageID, subject, fromAddr); err == nil {
				processed = true
			}
		}
	}

	if !processed {
		return fmt.Errorf("no qualifying attachments found")
	}
	return nil
}

// isSupportedAttachment returns true for PDF and image mimetypes/filenames
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

// isShopeeEmail returns true if the from address is a known Shopee domain
func (s *IMAPService) isShopeeEmail(from string) bool {
	from = strings.ToLower(from)
	for _, domain := range s.shopeeEmailDomains {
		if strings.HasSuffix(from, "@"+strings.ToLower(domain)) {
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
		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
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
