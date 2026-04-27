package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/anomaly"
	lineservice "billflow/internal/services/line"
	"billflow/internal/services/mapper"
	"billflow/internal/services/mistral"
	"billflow/internal/services/sml"
	"billflow/internal/worker"
)

// ── Conversation session store (PostgreSQL-backed) ───────────────────────────

// pendingOrderItem holds one resolved line item awaiting user confirmation
type pendingOrderItem struct {
	RawName  string
	ItemCode string
	UnitCode string
	Qty      float64
	Price    float64
	Name     string // product name from SML
}

// pendingOrderData is a resolved order waiting for user to confirm or cancel
type pendingOrderData struct {
	CustomerName  string
	CustomerPhone string
	Items         []pendingOrderItem
}

// convStore wraps ChatSessionRepo with helper methods used by the handler.
// All writes go through to the DB immediately so sessions survive restarts.
// Errors are silently swallowed to avoid disrupting the webhook flow; callers
// treat a missing session the same as an empty one.
type convStore struct {
	repo   *repository.ChatSessionRepo
	logger *zap.Logger
}

func newConvStore(repo *repository.ChatSessionRepo, logger *zap.Logger) *convStore {
	cs := &convStore{repo: repo, logger: logger}
	// Cleanup goroutine: prune sessions idle > 24 hours
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := cs.repo.PruneIdle(24 * time.Hour); err != nil && cs.logger != nil {
				cs.logger.Warn("convStore: prune idle sessions", zap.Error(err))
			}
		}
	}()
	return cs
}

func (cs *convStore) get(userID string) []ai.ChatMessage {
	rec, err := cs.repo.Get(userID)
	if err != nil {
		if cs.logger != nil {
			cs.logger.Warn("convStore.get", zap.String("user", userID), zap.Error(err))
		}
		return nil
	}
	if rec == nil {
		return nil
	}
	var msgs []ai.ChatMessage
	_ = json.Unmarshal(rec.History, &msgs)
	return msgs
}

func (cs *convStore) add(userID, userMsg, assistantReply string) {
	msgs := cs.get(userID)
	msgs = append(msgs, ai.ChatMessage{Role: "user", Content: userMsg})
	msgs = append(msgs, ai.ChatMessage{Role: "assistant", Content: assistantReply})
	// Keep last 10 turns (20 messages)
	if len(msgs) > 20 {
		msgs = msgs[len(msgs)-20:]
	}
	cs.upsertHistory(userID, msgs)
}

func (cs *convStore) clear(userID string) {
	if err := cs.repo.Delete(userID); err != nil && cs.logger != nil {
		cs.logger.Warn("convStore.clear", zap.String("user", userID), zap.Error(err))
	}
}

func (cs *convStore) setPending(userID string, order *pendingOrderData) {
	rec, _ := cs.repo.Get(userID)
	if rec == nil {
		rec = &repository.ChatSessionRecord{LineUserID: userID, History: []byte("[]")}
	}
	pendingRaw, _ := json.Marshal(order)
	rec.PendingOrder = pendingRaw
	rec.LastActive = time.Now()
	if err := cs.repo.Upsert(rec); err != nil && cs.logger != nil {
		cs.logger.Warn("convStore.setPending", zap.String("user", userID), zap.Error(err))
	}
}

func (cs *convStore) getPending(userID string) *pendingOrderData {
	rec, err := cs.repo.Get(userID)
	if err != nil || rec == nil || len(rec.PendingOrder) == 0 {
		return nil
	}
	var order pendingOrderData
	if err := json.Unmarshal(rec.PendingOrder, &order); err != nil {
		return nil
	}
	return &order
}

func (cs *convStore) clearPending(userID string) {
	rec, _ := cs.repo.Get(userID)
	if rec == nil {
		return
	}
	rec.PendingOrder = nil
	rec.LastActive = time.Now()
	if err := cs.repo.Upsert(rec); err != nil && cs.logger != nil {
		cs.logger.Warn("convStore.clearPending", zap.String("user", userID), zap.Error(err))
	}
}

// upsertHistory writes updated history back to the DB.
func (cs *convStore) upsertHistory(userID string, msgs []ai.ChatMessage) {
	histRaw, _ := json.Marshal(msgs)
	rec, _ := cs.repo.Get(userID)
	if rec == nil {
		rec = &repository.ChatSessionRecord{LineUserID: userID}
	}
	rec.History = histRaw
	rec.LastActive = time.Now()
	if err := cs.repo.Upsert(rec); err != nil && cs.logger != nil {
		cs.logger.Warn("convStore.upsertHistory", zap.String("user", userID), zap.Error(err))
	}
}

