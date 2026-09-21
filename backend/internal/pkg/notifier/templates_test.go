package notifier

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"gemstore/internal/models"
)

func sampleOrder(t *testing.T, engraving string) models.Order {
	t.Helper()

	gemstone := "sapphire"
	size := "US 6"
	orderID := uuid.New()

	return models.Order{
		ID:          orderID,
		TotalAmount: 582.50,
		Items: []models.OrderItem{
			{
				Quantity:  1,
				UnitPrice: 582.50,
				Customization: &models.Customization{
					Category:      models.CategoryRing,
					MetalType:     "rose gold",
					GemstoneType:  &gemstone,
					Size:          &size,
					EngravingText: &engraving,
				},
			},
		},
	}
}

func sampleContact() models.CustomerContact {
	return models.CustomerContact{FullName: "Nadeesha Perera", Email: "nadeesha@example.com"}
}

func TestRenderApprovalEmail_ContainsExpectedContent(t *testing.T) {
	order := sampleOrder(t, "N & K")
	notes := "Estimated ship date: 2 weeks"

	content, err := RenderApprovalEmail(sampleContact(), order, &notes)
	if err != nil {
		t.Fatalf("RenderApprovalEmail returned an error: %v", err)
	}

	ref := order.ID.String()[:8]
	if !strings.Contains(content.Subject, ref) {
		t.Errorf("subject %q does not contain the order reference %q", content.Subject, ref)
	}

	for _, want := range []string{"Nadeesha", "Ring", "Rose Gold", "Sapphire", "582.50", "Estimated ship date"} {
		if !strings.Contains(content.HTML, want) {
			t.Errorf("HTML body missing expected content %q", want)
		}
		if !strings.Contains(content.Text, want) {
			t.Errorf("text body missing expected content %q", want)
		}
	}
}

func TestRenderDeclineEmail_ContainsReason(t *testing.T) {
	order := sampleOrder(t, "")
	reason := "18k rose gold is not available in this gemstone setting."

	content, err := RenderDeclineEmail(sampleContact(), order, reason)
	if err != nil {
		t.Fatalf("RenderDeclineEmail returned an error: %v", err)
	}

	if !strings.Contains(content.HTML, "not available in this gemstone setting") {
		t.Error("HTML body does not contain the decline reason")
	}
	if !strings.Contains(content.Text, "not available in this gemstone setting") {
		t.Error("text body does not contain the decline reason")
	}
	if strings.Contains(content.HTML, "we've reviewed your custom design and it's confirmed") {
		t.Error("decline email contains approval copy — templates may be swapped")
	}
}

// This is the important one: engraving text and admin notes are
// customer/admin-supplied free text embedded directly into an HTML
// email. If templates.go ever used text/template or manual string
// concatenation instead of html/template, this would be a stored
// HTML-injection vector — an engraving of "<script>...</script>" or
// "<img src=x onerror=...>" would be emitted verbatim into the HTML
// part of a real email. html/template auto-escapes on render; this
// test confirms that's actually happening, not just assumed.
func TestRenderApprovalEmail_EscapesHTMLInjectionAttempts(t *testing.T) {
	malicious := `<script>alert(1)</script><img src=x onerror=alert(2)>`
	order := sampleOrder(t, malicious)

	content, err := RenderApprovalEmail(sampleContact(), order, nil)
	if err != nil {
		t.Fatalf("RenderApprovalEmail returned an error: %v", err)
	}

	if strings.Contains(content.HTML, "<script>") {
		t.Error("HTML body contains an unescaped <script> tag — engraving text is not being escaped")
	}
	if strings.Contains(content.HTML, "<img") {
		t.Error("HTML body contains an unescaped <img> tag — engraving text is not being escaped")
	}
	if !strings.Contains(content.HTML, "&lt;script&gt;") {
		t.Error("HTML body does not contain the expected escaped form of the injected script tag")
	}
}

func TestRenderApprovalEmail_HandlesNilNotesAndProductItems(t *testing.T) {
	order := models.Order{
		ID:          uuid.New(),
		TotalAmount: 99.00,
		Items: []models.OrderItem{
			{Quantity: 2, UnitPrice: 49.50, Product: &models.Product{Name: "Classic Silver Band"}},
		},
	}

	content, err := RenderApprovalEmail(sampleContact(), order, nil)
	if err != nil {
		t.Fatalf("RenderApprovalEmail returned an error: %v", err)
	}
	if !strings.Contains(content.HTML, "Classic Silver Band") {
		t.Error("HTML body does not describe the standard (non-customization) product item")
	}
}

func TestRenderDecisionSMS_IsShortAndDistinctByDecision(t *testing.T) {
	order := sampleOrder(t, "")

	confirmed := RenderDecisionSMS(order, models.DecisionConfirmed)
	declined := RenderDecisionSMS(order, models.DecisionDeclined)

	if confirmed == declined {
		t.Error("confirmed and declined SMS text should differ")
	}
	if len(confirmed) > 160 || len(declined) > 160 {
		t.Errorf("SMS text should fit in a single ~160-char segment; got %d and %d chars",
			len(confirmed), len(declined))
	}
	ref := order.ID.String()[:8]
	if !strings.Contains(confirmed, ref) || !strings.Contains(declined, ref) {
		t.Error("SMS text should reference the order")
	}
}
