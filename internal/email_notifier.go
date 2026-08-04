package notifier

import (
	"log"
	"time"
	"crypto/tls"
	mail "github.com/xhit/go-simple-mail/v2"
	"strings"
	"my-mailer/internal/configs"
)

func send (conf configs.Config, to string, subject string, body string) error {
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		err := sendOnce(conf, to, subject, body)
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}

	return lastErr
}

func sendOnce (conf configs.Config, to string, subject string, body string) error {
	server := mail.NewSMTPClient()

	server.Host = conf.EmailHost
	server.Port = conf.EmailPort
	server.Username = conf.EmailUsername
	server.Password = conf.EmailPassword
	server.Encryption = mail.EncryptionSSLTLS

	server.KeepAlive = true

	server.ConnectTimeout = 10 * time.Second

	server.SendTimeout = 10 * time.Second

	server.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	smtpClient,err := server.Connect()

	if err != nil {
		return err
	}

	email := mail.NewMSG()
	email.SetFrom(conf.EmailFrom).
		AddTo(to).
		SetSubject(subject)

	email.SetBody(mail.TextHTML, body)

	email.SetDSN([]mail.DSN{mail.SUCCESS, mail.FAILURE}, false)

	if email.Error != nil{
		return email.Error
	}

	err = email.Send(smtpClient)
	if err != nil {
		return err
	} else {
		log.Println("Письмо отправлено")
	}

	return nil
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