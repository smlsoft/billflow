package shippingcheck

import (
	"encoding/json"
	"testing"

	"billflow/internal/models"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestManualShippingSendGate(t *testing.T) {
	policy := Enrollment{Profile: testProfile(), Recipients: []string{"PD01"}, Revision: "current"}
	for _, tt := range []struct {
		name  string
		event *gateEvent
		ready bool
	}{
		{"no check", nil, false},
		{"automatic match", &gateEvent{Policy: policy, Result: Result{Status: "match", Version: Version}}, false},
		{"manual mismatch", &gateEvent{Policy: policy, Result: Result{Trigger: "manual", Status: "mismatch", Version: Version}}, false},
		{"manual unknown", &gateEvent{Policy: policy, Result: Result{Trigger: "manual", Status: "unverifiable", Version: Version}}, false},
		{"old parser", &gateEvent{Policy: policy, Result: Result{Trigger: "manual", Status: "match", Version: "old"}}, false},
		{"manual pass", &gateEvent{Policy: policy, Result: Result{Trigger: "manual", Status: "match", Version: Version}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateManualPass(&policy, tt.event)
			if !r.Required || r.CanSend != tt.ready {
				t.Fatalf("%+v", r)
			}
		})
	}
	old := policy
	old.Profile.House = "369"
	if evaluateManualPass(&policy, &gateEvent{Policy: old, Result: Result{Trigger: "manual", Status: "match", Version: Version}}).CanSend {
		t.Fatal("changed address rules must require another manual check")
	}
	old = policy
	old.Revision = "other-mailbox-config-change"
	if !evaluateManualPass(&policy, &gateEvent{Policy: old, Result: Result{Trigger: "manual", Status: "match", Version: Version}}).CanSend {
		t.Fatal("unrelated global revision must not invalidate the same rules")
	}
}
func TestShippingGateDoesNotTrustRawDataSummary(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	config, _ := json.Marshal(Settings{Config: Config{Profiles: []Profile{testProfile()}, Policies: []Policy{{AccountID: "mail", ProfileID: "office", Recipients: []string{"PD01"}, Enabled: true}}}, RequireManualPass: true})
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(config)))
	m.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows([]string{"bill_id", "policy", "result"}))
	states, err := NewStore(db).SendReadiness([]models.Bill{{ID: "00000000-0000-0000-0000-000000000001", BillType: "purchase", Source: "lazada_email", RawData: json.RawMessage(`{"imap_account_id":"mail","shipping_check_summary":{"status":"match","trigger":"manual"}}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if states["00000000-0000-0000-0000-000000000001"].CanSend {
		t.Fatal("summary without ledger evidence must not permit SML")
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestShippingGateIsOffForUnconfiguredInstances(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"profiles":[],"policies":[]}`))
	states, err := NewStore(db).SendReadiness([]models.Bill{{ID: "bill", BillType: "purchase", Source: "lazada_email"}})
	if err != nil || states["bill"].Required || !states["bill"].CanSend {
		t.Fatalf("%+v %v", states, err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestShippingReadinessBatchesSourcesAndKeepsLedgerAuthority(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := testProfile()
	config, _ := json.Marshal(Settings{RequireManualPass: true, Config: Config{Profiles: []Profile{p}, Policies: []Policy{{AccountID: "mail", ProfileID: p.ID, Recipients: []string{"PD01"}, Enabled: false}}}})
	policy, _ := json.Marshal(Enrollment{Profile: p, Recipients: []string{"PD01"}})
	passed, _ := json.Marshal(Result{Trigger: "manual", Status: "match", Version: Version})
	automatic, _ := json.Marshal(Result{Status: "match", Version: Version})
	m.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(config)))
	m.ExpectQuery("SELECT DISTINCT ON").WillReturnRows(sqlmock.NewRows([]string{"bill_id", "policy", "result"}).AddRow("one", policy, passed).AddRow("two", policy, automatic))
	states, err := NewStore(db).SendReadiness([]models.Bill{
		{ID: "one", BillType: "purchase", Source: "lazada_email", RawData: []byte(`{"imap_account_id":"mail"}`)},
		{ID: "two", BillType: "purchase", Source: "shopee_shipped", RawData: []byte(`{"imap_account_id":"mail"}`)},
		{ID: "ordinary", BillType: "sale", Source: "line"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || !states["one"].CanSend || states["two"].CanSend || !states["two"].Required {
		t.Fatalf("%+v", states)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
