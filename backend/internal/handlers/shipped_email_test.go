package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	"billflow/internal/services/artifact"
	"billflow/internal/services/catalog"
	emailservice "billflow/internal/services/email"
)

func TestFindExistingShopeeShippedBillIDNormalizesHashPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("92b142e9-19bc-432b-8d8e-67d4e984e3ef"))

	got, exists, err := h.findExistingShopeeShippedBillID("#2604294EP99PKT")
	if err != nil {
		t.Fatalf("findExistingShopeeShippedBillID: %v", err)
	}
	if !exists || got != "92b142e9-19bc-432b-8d8e-67d4e984e3ef" {
		t.Fatalf("got id=%q exists=%v", got, exists)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestFindExistingShopeeShippedBillIDMissingReturnsFalse(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnError(sql.ErrNoRows)

	got, exists, err := h.findExistingShopeeShippedBillID("2604294EP99PKT")
	if err != nil {
		t.Fatalf("findExistingShopeeShippedBillID: %v", err)
	}
	if exists || got != "" {
		t.Fatalf("got id=%q exists=%v, want missing", got, exists)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestShopeeCoinAmountForItemsUsesGrossLineTotal(t *testing.T) {
	price67 := 67.0
	price68 := 68.0
	shipping := 130.0
	items := []models.BillItem{
		{RawName: "ตะกร้าพลาสติก", Qty: 5, Price: &price67},
		{RawName: "ตะกร้าพลาสติก", Qty: 1, Price: &price68},
		{RawName: "ค่าขนส่งบิลซื้อ", SourceSKU: models.ShopeeShippingSourceSKU, Qty: 1, Price: &shipping},
	}
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #260725KGHQSU9U",
		"ยอดรวมค่าสินค้า: ฿401",
		"ค่าจัดส่งสินค้า: ฿130",
		"ยอดที่ต้องชำระทั้งหมด: ฿531",
	}, "\n")

	coin, ok := shopeeCoinAmountForItems(items, body, "", "260725KGHQSU9U", 0)
	if !ok || coin != 2 {
		t.Fatalf("coin = (%v, %v), want (2, true)", coin, ok)
	}
}

func TestShopeeCoinAmountForItemsCombinesCouponAndCoin(t *testing.T) {
	price := 67.0
	shipping := 72.0
	items := []models.BillItem{
		{RawName: "ตะกร้าพลาสติก", Qty: 5, Price: &price},
		{RawName: "ค่าขนส่งบิลซื้อ", SourceSKU: models.ShopeeShippingSourceSKU, Qty: 1, Price: &shipping},
	}
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #26080181CVHFBN",
		"ยอดรวมค่าสินค้า: ฿333",
		"โค้ดส่วนลดของ Shopee: ฿57",
		"ค่าจัดส่งสินค้า: ฿72",
		"ยอดที่ต้องชำระทั้งหมด: ฿348",
	}, "\n")

	coin, ok := shopeeCoinAmountForItems(items, body, "", "26080181CVHFBN", 57)
	if !ok || coin != 2 {
		t.Fatalf("coin = (%v, %v), want (2, true)", coin, ok)
	}
}

func TestShopeeCoinAmountForItemsCombinesCouponAndCoinAcrossLines(t *testing.T) {
	price68 := 68.0
	price67 := 67.0
	shipping := 92.0
	items := []models.BillItem{
		{RawName: "ตะกร้าพลาสติก", Qty: 1, Price: &price68},
		{RawName: "ตะกร้าพลาสติก", Qty: 5, Price: &price67},
		{RawName: "ค่าขนส่งบิลซื้อ", SourceSKU: models.ShopeeShippingSourceSKU, Qty: 1, Price: &shipping},
	}
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #26080181A2WE87",
		"ยอดรวมค่าสินค้า: ฿401",
		"โค้ดส่วนลดของ Shopee: ฿101",
		"ค่าจัดส่งสินค้า: ฿92",
		"ยอดที่ต้องชำระทั้งหมด: ฿392",
	}, "\n")

	coin, ok := shopeeCoinAmountForItems(items, body, "", "26080181A2WE87", 101)
	if !ok || coin != 2 {
		t.Fatalf("coin = (%v, %v), want (2, true)", coin, ok)
	}
}