// LineHandler handles LINE webhook events
type LineHandler struct {
	lineSvc    *lineservice.Service
	aiClient   *ai.Client
	ocrClient  *mistral.OCRClient
	mapperSvc  *mapper.Service
	anomalySvc *anomaly.Service
	smlClient  *sml.Client
	mcpClient  *sml.MCPClient
	billRepo   *repository.BillRepo
	auditRepo  *repository.AuditLogRepo
	pool       *worker.Pool
	threshold  float64
	conv       *convStore
	logger     *zap.Logger
}

func NewLineHandler(
	lineSvc *lineservice.Service,
	aiClient *ai.Client,
	ocrClient *mistral.OCRClient,
	mapperSvc *mapper.Service,
	anomalySvc *anomaly.Service,
	smlClient *sml.Client,
	mcpClient *sml.MCPClient,
	billRepo *repository.BillRepo,
	auditRepo *repository.AuditLogRepo,
	chatRepo *repository.ChatSessionRepo,
	pool *worker.Pool,
	threshold float64,
	logger *zap.Logger,
) *LineHandler {
	return &LineHandler{
		lineSvc:    lineSvc,
		aiClient:   aiClient,
		ocrClient:  ocrClient,
		mapperSvc:  mapperSvc,
		anomalySvc: anomalySvc,
		smlClient:  smlClient,
		mcpClient:  mcpClient,
		billRepo:   billRepo,
		auditRepo:  auditRepo,
		pool:       pool,
		threshold:  threshold,
		conv:       newConvStore(chatRepo, logger),
		logger:     logger,
	}
}

// Cart edit regex patterns
var (
	reDeleteItem = regexp.MustCompile(`(?:ลบ|เอาออก).*?(\d+)`)
	reEditQty    = regexp.MustCompile(`(?:แก้|แก้ไข|เปลี่ยน).*?(\d+).*?เป็น.*?(\d+(?:\.\d+)?)`)
)

func parseDeleteItem(text string) (int, bool) {
	m := reDeleteItem.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseEditQty(text string) (int, float64, bool) {
	m := reEditQty.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(m[1])
	q, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil || q <= 0 {
		return 0, 0, false
	}
	return n, q, true
}

// Product inquiry regex: "มีปูนอะไรบ้าง", "ขายเหล็กไหม", "หาปูนซีเมนต์บ้าง"
var reInquiry = regexp.MustCompile(`(?:มี|ขาย|หา|อยากได้|อยากซื้อ)(.+?)(?:ไหม|มั้ย|อะไรบ้าง|บ้าง)`)

// detectInquiry checks if the message is a product inquiry and returns the search keyword.
func detectInquiry(text string) (keyword string, isInquiry bool) {
	m := reInquiry.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	kw := strings.TrimSpace(m[1])
	if kw == "" || len([]rune(kw)) > 30 {
		return "", false
	}
	return kw, true
}

// ── Minimal webhook structs (avoids SDK internal type complexity) ────────────

type linePayload struct {
	Destination string      `json:"destination"`
	Events      []lineEvent `json:"events"`
}

type lineEvent struct {
	Type       string        `json:"type"`
	Timestamp  int64         `json:"timestamp"`
	ReplyToken string        `json:"replyToken"`
	Source     lineSource    `json:"source"`
	Message    *lineMessage  `json:"message,omitempty"`
	Postback   *linePostback `json:"postback,omitempty"`
}

type lineSource struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
}

type lineMessage struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Text     string `json:"text,omitempty"`
	Duration int    `json:"duration,omitempty"` // ms for audio
	FileName string `json:"fileName,omitempty"`
}

type linePostback struct {
	Data string `json:"data"`
}

// ── Webhook handler ──────────────────────────────────────────────────────────

// POST /webhook/line
func (h *LineHandler) Webhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// Verify X-Line-Signature (skip when LINE not configured)
	if h.lineSvc != nil {
		sig := c.GetHeader("X-Line-Signature")
		if sig == "" || !h.lineSvc.ValidateSignature(body, sig) {
			h.logger.Warn("invalid LINE signature")
			c.Status(http.StatusBadRequest)
			return
		}
	}

	// Respond 200 immediately — LINE requires < 1 second
	c.Status(http.StatusOK)

	if h.lineSvc == nil {
		return
	}

	var payload linePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("parse LINE webhook", zap.Error(err))
		return
	}

	for _, event := range payload.Events {
		ev := event // capture
		h.pool.Submit(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			h.processEvent(ctx, ev)
		})
	}
}

// ── Event routing ─────────────────────────────────────────────────────────────

func (h *LineHandler) processEvent(ctx context.Context, event lineEvent) {
	switch event.Type {
	case "message":
		if event.Message != nil {
			h.processMessage(ctx, event)
		}
	case "postback":
		if event.Postback != nil {
			h.processPostback(ctx, event)
		}
	}
	// follow, unfollow, join, leave — silently ignore
}

