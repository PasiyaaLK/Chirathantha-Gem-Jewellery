package notifier

import (
	"context"
	"log/slog"
)

// LogNotifier is a NotificationService that writes to structured logs
// instead of calling a real provider. It's the default wiring for local
// development and for exercising the approval workflow end-to-end
// before provider credentials exist — and the automatic fallback in
// cmd/api/main.go's buildNotifier for whichever of email/SMS doesn't
// have credentials configured, so the app degrades gracefully instead
// of failing to start.
//
// Real providers: SendGridEmailNotifier and TwilioSMSNotifier (see
// providers.go), wired automatically once SENDGRID_API_KEY /
// TWILIO_ACCOUNT_SID etc. are set — see buildNotifier in cmd/api/main.go.
type LogNotifier struct{}

func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

func (n *LogNotifier) SendEmail(ctx context.Context, to, subject, htmlBody, textBody string) error {
	slog.InfoContext(ctx, "notifier: email (logged, not sent)",
		"to", to, "subject", subject, "text_preview", preview(textBody), "html_len", len(htmlBody))
	return nil
}

func (n *LogNotifier) SendSMS(ctx context.Context, to, body string) error {
	slog.InfoContext(ctx, "notifier: sms (logged, not sent)",
		"to", to, "body_preview", preview(body))
	return nil
}

func preview(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
