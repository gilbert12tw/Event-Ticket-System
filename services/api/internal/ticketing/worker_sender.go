package ticketing

import (
	"context"
	"errors"
)

const notificationSenderPanicError = "notification sender panic"

func sendNotificationSafely(ctx context.Context, sender NotificationSender, message DeliveryMessage) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New(notificationSenderPanicError)
		}
	}()
	return sender.Send(ctx, message)
}