// ── Message processing ────────────────────────────────────────────────────────

func (h *LineHandler) processMessage(ctx context.Context, event lineEvent) {
	msg := event.Message
	var extracted *ai.ExtractedBill
	var fromVoice bool

	switch msg.Type {
	case "text":
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			return
		}
		userID := event.Source.UserID

		// Check if user is confirming / cancelling a pending order
		if pending := h.conv.getPending(userID); pending != nil {
			lower := strings.ToLower(text)
			switch {
			case strings.Contains(lower, "ยืนยัน") || lower == "confirm" || lower == "ok" || lower == "yes" ||
				strings.Contains(lower, "โอเค") || lower == "ตกลง" || lower == "เอาเลย" || lower == "ได้เลย":
				// Block if any item is missing its SML item code
				if missing := pendingMissingCodes(pending); len(missing) > 0 {
					msg := "⚠️ ยังไม่พบรหัสสินค้าต่อไปนี้ในระบบ SML:\n"
					for _, m := range missing {
						msg += "• " + m + "\n"
					}
					msg += "\nกรุณาแจ้ง admin เพื่อเพิ่มรหัสสินค้า หรือพิมพ์ ❌ยกเลิก เพื่อยกเลิกออเดอร์"
					h.lineSvc.ReplyText(event.ReplyToken, msg)
					return
				}
				h.conv.clearPending(userID)
				h.submitPendingOrder(ctx, event, pending)
				return
			case strings.Contains(lower, "ยกเลิก") || lower == "cancel" || lower == "no":
				h.conv.clearPending(userID)
				h.lineSvc.ReplyText(event.ReplyToken, "ยกเลิกออเดอร์แล้วครับ 👍")
				return
			}

			// Cart edit: ลบรายการที่ N
			if itemNum, ok := parseDeleteItem(lower); ok {
				if itemNum >= 1 && itemNum <= len(pending.Items) {
					name := pending.Items[itemNum-1].RawName
					pending.Items = append(pending.Items[:itemNum-1], pending.Items[itemNum:]...)
					if len(pending.Items) == 0 {
						h.conv.clearPending(userID)
						h.lineSvc.ReplyText(event.ReplyToken, "ลบ \""+name+"\" แล้ว\n📦 ตะกร้าว่างแล้ว — พิมพ์สั่งสินค้าใหม่ได้เลยครับ")
					} else {
						h.lineSvc.ReplyText(event.ReplyToken, "ลบ \""+name+"\" แล้ว ✅\n\n"+buildPendingSummary(pending)+"\n\nพิมพ์ ✅ยืนยัน หรือ ❌ยกเลิก")
					}
				} else {
					h.lineSvc.ReplyText(event.ReplyToken, fmt.Sprintf("ไม่มีรายการที่ %d ในตะกร้าครับ (มีทั้งหมด %d รายการ)", itemNum, len(pending.Items)))
				}
				return
			}

			// Cart edit: แก้จำนวนรายการที่ N เป็น Y
			if itemNum, qty, ok := parseEditQty(lower); ok {
				if itemNum >= 1 && itemNum <= len(pending.Items) {
					pending.Items[itemNum-1].Qty = qty
					h.lineSvc.ReplyText(event.ReplyToken, fmt.Sprintf("แก้ไขรายการที่ %d เป็น %.0f แล้ว ✅\n\n", itemNum, qty)+buildPendingSummary(pending)+"\n\nพิมพ์ ✅ยืนยัน หรือ ❌ยกเลิก")
				} else {
					h.lineSvc.ReplyText(event.ReplyToken, fmt.Sprintf("ไม่มีรายการที่ %d ในตะกร้าครับ (มีทั้งหมด %d รายการ)", itemNum, len(pending.Items)))
				}
				return
			}

			// Any other input while pending — re-show summary with edit hint with edit hint
			h.lineSvc.ReplyText(event.ReplyToken, buildPendingSummary(pending)+"\n\nพิมพ์ ✅ยืนยัน เพื่อบันทึก หรือ ❌ยกเลิก\n🗑 ลบ: 'ลบรายการที่ 1'\n✏️ แก้จำนวน: 'แก้รายการที่ 1 เป็น 5'")
			return
		}

		// Chatbot: conversational sales AI with session history
		history := h.conv.get(userID)

		// Phase D: Detect product inquiry ("มีปูนอะไรบ้าง" etc.) and inject catalog context
		var result *ai.SalesChatResult
		var chatErr error
		if keyword, isInquiry := detectInquiry(text); isInquiry && h.mcpClient != nil {
			mcpResults, srchErr := h.mcpClient.SearchProduct(keyword, 5)
			if srchErr == nil && len(mcpResults) > 0 {
				var catSB strings.Builder
				for i, r := range mcpResults {
					unit := r.UnitStandard
					if unit == "" {
						unit = "-"
					}
					catSB.WriteString(fmt.Sprintf("%d. %s [%s] หน่วย: %s\n", i+1, r.Name, r.Code, unit))
				}
				result, chatErr = h.aiClient.ChatSalesWithContext(history, text, catSB.String())
			}
		}
		if result == nil {
			result, chatErr = h.aiClient.ChatSales(history, text)
		}
		if chatErr != nil {
			h.logger.Error("ChatSales", zap.Error(chatErr))
			h.replyErr(event.ReplyToken, "ขอโทษ ระบบขัดข้องชั่วคราว กรุณาลองใหม่อีกครั้งนะครับ")
			return
		}

		// Phase B: Fallback — AI said "กำลังบันทึก" but omitted <BILL> tag
		if result.Order == nil && (strings.Contains(result.Reply, "กำลังบันทึก") || strings.Contains(result.Reply, "บันทึกรายการ")) {
			allHistory := append(history,
				ai.ChatMessage{Role: "user", Content: text},
				ai.ChatMessage{Role: "assistant", Content: result.Reply},
			)
			if extracted, ferr := h.aiClient.ExtractOrderFromHistory(allHistory); ferr == nil && extracted != nil && len(extracted.Items) > 0 {
				result.Order = extracted
			} else {
				h.conv.add(userID, text, result.Reply)
				h.lineSvc.ReplyText(event.ReplyToken, result.Reply+"\n\n⚠️ กรุณาพิมพ์รายการอีกครั้ง เช่น\n'สั่งปูนฉาบ 10 ถุง'")
				return
			}
		}

		if result.Order != nil {
			// AI extracted a complete order — do MCP product lookup before confirming
			h.conv.clear(userID)
			pending, lookupErr := h.buildPendingFromOrder(result.Order)
			if lookupErr != nil {
				h.logger.Warn("MCP product lookup", zap.Error(lookupErr))
				// Fall back: send directly to SML without confirm step
				h.handleExtractedWithUserID(ctx, event, result.Order, userID)
				return
			}
			h.conv.setPending(userID, pending)
			summary := buildPendingSummary(pending)
			h.lineSvc.ReplyText(event.ReplyToken, summary+"\n\nพิมพ์ ✅ยืนยัน เพื่อบันทึก หรือ ❌ยกเลิก")
			return
		}

		// Normal chat — save to history and reply
		h.conv.add(userID, text, result.Reply)
		if err := h.lineSvc.ReplyText(event.ReplyToken, result.Reply); err != nil {
			h.logger.Warn("reply chat", zap.Error(err))
		}
		return

	case "image":
		data, mimeType, err := h.lineSvc.DownloadContent(msg.ID)
		if err != nil {
			h.logger.Error("download image", zap.Error(err))
			return
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		extracted, err = h.aiClient.ExtractImage(b64, mimeType)
		if err != nil {
			h.logger.Error("AI extract image", zap.Error(err))
			h.replyErr(event.ReplyToken, "ขอโทษ ไม่สามารถอ่านรูปภาพได้ กรุณาลองใหม่")
			return
		}

	case "file":
		data, mimeType, err := h.lineSvc.DownloadContent(msg.ID)
		if err != nil {
			h.logger.Error("download file", zap.Error(err))
			return
		}
		if !strings.Contains(strings.ToLower(mimeType), "pdf") {
			h.lineSvc.ReplyText(event.ReplyToken, "รองรับเฉพาะไฟล์ PDF ครับ")
			return
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		if h.ocrClient != nil && h.ocrClient.IsConfigured() {
			ocrText, ocrErr := h.ocrClient.ExtractTextFromPDF(b64)
			if ocrErr != nil {
				h.logger.Error("Mistral OCR PDF", zap.Error(ocrErr))
				h.replyErr(event.ReplyToken, "ขอโทษ ไม่สามารถอ่าน PDF ได้ กรุณาลองใหม่")
				return
			}
			extracted, err = h.aiClient.ExtractText(ocrText)
		} else {
			extracted, err = h.aiClient.ExtractPDF(b64)
		}
		if err != nil {
			h.logger.Error("AI extract PDF", zap.Error(err))
			h.replyErr(event.ReplyToken, "ขอโทษ ไม่สามารถอ่าน PDF ได้ กรุณาลองใหม่")
			return
		}

	case "audio":
		// F3: Voice input — max 60 seconds
		if msg.Duration > 60_000 {
			h.lineSvc.ReplyText(event.ReplyToken, "กรุณาส่ง voice message ที่สั้นกว่า 60 วินาทีครับ")
			return
		}
		data, _, err := h.lineSvc.DownloadContent(msg.ID)
		if err != nil {
			h.logger.Error("download audio", zap.Error(err))
			return
		}
		text, err := h.aiClient.TranscribeAudio(data)
		if err != nil {
			h.logger.Error("transcribe audio", zap.Error(err))
			h.replyErr(event.ReplyToken, "ขอโทษ ไม่สามารถแปลง voice เป็นข้อความได้")
			return
		}
		extracted, err = h.aiClient.ExtractText(text)
		if err != nil {
			h.logger.Error("AI extract voice text", zap.Error(err))
			h.replyErr(event.ReplyToken, "ขอโทษ ระบบ AI ขัดข้อง กรุณาลองใหม่")
			return
		}
		fromVoice = true

	default:
		// sticker, location, video — silently ignore
		return
	}

	if extracted == nil {
		return
	}

	// F3: voice reduces confidence by 0.1
	if fromVoice {
		extracted.Confidence -= 0.1
		if extracted.Confidence < 0 {
			extracted.Confidence = 0
		}
	}

	if len(extracted.Items) == 0 {
		// Only guide text messages (avoid spamming for every sticker)
		if msg.Type == "text" {
			h.lineSvc.ReplyText(event.ReplyToken,
				"ไม่พบรายการสินค้าในข้อความครับ กรุณาส่งใบสั่งซื้อหรือระบุรายการสินค้าที่ต้องการ")
		}
		return
	}

	h.handleExtracted(ctx, event, extracted)
}

// handleExtractedWithUserID is like handleExtracted but pushes success/preview to userID instead of replying
func (h *LineHandler) handleExtractedWithUserID(ctx context.Context, event lineEvent, extracted *ai.ExtractedBill, userID string) {
	// Create a synthetic event where the source userID is known
	event.Source.UserID = userID
	h.handleExtracted(ctx, event, extracted)
}

// ── Pending Order helpers ─────────────────────────────────────────────────────

// buildPendingFromOrder uses MCP to look up product codes and prices for each item,
// then returns a pendingOrderData ready for user confirmation.
func (h *LineHandler) buildPendingFromOrder(order *ai.ExtractedBill) (*pendingOrderData, error) {
	pending := &pendingOrderData{
		CustomerName: order.CustomerName,
	}
	if order.CustomerPhone != nil {
		pending.CustomerPhone = *order.CustomerPhone
	}

	for _, extItem := range order.Items {
		pItem := pendingOrderItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Name:    extItem.RawName, // fallback to raw name
		}

		// Try mapper first (exact / fuzzy match from DB)
		match := h.mapperSvc.Match(extItem.RawName)
		if match.Mapping != nil && !match.NeedsReview {
			pItem.ItemCode = match.Mapping.ItemCode
			pItem.UnitCode = match.Mapping.UnitCode
		} else if h.mcpClient != nil {
			// MCP product search
			results, err := h.mcpClient.SearchProduct(extItem.RawName, 3)
			if err == nil && len(results) > 0 {
				best := results[0]
				pItem.ItemCode = best.Code
				pItem.Name = best.Name
				pItem.UnitCode = best.UnitStandard

				// Get price from MCP
				priceEntry, err := h.mcpClient.GetProductPrice(best.Code)
				if err == nil && priceEntry != nil {
					pItem.Price = priceEntry.Price
					if pItem.UnitCode == "" {
						pItem.UnitCode = priceEntry.UnitCode
					}
				}
			}
		}

		// Use AI-extracted price as override if provided
		if extItem.Price != nil && *extItem.Price > 0 {
			pItem.Price = *extItem.Price
		}

		pending.Items = append(pending.Items, pItem)
	}
	return pending, nil
}

