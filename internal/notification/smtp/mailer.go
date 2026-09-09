package smtp

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// sanitizeHeader strips CR/LF so a value that ends up in a header (e.g. a
// user-supplied file name in the Subject) can't inject extra headers or
// SMTP commands.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

type Mailer struct {
	host     string
	port     string
	user     string
	password string
	from     string
}

func NewMailer(host, port, user, password, from string) *Mailer {
	return &Mailer{host: host, port: port, user: user, password: password, from: from}
}

func (m *Mailer) NotifyCompleted(ctx context.Context, to, fileName string) error {
	fileName = sanitizeHeader(fileName)
	subject := fmt.Sprintf("Your video is ready: %s", fileName)
	body := fmt.Sprintf(
		"Hi,\n\nYour video %q has finished processing. Log in to the app to view the status and download the result.\n\n— FIAP X",
		fileName,
	)
	return m.send(to, subject, body)
}

func (m *Mailer) NotifyFailed(ctx context.Context, to, fileName, reason string) error {
	fileName = sanitizeHeader(fileName)
	reason = sanitizeHeader(reason)
	subject := fmt.Sprintf("Your video failed to process: %s", fileName)
	body := fmt.Sprintf(
		"Hi,\n\nWe couldn't process your video %q.\n\nReason: %s\n\nYou can retry from the app.\n\n— FIAP X",
		fileName, reason,
	)
	return m.send(to, subject, body)
}

func (m *Mailer) send(to, subject, body string) error {
	addr := m.host + ":" + m.port

	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.password, m.host)
	}

	from := sanitizeHeader(m.from)
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)

	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", from, to, subject, body))
	return smtp.SendMail(addr, auth, m.from, []string{to}, msg)
}
