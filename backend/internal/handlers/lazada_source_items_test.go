package handlers

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestLazadaInvalidSourceItemsHaveDistinctError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const orderID = "1234567890123456"
	mock.ExpectQuery("SELECT id::text").WithArgs("lazada_email", orderID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	_, err = h.processOneLazadaEmailOrder(ai.ExtractedOrder{OrderID: orderID},
		"confirmation", "", "", "<p>จำนวน: 2</p>", "", "test", time.Now(), emailservice.MailSource{})
	var sourceErr *lazadaSourceItemsError
	if !errors.As(err, &sourceErr) {
		t.Fatalf("expected item evidence failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLazadaQuarantineRequiresSavedEvidence(t *testing.T) {
	for _, storeFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved", true: "storage unavailable"}[storeFails], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			const messageID = "lazada-review@example.test"
			const orderID = "1234567890123456"
			const body = "<p>จำนวน: 1</p>"
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id::text FROM marketplace_email_groups").
				WithArgs(lazadaEmailSource, messageID).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("group"))
			mock.ExpectQuery("FROM marketplace_email_group_orders o").
				WithArgs(lazadaEmailSource, messageID, "group").
				WillReturnRows(sqlmock.NewRows([]string{"id", "order_id", "bill_id", "archived", "same_message"}).
					AddRow("order-row", orderID, "", false, false))
			mock.ExpectExec("UPDATE marketplace_email_group_orders").
				WithArgs("order-row", nil, "failed", lazadaSourceItemsFailure, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE marketplace_email_groups").
				WithArgs("group", "attention", 0, 1, lazadaSourceItemsFailure, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			store := mock.ExpectExec("INSERT INTO email_ingestion_failures").
				WithArgs(lazadaEmailSource, messageID, "account", "INBOX", "confirmation", "", "", "", body, lazadaSourceItemsFailure, sqlmock.AnyArg())
			if storeFails {
				store.WillReturnError(errors.New("storage unavailable"))
			} else {
				store.WillReturnResult(sqlmock.NewResult(0, 1))
			}
			h := &EmailHandler{
				emailGroupRepo: repository.NewMarketplaceEmailGroupRepo(db),
				failureRepo:    repository.NewEmailIngestionFailureRepo(db),
			}
			outcome, err := h.quarantineLazadaSourceItems(models.EmailIngestionFailure{
				Source: lazadaEmailSource, MessageID: messageID, IMAPAccountID: "account",
				IMAPMailbox: "INBOX", Subject: "confirmation", BodyHTML: body,
				FailureCode: lazadaSourceItemsFailure,
			})
			if (err != nil) != storeFails || (outcome.Kind == emailservice.ProcessOutcomeQuarantined) == storeFails {
				t.Fatalf("outcome=%+v err=%v", outcome, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLazadaPreviouslyQuarantinedMessageSkipsAI(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const messageID = "lazada-review@example.test"
	mock.ExpectQuery("SELECT.*COUNT").WithArgs(lazadaEmailSource, messageID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT EXISTS .*FROM email_ingestion_failures").
		WithArgs(lazadaEmailSource, messageID, lazadaSourceItemsFailure).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	h := &EmailHandler{
		billRepo: repository.NewBillRepo(db), failureRepo: repository.NewEmailIngestionFailureRepo(db),
		logger: zap.NewNop(),
	}
	outcome, err := h.ProcessLazadaEmailBody("confirmation", "", "", "<p>จำนวน: 1</p>", messageID, emailservice.MailSource{})
	if err != nil || outcome.Kind != emailservice.ProcessOutcomeQuarantined {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExistingLazadaOrderSkipsUnsupportedFollowupTemplate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT id::text").WithArgs("lazada_email", "1124675871588966").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("existing-bill"))
	h := &EmailHandler{billRepo: repository.NewBillRepo(db), logger: zap.NewNop()}
	created, err := h.processOneLazadaEmailOrder(ai.ExtractedOrder{OrderID: "1124675871588966"}, "shipped", "", "", `<p>จำนวน: 2</p>`, "", "test", time.Now(), emailservice.MailSource{})
	if err != nil || created {
		t.Fatalf("duplicate followup must skip safely: created=%v err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUnverifiedLazadaSourceCannotSendEvenWhenMoneyIsOK(t *testing.T) {
	for _, sourceStatus := range []string{"verified_html", "unverified"} {
		raw, _ := json.Marshal(map[string]interface{}{"order_id": "123", "source_items_status": sourceStatus, "amount_reconciliation_status": "ok", "lazada_charge_group_key": "group", "payment_method": "Credit or Debit Card", "paid_total_amount": 2558})
		err := validateMarketplaceAmountReadyForSend(&models.Bill{Source: "lazada_email", BillType: "purchase", RawData: raw})
		if (err == nil) != (sourceStatus == "verified_html") {
			t.Fatalf("status=%s err=%v", sourceStatus, err)
		}
	}
}

func TestPrepareLazadaSourceItemsReplacesCollapsedAIRow(t *testing.T) {
	body := `<table><tr><td><img src="https://th-live-01.slatic.net/p/white.jpg"></td><td><a href="https://lazada.co.th/products/i1-s11.html">นาฬิกา LED</a><div>THB 678.00</div><div>จำนวน: 2</div></td></tr></table><table><tr><td><img src="https://th-live-01.slatic.net/p/green.jpg"></td><td><a href="https://lazada.co.th/products/i1-s12.html">นาฬิกา LED</a><div>THB 678.00</div><div>จำนวน: 2</div></td></tr></table>`
	price := 1356.0
	items, metadata, verified, err := prepareLazadaSourceItems([]ai.ExtractedItem{{RawName: "นาฬิกา LED", Qty: 2, Price: &price}}, body, repository.LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712})
	if err != nil || !verified || len(items) != 2 || len(metadata) != 2 {
		t.Fatalf("items=%+v metadata=%+v verified=%v err=%v", items, metadata, verified, err)
	}
	if *items[0].Price != 678 || items[1].Qty != 2 || metadata[0].SourceSKU != "11" || metadata[1].SourceSKU != "12" {
		t.Fatalf("wrong source rows: %+v %+v", items, metadata)
	}
	items, _, verified, err = prepareLazadaSourceItems(nil, body, repository.LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712})
	if err != nil || !verified || len(items) != 2 {
		t.Fatalf("source must also recover omitted AI items: items=%+v verified=%v err=%v", items, verified, err)
	}
}

func TestPrepareLazadaSourceItemsNeverInflatesFallbackPrice(t *testing.T) {
	price := 678.0
	items, _, verified, err := prepareLazadaSourceItems([]ai.ExtractedItem{{RawName: "นาฬิกา LED", Qty: 2, Price: &price}}, "", repository.LazadaAmountSummary{HasGoodsTotalAmount: true, GoodsTotalAmount: 2712})
	if err != nil || verified || *items[0].Price != 678 {
		t.Fatalf("price must remain unmodified: %+v verified=%v err=%v", items, verified, err)
	}
	raw := map[string]interface{}{"amount_reconciliation_status": repository.LazadaReconciliationOK}
	applyLazadaSourceVerification(raw, verified, len(items))
	if raw["amount_reconciliation_status"] != repository.LazadaReconciliationMissing {
		t.Fatalf("unverified source must block SML: %+v", raw)
	}
}