// submitPendingOrder runs the confirmed pending order through the full pipeline and sends to SML.
func (h *LineHandler) submitPendingOrder(ctx context.Context, event lineEvent, pending *pendingOrderData) {
	var smlItems []sml.SMLItem
	var billItems []models.BillItem

	for _, it := range pending.Items {
		smlItems = append(smlItems, sml.SMLItem{
			ItemCode: it.ItemCode,
			Qty:      it.Qty,
			UnitCode: it.UnitCode,
			Price:    it.Price,
		})
		price := it.Price
		billItems = append(billItems, models.BillItem{
			RawName:  it.RawName,
			ItemCode: &it.ItemCode,
			Qty:      it.Qty,
			UnitCode: &it.UnitCode,
			Price:    &price,
			Mapped:   it.ItemCode != "" && it.ItemCode != it.RawName,
		})
	}

	// Save bill
	rawDataBytes, _ := json.Marshal(map[string]interface{}{
		"customer_name":  pending.CustomerName,
		"customer_phone": pending.CustomerPhone,
		"note":           "",
	})
	conf := 1.0
	bill := &models.Bill{
		BillType:     "sale",
		Source:       "line",
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
	}
	if err := h.billRepo.Create(bill); err != nil {
		h.logger.Error("submit pending: create bill", zap.Error(err))
		h.lineSvc.ReplyText(event.ReplyToken, "ขอโทษ ระบบขัดข้อง กรุณาลองใหม่")
		return
	}

	// Audit: bill created from LINE chatbot
	if h.auditRepo != nil {
		billIDStr := bill.ID
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:   "bill_created",
			TargetID: &billIDStr,
			Source:   "line",
			Level:    "info",
			TraceID:  bill.ID,
			Detail: map[string]interface{}{
				"customer_name": pending.CustomerName,
				"items_count":   len(pending.Items),
				"raw_data":      json.RawMessage(rawDataBytes),
			},
		})
	}

	for i := range billItems {
		billItems[i].BillID = bill.ID
		h.billRepo.InsertItem(&billItems[i])
	}

	customerPhone := &pending.CustomerPhone
	h.sendToSML(ctx, bill.ID, event.ReplyToken, event.Source.UserID,
		pending.CustomerName, customerPhone, smlItems, len(smlItems))
}

