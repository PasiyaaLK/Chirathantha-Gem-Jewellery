package notifier

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/google/uuid"

	"gemstore/internal/models"
)

// storeName is used in the email header/greeting. Hardcoded for now —
// the proposal has no store-branding/settings concept yet, so there's
// nowhere else for this to live; move it into Config (and eventually an
// admin-editable settings table) once one exists.
const storeName = "Gem & Jewelry Store"

// EmailContent is a fully rendered email, ready to hand straight to
// NotificationService.SendEmail.
type EmailContent struct {
	Subject string
	HTML    string
	Text    string
}

// emailViewModel is what the templates actually render. It's
// deliberately flat and pre-formatted (no nested Customization/Product
// pointers, no *string fields) so the templates never nil-check and
// don't couple to the exact shape of the DB models — buildEmailViewModel
// is the one place that translation happens.
type emailViewModel struct {
	StoreName    string
	CustomerName string
	Items        []templateItem
	Total        float64
	Notes        string // admin notes (shown on approval) or decline reason
	OrderRef     string
}

type templateItem struct {
	Description string   // e.g. "Ring — Rose Gold, Sapphire"
	Details     []string // e.g. {"Size: US 6", `Engraving: "N & K"`}
	Quantity    int
	UnitPrice   float64
}

func buildEmailViewModel(contact models.CustomerContact, order models.Order, notes string) emailViewModel {
	return emailViewModel{
		StoreName:    storeName,
		CustomerName: firstName(contact.FullName),
		Items:        buildTemplateItems(order),
		Total:        order.TotalAmount,
		Notes:        strings.TrimSpace(notes),
		OrderRef:     shortRef(order.ID),
	}
}

func buildTemplateItems(order models.Order) []templateItem {
	items := make([]templateItem, 0, len(order.Items))

	for _, oi := range order.Items {
		item := templateItem{Quantity: oi.Quantity, UnitPrice: oi.UnitPrice}

		switch {
		case oi.Customization != nil:
			c := oi.Customization
			parts := []string{titleCase(string(c.Category)), titleCase(c.MetalType)}
			if c.GemstoneType != nil && strings.TrimSpace(*c.GemstoneType) != "" {
				parts = append(parts, titleCase(*c.GemstoneType))
			}
			item.Description = strings.Join(parts, " — ")

			if c.Size != nil && strings.TrimSpace(*c.Size) != "" {
				item.Details = append(item.Details, "Size: "+*c.Size)
			}
			if c.EngravingText != nil && strings.TrimSpace(*c.EngravingText) != "" {
				item.Details = append(item.Details, fmt.Sprintf("Engraving: %q", *c.EngravingText))
			}
		case oi.Product != nil:
			item.Description = oi.Product.Name
		default:
			item.Description = "Item"
		}

		items = append(items, item)
	}

	return items
}

func firstName(fullName string) string {
	fields := strings.Fields(fullName)
	if len(fields) == 0 {
		return "there"
	}
	return fields[0]
}

func shortRef(id uuid.UUID) string {
	s := id.String()
	if len(s) < 8 {
		return s
	}
	return s[:8]
}

// titleCase upper-cases the first letter of each word — "yellow gold"
// -> "Yellow Gold". Distinct from strings.Title (deprecated) and good
// enough for the small, ASCII, English catalog values (metals,
// gemstones, categories) this is ever called on.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// --- Templates ---------------------------------------------------------
//
// Parsed once at package init via template.Must — a malformed template
// string is a compile-time-adjacent programming error that should fail
// immediately and loudly (a panic on startup), not be discovered the
// first time an order happens to get approved in production.
//
// Uses html/template (auto-escaping), not text/template or manual
// string concatenation — every field interpolated into the HTML below
// includes customer-supplied text (engraving, admin notes), so this is
// a real XSS/HTML-injection surface if it weren't escaped. See
// templates_test.go for a test that specifically confirms this.
//
// Inline styles throughout, no <style> block — most email clients strip
// <style> tags or support them inconsistently; inline styles on each
// element is the standard (if verbose) practice for transactional email
// that needs to render reasonably the same across Gmail, Outlook, Apple
// Mail, etc. A light background with garnet/gold accents (rather than
// the site's full dark theme) is a deliberate choice for the same
// reason: dark-background HTML email has notoriously inconsistent
// cross-client support.

var approvalTmpl = template.Must(template.New("approval").Parse(approvalHTMLSource))
var declineTmpl = template.Must(template.New("decline").Parse(declineHTMLSource))

const emailItemsTableSource = `
      <table role="presentation" width="100%" cellpadding="8" cellspacing="0" style="border:1px solid #E4DDCF;border-radius:6px;margin:24px 0;font-size:14px;border-collapse:collapse;">
        {{range .Items}}
        <tr style="border-bottom:1px solid #E4DDCF;">
          <td style="padding:12px;">
            <strong>{{.Description}}</strong>
            {{range .Details}}<br><span style="color:#6E6459;font-size:13px;">{{.}}</span>{{end}}
          </td>
          <td style="padding:12px;text-align:right;white-space:nowrap;vertical-align:top;">
            Qty {{.Quantity}}<br>${{printf "%.2f" .UnitPrice}}
          </td>
        </tr>
        {{end}}
      </table>
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:0 0 24px;">
        <tr>
          <td style="font-size:16px;font-weight:bold;color:#2B2620;">Order total</td>
          <td style="font-size:16px;font-weight:bold;text-align:right;color:#8C3B4A;">${{printf "%.2f" .Total}}</td>
        </tr>
      </table>`

