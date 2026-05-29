package ticketing

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
		Send(context.Background(), DeliveryMessage{To: "e1001@cets.local", Subject: "accepted", Body: "body"})

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
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			writeSMTPLine(t, writer, "250 localhost")
		case strings.HasPrefix(upper, "MAIL FROM:"), strings.HasPrefix(upper, "RCPT TO:"):
			writeSMTPLine(t, writer, "250 ok")
		case upper == "DATA":
			writeSMTPLine(t, writer, "354 end data")
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
			}
			writeSMTPLine(t, writer, "250 queued")
		case upper == "QUIT":
			return
		default:
			writeSMTPLine(t, writer, "250 ok")
		}
	}
}

func writeSMTPLine(t *testing.T, writer *bufio.Writer, line string) {
	t.Helper()
	_, err := fmt.Fprintf(writer, "%s\r\n", line)
	require.NoError(t, err)
	require.NoError(t, writer.Flush())
}