// buildPendingSummary formats a human-readable order summary in Thai
func buildPendingSummary(p *pendingOrderData) string {
	var sb strings.Builder
	sb.WriteString("📋 สรุปใบสั่งจอง\n")
	sb.WriteString("─────────────────\n")
	total := 0.0
	hasMissing := false
	for i, it := range p.Items {
		name := it.Name
		if name == "" {
			name = it.RawName
		}
		unit := it.UnitCode
		if unit == "" {
			unit = "หน่วย"
		}
		if it.ItemCode == "" {
			// Item not found in SML catalog
			hasMissing = true
			sb.WriteString(fmt.Sprintf("%d. %s × %.0f %s\n   ⚠️ ไม่พบรหัสสินค้าในระบบ\n",
				i+1, name, it.Qty, unit))
		} else {
			lineTotal := it.Price * it.Qty
			total += lineTotal
			priceStr := ""
			if it.Price > 0 {
				priceStr = fmt.Sprintf(" = %.0f บาท", lineTotal)
			} else {
				priceStr = " (ราคาไม่ระบุ)"
			}
			sb.WriteString(fmt.Sprintf("%d. %s [%s] × %.0f %s%s\n",
				i+1, name, it.ItemCode, it.Qty, unit, priceStr))
		}
	}
	sb.WriteString("─────────────────\n")
	if total > 0 {
		sb.WriteString(fmt.Sprintf("💰 รวม: %.0f บาท", total))
	} else {
		sb.WriteString("💰 รวม: (ราคาไม่ระบุ)")
	}
	if hasMissing {
		sb.WriteString("\n\n⚠️ มีสินค้าที่ไม่พบรหัสในระบบ SML\nกรุณาแจ้ง admin เพื่อเพิ่มรหัสสินค้าก่อนยืนยัน")
	}
	return sb.String()
}

