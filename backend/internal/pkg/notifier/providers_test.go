package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendGridEmailNotifier_SendEmail_Success(t *testing.T) {
	var gotAuth, gotPath, gotMethod string
	var gotBody sendGridSendRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("server: decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted) // SendGrid's real success code
	}))
	defer server.Close()

	n := NewSendGridEmailNotifier("test-api-key", "orders@store.test", "Test Store",
		WithSendGridBaseURL(server.URL))

	err := n.SendEmail(context.Background(), "customer@example.com", "Subject line", "<p>hi</p>", "hi")
	if err != nil {
		t.Fatalf("SendEmail returned an error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/mail/send" {
		t.Errorf("path = %q, want /mail/send", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-api-key")
	}
	if gotBody.Subject != "Subject line" {
		t.Errorf("subject = %q, want %q", gotBody.Subject, "Subject line")
	}
	if len(gotBody.Personalizations) != 1 || gotBody.Personalizations[0].To[0].Email != "customer@example.com" {
		t.Errorf("unexpected personalizations: %+v", gotBody.Personalizations)
	}
	if gotBody.From.Email != "orders@store.test" || gotBody.From.Name != "Test Store" {
		t.Errorf("unexpected from address: %+v", gotBody.From)
	}
	if len(gotBody.Content) != 2 || gotBody.Content[0].Type != "text/plain" || gotBody.Content[1].Type != "text/html" {
		t.Errorf("unexpected content parts: %+v", gotBody.Content)
	}
}

func TestSendGridEmailNotifier_SendEmail_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(sendGridErrorResponse{
			Errors: []sendGridErrorItem{{Message: "does not contain a valid address", Field: "personalizations.0.to.0.email"}},
		})
	}))
	defer server.Close()

	n := NewSendGridEmailNotifier("test-api-key", "orders@store.test", "Test Store",
		WithSendGridBaseURL(server.URL))

	err := n.SendEmail(context.Background(), "not-an-email", "Subject", "<p>hi</p>", "hi")
	if err == nil {
		t.Fatal("expected an error for a 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "does not contain a valid address") {
		t.Errorf("error message %q does not surface the SendGrid error detail", err.Error())
	}
}

func TestTwilioSMSNotifier_SendSMS_Success(t *testing.T) {
	var gotAuthUser, gotAuthPass, gotPath string
	var gotForm url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotAuthUser, gotAuthPass, ok = r.BasicAuth()
		if !ok {
			t.Fatal("server: request did not use HTTP Basic auth")
		}
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("server: parse form: %v", err)
		}
		gotForm = r.PostForm
		w.WriteHeader(http.StatusCreated) // Twilio's real success code
		_, _ = w.Write([]byte(`{"sid":"SM_test","status":"queued"}`))
	}))
	defer server.Close()

	n := NewTwilioSMSNotifier("AC_test_sid", "test_auth_token", "+15550001111",
		WithTwilioBaseURL(server.URL))

	err := n.SendSMS(context.Background(), "+15559998888", "Your order is confirmed!")
	if err != nil {
		t.Fatalf("SendSMS returned an error: %v", err)
	}

	if gotAuthUser != "AC_test_sid" || gotAuthPass != "test_auth_token" {
		t.Errorf("Basic auth = %q/%q, want AC_test_sid/test_auth_token", gotAuthUser, gotAuthPass)
	}
	wantPath := "/Accounts/AC_test_sid/Messages.json"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotForm.Get("To") != "+15559998888" {
		t.Errorf("To = %q, want +15559998888", gotForm.Get("To"))
	}
	if gotForm.Get("From") != "+15550001111" {
		t.Errorf("From = %q, want +15550001111", gotForm.Get("From"))
	}
	if gotForm.Get("Body") != "Your order is confirmed!" {
		t.Errorf("Body = %q, want %q", gotForm.Get("Body"), "Your order is confirmed!")
	}
}

func TestTwilioSMSNotifier_SendSMS_TruncatesLongMessages(t *testing.T) {
	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotBody = r.PostForm.Get("Body")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	n := NewTwilioSMSNotifier("AC_test", "token", "+15550001111", WithTwilioBaseURL(server.URL))

	longMessage := strings.Repeat("a", smsSegmentLimit+50)
	if err := n.SendSMS(context.Background(), "+15559998888", longMessage); err != nil {
		t.Fatalf("SendSMS returned an error: %v", err)
	}

	if len(gotBody) != smsSegmentLimit {
		t.Errorf("sent body length = %d, want exactly smsSegmentLimit (%d)", len(gotBody), smsSegmentLimit)
	}
	if !strings.HasSuffix(gotBody, "…") {
		t.Errorf("truncated body should end with an ellipsis marker, got: %q", gotBody[len(gotBody)-10:])
	}
}

func TestTwilioSMSNotifier_SendSMS_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(twilioErrorResponse{Message: "The 'To' number is not a valid phone number", Code: 21211})
	}))
	defer server.Close()

	n := NewTwilioSMSNotifier("AC_test", "token", "+15550001111", WithTwilioBaseURL(server.URL))

	err := n.SendSMS(context.Background(), "not-a-number", "test")
	if err == nil {
		t.Fatal("expected an error for a 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid phone number") {
		t.Errorf("error message %q does not surface the Twilio error detail", err.Error())
	}
}

// --- CompositeNotifier ---------------------------------------------------

type fakeEmailSender struct{ called bool }

func (f *fakeEmailSender) SendEmail(ctx context.Context, to, subject, html, text string) error {
	f.called = true
	return nil
}

type fakeSMSSender struct{ called bool }

func (f *fakeSMSSender) SendSMS(ctx context.Context, to, body string) error {
	f.called = true
	return nil
}

func TestCompositeNotifier_DelegatesToEachSender(t *testing.T) {
	email := &fakeEmailSender{}
	sms := &fakeSMSSender{}
	c := &CompositeNotifier{Email: email, SMS: sms}

	if err := c.SendEmail(context.Background(), "a@b.com", "subj", "<p>x</p>", "x"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if err := c.SendSMS(context.Background(), "+1555", "x"); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}

	if !email.called {
		t.Error("CompositeNotifier.SendEmail did not delegate to the Email sender")
	}
	if !sms.called {
		t.Error("CompositeNotifier.SendSMS did not delegate to the SMS sender")
	}
}

// Compile-time checks that LogNotifier, SendGridEmailNotifier, and
// TwilioSMSNotifier all actually satisfy the interfaces CompositeNotifier
// and main.go's buildNotifier expect them to.
var (
	_ NotificationService = (*LogNotifier)(nil)
	_ emailSender         = (*SendGridEmailNotifier)(nil)
	_ smsSender           = (*TwilioSMSNotifier)(nil)
	_ NotificationService = (*CompositeNotifier)(nil)
)
