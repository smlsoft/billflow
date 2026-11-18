package shippingcheck

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"billflow/internal/models"
	"github.com/lib/pq"
)

type gateEvent struct {
	Policy Enrollment
	Result Result
}

func eligibleForShippingGate(b models.Bill) bool {
	return b.BillType == "purchase" && (b.Source == "shopee_shipped" || b.Source == "lazada_email")
}

// SendReadiness reads the authoritative ledger, never a client/raw_data claim.
// One settings read and one batched latest-event query serve an entire list page.
func (s *Store) SendReadiness(bills []models.Bill) (map[string]models.ShippingSendCheck, error) {
	states := map[string]models.ShippingSendCheck{}
	eligible := []models.Bill{}
	ids := []string{}
	for _, b := range bills {
		if eligibleForShippingGate(b) {
			eligible = append(eligible, b)
			ids = append(ids, b.ID)
		}
	}
	if len(ids) == 0 {
		return states, nil
	}
	config, err := s.Load()
	if err != nil {
		return nil, err
	}
	if !config.RequireManualPass {
		for _, b := range eligible {
			states[b.ID] = models.ShippingSendCheck{CanSend: true}
		}
		return states, nil
	}
	latest := map[string]gateEvent{}
	rows, err := s.db.Query(`SELECT DISTINCT ON (bill_id) bill_id,policy,result FROM bill_shipping_checks WHERE bill_id=ANY($1::uuid[]) ORDER BY bill_id,created_at DESC,id DESC`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var policy, result []byte
		var e gateEvent
		if err = rows.Scan(&id, &policy, &result); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(policy, &e.Policy); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(result, &e.Result); err != nil {
			return nil, err
		}
		latest[id] = e
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, b := range eligible {
		var raw struct {
			AccountID string `json:"imap_account_id"`
		}
		if err = json.Unmarshal(b.RawData, &raw); err != nil {
			return nil, err
		}
		policy := policyFromSettings(config, raw.AccountID)
		var event *gateEvent
		if e, ok := latest[b.ID]; ok {
			event = &e
		}
		states[b.ID] = evaluateManualPass(policy, event)
	}
	return states, nil
}
func evaluateManualPass(policy *Enrollment, latest *gateEvent) models.ShippingSendCheck {
	r := models.ShippingSendCheck{Required: true}
	switch {
	case policy == nil:
		r.Reason = "ยังไม่มีกติกาจัดส่งพร้อมใช้ของกล่องอีเมลนี้ ให้ผู้ดูแลตั้งค่าโปรไฟล์และชื่อผู้รับก่อน"
	case latest == nil:
		r.Reason = "ต้องกดตรวจข้อมูลจัดส่งและผลต้องผ่านก่อนส่ง SML"
	case latest.Result.Trigger != "manual":
		r.Reason = "ต้องกดตรวจข้อมูลจัดส่งให้ผ่าน ผลตรวจอัตโนมัติอย่างเดียวยังส่ง SML ไม่ได้"
	case latest.Result.Status != "match":
		r.Reason = "ผลตรวจจัดส่งล่าสุดยังไม่ผ่าน แก้ไขหรือทบทวนข้อมูล แล้วกดตรวจอีกครั้งก่อนส่ง SML"
	case latest.Result.Version != Version:
		r.Reason = "ตัวตรวจจัดส่งอัปเดตแล้ว กรุณากดตรวจข้อมูลจัดส่งอีกครั้งก่อนส่ง SML"
	case !sameShippingRules(*policy, latest.Policy):
		r.Reason = "กติกาจัดส่งของกล่องอีเมลเปลี่ยนแล้ว กรุณากดตรวจอีกครั้งก่อนส่ง SML"
	default:
		r.CanSend = true
	}
	return r
}
func sameShippingRules(a, b Enrollment) bool {
	// Global revision and profile display labels are not changes to this bill's rules.
	clean := func(e Enrollment) Enrollment {
		e.Revision = ""
		e.Profile.ID = ""
		e.Profile.Name = ""
		e.Profile.House = strings.TrimSpace(e.Profile.House)
		e.Profile.Moo = strings.TrimSpace(e.Profile.Moo)
		e.Profile.Postcode = strings.TrimSpace(e.Profile.Postcode)
		e.Profile.District = norm(e.Profile.District)
		e.Profile.Amphoe = norm(e.Profile.Amphoe)
		e.Profile.Province = norm(e.Profile.Province)
		e.Recipients = normalizedSet(e.Recipients)
		e.Profile.DistrictAliases = normalizedSet(e.Profile.DistrictAliases)
		e.Profile.AmphoeAliases = normalizedSet(e.Profile.AmphoeAliases)
		e.Profile.ProvinceAliases = normalizedSet(e.Profile.ProvinceAliases)
		return e
	}
	return reflect.DeepEqual(clean(a), clean(b))
}
func normalizedSet(values []string) []string {
	unique := map[string]bool{}
	for _, v := range values {
		unique[norm(v)] = true
	}
	out := make([]string, 0, len(unique))
	for v := range unique {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