// pendingMissingCodes returns raw names of items that have no SML item code
func pendingMissingCodes(p *pendingOrderData) []string {
	var missing []string
	for _, it := range p.Items {
		if it.ItemCode == "" {
			missing = append(missing, it.RawName)
		}
	}
	return missing
}

// ── AI pipeline: map → anomaly → auto-confirm or preview ─────────────────────

func (h *LineHandler) handleExtracted(ctx context.Context, event lineEvent, extracted *ai.ExtractedBill) {
	// Fallback: distribute total_amount to items that have no per-unit price
	if extracted.TotalAmount != nil && *extracted.TotalAmount > 0 {
		var noPriceQtySum float64
		var priceAlreadyKnown float64
		for _, it := range extracted.Items {
			if it.Price == nil {
				noPriceQtySum += it.Qty
			} else {
				priceAlreadyKnown += *it.Price * it.Qty
			}
		}
		if noPriceQtySum > 0 {
			remaining := *extracted.TotalAmount - priceAlreadyKnown
			if remaining > 0 {
				perUnit := remaining / noPriceQtySum
				for i := range extracted.Items {
					if extracted.Items[i].Price == nil {
						p := perUnit
						extracted.Items[i].Price = &p
					}
				}
			}
		}
	}

	// 1. F1 Mapper — match each item to item_code
	var billItems []models.BillItem
	var previewItems []lineservice.BillPreviewItem
	var smlItems []sml.SMLItem
	var itemCodes []string

	for _, extItem := range extracted.Items {
		match := h.mapperSvc.Match(extItem.RawName)

		item := models.BillItem{
			RawName: extItem.RawName,
			Qty:     extItem.Qty,
			Mapped:  match.Mapping != nil && !match.NeedsReview,
		}
		if extItem.Price != nil {
			item.Price = extItem.Price
		}

		itemCode := extItem.RawName // fallback: use raw name if not mapped
		unitCode := extItem.Unit

		if match.Mapping != nil {
			item.ItemCode = &match.Mapping.ItemCode
			item.UnitCode = &match.Mapping.UnitCode
			item.MappingID = &match.Mapping.ID
			itemCode = match.Mapping.ItemCode
			unitCode = match.Mapping.UnitCode
			itemCodes = append(itemCodes, itemCode)
		}

		price := 0.0
		if extItem.Price != nil {
			price = *extItem.Price
		}

		billItems = append(billItems, item)
		smlItems = append(smlItems, sml.SMLItem{
			ItemCode: itemCode,
			Qty:      extItem.Qty,
			UnitCode: unitCode,
			Price:    price,
		})
		previewItems = append(previewItems, lineservice.BillPreviewItem{
			RawName:  extItem.RawName,
			ItemCode: itemCode,
			Qty:      extItem.Qty,
			Unit:     unitCode,
			Price:    price,
		})
	}

	// 2. F2 Anomaly detection — include historical prices if available
	avgPrices, maxQtys, _ := h.billRepo.GetPriceHistories(itemCodes)
	checkItems := make([]models.BillItem, len(billItems))
	copy(checkItems, billItems)
	anomalies := h.anomalySvc.Check(anomaly.CheckInput{
		Items:     checkItems,
		AvgPrices: avgPrices,
		MaxQtys:   maxQtys,
	})

	// 3. Save bill to DB
	conf := extracted.Confidence
	rawDataBytes, _ := json.Marshal(map[string]interface{}{
		"customer_name":  extracted.CustomerName,
		"customer_phone": extracted.CustomerPhone,
		"note":           extracted.Note,
	})
	bill := &models.Bill{
		BillType:     "sale",
		Source:       "line",
		AIConfidence: &conf,
		RawData:      json.RawMessage(rawDataBytes),
	}
	if err := h.billRepo.Create(bill); err != nil {
		h.logger.Error("create bill", zap.Error(err))
		h.replyErr(event.ReplyToken, "ขอโทษ ระบบขัดข้อง กรุณาลองใหม่")
		return
	}

	// Save anomalies
	if len(anomalies) > 0 {
		if err := h.billRepo.UpdateAnomalies(bill.ID, anomalies); err != nil {
			h.logger.Warn("update anomalies", zap.Error(err))
		}
	}

	// Save items
	for i := range billItems {
		billItems[i].BillID = bill.ID
		if err := h.billRepo.InsertItem(&billItems[i]); err != nil {
			h.logger.Warn("insert bill item", zap.Error(err))
		}
	}

	// 4. Auto-confirm or send preview
	canAutoConfirm := anomaly.CanAutoConfirm(anomalies, extracted.Confidence, h.threshold)
	hasAnomaly := len(anomalies) > 0

	if canAutoConfirm {
		customerName := extracted.CustomerName
		h.sendToSML(ctx, bill.ID, event.ReplyToken, event.Source.UserID, customerName, extracted.CustomerPhone, smlItems, len(billItems))
	} else {
		customerName := extracted.CustomerName
		if customerName == "" {
			customerName = "ลูกค้า"
		}
		if err := h.lineSvc.ReplyBillPreview(
			event.ReplyToken, bill.ID, customerName, previewItems, hasAnomaly,
		); err != nil {
			h.logger.Error("reply bill preview", zap.Error(err))
		}
	}
}

