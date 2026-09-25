package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// sendTimeout bounds the whole SMTP exchange (connect + handshake + auth +
// data transfer). Notifications are best-effort (see Processor.notifyBestEffort)
// and must never block the worker's single-threaded queue consumer — without
// this, a slow or misconfigured SMTP host stalls every subsequent video.
const sendTimeout = 10 * time.Second

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

	conn, err := net.DialTimeout("tcp", addr, sendTimeout)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(sendTimeout)); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	} else if m.user != "" {
		// Credentials must never go out in cleartext (PlainAuth sends them
		// unencrypted). Refuse rather than silently downgrade.
		return fmt.Errorf("smtp server does not support STARTTLS; refusing to send credentials in cleartext")
	}

	if m.user != "" {
		auth := smtp.PlainAuth("", m.user, m.password, m.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	from := sanitizeHeader(m.from)
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", from, to, subject, body))
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write smtp body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close smtp data: %w", err)
	}
	return client.Quit()
}
