package sml

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// PartyConfig holds the auth headers shared by GET /v3/api/customer and supplier.
// All fields reuse the SHOPEE_SML_* config (party master lives on the same SML 248
// instance as saleinvoice/purchaseorder).
type PartyConfig struct {
	BaseURL    string
	GUID       string
	Provider   string
	ConfigFile string
	Database   string
}

// Party is the subset of customer/supplier fields BillFlow needs for the
// per-channel default picker. Customer responses lack address/telephone in
// the list view (they appear only in /customer/{code}) so those columns may
// be empty; supplier responses do include them.
type Party struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	TaxID     string `json:"tax_id,omitempty"`
	Telephone string `json:"telephone,omitempty"`
	Address   string `json:"address,omitempty"`
}

// PartyClient is a paginated GET-only client for SML 248 party master.
//
// Quick-create (POST) is intentionally NOT supported — the legacy /restapi/
// schema requires ~25 fields with non-obvious naming and the v3 endpoint
// returns NullPointerException without documenting the required shape.
// Admin must create parties in SML manually then click "refresh" in BillFlow.
type PartyClient struct {
	cfg        PartyConfig
	httpClient *http.Client
	log        *zap.Logger
}

func NewPartyClient(cfg PartyConfig, log *zap.Logger) *PartyClient {
	return &PartyClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		log:        log,
	}
}

func (c *PartyClient) headers() map[string]string {
	return map[string]string{
		"guid":           c.cfg.GUID,
		"provider":       c.cfg.Provider,
		"configFileName": c.cfg.ConfigFile,
		"databaseName":   c.cfg.Database,
		"Accept":         "application/json",
	}
}

type partyListResponse struct {
	Success bool    `json:"success"`
	Data    []Party `json:"data"`
	Pages   struct {
		Size        int `json:"size"`
		Page        int `json:"page"`
		TotalRecord int `json:"total_record"`
		MaxPage     int `json:"max_page"`
	} `json:"pages"`
}

// fetchPage returns one page of {endpoint} (e.g. "customer" or "supplier").
func (c *PartyClient) fetchPage(ctx context.Context, endpoint string, page, size int) (*partyListResponse, error) {
	u := fmt.Sprintf("%s/SMLJavaRESTService/v3/api/%s?page=%d&size=%d",
		c.cfg.BaseURL, endpoint, page, size)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers() {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sml %s fetch page %d: %w", endpoint, page, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sml %s read body: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sml %s page %d HTTP %d: %s",
			endpoint, page, resp.StatusCode, string(body))
	}
	var pr partyListResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("sml %s decode page %d: %w", endpoint, page, err)
	}
	if !pr.Success {
		return nil, fmt.Errorf("sml %s page %d: success=false", endpoint, page)
	}
	return &pr, nil
}

// fetchAll loops pages until total_record reached.
func (c *PartyClient) fetchAll(ctx context.Context, endpoint string) ([]Party, error) {
	const pageSize = 200
	var out []Party
	for page := 1; ; page++ {
		pr, err := c.fetchPage(ctx, endpoint, page, pageSize)
		if err != nil {
			return nil, err
		}
		out = append(out, pr.Data...)
		if len(out) >= pr.Pages.TotalRecord || len(pr.Data) == 0 {
			break
		}
		if page >= pr.Pages.MaxPage {
			break
		}
	}
	return out, nil
}

// FetchAllCustomers returns the full customer list (all pages). 1004 records
// at the time of writing — ~5 pages × 200 each.
func (c *PartyClient) FetchAllCustomers(ctx context.Context) ([]Party, error) {
	return c.fetchAll(ctx, "customer")
}

// FetchAllSuppliers returns the full supplier list. 500 records at writing.
func (c *PartyClient) FetchAllSuppliers(ctx context.Context) ([]Party, error) {
	return c.fetchAll(ctx, "supplier")
}

type partyDetailResponse struct {
	Success bool  `json:"success"`
	Data    Party `json:"data"`
}

// GetCustomer fetches a single customer by code. Returns nil (no error) when
// SML responds with 404 / data:null.
func (c *PartyClient) GetCustomer(ctx context.Context, code string) (*Party, error) {
	return c.getOne(ctx, "customer", code)
}

// GetSupplier fetches a single supplier by code.
func (c *PartyClient) GetSupplier(ctx context.Context, code string) (*Party, error) {
	return c.getOne(ctx, "supplier", code)
}

func (c *PartyClient) getOne(ctx context.Context, endpoint, code string) (*Party, error) {
	u := fmt.Sprintf("%s/SMLJavaRESTService/v3/api/%s/%s",
		c.cfg.BaseURL, endpoint, url.PathEscape(code))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers() {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sml %s/%s: %w", endpoint, code, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sml %s/%s HTTP %d: %s",
			endpoint, code, resp.StatusCode, string(body))
	}
	var pr partyDetailResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}
	if !pr.Success || pr.Data.Code == "" {
		return nil, nil
	}
	return &pr.Data, nil
}

// IsConfigured reports whether the client has the SML 248 base URL + headers
// needed to fetch parties. Used at boot to decide whether to start the cache.
func (c *PartyClient) IsConfigured() bool {
	return c.cfg.BaseURL != "" && c.cfg.GUID != "" &&
		c.cfg.Provider != "" && c.cfg.ConfigFile != "" && c.cfg.Database != ""
}