const emailShellHeaderSource = `<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background-color:#F3EDE2;font-family:Georgia,'Times New Roman',serif;color:#2B2620;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#F3EDE2;padding:32px 0;">
    <tr>
      <td align="center">
        <table role="presentation" width="560" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:8px;overflow:hidden;">
          <tr>
            <td style="background-color:#8C3B4A;padding:24px 32px;">
              <span style="color:#F3EDE2;font-size:20px;">{{.StoreName}}</span>
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">`

const emailShellFooterSource = `
              <p style="font-size:13px;color:#8A8071;margin:24px 0 0;">Order reference #{{.OrderRef}}</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

var approvalHTMLSource = emailShellHeaderSource + `
              <h1 style="font-size:22px;margin:0 0 16px;color:#2B2620;">Your custom order has been confirmed</h1>
              <p style="font-size:15px;line-height:1.6;margin:0 0 16px;">Hi {{.CustomerName}},</p>
              <p style="font-size:15px;line-height:1.6;margin:0 0 16px;">
                Good news — we've reviewed your custom design and it's confirmed.
                It now moves into production, and we'll be in touch again once it ships.
              </p>` + emailItemsTableSource + `
              {{if .Notes}}
              <p style="font-size:14px;line-height:1.6;background-color:#F8F5EE;border-left:3px solid #C9A46A;padding:12px 16px;margin:0 0 24px;">
                <strong>Note from our team:</strong> {{.Notes}}
              </p>
              {{end}}` + emailShellFooterSource

var declineHTMLSource = emailShellHeaderSource + `
              <h1 style="font-size:22px;margin:0 0 16px;color:#2B2620;">An update on your custom order</h1>
              <p style="font-size:15px;line-height:1.6;margin:0 0 16px;">Hi {{.CustomerName}},</p>
              <p style="font-size:15px;line-height:1.6;margin:0 0 16px;">
                We're sorry — after review, we're unable to proceed with your custom order as specified.
              </p>` + emailItemsTableSource + `
              {{if .Notes}}
              <p style="font-size:14px;line-height:1.6;background-color:#F8F5EE;border-left:3px solid #6E6459;padding:12px 16px;margin:0 0 24px;">
                <strong>Reason:</strong> {{.Notes}}
              </p>
              {{end}}
              <p style="font-size:15px;line-height:1.6;margin:0 0 16px;">
                If you'd like to adjust the design and resubmit, we'd be glad to take another look.
              </p>` + emailShellFooterSource

// RenderApprovalEmail composes the "your custom order is confirmed"
// notification (Section 8, steps 4-5), including a recap of what was
// ordered so the customer sees a summary, not just a bare decision.
func RenderApprovalEmail(contact models.CustomerContact, order models.Order, adminNotes *string) (EmailContent, error) {
	notes := ""
	if adminNotes != nil {
		notes = *adminNotes
	}
	vm := buildEmailViewModel(contact, order, notes)

	var htmlBuf bytes.Buffer
	if err := approvalTmpl.Execute(&htmlBuf, vm); err != nil {
		return EmailContent{}, fmt.Errorf("notifier: render approval email: %w", err)
	}

	return EmailContent{
		Subject: fmt.Sprintf("Your custom order #%s has been confirmed", vm.OrderRef),
		HTML:    htmlBuf.String(),
		Text:    renderApprovalText(vm),
	}, nil
}

// RenderDeclineEmail composes the "we can't proceed with this design"
// notification. reason is the admin's required decline notes (see
// ApprovalService.DeclineCustomOrder — this is never called with an
// empty reason in normal operation, but the template renders fine
// either way).
func RenderDeclineEmail(contact models.CustomerContact, order models.Order, reason string) (EmailContent, error) {
	vm := buildEmailViewModel(contact, order, reason)

	var htmlBuf bytes.Buffer
	if err := declineTmpl.Execute(&htmlBuf, vm); err != nil {
		return EmailContent{}, fmt.Errorf("notifier: render decline email: %w", err)
	}

	return EmailContent{
		Subject: fmt.Sprintf("Update on your custom order #%s", vm.OrderRef),
		HTML:    htmlBuf.String(),
		Text:    renderDeclineText(vm),
	}, nil
}

func renderApprovalText(vm emailViewModel) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Hi %s,\n\n", vm.CustomerName)
	sb.WriteString("Good news — we've reviewed your custom design and it's confirmed. ")
	sb.WriteString("It now moves into production, and we'll be in touch again once it ships.\n\n")
	writeItemsText(&sb, vm.Items)
	fmt.Fprintf(&sb, "\nOrder total: $%.2f\n", vm.Total)
	if vm.Notes != "" {
		fmt.Fprintf(&sb, "\nNote from our team: %s\n", vm.Notes)
	}
	fmt.Fprintf(&sb, "\nOrder reference #%s\n", vm.OrderRef)
	return sb.String()
}

func renderDeclineText(vm emailViewModel) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Hi %s,\n\n", vm.CustomerName)
	sb.WriteString("We're sorry — after review, we're unable to proceed with your custom order as specified.\n\n")
	writeItemsText(&sb, vm.Items)
	if vm.Notes != "" {
		fmt.Fprintf(&sb, "\nReason: %s\n", vm.Notes)
	}
	fmt.Fprintf(&sb, "\nOrder reference #%s\n\nIf you'd like to adjust the design and resubmit, we'd be glad to take another look.\n", vm.OrderRef)
	return sb.String()
}

func writeItemsText(sb *strings.Builder, items []templateItem) {
	for _, item := range items {
		fmt.Fprintf(sb, "- %s (qty %d) — $%.2f\n", item.Description, item.Quantity, item.UnitPrice)
		for _, d := range item.Details {
			fmt.Fprintf(sb, "    %s\n", d)
		}
	}
}
