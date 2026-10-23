package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"billflow/internal/models"
	"billflow/internal/repository"
	"billflow/internal/services/ai"
	emailservice "billflow/internal/services/email"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

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
