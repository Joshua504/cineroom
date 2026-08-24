package mail

import (
	"fmt"
	"net/smtp"
)

type Mailer struct {
	Host, Port, Username, Password, From string
}

func (m Mailer) SendOTP(to, code string) error {
	if m.Host == "" {
		return fmt.Errorf("SMTP_HOST is not configured")
	}
	addr := m.Host + ":" + m.Port
	auth := smtp.Auth(nil)
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	message := []byte("From: " + m.From + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: Cineroom verification code\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Your Cineroom verification code is " + code + ". It expires in 10 minutes.\r\n")
	return smtp.SendMail(addr, auth, m.From, []string{to}, message)
}