func TestShopeeCoinAmountForItemsRejectsInconsistentEmailTotals(t *testing.T) {
	price := 100.0
	items := []models.BillItem{{RawName: "สินค้า", Qty: 1, Price: &price}}
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #260725KGHQSU9U",
		"ยอดรวมค่าสินค้า: ฿90",
		"ค่าจัดส่งสินค้า: ฿10",
		"ยอดที่ต้องชำระทั้งหมด: ฿95",
	}, "\n")

	coin, ok := shopeeCoinAmountForItems(items, body, "", "260725KGHQSU9U", 0)
	if ok || coin != 0 {
		t.Fatalf("coin = (%v, %v), want (0, false)", coin, ok)
	}
}

func TestProcessOneShippedOrderRecordsEventOnExistingBill(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		billRepo:    repository.NewBillRepo(db),
		artifactSvc: artifact.New(t.TempDir(), 10<<20, repository.NewBillArtifactRepo(db), zap.NewNop()),
		logger:      zap.NewNop(),
	}
	messageID := "shipped-message@example.test"
	orderID := "#2604294EP99PKT"
	existingBillID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	htmlBody := "<html></html>"

	mock.ExpectQuery("SELECT").
		WithArgs(messageID, "2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM bills").
		WithArgs("2604294EP99PKT").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingBillID))
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs(existingBillID, "2604294EP99PKT", shopeeEventShipped, "ถูกจัดส่งแล้ว", "คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว", "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE shopee_order_events").
		WithArgs("2604294EP99PKT", existingBillID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8", int64(len(htmlBody)), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", time.Now()))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_envelope", "envelope.json", "application/json", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", time.Now()))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "2604294EP99PKT").
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := h.processOneShippedOrder(
		aiExtractedOrderForTest(orderID),
		"คำสั่งซื้อ #2604294EP99PKT ถูกจัดส่งแล้ว",
		"info@mail.shopee.co.th",
		"body",
		htmlBody,
		messageID,
		nil,
		"trace-1",
		time.Now(),
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("processOneShippedOrder: %v", err)
	}
	if created {
		t.Fatal("expected existing shipped event to skip creating a new bill")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestProcessShopeeShippedEmailBodySkipsShippingWithoutPaymentBill(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		billRepo: repository.NewBillRepo(db),
		logger:   zap.NewNop(),
	}
	messageID := "shipping-only-message@example.test"
	subject := "คำสั่งซื้อ #260611SACGW9M8 ถูกจัดส่งแล้ว"
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ: #260611SACGW9M8",
		"ยอดที่ต้องชำระทั้งหมด ฿772",
		"Shopee",
	}, "\n")

	mock.ExpectQuery("FROM bills").
		WithArgs("260611SACGW9M8").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs("", "260611SACGW9M8", shopeeEventShipped, "ถูกจัดส่งแล้ว", subject, "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "260611SACGW9M8").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	outcome, err := h.ProcessShopeeShippedEmailBody(
		subject,
		"info@mail.shopee.co.th",
		body,
		"",
		messageID,
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("ProcessShopeeShippedEmailBody: %v", err)
	}
	if outcome.Kind != emailservice.ProcessOutcomeSkipped || outcome.Code != "shopee_shipped_without_payment_bill" {
		t.Fatalf("outcome = %#v, want skipped shipping-only", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestProcessShopeeShippedEmailBodyRecordsShippingOnExistingBillWithoutAI(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		billRepo:    repository.NewBillRepo(db),
		artifactSvc: artifact.New(t.TempDir(), 10<<20, repository.NewBillArtifactRepo(db), zap.NewNop()),
		logger:      zap.NewNop(),
	}
	messageID := "shipping-existing-message@example.test"
	existingBillID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	subject := "คำสั่งซื้อ #260611SACGW9M8 ถูกจัดส่งแล้ว"
	htmlBody := "<html><body>ถูกจัดส่งแล้ว</body></html>"

	mock.ExpectQuery("FROM bills").
		WithArgs("260611SACGW9M8").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingBillID))
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs(existingBillID, "260611SACGW9M8", shopeeEventShipped, "ถูกจัดส่งแล้ว", subject, "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE shopee_order_events").
		WithArgs("260611SACGW9M8", existingBillID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_html", "shopee-shipped.html", "text/html; charset=utf-8", int64(len(htmlBody)), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", time.Now()))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_envelope", "envelope.json", "application/json", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", time.Now()))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "260611SACGW9M8").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	outcome, err := h.ProcessShopeeShippedEmailBody(
		subject,
		"info@mail.shopee.co.th",
		"",
		htmlBody,
		messageID,
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("ProcessShopeeShippedEmailBody: %v", err)
	}
	if outcome.Kind != emailservice.ProcessOutcomeUpdatedExisting || outcome.Code != "shopee_shipping_status_recorded" {
		t.Fatalf("outcome = %#v, want updated shipping status", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestProcessShopeeShippedEmailBodyRecordsPaymentEventBeforeAI(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		billRepo:    repository.NewBillRepo(db),
		artifactSvc: artifact.New(t.TempDir(), 10<<20, repository.NewBillArtifactRepo(db), zap.NewNop()),
		logger:      zap.NewNop(),
	}
	messageID := "payment-message@example.test"
	existingBillID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	subject := "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260608HPC8A42A"
	body := "ยืนยันการชำระเงินแล้ว"

	mock.ExpectQuery("SELECT").
		WithArgs(messageID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("FROM bills").
		WithArgs("260608HPC8A42A").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingBillID))
	mock.ExpectExec("INSERT INTO shopee_order_events").
		WithArgs(existingBillID, "260608HPC8A42A", shopeeEventPaymentConfirmed, "ยืนยันการชำระเงินแล้ว", subject, "info@mail.shopee.co.th", messageID, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE shopee_order_events").
		WithArgs("260608HPC8A42A", existingBillID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_text", "shopee-shipped.txt", "text/plain; charset=utf-8", int64(len(body)), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", time.Now()))
	mock.ExpectQuery("INSERT INTO bill_artifacts").
		WithArgs(existingBillID, "email_envelope", "envelope.json", "application/json", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", time.Now()))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "260608HPC8A42A").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO processed_email_keys").
		WithArgs("shopee_shipped", messageID, "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	outcome, err := h.ProcessShopeeShippedEmailBody(
		subject,
		"info@mail.shopee.co.th",
		body,
		"",
		messageID,
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("ProcessShopeeShippedEmailBody: %v", err)
	}
	if outcome.Kind != emailservice.ProcessOutcomeUpdatedExisting || outcome.Code != "status_event_recorded" {
		t.Fatalf("outcome = %#v, want updated status event", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestBuildShopeeExtractionJobsChunksMultiOrderEvenWhenSubjectHasOneOrder(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260608AAAAAAA",
		"สินค้า A",
		"หมายเลขคำสั่งซื้อ #260608BBBBBBB",
		"สินค้า B",
	}, "\n")
	jobs := buildShopeeExtractionJobs("ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260608BBBBBBB", body, strings.Repeat("<div>html</div>", 2000))
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 compact chunk", len(jobs))
	}
	if jobs[0].OrderID != "260608AAAAAAA,260608BBBBBBB" {
		t.Fatalf("OrderID = %q", jobs[0].OrderID)
	}
	if !strings.Contains(jobs[0].Text, "สินค้า A") || !strings.Contains(jobs[0].Text, "สินค้า B") {
		t.Fatalf("multi-order text = %q", jobs[0].Text)
	}
	if !jobs[0].Compact || jobs[0].HTML != "" {
		t.Fatalf("large html should use compact text-only job: %#v", jobs[0])
	}
}

