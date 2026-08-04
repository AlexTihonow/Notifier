package notifier

import (
	"bytes"
	"encoding/json"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"my-mailer/internal/configs"
)

type pushRequest struct {
	Scenario []pushScenario `json:"scenario"`
}

type pushScenario struct {
	Channel    string          `json:"channel"`
	Sender     string          `json:"sender"`
	Recipients []pushRecipient `json:"recipients"`
	Message    pushMessage     `json:"message"`
}

type pushRecipient struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type pushMessage struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type PushSender struct {
	cfg    configs.Config
	client *http.Client
}

func newPushSender(cfg configs.Config) *PushSender {
	return &PushSender{
		cfg:    cfg,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *PushSender) send(recipients []string, subject string, body string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("список получателей пуст")
	}

	var list []pushRecipient
	for _, address := range recipients {
		list = append(list, pushRecipient{Type: "email", Value: address})
	}

	request := pushRequest{
		Scenario: []pushScenario{{
			Channel:    "both",
			Sender:     "seasons.rudn.ru",
			Recipients: list,
			Message: pushMessage{
				Title: subject,
				Text:  body,
			},
		}},
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		canRetry, err := s.doRequest(payload)
		if err == nil {
			return nil
		}

		lastErr = err
		if !canRetry {
			return err
		}

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}

	return lastErr
}

func (s *PushSender) doRequest(payload []byte) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://push.rudn.ru/api/v1/push", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", s.cfg.RudnAPIKey)
	request.Header.Set("x-client-login", s.cfg.RudnClientLogin)

	response, err := s.client.Do(request)
	if err != nil {
		return true, fmt.Errorf("запрос к api не прошёл: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return false, nil
	}

	answer, _ := io.ReadAll(io.LimitReader(response.Body, 512))
	err = fmt.Errorf("api вернуло код %d: %s", response.StatusCode, strings.TrimSpace(string(answer)))

	canRetry := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	return canRetry, err
}
