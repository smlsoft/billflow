package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Client struct {
	apiKey        string
	model         string
	fallbackModel string
	audioModel    string
	httpClient    *http.Client
}

func NewClient(apiKey, model, fallbackModel, audioModel string) *Client {
	return &Client{
		apiKey:        apiKey,
		model:         model,
		fallbackModel: fallbackModel,
		audioModel:    audioModel,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
	}
}

type ExtractedBill struct {
	DocType       string          `json:"doc_type"`
	CustomerName  string          `json:"customer_name"`
	CustomerPhone *string         `json:"customer_phone"`
	Items         []ExtractedItem `json:"items"`
	TotalAmount   *float64        `json:"total_amount"`
	Note          *string         `json:"note"`
	Confidence    float64         `json:"confidence"`
}

type ExtractedItem struct {
	RawName string   `json:"raw_name"`
	Qty     float64  `json:"qty"`
	Unit    string   `json:"unit"`
	Price   *float64 `json:"price"`
}

type openRouterRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *imageURLObj `json:"image_url,omitempty"`
}

type imageURLObj struct {
	URL string `json:"url"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ExtractText sends text to OpenRouter and returns parsed bill data
func (c *Client) ExtractText(text string) (*ExtractedBill, error) {
	return c.extract(c.model, []contentPart{
		{Type: "text", Text: ExtractPrompt},
		{Type: "text", Text: text},
	})
}

// ExtractImage sends base64 image to OpenRouter
func (c *Client) ExtractImage(base64Data, mimeType string) (*ExtractedBill, error) {
	return c.extract(c.model, []contentPart{
		{Type: "text", Text: ExtractPrompt},
		{Type: "image_url", ImageURL: &imageURLObj{
			URL: fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data),
		}},
	})
}

func (c *Client) extract(model string, parts []contentPart) (*ExtractedBill, error) {
	reqBody := openRouterRequest{
		Model: model,
		Messages: []message{
			{Role: "user", Content: parts},
		},
	}

	body, err := c.doRequest(reqBody)
	if err != nil {
		// Retry with fallback model
		if model != c.fallbackModel {
			return c.extract(c.fallbackModel, parts)
		}
		return nil, err
	}

	var bill ExtractedBill
	if err := json.Unmarshal([]byte(body), &bill); err != nil {
		// Non-JSON response — retry with fallback
		if model != c.fallbackModel {
			return c.extract(c.fallbackModel, parts)
		}
		return nil, fmt.Errorf("parse response: %w — body: %s", err, body)
	}
	return &bill, nil
}

// ExtractPDF sends a base64-encoded PDF to OpenRouter (Gemini Flash supports inline PDF)
func (c *Client) ExtractPDF(base64Data string) (*ExtractedBill, error) {
	return c.extract(c.model, []contentPart{
		{Type: "text", Text: ExtractPrompt},
		{Type: "image_url", ImageURL: &imageURLObj{
			URL: "data:application/pdf;base64," + base64Data,
		}},
	})
}

// TranscribeAudio calls OpenRouter Whisper endpoint and returns transcribed text
func (c *Client) TranscribeAudio(audioData []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", "audio.m4a")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(audioData); err != nil {
		return "", err
	}
	if err := w.WriteField("model", c.audioModel); err != nil {
		return "", err
	}
	w.Close()

	req, err := http.NewRequest("POST",
		"https://openrouter.ai/api/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcribe request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transcribe status %d: %s", resp.StatusCode, respData)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respData, &result); err != nil {
		return "", fmt.Errorf("parse transcription: %w — body: %s", err, respData)
	}
	return result.Text, nil
}

// GenerateInsight generates daily AI insight text
func (c *Client) GenerateInsight(statsJSON string) (string, error) {
	prompt := fmt.Sprintf(InsightPrompt, statsJSON)
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []message{
			{Role: "user", Content: []contentPart{{Type: "text", Text: prompt}}},
		},
	}
	return c.doRequest(reqBody)
}

// ChatMessage is a single turn in a conversation history
type ChatMessage struct {
	Role    string
	Content string
}

// SalesChatResult holds the chatbot reply and optional extracted order
// ChatSales / ChatSalesWithContext / ExtractOrderFromHistory were removed in
// session 13 along with the AI chatbot ("น้องบิล"). LINE conversations are now
// human-to-human via the /messages inbox; AI is only used for media extraction
// (ExtractImage / ExtractPDF / ExtractText / TranscribeAudio) which is still
// reachable from the chat inbox manual-trigger button + email pipeline.

func (c *Client) doRequest(reqBody openRouterRequest) (string, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openrouter request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter status %d: %s", resp.StatusCode, string(respData))
	}

	var orResp openRouterResponse
	if err := json.Unmarshal(respData, &orResp); err != nil {
		return "", fmt.Errorf("parse openrouter response: %w", err)
	}
	if len(orResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices from openrouter")
	}
	return orResp.Choices[0].Message.Content, nil
}