func TestBuildShopeeExtractionJobsScopesSubjectOrderWhenBodyHasSingleOrder(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260608BBBBBBB",
		"สินค้า B",
		"รายละเอียดอื่นที่ไม่ใช่ออเดอร์",
	}, "\n")
	jobs := buildShopeeExtractionJobs("ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260608BBBBBBB", body, strings.Repeat("<div>html</div>", 2000))
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].OrderID != "260608BBBBBBB" {
		t.Fatalf("OrderID = %q", jobs[0].OrderID)
	}
	if !strings.Contains(jobs[0].Text, "สินค้า B") {
		t.Fatalf("subject-scoped text = %q", jobs[0].Text)
	}
	if !jobs[0].Compact || jobs[0].HTML != "" {
		t.Fatalf("large html should use compact text-only job: %#v", jobs[0])
	}
}

func TestBuildShopeeExtractionJobsChunksLargeMultiOrderEmail(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260608AAAAAAA\nสินค้า A",
		"หมายเลขคำสั่งซื้อ #260608BBBBBBB\nสินค้า B",
		"หมายเลขคำสั่งซื้อ #260608CCCCCCC\nสินค้า C",
		"หมายเลขคำสั่งซื้อ #260608DDDDDDD\nสินค้า D",
	}, "\n")
	jobs := buildShopeeExtractionJobs("ยืนยันการชำระเงินคำสั่งซื้อ", body, "")
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	for _, job := range jobs {
		if !job.Compact || job.HTML != "" {
			t.Fatalf("multi-order chunks should be compact text-only jobs: %#v", job)
		}
	}
	if strings.Contains(jobs[0].Text, "สินค้า D") {
		t.Fatalf("first chunk should not contain fourth order: %q", jobs[0].Text)
	}
}

