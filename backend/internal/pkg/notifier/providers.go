package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// --- SendGrid (email) --------------------------------------------------

const defaultSendGridBaseURL = "https://api.sendgrid.com/v3"

// SendGridEmailNotifier sends transactional email via SendGrid's v3
// Mail Send API:
// https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send
//
// Only implements the email half of NotificationService — see
// TwilioSMSNotifier for SMS and CompositeNotifier for combining the two
// into one NotificationService.
//
// Swapping to Resend later (the other provider named in the proposal's
// suggested stack) means writing an equivalent ResendEmailNotifier
// against Resend's simpler REST shape (POST /emails, a flat JSON body)
// and changing one constructor call in cmd/api/main.go — nothing that
// calls NotificationService needs to know or care which provider is
// behind it.
type SendGridEmailNotifier struct {
	apiKey     string
	fromAddr   string
	fromName   string
	baseURL    string
	httpClient *http.Client
}

// SendGridOption configures a SendGridEmailNotifier at construction time.
type SendGridOption func(*SendGridEmailNotifier)

// WithSendGridBaseURL overrides the API base URL — used by tests to
// point at an httptest server instead of the real SendGrid API.
// Production code never needs this; it defaults to the real endpoint.
func WithSendGridBaseURL(u string) SendGridOption {
	return func(s *SendGridEmailNotifier) { s.baseURL = u }
}

// WithSendGridHTTPClient overrides the HTTP client (e.g. a shorter
// timeout in tests). Defaults to a 10s-timeout client.
func WithSendGridHTTPClient(client *http.Client) SendGridOption {
	return func(s *SendGridEmailNotifier) { s.httpClient = client }
}

