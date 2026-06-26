package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const apiTimeout = 5 * time.Second

type Service struct {
	botToken string
	chatID   string
	client   *http.Client
}

func New(botToken, chatID string) *Service {
	return &Service{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: apiTimeout},
	}
}

func (s *Service) PushAdmin(text string) error {
	if s.botToken == "" || s.chatID == "" {
		return nil
	}

	body, _ := json.Marshal(map[string]string{
		"chat_id": s.chatID,
		"text":    text,
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}
	return nil
}