func TestExtractShopeeOrdersBoundedRejectsOrderNotPresentInEmail(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260722C0U38XVU\nสินค้า ชุดถูพื้น",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVV\nสินค้า เก้าอี้",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVW\nสินค้า เก้าอี้",
	}, "\n")
	extractor := &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{{
		{OrderID: "260722C0U38XVU"},
		{OrderID: "260722C0U38XVV"},
		{OrderID: "260722C0U38XVW"},
		// Regression: AI must never create a bill for an ID absent from the source email.
		{OrderID: "260722C0U38XWV"},
	}}}
	h := &EmailHandler{shopeeAI: extractor, logger: zap.NewNop()}

	_, err := h.extractShopeeOrdersBounded("ยืนยันการชำระเงินคำสั่งซื้อ", body, "", "trace-test")
	if err == nil {
		t.Fatal("expected unexpected order ID to reject the extraction")
	}
	if !strings.Contains(err.Error(), "unexpected") || !strings.Contains(err.Error(), "260722C0U38XWV") {
		t.Fatalf("error = %v, want unexpected source order ID", err)
	}
}

func TestExtractShopeeOrdersBoundedRejectsOrderFromAnotherChunk(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260722C0U38XVU\nสินค้า A",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVV\nสินค้า B",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVW\nสินค้า C",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVX\nสินค้า D",
	}, "\n")
	extractor := &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{
		{
			{OrderID: "260722C0U38XVU"},
			{OrderID: "260722C0U38XVV"},
			{OrderID: "260722C0U38XVW"},
		},
		{
			// The second chunk is allowed to return XVX only, not an ID from chunk one.
			{OrderID: "260722C0U38XVV"},
		},
	}}
	h := &EmailHandler{shopeeAI: extractor, logger: zap.NewNop()}

	_, err := h.extractShopeeOrdersBounded("ยืนยันการชำระเงินคำสั่งซื้อ", body, "", "trace-test")
	if err == nil {
		t.Fatal("expected cross-chunk order ID to reject the extraction")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("error = %v, want missing and unexpected source order IDs", err)
	}
}

func TestExtractShopeeOrdersBoundedRejectsDuplicateOrderID(t *testing.T) {
	body := "หมายเลขคำสั่งซื้อ #260722C0U38XVU\nสินค้า ชุดถูพื้น"
	extractor := &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{{
		{OrderID: "260722C0U38XVU"},
		{OrderID: "#260722C0U38XVU"},
	}}}
	h := &EmailHandler{shopeeAI: extractor, logger: zap.NewNop()}

	_, err := h.extractShopeeOrdersBounded("ยืนยันการชำระเงินคำสั่งซื้อ #260722C0U38XVU", body, "", "trace-test")
	if err == nil {
		t.Fatal("expected duplicate order ID to reject the extraction")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v, want duplicate source order ID", err)
	}
}

func TestExtractShopeeOrdersBoundedAcceptsExactSourceOrderIDs(t *testing.T) {
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260722C0U38XVU\nสินค้า A",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVV\nสินค้า B",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVW\nสินค้า C",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVX\nสินค้า D",
	}, "\n")
	extractor := &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{
		{
			{OrderID: "260722C0U38XVU"},
			{OrderID: "260722C0U38XVV"},
			{OrderID: "260722C0U38XVW"},
		},
		{{OrderID: "260722C0U38XVX"}},
	}}
	h := &EmailHandler{shopeeAI: extractor, logger: zap.NewNop()}

	orders, err := h.extractShopeeOrdersBounded("ยืนยันการชำระเงินคำสั่งซื้อ", body, "", "trace-test")
	if err != nil {
		t.Fatalf("extractShopeeOrdersBounded: %v", err)
	}
	got := make([]string, 0, len(orders))
	for _, order := range orders {
		got = append(got, order.OrderID)
	}
	want := []string{"260722C0U38XVU", "260722C0U38XVV", "260722C0U38XVW", "260722C0U38XVX"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order IDs = %#v, want %#v", got, want)
	}
}