// NewSendGridEmailNotifier builds a SendGrid-backed email sender.
// fromAddr must be a verified sender identity (or authenticated domain)
// in your SendGrid account — SendGrid rejects sends from unverified
// senders regardless of API key validity.
func NewSendGridEmailNotifier(apiKey, fromAddr, fromName string, opts ...SendGridOption) *SendGridEmailNotifier {
	s := &SendGridEmailNotifier{
		apiKey:     apiKey,
		fromAddr:   fromAddr,
		fromName:   fromName,
		baseURL:    defaultSendGridBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type sendGridEmailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sendGridPersonalization struct {
	To []sendGridEmailAddress `json:"to"`
}

type sendGridContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sendGridSendRequest struct {
	Personalizations []sendGridPersonalization `json:"personalizations"`
	From             sendGridEmailAddress      `json:"from"`
	Subject          string                    `json:"subject"`
	Content          []sendGridContent         `json:"content"`
}

type sendGridErrorItem struct {
	Message string `json:"message"`
	Field   string `json:"field"`
}

type sendGridErrorResponse struct {
	Errors []sendGridErrorItem `json:"errors"`
}

// SendEmail implements notifier.NotificationService.
func (s *SendGridEmailNotifier) SendEmail(ctx context.Context, to, subject, htmlBody, textBody string) error {
	payload := sendGridSendRequest{
		Personalizations: []sendGridPersonalization{{To: []sendGridEmailAddress{{Email: to}}}},
		From:             sendGridEmailAddress{Email: s.fromAddr, Name: s.fromName},
		Subject:          subject,
		// SendGrid wants text/plain listed before text/html when both
		// are present. Some clients render whichever MIME part they
		// find first that they support; plain text should always be
		// the safe fallback offered first.
		Content: []sendGridContent{
			{Type: "text/plain", Value: textBody},
			{Type: "text/html", Value: htmlBody},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notifier: marshal sendgrid request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/mail/send", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notifier: build sendgrid request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notifier: sendgrid request failed: %w", err)
	}
	defer resp.Body.Close()

	// SendGrid returns 202 Accepted on success, not 200 — anything
	// outside the 2xx range is an error.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notifier: sendgrid returned %d: %s", resp.StatusCode, decodeSendGridError(resp.Body))
	}

	return nil
}

func decodeSendGridError(body io.Reader) string {
	var errResp sendGridErrorResponse
	if err := json.NewDecoder(body).Decode(&errResp); err != nil || len(errResp.Errors) == 0 {
		return "unknown error"
	}
	msgs := make([]string, 0, len(errResp.Errors))
	for _, e := range errResp.Errors {
		if e.Field != "" {
			msgs = append(msgs, fmt.Sprintf("%s (%s)", e.Message, e.Field))
		} else {
			msgs = append(msgs, e.Message)
		}
	}
	return strings.Join(msgs, "; ")
}

// --- Twilio (SMS) --------------------------------------------------------

const defaultTwilioBaseURL = "https://api.twilio.com/2010-04-01"

// smsSegmentLimit is a soft cap applied before sending. Twilio splits
// (and bills) messages over roughly 160 GSM-7 characters into multiple
// segments; truncating here stops a verbose decision message from
// silently costing several times what was intended. This is a
// pragmatic client-side guard, not a substitute for checking your
// actual Twilio pricing/encoding if SMS volume matters to you —
// RenderDecisionSMS (message.go) already keeps messages short enough
// that this should rarely if ever trigger in normal operation.
const smsSegmentLimit = 300

// truncateSMSBody caps body at smsSegmentLimit bytes, leaving room for
// a trailing ellipsis. Two things the naive `body[:limit-1] + "…"`
// version got wrong (caught by TestTwilioSMSNotifier_SendSMS_TruncatesLongMessages):
// "…" (U+2026) is 3 bytes in UTF-8, not 1, so that version could
// overshoot the limit by 2 bytes; and slicing a string at an arbitrary
// byte offset can land in the middle of a multi-byte rune, producing
// invalid UTF-8. This backs off to the nearest rune boundary at or
// before the cut point.
func truncateSMSBody(body string) string {
	if len(body) <= smsSegmentLimit {
		return body
	}
	const ellipsis = "…"
	cut := smsSegmentLimit - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + ellipsis
}

// TwilioSMSNotifier sends SMS via the Twilio REST API:
// https://www.twilio.com/docs/sms/api/message-resource
//
// Only implements the SMS half of NotificationService — see
// SendGridEmailNotifier for email.
type TwilioSMSNotifier struct {
	accountSID string
	authToken  string
	fromNumber string
	baseURL    string
	httpClient *http.Client
}

// TwilioOption configures a TwilioSMSNotifier at construction time.
type TwilioOption func(*TwilioSMSNotifier)

// WithTwilioBaseURL overrides the API base URL — used by tests to point
// at an httptest server instead of the real Twilio API.
func WithTwilioBaseURL(u string) TwilioOption {
	return func(t *TwilioSMSNotifier) { t.baseURL = u }
}

// WithTwilioHTTPClient overrides the HTTP client. Defaults to a
// 10s-timeout client.
func WithTwilioHTTPClient(client *http.Client) TwilioOption {
	return func(t *TwilioSMSNotifier) { t.httpClient = client }
}

// NewTwilioSMSNotifier builds a Twilio-backed SMS sender. fromNumber
// must be a phone number provisioned on your Twilio account, in E.164
// format (e.g. "+15551234567").
func NewTwilioSMSNotifier(accountSID, authToken, fromNumber string, opts ...TwilioOption) *TwilioSMSNotifier {
	t := &TwilioSMSNotifier{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		baseURL:    defaultTwilioBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

type twilioErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// SendSMS implements notifier.NotificationService.
func (t *TwilioSMSNotifier) SendSMS(ctx context.Context, to, body string) error {
	body = truncateSMSBody(body)

	form := url.Values{}
	form.Set("To", to)
	form.Set("From", t.fromNumber)
	form.Set("Body", body)

	endpoint := fmt.Sprintf("%s/Accounts/%s/Messages.json", t.baseURL, t.accountSID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("notifier: build twilio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Twilio authenticates via HTTP Basic auth: Account SID as the
	// username, Auth Token as the password — not a bearer token.
	req.SetBasicAuth(t.accountSID, t.authToken)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notifier: twilio request failed: %w", err)
	}
	defer resp.Body.Close()

	// Twilio returns 201 Created on success — anything outside 2xx is
	// an error.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notifier: twilio returned %d: %s", resp.StatusCode, decodeTwilioError(resp.Body))
	}

	return nil
}

func decodeTwilioError(body io.Reader) string {
	var errResp twilioErrorResponse
	if err := json.NewDecoder(body).Decode(&errResp); err != nil || errResp.Message == "" {
		return "unknown error"
	}
	return fmt.Sprintf("%s (code %d)", errResp.Message, errResp.Code)
}

// --- Composing email + SMS into one NotificationService -----------------

// emailSender and smsSender are the minimal single-method interfaces
// CompositeNotifier needs — narrower than NotificationService itself,
// so *LogNotifier, *SendGridEmailNotifier, and *TwilioSMSNotifier all
// satisfy the relevant one without any explicit "implements" wiring.
type emailSender interface {
	SendEmail(ctx context.Context, to, subject, htmlBody, textBody string) error
}

type smsSender interface {
	SendSMS(ctx context.Context, to, body string) error
}

// CompositeNotifier implements NotificationService by delegating each
// method to a specialized sender — a SendGrid-backed emailer and a
// Twilio-backed texter, in the common case (see buildNotifier in
// cmd/api/main.go), but any two implementations work, including
// LogNotifier for one side while a real provider handles the other
// during a staged rollout.
type CompositeNotifier struct {
	Email emailSender
	SMS   smsSender
}

func (c *CompositeNotifier) SendEmail(ctx context.Context, to, subject, htmlBody, textBody string) error {
	return c.Email.SendEmail(ctx, to, subject, htmlBody, textBody)
}

func (c *CompositeNotifier) SendSMS(ctx context.Context, to, body string) error {
	return c.SMS.SendSMS(ctx, to, body)
}
