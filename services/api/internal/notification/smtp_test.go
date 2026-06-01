package notification

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMTPNotificationSenderReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SMTPNotificationSender{Host: "127.0.0.1", Port: 1, From: "noreply@cets.local"}.
		Send(ctx, ticketing.DeliveryMessage{To: "e1001@cets.local", Subject: "test", Body: "body"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSMTPNotificationSenderRedirectsRecipientInEnvelopeAndBody(t *testing.T) {
	sender := SMTPNotificationSender{
		From:       "noreply@cets.local",
		RedirectTo: "notifications@cets.local",
	}
	message := ticketing.DeliveryMessage{
		To:             "e1001@cets.local",
		Subject:        "test",
		Body:           "body",
		IdempotencyKey: "del_abc123",
	}

	recipient := sender.deliveryRecipient(message)
	body := sender.deliveryBody(message, recipient)

	assert.Equal(t, "notifications@cets.local", recipient)
	assert.NotContains(t, body, "e1001@cets.local", "body leaked employee recipient")
	assert.Contains(t, body, "To: notifications@cets.local")
	assert.Contains(t, body, "Message-ID: <del_abc123@cets.local>")
	assert.Contains(t, body, "X-Idempotency-Key: del_abc123")
}

func TestSMTPNotificationSenderTreatsDataAcceptedQuitFailureAsSent(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go serveSMTPDataAcceptedQuitClosed(t, listener)
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	err = SMTPNotificationSender{Host: host, Port: port, From: "noreply@cets.local"}.
		Send(context.Background(), ticketing.DeliveryMessage{To: "e1001@cets.local", Subject: "accepted", Body: "body"})

	require.NoError(t, err)
}

func serveSMTPDataAcceptedQuitClosed(t *testing.T, listener net.Listener) {
	t.Helper()
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTPLine(t, writer, "220 cets test smtp")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if handleSMTPCommand(t, reader, writer, line) {
			return
		}
	}
}

func handleSMTPCommand(t *testing.T, reader *bufio.Reader, writer *bufio.Writer, line string) bool {
	t.Helper()
	upper := strings.ToUpper(strings.TrimSpace(line))
	switch {
	case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
		writeSMTPLine(t, writer, "250 localhost")
	case strings.HasPrefix(upper, "MAIL FROM:"), strings.HasPrefix(upper, "RCPT TO:"):
		writeSMTPLine(t, writer, "250 ok")
	case upper == "DATA":
		writeSMTPLine(t, writer, "354 end data")
		consumeSMTPData(t, reader)
		writeSMTPLine(t, writer, "250 queued")
	case upper == "QUIT":
		return true
	default:
		writeSMTPLine(t, writer, "250 ok")
	}
	return false
}

func consumeSMTPData(t *testing.T, reader *bufio.Reader) {
	t.Helper()
	for {
		dataLine, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(dataLine) == "." {
			return
		}
	}
}

func writeSMTPLine(t *testing.T, writer *bufio.Writer, line string) {
	t.Helper()
	_, err := fmt.Fprintf(writer, "%s\r\n", line)
	require.NoError(t, err)
	require.NoError(t, writer.Flush())
}