func TestProcessShopeeShippedEmailBodySkipsInvalidOrderIDsWithoutCreatingBills(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	messageID := "shopee-invalid-order-ids@example.test"
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260722C0U38XVU\nสินค้า ชุดถูพื้น",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVV\nสินค้า เก้าอี้",
		"หมายเลขคำสั่งซื้อ #260722C0U38XVW\nสินค้า เก้าอี้",
	}, "\n")
	extractor := &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{{
		{OrderID: "260722C0U38XVU"},
		{OrderID: "260722C0U38XVV"},
		{OrderID: "260722C0U38XVW"},
		{OrderID: "260722C0U38XWV"},
	}}}
	h := &EmailHandler{
		billRepo:   repository.NewBillRepo(db),
		catalogSvc: &catalog.SMLCatalogService{},
		shopeeAI:   extractor,
		logger:     zap.NewNop(),
	}

	outcome, err := h.ProcessShopeeShippedEmailBody(
		"ยืนยันการชำระเงินคำสั่งซื้อ",
		"info@mail.shopee.co.th",
		body,
		"",
		messageID,
		mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("ProcessShopeeShippedEmailBody: %v", err)
	}
	if outcome.Kind != emailservice.ProcessOutcomeQuarantined || outcome.Code != "shopee_shipped_order_ids_quarantined" {
		t.Fatalf("outcome = %#v, want quarantined invalid order IDs", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestExtractShopeeOrdersBoundedRecoversSingleScopedOrderWithSourceID(t *testing.T) {
	const sourceOrderID = "260724H2923XSV"
	price := 99.0
	h := &EmailHandler{
		shopeeAI: &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{
			{{OrderID: "260724H2923XSS", Items: []ai.ExtractedItem{{RawName: "สินค้า", Qty: 1, Price: &price}}}},
			{{OrderID: "ยังอ่านเลขผิด", Items: []ai.ExtractedItem{{RawName: "สินค้า", Qty: 1, Price: &price}}}},
		}},
		logger: zap.NewNop(),
	}

	orders, err := h.extractShopeeOrdersBounded(
		"ยืนยันการชำระเงินคำสั่งซื้อ #"+sourceOrderID,
		"หมายเลขคำสั่งซื้อ #"+sourceOrderID+"\nสินค้า 1 ชิ้น",
		"",
		"trace-test",
	)
	if err != nil {
		t.Fatalf("extractShopeeOrdersBounded: %v", err)
	}
	if len(orders) != 1 || orders[0].OrderID != sourceOrderID {
		t.Fatalf("orders = %#v, want one source order %s", orders, sourceOrderID)
	}
}

func TestProcessShopeeShippedEmailBodyStoresQuarantinedMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const messageID = "shopee-quarantine@example.test"
	body := strings.Join([]string{
		"หมายเลขคำสั่งซื้อ #260724H2H2NBCY\nสินค้า A",
		"หมายเลขคำสั่งซื้อ #260724H2H2NBD0\nสินค้า B",
	}, "\n")
	mock.ExpectExec("INSERT INTO email_ingestion_failures").
		WithArgs(
			"shopee_shipped", messageID, "imap-account-id", "", "ยืนยันการชำระเงินคำสั่งซื้อ",
			"info@mail.shopee.co.th", mailSourceForTest().EmailDate, body, "", "shopee_order_id_mismatch", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	h := &EmailHandler{
		catalogSvc:  &catalog.SMLCatalogService{},
		failureRepo: repository.NewEmailIngestionFailureRepo(db),
		shopeeAI: &testShopeeOrderExtractor{compactResults: [][]ai.ExtractedOrder{{
			{OrderID: "260724H2H2NBCS"},
		}}},
		logger: zap.NewNop(),
	}

	outcome, err := h.ProcessShopeeShippedEmailBody(
		"ยืนยันการชำระเงินคำสั่งซื้อ", "info@mail.shopee.co.th", body, "", messageID, mailSourceForTest(),
	)
	if err != nil {
		t.Fatalf("ProcessShopeeShippedEmailBody: %v", err)
	}
	if outcome.Kind != emailservice.ProcessOutcomeQuarantined {
		t.Fatalf("outcome = %#v, want quarantined", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestConfiguredShopeeShippingLineDisabledDoesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		channelDefaults: repository.NewChannelDefaultRepo(db),
		logger:          zap.NewNop(),
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", false, "", "", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))

	item, ready := h.configuredShopeeShippingLine("#2601AAA", 38, true)
	if item != nil || ready {
		t.Fatalf("item=%+v ready=%v, want disabled nil false", item, ready)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestConfiguredShopeeShippingLineUsesConfiguredItem(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		channelDefaults: repository.NewChannelDefaultRepo(db),
		logger:          zap.NewNop(),
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_TEST", "ครั้ง", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))

	item, ready := h.configuredShopeeShippingLine("#2601AAA", 38, true)
	if item == nil {
		t.Fatal("expected shipping item")
	}
	if !ready {
		t.Fatal("expected ready shipping item")
	}
	if item.SourceSKU != models.ShopeeShippingSourceSKU {
		t.Fatalf("source_sku = %q, want sentinel", item.SourceSKU)
	}
	if item.ItemCode == nil || *item.ItemCode != "SHIP_TEST" {
		t.Fatalf("item_code = %v, want SHIP_TEST", item.ItemCode)
	}
	if item.UnitCode == nil || *item.UnitCode != "ครั้ง" {
		t.Fatalf("unit_code = %v, want ครั้ง", item.UnitCode)
	}
	if item.Price == nil || *item.Price != 38 || item.Qty != 1 || !item.Mapped {
		t.Fatalf("item = %+v, want qty=1 price=38 mapped=true", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestConfiguredShopeeShippingLineAllowsZeroAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &EmailHandler{
		channelDefaults: repository.NewChannelDefaultRepo(db),
		logger:          zap.NewNop(),
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_TEST", "ครั้ง", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))

	item, ready := h.configuredShopeeShippingLine("#2601AAA", 0, true)
	if item == nil {
		t.Fatal("expected zero-baht shipping item")
	}
	if !ready {
		t.Fatal("expected ready shipping item")
	}
	if item.Price == nil || *item.Price != 0 {
		t.Fatalf("price = %v, want 0", item.Price)
	}
	if item.ItemCode == nil || *item.ItemCode != "SHIP_TEST" {
		t.Fatalf("item_code = %v, want SHIP_TEST", item.ItemCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestEnsureShopeeShippingLineForSendAddsMissingConfiguredLine(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &BillHandler{
		billRepo:        repository.NewBillRepo(db),
		channelDefaults: repository.NewChannelDefaultRepo(db),
		log:             zap.NewNop(),
	}
	raw, _ := json.Marshal(map[string]interface{}{"shipping_amount": 48.0})
	bill := &models.Bill{
		ID:       "ff6fb63d-ab51-4041-a943-c5a2cea6bbca",
		Source:   "shopee_shipped",
		BillType: "purchase",
		RawData:  raw,
		Items: []models.BillItem{{
			ID:       "item-1",
			RawName:  "สินค้า",
			Qty:      1,
			Mapped:   true,
			ItemCode: testStringPtr("BF0004"),
		}},
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_POL", "บาท", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))
	mock.ExpectQuery("INSERT INTO bill_items").
		WithArgs(
			bill.ID, "ค่าจัดส่งสินค้า", models.ShopeeShippingSourceSKU, "",
			"", 0, sqlmock.AnyArg(), float64(1), sqlmock.AnyArg(), sqlmock.AnyArg(), float64(0), true, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ship-item"))

	inserted, err := h.ensureShopeeShippingLineForSend(bill)
	if err != nil {
		t.Fatalf("ensureShopeeShippingLineForSend: %v", err)
	}
	if inserted == nil {
		t.Fatalf("inserted item is nil")
	}
	if len(bill.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(bill.Items))
	}
	ship := bill.Items[1]
	if ship.SourceSKU != models.ShopeeShippingSourceSKU {
		t.Fatalf("source_sku = %q, want shipping sentinel", ship.SourceSKU)
	}
	if ship.ItemCode == nil || *ship.ItemCode != "SHIP_POL" {
		t.Fatalf("item_code = %v, want SHIP_POL", ship.ItemCode)
	}
	if ship.UnitCode == nil || *ship.UnitCode != "บาท" {
		t.Fatalf("unit_code = %v, want บาท", ship.UnitCode)
	}
	if ship.Price == nil || *ship.Price != 48 || ship.Qty != 1 || !ship.Mapped {
		t.Fatalf("shipping item = %+v, want qty=1 price=48 mapped=true", ship)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestEnsureShopeeShippingLineForSendAddsZeroAmountLine(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &BillHandler{
		billRepo:        repository.NewBillRepo(db),
		channelDefaults: repository.NewChannelDefaultRepo(db),
		log:             zap.NewNop(),
	}
	raw, _ := json.Marshal(map[string]interface{}{"shipping_amount": 0.0})
	bill := &models.Bill{
		ID:       "ff6fb63d-ab51-4041-a943-c5a2cea6bbca",
		Source:   "shopee_shipped",
		BillType: "purchase",
		RawData:  raw,
		Items: []models.BillItem{{
			ID:       "item-1",
			RawName:  "สินค้า",
			Qty:      1,
			Mapped:   true,
			ItemCode: testStringPtr("BF0004"),
		}},
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("shopee_shipped", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"shopee_shipped", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "SHIP_POL", "บาท", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))
	mock.ExpectQuery("INSERT INTO bill_items").
		WithArgs(
			bill.ID, "ค่าจัดส่งสินค้า", models.ShopeeShippingSourceSKU, "",
			"", 0, sqlmock.AnyArg(), float64(1), sqlmock.AnyArg(), sqlmock.AnyArg(), float64(0), true, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("ship-item"))

	inserted, err := h.ensureShopeeShippingLineForSend(bill)
	if err != nil {
		t.Fatalf("ensureShopeeShippingLineForSend: %v", err)
	}
	if inserted == nil {
		t.Fatal("inserted item is nil")
	}
	ship := bill.Items[1]
	if ship.Price == nil || *ship.Price != 0 {
		t.Fatalf("shipping price = %v, want 0", ship.Price)
	}
	if ship.ItemCode == nil || *ship.ItemCode != "SHIP_POL" {
		t.Fatalf("item_code = %v, want SHIP_POL", ship.ItemCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestEnsureShopeeShippingLineForSendSkipsExistingLine(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &BillHandler{
		billRepo:        repository.NewBillRepo(db),
		channelDefaults: repository.NewChannelDefaultRepo(db),
		log:             zap.NewNop(),
	}
	raw, _ := json.Marshal(map[string]interface{}{"shipping_amount": 48.0})
	bill := &models.Bill{
		ID:       "bill-1",
		Source:   "shopee_shipped",
		BillType: "purchase",
		RawData:  raw,
		Items: []models.BillItem{{
			ID:        "ship-item",
			SourceSKU: models.ShopeeShippingSourceSKU,
			RawName:   "ค่าจัดส่งสินค้า",
			Qty:       1,
			Mapped:    true,
			ItemCode:  testStringPtr("SHIP_POL"),
		}},
	}

	inserted, err := h.ensureShopeeShippingLineForSend(bill)
	if err != nil {
		t.Fatalf("ensureShopeeShippingLineForSend: %v", err)
	}
	if inserted != nil {
		t.Fatalf("inserted item = %+v, want nil for existing line", inserted)
	}
	if len(bill.Items) != 1 {
		t.Fatalf("items len = %d, want unchanged 1", len(bill.Items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestEnsureMarketplaceFeeLineForSendAddsLazadaConfiguredLine(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &BillHandler{
		billRepo:        repository.NewBillRepo(db),
		channelDefaults: repository.NewChannelDefaultRepo(db),
		log:             zap.NewNop(),
	}
	raw, _ := json.Marshal(map[string]interface{}{
		"shipping_amount":    139.0,
		"service_fee_amount": 5.0,
	})
	bill := &models.Bill{
		ID:       "ff6fb63d-ab51-4041-a943-c5a2cea6bbca",
		Source:   "lazada_email",
		BillType: "purchase",
		RawData:  raw,
		Items: []models.BillItem{{
			ID:       "item-1",
			RawName:  "สินค้า",
			Qty:      1,
			Mapped:   true,
			ItemCode: testStringPtr("BF0004"),
		}},
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"lazada_email", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", true, "FEE_LZD", "บาท", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))
	mock.ExpectQuery("INSERT INTO bill_items").
		WithArgs(
			bill.ID, "ค่าจัดส่ง/ค่าธรรมเนียม Lazada", models.LazadaFeeSourceSKU, "",
			"", 0, sqlmock.AnyArg(), float64(1), sqlmock.AnyArg(), sqlmock.AnyArg(), float64(0), true, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("lazada-fee-item"))

	inserted, err := h.ensureMarketplaceFeeLineForSend(bill)
	if err != nil {
		t.Fatalf("ensureMarketplaceFeeLineForSend: %v", err)
	}
	if inserted == nil {
		t.Fatalf("inserted item is nil")
	}
	if len(bill.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(bill.Items))
	}
	fee := bill.Items[1]
	if fee.SourceSKU != models.LazadaFeeSourceSKU {
		t.Fatalf("source_sku = %q, want Lazada fee sentinel", fee.SourceSKU)
	}
	if fee.ItemCode == nil || *fee.ItemCode != "FEE_LZD" {
		t.Fatalf("item_code = %v, want FEE_LZD", fee.ItemCode)
	}
	if fee.UnitCode == nil || *fee.UnitCode != "บาท" {
		t.Fatalf("unit_code = %v, want บาท", fee.UnitCode)
	}
	if fee.Price == nil || *fee.Price != 144 || fee.Qty != 1 || !fee.Mapped {
		t.Fatalf("fee item = %+v, want qty=1 price=144 mapped=true", fee)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestEnsureMarketplaceFeeLineForSendBlocksLazadaWithoutConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	h := &BillHandler{
		billRepo:        repository.NewBillRepo(db),
		channelDefaults: repository.NewChannelDefaultRepo(db),
		log:             zap.NewNop(),
	}
	raw, _ := json.Marshal(map[string]interface{}{
		"shipping_amount":    54.0,
		"service_fee_amount": 0.0,
	})
	bill := &models.Bill{
		ID:       "ff6fb63d-ab51-4041-a943-c5a2cea6bbca",
		Source:   "lazada_email",
		BillType: "purchase",
		RawData:  raw,
		Items:    []models.BillItem{},
	}
	mock.ExpectQuery("FROM channel_defaults").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(channelDefaultRows().AddRow(
			"lazada_email", "purchase", "", "", "", "", "", "PO", "/api/v1/ic/purchase-orders",
			"BF-PO", "YYMM####", "", "", "", "", false, "", "", "", "", "", "", "", "", "", "", -1, -1.0, -1, "", []byte("{}"), nil, time.Now(),
		))

	inserted, err := h.ensureMarketplaceFeeLineForSend(bill)
	if err == nil {
		t.Fatal("expected config error")
	}
	if inserted != nil {
		t.Fatalf("inserted item = %+v, want nil", inserted)
	}
	if !isMarketplaceFeeConfigError(err) {
		t.Fatalf("err = %T %v, want marketplace config error", err, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func testStringPtr(v string) *string {
	return &v
}

func channelDefaultRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"channel", "bill_type", "party_code", "party_name", "party_phone",
		"party_address", "party_tax_id", "doc_format_code", "endpoint",
		"doc_prefix", "doc_running_format",
		"branch_code", "sale_code", "unit_code", "doc_time",
		"shipping_item_enabled", "shipping_item_code", "shipping_item_unit_code",
		"passbook_code", "passbook_name", "bank_code", "bank_branch", "expense_code", "expense_name",
		"wh_code", "shelf_code", "vat_type", "vat_rate",
		"inquiry_type", "remark_2",
		"print_policy",
		"updated_by", "updated_at",
	})
}

func aiExtractedOrderForTest(orderID string) ai.ExtractedOrder {
	price := 131.0
	return ai.ExtractedOrder{
		OrderID:    orderID,
		Confidence: 0.9,
		Items: []ai.ExtractedItem{{
			RawName: "SPIN MOP",
			Qty:     1,
			Price:   &price,
		}},
	}
}

type testShopeeOrderExtractor struct {
	compactResults []([]ai.ExtractedOrder)
	compactCalls   int
}

func (e *testShopeeOrderExtractor) ExtractOrdersCompact(string) ([]ai.ExtractedOrder, error) {
	if e.compactCalls >= len(e.compactResults) {
		return nil, nil
	}
	orders := e.compactResults[e.compactCalls]
	e.compactCalls++
	return orders, nil
}

func (e *testShopeeOrderExtractor) ExtractOrdersWithHTML(text, html string) ([]ai.ExtractedOrder, error) {
	return e.ExtractOrdersCompact(text + html)
}

func mailSourceForTest() emailservice.MailSource {
	return emailservice.MailSource{
		AccountID: "imap-account-id",
		Username:  "pd.thaisunsport2@gmail.com",
		EmailDate: time.Date(2026, 5, 3, 12, 10, 3, 0, time.UTC).Format(time.RFC3339),
	}
}
