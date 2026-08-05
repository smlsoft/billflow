package googledrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRenderedPDFBytes = 20 * 1024 * 1024

type PDFRenderResult struct {
	PDF      []byte
	Warnings []string
}

type PDFRenderer interface {
	Health(context.Context) error
	Render(context.Context, string) (PDFRenderResult, error)
}

type httpPDFRenderer struct {
	baseURL string
	token   string
	client  *http.Client
}

func newHTTPPDFRenderer(baseURL, token string) *httpPDFRenderer {
	return &httpPDFRenderer{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 75 * time.Second},
	}
}

func (r *httpPDFRenderer) Health(ctx context.Context) error {
	if r == nil || r.baseURL == "" {
		return errors.New("ยังไม่ได้ตั้งค่า EMAIL_PDF_RENDERER_URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	if r.token != "" {
		req.Header.Set("X-BillFlow-Renderer-Token", r.token)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("renderer health HTTP %d", res.StatusCode)
	}
	return nil
}

func (r *httpPDFRenderer) Render(ctx context.Context, html string) (PDFRenderResult, error) {
	if r == nil || r.baseURL == "" {
		return PDFRenderResult{}, errors.New("ยังไม่ได้ตั้งค่า EMAIL_PDF_RENDERER_URL")
	}
	if r.token == "" {
		return PDFRenderResult{}, errors.New("ยังไม่ได้ตั้งค่า EMAIL_PDF_RENDERER_TOKEN")
	}
	body, err := json.Marshal(struct {
		HTML string `json:"html"`
	}{HTML: html})
	if err != nil {
		return PDFRenderResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/render", bytes.NewReader(body))
	if err != nil {
		return PDFRenderResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BillFlow-Renderer-Token", r.token)
	res, err := r.client.Do(req)
	if err != nil {
		return PDFRenderResult{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxRenderedPDFBytes+1))
	if err != nil {
		return PDFRenderResult{}, err
	}
	if res.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(data))
		if len(message) > 300 {
			message = message[:300]
		}
		if message == "" {
			message = "renderer ไม่ได้ส่งรายละเอียด"
		}
		return PDFRenderResult{}, fmt.Errorf("สร้าง PDF ไม่สำเร็จ: %s", message)
	}
	if len(data) == 0 || len(data) > maxRenderedPDFBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return PDFRenderResult{}, errors.New("renderer ส่งไฟล์ PDF ไม่ถูกต้อง")
	}
	return PDFRenderResult{PDF: data, Warnings: parseRendererWarnings(res.Header.Get("X-BillFlow-Render-Warnings"))}, nil
}

func parseRendererWarnings(encoded string) []string {
	if encoded == "" {
		return nil
	}
	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return nil
	}
	var warnings []string
	if json.Unmarshal([]byte(decoded), &warnings) != nil {
		return nil
	}
	if len(warnings) > 10 {
		return warnings[:10]
	}
	return warnings
}
