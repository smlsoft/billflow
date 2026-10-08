package repository

import "testing"

func TestExtractShopeeMarketplaceOrderURLFromSendGridOrderAnchor(t *testing.T) {
	trackedURL := "https://u35776146.ct.sendgrid.net/ls/click?upn=u001.synthetic-order-token"
	html := `
		<a href="https://u35776146.ct.sendgrid.net/ls/click?upn=u001.first-order-token">#2610071VH42598</a>
		<a href="` + trackedURL + `"><font color="#e67e22"> #2610071VH4259C </font></a>
		<a href="https://u35776146.ct.sendgrid.net/ls/click?upn=u001.next-order-token">#2610071VH4259D</a>
	`
	got := ExtractShopeeMarketplaceOrderURL("", html, "2610071VH4259C")
	if got != trackedURL {
		t.Fatalf("url = %q, want %q", got, trackedURL)
	}
}

func TestExtractShopeeMarketplaceOrderURLRejectsSendGridAnchorForDifferentOrder(t *testing.T) {
	html := `<a href="https://u35776146.ct.sendgrid.net/ls/click?upn=u001.other-order-token">#2610071VH42598</a>`
	if got := ExtractShopeeMarketplaceOrderURL("", html, "2610071VH4259C"); got != "" {
		t.Fatalf("url = %q, want empty", got)
	}
}

func TestExtractShopeeMarketplaceOrderURLRejectsLookalikeSendGridHost(t *testing.T) {
	html := `<a href="https://u35776146.ct.sendgrid.net.evil.example/ls/click?upn=u001.token">#2610071VH4259C</a>`
	if got := ExtractShopeeMarketplaceOrderURL("", html, "2610071VH4259C"); got != "" {
		t.Fatalf("url = %q, want empty", got)
	}
}

func TestExtractShopeeMarketplaceOrderURLFromRedirectNearOrderBlock(t *testing.T) {
	html := `
		<a href="https://th.shp.ee/open/noti_email?redir=https%3A%2F%2Fshopee.co.th%2Funiversal-link%2Fuser%2Fpurchase%2Forder%2F234608149246041%2F%3Fdeep_and_deferred%3D1%26shopid%3D123%26utm_source%3Dnoti">ดูคำสั่งซื้อ</a>
		<table><tr><td>หมายเลขคำสั่งซื้อ:</td><td>#260608HPC8A42R</td></tr></table>
	`
	got := ExtractShopeeMarketplaceOrderURL("", html, "260608HPC8A42R")
	want := "https://shopee.co.th/user/purchase/order/234608149246041?type=6"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestExtractShopeeMarketplaceOrderURLDoesNotGuessWhenOrderMissing(t *testing.T) {
	html := `<a href="https://shopee.co.th/universal-link/user/purchase/order/234608149246041/">ดูคำสั่งซื้อ</a>`
	if got := ExtractShopeeMarketplaceOrderURL("", html, "260608HPC8A42R"); got != "" {
		t.Fatalf("url = %q, want empty", got)
	}
}

func TestCanonicalShopeeMarketplaceOrderURLRejectsNonMarketplaceHost(t *testing.T) {
	raw := `https://example.test/user/purchase/order/234608149246041`
	if got, ok := CanonicalShopeeMarketplaceOrderURL(raw); ok || got != "" {
		t.Fatalf("url = %q ok=%v, want empty false", got, ok)
	}
}

func TestCanonicalLazadaMarketplaceOrderURLStripsBuyerEmailAndTracking(t *testing.T) {
	raw := `https://c.lazada.co.th/t/c.ZcDvYn?sub_id1=promotion&url=https%3A%2F%2Fmy.lazada.co.th%2Fcustomer%2Forder%2Fview%2F%3FtradeOrderId%3D1110789339595692%26buyerEmail%3Dpd.thaisunsport2%40gmail.com&lstg.name=open_app_fallback_msite`
	got, ok := CanonicalLazadaMarketplaceOrderURL(raw, "1110789339595692")
	if !ok {
		t.Fatal("expected lazada order URL")
	}
	want := "https://my.lazada.co.th/customer/order/view/?tradeOrderId=1110789339595692"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestCanonicalLazadaMarketplaceOrderURLRejectsWrongOrder(t *testing.T) {
	raw := `https://my.lazada.co.th/customer/order/view/?tradeOrderId=1110789339595692&buyerEmail=x@example.test`
	if got, ok := CanonicalLazadaMarketplaceOrderURL(raw, "9999999999999999"); ok || got != "" {
		t.Fatalf("url = %q ok=%v, want empty false", got, ok)
	}
}

func TestCanonicalLazadaMarketplaceOrderURLRejectsNonMarketplaceHost(t *testing.T) {
	raw := `https://example.test/customer/order/view/?tradeOrderId=1110789339595692`
	if got, ok := CanonicalLazadaMarketplaceOrderURL(raw, "1110789339595692"); ok || got != "" {
		t.Fatalf("url = %q ok=%v, want empty false", got, ok)
	}
}
