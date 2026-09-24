package notifier

import (
	"fmt"

	"gemstore/internal/models"
)

// RenderDecisionSMS composes a short SMS notification for a custom-
// order confirm/decline decision (Section 8: "the system automatically
// sends an email or SMS to the customer with the decision"). This is
// deliberately much shorter than the email — SMS gateways typically
// bill per ~160-character (GSM-7) segment, and the customer already
// gets the full recap via RenderApprovalEmail/RenderDeclineEmail, so the
// text just needs to say what happened and point them there.
func RenderDecisionSMS(order models.Order, decision models.ApprovalDecision) string {
	ref := shortRef(order.ID)
	if decision == models.DecisionConfirmed {
		return fmt.Sprintf("Your custom order #%s has been confirmed! Check your email for details.", ref)
	}
	return fmt.Sprintf("There's an update on your custom order #%s. Check your email for details.", ref)
}
