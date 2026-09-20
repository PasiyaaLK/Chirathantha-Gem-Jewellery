// Package notifier defines the outbound customer-notification contract
// (Section 6 of the proposal: "Notification Service ... integrates with
// an email service and/or SMS gateway"), real provider implementations,
// and email template rendering.
//
// The interface is deliberately about channels (email, SMS), not about
// order-approval specifically, so it stays reusable for any future
// transactional message — shipping updates, password resets, etc.
// Order-decision message/template composition lives in message.go
// (SMS) and templates.go (email), one layer up from the raw send calls.
package notifier

import "context"

// NotificationService sends a message to a customer over a given
// channel. Implementations must be safe for concurrent use — the
// approval service invokes these from a goroutine detached from the
// originating HTTP request, and multiple approvals can be in flight at
// once.
type NotificationService interface {
	// SendEmail delivers a transactional email. htmlBody is the primary
	// rendered version most clients display; textBody is the plain-text
	// fallback shown by clients that don't render HTML, read by
	// accessibility tools, and used as a spam-filter deliverability
	// signal (HTML-only transactional mail is itself somewhat spammy-
	// looking). Both should always be provided — see templates.go's
	// RenderApprovalEmail/RenderDeclineEmail, which always render both
	// from the same data so they can never drift apart.
	SendEmail(ctx context.Context, to, subject, htmlBody, textBody string) error

	// SendSMS delivers body as a text message to the given phone number.
	// Implementations backed by a real gateway (Twilio, SNS, ...) are
	// responsible for any provider-specific length/segment limits.
	SendSMS(ctx context.Context, to, body string) error
}