// ── SML submission ─────────────────────────────────────────────────────────────

func (h *LineHandler) sendToSML(ctx context.Context, billID, replyToken, userID, customerName string, customerPhone *string, items []sml.SMLItem, itemCount int) {
	phone := ""
	if customerPhone != nil {
		phone = *customerPhone
	}

	smlReq := sml.SaleReserveRequest{
		ContactName:  customerName,
		ContactPhone: phone,
		Items:        items,
	}
	reqJSON, _ := json.Marshal(smlReq)

	result, err := h.smlClient.CreateSaleReserve(smlReq)
	if err != nil {
		h.logger.Error("SML create sale reserve",
			zap.String("bill_id", billID), zap.Error(err))
		errMsg := err.Error()
		respJSON, _ := json.Marshal(map[string]string{"error": errMsg})
		h.billRepo.UpdateStatus(billID, "failed", nil, respJSON, &errMsg)
		// Audit: SML failed
		if h.auditRepo != nil {
			billIDStr := billID
			_ = h.auditRepo.Log(models.AuditEntry{
				Action:   "sml_failed",
				TargetID: &billIDStr,
				Source:   "line",
				Level:    "error",
				TraceID:  billID,
				Detail: map[string]interface{}{
					"sml_payload": json.RawMessage(reqJSON),
					"error":       errMsg,
				},
			})
		}
		// Notify admin
		if h.lineSvc != nil {
			h.lineSvc.PushAdmin("⚠️ SML Error\nBill: " + billID + "\nError: " + errMsg)
		}
		if replyToken != "" {
			h.lineSvc.ReplyText(replyToken, "ขอโทษ ส่งข้อมูลเข้าระบบไม่สำเร็จ กรุณาแจ้ง admin")
		}
		return
	}

	respJSON, _ := json.Marshal(result)
	h.billRepo.UpdateStatus(billID, "sent", &result.DocNo, respJSON, nil)
	h.billRepo.UpdateSMLPayload(billID, reqJSON)
	// Audit: SML sent successfully
	if h.auditRepo != nil {
		billIDStr := billID
		_ = h.auditRepo.Log(models.AuditEntry{
			Action:   "sml_sent",
			TargetID: &billIDStr,
			Source:   "line",
			Level:    "info",
			TraceID:  billID,
			Detail: map[string]interface{}{
				"doc_no":       result.DocNo,
				"sml_payload":  json.RawMessage(reqJSON),
				"sml_response": json.RawMessage(respJSON),
			},
		})
	}

	// F2: Update price history for accurate future anomaly detection
	if bill, err := h.billRepo.FindByID(billID); err == nil && bill != nil {
		h.billRepo.UpdatePriceHistory(bill.Items)
	}

	if replyToken != "" {
		if err := h.lineSvc.ReplySuccessFlex(replyToken, result.DocNo, itemCount); err != nil {
			h.logger.Warn("reply success", zap.Error(err))
		}
	} else if userID != "" {
		// Chat flow: replyToken already used, push success to user
		h.lineSvc.PushSuccessToUser(userID, result.DocNo, itemCount)
	}
}

