package notification

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"event-ticket-system/internal/ticketing"
)

type SMTPNotificationSender struct {
	Host       string
	Port       int
	From       string
	RedirectTo string
}

func (s SMTPNotificationSender) Send(ctx context.Context, message ticketing.DeliveryMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	recipient := s.deliveryRecipient(message)
	body := s.deliveryBody(message, recipient)
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return contextError(ctx, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return contextError(ctx, err)
	}
	defer func() { _ = client.Close() }()
	if err := client.Mail(s.From); err != nil {
		return contextError(ctx, err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return contextError(ctx, err)
	}
	writer, err := client.Data()
	if err != nil {
		return contextError(ctx, err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return contextError(ctx, err)
	}
	if err := writer.Close(); err != nil {
		return contextError(ctx, err)
	}
	_ = client.Quit()
	return nil
}

func (s SMTPNotificationSender) deliveryRecipient(message ticketing.DeliveryMessage) string {
	if recipient := strings.TrimSpace(s.RedirectTo); recipient != "" {
		return recipient
	}
	return message.To
}

func (s SMTPNotificationSender) deliveryBody(message ticketing.DeliveryMessage, recipient string) string {
	headers := []string{
		fmt.Sprintf("From: %s", s.From),
		fmt.Sprintf("To: %s", recipient),
		fmt.Sprintf("Subject: %s", message.Subject),
	}
	if key := strings.TrimSpace(message.IdempotencyKey); key != "" {
		headers = append(headers, fmt.Sprintf("Message-ID: <%s@cets.local>", key))
		headers = append(headers, fmt.Sprintf("X-Idempotency-Key: %s", key))
	}
	return fmt.Sprintf("%s\r\n\r\n%s\r\n", strings.Join(headers, "\r\n"), message.Body)
}

func contextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}
