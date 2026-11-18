package repository

// Shared by the counters and list filters: historical warnings are not open work.
const attentionWorkSQL = `b.archived_at IS NULL AND b.status IN ('pending', 'needs_review', 'failed') AND b.bill_type = 'purchase' AND b.source IN ('shopee_shipped', 'lazada_email')`
const shippingAttentionSQL = `b.raw_data->'shipping_check_summary'->>'status' IN ('mismatch', 'unverifiable')`
const emailAttentionSQL = `EXISTS (
 SELECT 1 FROM marketplace_email_groups meg
 WHERE meg.source = b.source
 AND meg.message_id = COALESCE(NULLIF(b.raw_data->>'email_message_id', ''), NULLIF(b.raw_data->>'message_id', ''))
 AND meg.status IN ('processing', 'attention')
)`

type BillAttentionCounts struct {
	Shipping int `json:"shipping"`
	Email    int `json:"email"`
}

// Count bills, not email groups. EXISTS prevents multi-order/join duplication.
// Independent of search, other filters and pagination; one snapshot for both.
func (r *BillRepo) AttentionCounts() (BillAttentionCounts, error) {
	var counts BillAttentionCounts
	err := r.db.QueryRow(`SELECT COUNT(*) FILTER (WHERE `+shippingAttentionSQL+`), COUNT(*) FILTER (WHERE `+emailAttentionSQL+`) FROM bills b WHERE `+attentionWorkSQL).Scan(&counts.Shipping, &counts.Email)
	return counts, err
}