// ── Postback: user taps confirm/cancel ────────────────────────────────────────

func (h *LineHandler) processPostback(ctx context.Context, event lineEvent) {
	params, err := url.ParseQuery(event.Postback.Data)
	if err != nil {
		h.logger.Error("parse postback data", zap.Error(err))
		return
	}

	action := params.Get("action")
	billID := params.Get("bill_id")
	if billID == "" {
		return
	}

	switch action {
	case "confirm":
		h.confirmBill(ctx, billID, event.ReplyToken, event.Source.UserID)
	case "cancel":
		h.cancelBill(ctx, billID, event.ReplyToken)
	}
}

func (h *LineHandler) confirmBill(ctx context.Context, billID, replyToken, userID string) {
	bill, err := h.billRepo.FindByID(billID)
	if err != nil {
		h.logger.Error("confirmBill FindByID", zap.String("bill_id", billID), zap.Error(err))
		h.lineSvc.ReplyText(replyToken, "ไม่พบบิลที่ระบุ")
		return
	}
	if bill == nil {
		h.logger.Warn("confirmBill bill not found", zap.String("bill_id", billID))
		h.lineSvc.ReplyText(replyToken, "ไม่พบบิลที่ระบุ")
		return
	}
	if bill.Status != "pending" {
		h.lineSvc.ReplyText(replyToken, "บิลนี้ถูกดำเนินการไปแล้ว")
		return
	}

	// Rebuild SML items from stored bill items
	var smlItems []sml.SMLItem
	for _, item := range bill.Items {
		itemCode := item.RawName
		if item.ItemCode != nil {
			itemCode = *item.ItemCode
		}
		unitCode := ""
		if item.UnitCode != nil {
			unitCode = *item.UnitCode
		}
		price := 0.0
		if item.Price != nil {
			price = *item.Price
		}
		smlItems = append(smlItems, sml.SMLItem{
			ItemCode: itemCode,
			Qty:      item.Qty,
			UnitCode: unitCode,
			Price:    price,
		})
	}

	// Get customer info from raw_data
	var rawData struct {
		CustomerName  string  `json:"customer_name"`
		CustomerPhone *string `json:"customer_phone"`
	}
	json.Unmarshal(bill.RawData, &rawData)

	h.sendToSML(ctx, billID, replyToken, userID, rawData.CustomerName, rawData.CustomerPhone, smlItems, len(bill.Items))
}

func (h *LineHandler) cancelBill(ctx context.Context, billID, replyToken string) {
	h.billRepo.UpdateStatus(billID, "skipped", nil, nil, nil)
	h.lineSvc.ReplyText(replyToken, "ยกเลิกใบสั่งซื้อแล้วครับ")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *LineHandler) replyErr(replyToken, msg string) {
	if h.lineSvc != nil && replyToken != "" {
		h.lineSvc.ReplyText(replyToken, msg)
	}
}
