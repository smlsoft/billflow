package repository

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"billflow/internal/models"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordEmailPrintEventUsesArtifactSourceMeta(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	artifactID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	userID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(billID, artifactID).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "subject", "from_addr", "kind"}).
			AddRow("artifact-message@example.test", "artifact subject", "artifact sender", "email_html"))
	mock.ExpectQuery("SELECT source").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source", "bill_type", "status", "order_id", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).AddRow("email", "sale", "sent", "ORDER-1", "DOC-1", "", "", "", ""))
	mock.ExpectQuery("INSERT INTO email_print_events").
		WithArgs(billID, artifactID, "artifact-message@example.test", emailGroupKey("artifact-message@example.test"), "artifact subject", "artifact sender", userID, "admin@example.test").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "artifact_id", "email_message_id", "email_group_key",
			"subject", "from_addr", "requested_by", "requested_by_email", "created_at",
		}).AddRow(
			"22222222-2222-2222-2222-222222222222", billID, artifactID, "artifact-message@example.test",
			emailGroupKey("artifact-message@example.test"), "artifact subject", "artifact sender", userID, "admin@example.test", time.Now(),
		))

	event, err := repo.RecordEmailPrintEvent(billID, artifactID, userID, "admin@example.test")
	if err != nil {
		t.Fatalf("RecordEmailPrintEvent: %v", err)
	}
	if event == nil {
		t.Fatal("expected print event")
	}
	if event.EmailMessageID != "artifact-message@example.test" || event.Subject != "artifact subject" || event.From != "artifact sender" {
		t.Fatalf("event used wrong metadata: %#v", event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestRecordEmailPrintEventBlocksMarketplaceEmailGroupMissingSMLDoc(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	artifactID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	messageID := "lazada-message@example.test"

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(billID, artifactID).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "subject", "from_addr", "kind"}).
			AddRow(messageID, "Lazada order", "noreply@lazada.co.th", "email_html"))
	mock.ExpectQuery("SELECT source").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source", "bill_type", "status", "order_id", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).AddRow("lazada_email", "purchase", "sent", "1107473377495692", "PO260600001", "AF00007", "TT3086", "", "TT3086"))
	mock.ExpectQuery("SELECT COALESCE").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(sqlmock.NewRows([]string{"print_policy"}).AddRow([]byte("{}")))
	mock.ExpectQuery("SELECT b.id::text").
		WithArgs(messageID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "source", "bill_type", "status", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).
			AddRow("bill-1", "1107473377495692", "lazada_email", "purchase", "sent", "PO260600001", "AF00007", "TT3086", "", "TT3086").
			AddRow("bill-2", "1107473377495693", "lazada_email", "purchase", "pending", "", "AF00007", "TT3086", "", "TT3086"))

	event, err := repo.RecordEmailPrintEvent(billID, artifactID, "", "admin@example.test")
	if event != nil {
		t.Fatalf("event = %#v, want nil", event)
	}
	var readinessErr *EmailPrintReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("err = %T %v, want EmailPrintReadinessError", err, err)
	}
	if readinessErr.RelatedOrderCount != 2 || readinessErr.SMLDocCount != 1 {
		t.Fatalf("counts = related:%d doc:%d, want 2/1", readinessErr.RelatedOrderCount, readinessErr.SMLDocCount)
	}
	if len(readinessErr.MissingOrders) != 1 || readinessErr.MissingOrders[0] != "1107473377495693" {
		t.Fatalf("missing orders = %#v", readinessErr.MissingOrders)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestRecordEmailPrintEventBlocksMarketplaceEmailGroupNonMatchingPaymentMethod(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	artifactID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	messageID := "lazada-message@example.test"

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(billID, artifactID).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "subject", "from_addr", "kind"}).
			AddRow(messageID, "Lazada order", "noreply@lazada.co.th", "email_html"))
	mock.ExpectQuery("SELECT source").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source", "bill_type", "status", "order_id", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).AddRow("lazada_email", "purchase", "sent", "1107473377495692", "PO260600001", "AF00007", "ACME", "โอน Kbank", "โอน Kbank"))
	mock.ExpectQuery("SELECT COALESCE").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(sqlmock.NewRows([]string{"print_policy"}).AddRow([]byte("{}")))
	mock.ExpectQuery("SELECT b.id::text").
		WithArgs(messageID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "source", "bill_type", "status", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).
			AddRow("bill-1", "1107473377495692", "lazada_email", "purchase", "sent", "PO260600001", "AF00007", "ACME", "โอน Kbank", "โอน Kbank"))

	event, err := repo.RecordEmailPrintEvent(billID, artifactID, "", "admin@example.test")
	if event != nil {
		t.Fatalf("event = %#v, want nil", event)
	}
	var readinessErr *EmailPrintReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("err = %T %v, want EmailPrintReadinessError", err, err)
	}
	if len(readinessErr.NonMatchingPaymentMethodOrders) != 1 || readinessErr.NonMatchingPaymentMethodOrders[0] != "1107473377495692" {
		t.Fatalf("non matching orders = %#v", readinessErr.NonMatchingPaymentMethodOrders)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestRecordEmailPrintEventAllowsConfiguredPaymentMethodPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	billID := "768a0068-cad3-4b6e-b229-a5d2ce2ede73"
	artifactID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	messageID := "lazada-message@example.test"
	userID := "11111111-1111-1111-1111-111111111111"
	policyJSON, err := json.Marshal(models.MarketplacePrintPolicy{
		RequiresAllOrdersSMLDoc:    true,
		PaymentMethodPrefixEnabled: true,
		PaymentMethodPrefixes:      []string{"AF"},
		PaymentMethods:             []string{"AF00007"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(billID, artifactID).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "subject", "from_addr", "kind"}).
			AddRow(messageID, "Lazada order", "noreply@lazada.co.th", "email_html"))
	mock.ExpectQuery("SELECT source").
		WithArgs(billID).
		WillReturnRows(sqlmock.NewRows([]string{
			"source", "bill_type", "status", "order_id", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).AddRow("lazada_email", "purchase", "sent", "1107473377495692", "PO260600001", "AF00007", "ACME", "AF00007", "AF00007"))
	mock.ExpectQuery("SELECT COALESCE").
		WithArgs("lazada_email", "purchase").
		WillReturnRows(sqlmock.NewRows([]string{"print_policy"}).AddRow(policyJSON))
	mock.ExpectQuery("SELECT b.id::text").
		WithArgs(messageID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_id", "source", "bill_type", "status", "sml_doc_no", "party_code", "party_name",
			"print_payment_method", "effective_print_payment_method",
		}).
			AddRow("bill-1", "1107473377495692", "lazada_email", "purchase", "sent", "PO260600001", "AF00007", "ACME", "AF00007", "AF00007"))
	mock.ExpectQuery("INSERT INTO email_print_events").
		WithArgs(billID, artifactID, messageID, emailGroupKey(messageID), "Lazada order", "noreply@lazada.co.th", userID, "admin@example.test").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "bill_id", "artifact_id", "email_message_id", "email_group_key",
			"subject", "from_addr", "requested_by", "requested_by_email", "created_at",
		}).AddRow(
			"22222222-2222-2222-2222-222222222222", billID, artifactID, messageID,
			emailGroupKey(messageID), "Lazada order", "noreply@lazada.co.th", userID, "admin@example.test", time.Now(),
		))

	event, err := repo.RecordEmailPrintEvent(billID, artifactID, userID, "admin@example.test")
	if err != nil {
		t.Fatalf("RecordEmailPrintEvent: %v", err)
	}
	if event == nil {
		t.Fatal("expected print event")
	}
	if event.RelatedOrderCount != 1 || event.SMLDocCount != 1 {
		t.Fatalf("counts = related:%d doc:%d, want 1/1", event.RelatedOrderCount, event.SMLDocCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestAttachEmailGroupsIncludesNoPrintSummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	raw, err := json.Marshal(map[string]interface{}{
		"email_message_id": "message@example.test",
		"subject":          "คำสั่งซื้อ #A ถูกจัดส่งแล้ว",
		"from":             "Shopee <noreply@shopee.test>",
	})
	if err != nil {
		t.Fatal(err)
	}
	bills := []models.Bill{{ID: "bill-1", RawData: raw}}

	mock.ExpectQuery("WITH matched_bills").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"message_id", "order_count", "has_printable_email", "print_count",
			"last_printed_at", "last_printed_by_email", "last_printed_by_name",
		}).AddRow("message@example.test", 1, true, 0, nil, "", ""))

	if err := repo.attachEmailGroups(bills); err != nil {
		t.Fatalf("attachEmailGroups: %v", err)
	}
	if bills[0].EmailGroup == nil {
		t.Fatal("expected email group")
	}
	if bills[0].EmailGroup.PrintCount != 0 {
		t.Fatalf("print_count = %d, want 0", bills[0].EmailGroup.PrintCount)
	}
	if bills[0].EmailGroup.LastPrintedAt != nil {
		t.Fatalf("last_printed_at = %v, want nil", bills[0].EmailGroup.LastPrintedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestAttachEmailGroupsIncludesPrintSummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	repo := NewBillRepo(db)
	raw, err := json.Marshal(map[string]interface{}{
		"email_message_id": "message@example.test",
		"subject":          "คำสั่งซื้อ #A ถูกจัดส่งแล้ว",
		"from":             "Shopee <noreply@shopee.test>",
	})
	if err != nil {
		t.Fatal(err)
	}
	bills := []models.Bill{{ID: "bill-1", RawData: raw}}
	printedAt := time.Date(2026, 5, 25, 23, 10, 0, 0, time.UTC)

	mock.ExpectQuery("WITH matched_bills").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"message_id", "order_count", "has_printable_email", "print_count",
			"last_printed_at", "last_printed_by_email", "last_printed_by_name",
		}).AddRow("message@example.test", 14, true, 2, printedAt, "admin@example.test", "Admin"))

	if err := repo.attachEmailGroups(bills); err != nil {
		t.Fatalf("attachEmailGroups: %v", err)
	}
	group := bills[0].EmailGroup
	if group == nil {
		t.Fatal("expected email group")
	}
	if group.OrderCount != 14 || group.PrintCount != 2 {
		t.Fatalf("group counts = order:%d print:%d, want order:14 print:2", group.OrderCount, group.PrintCount)
	}
	if group.LastPrintedAt == nil || !group.LastPrintedAt.Equal(printedAt) {
		t.Fatalf("last_printed_at = %v, want %v", group.LastPrintedAt, printedAt)
	}
	if group.LastPrintedByEmail != "admin@example.test" || group.LastPrintedByName != "Admin" {
		t.Fatalf("last printed by = %q/%q", group.LastPrintedByEmail, group.LastPrintedByName)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}
