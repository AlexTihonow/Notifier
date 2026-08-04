package notifier

import (
	"time"
	"crypto/tls"
	"fmt"
	"sync"
	mail "github.com/xhit/go-simple-mail/v2"
	"strings"
	"my-mailer/internal/config"
)

type mailer struct {
	cfg    config.Config
	mu     sync.Mutex
	client *mail.SMTPClient
}

func newMailer(cfg config.Config) (*mailer, error) {
	return &mailer{cfg: cfg}, nil
}

func connect(cfg config.Config) (*mail.SMTPClient, error) {
	server := mail.NewSMTPClient()

	server.Host = cfg.EmailHost
	server.Port = cfg.EmailPort
	server.Username = cfg.EmailUsername
	server.Password = cfg.EmailPassword
	server.Encryption = mail.EncryptionSSLTLS

	server.KeepAlive = true

	server.ConnectTimeout = 10 * time.Second

	server.SendTimeout = 10 * time.Second

	server.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	return server.Connect()
}

func (m *mailer) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
}

func (m *mailer) ensure() (*mail.SMTPClient, error) {
	if m.client != nil {
		return m.client, nil
	}
	client, err := connect(m.cfg)
	if err != nil {
		return nil, err
	}
	m.client = client
	return m.client, nil
}

func (m *mailer) send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, err := m.ensure()
	if err != nil {
		return fmt.Errorf("подключение к SMTP не удалось: %w", err)
	}

	if err := m.trySend(client, to, subject, body); err == nil {
		return nil
	} else {
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		client, cerr := m.ensure()
		if cerr != nil {
			return fmt.Errorf("переподключение к SMTP не удалось: %w (исходная: %v)", cerr, err)
		}
		return m.trySend(client, to, subject, body)
	}
}

func (m *mailer) trySend(client *mail.SMTPClient, to, subject, body string) error {
	email := mail.NewMSG()
	email.SetFrom(m.cfg.EmailFrom).
		AddTo(to).
		SetSubject(subject)

	email.SetBody(mail.TextHTML, body)

	email.SetDSN([]mail.DSN{mail.SUCCESS, mail.FAILURE}, false)

	if email.Error != nil {
		return email.Error
	}
	return email.Send(client)
}

func splitRecipients(value string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key := strings.ToLower(part)
		if seen[key] {
			continue
		}

		seen[key] = true
		result = append(result, part)
	}
	return result
}