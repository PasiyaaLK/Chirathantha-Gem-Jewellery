package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"gemstore/internal/models"
	"gemstore/internal/pkg/notifier"
	"gemstore/internal/repository"
)

// ErrOrderNotPending re-exports repository.ErrOrderNotPending so callers
// (handlers) only need to import the service package's error set, not
// reach into repository for it too.
var ErrOrderNotPending = repository.ErrOrderNotPending

type ApprovalService struct {
	repo     *repository.ApprovalRepository
	notifier notifier.NotificationService
}

func NewApprovalService(repo *repository.ApprovalRepository, n notifier.NotificationService) *ApprovalService {
	return &ApprovalService{repo: repo, notifier: n}
}

func (s *ApprovalService) ListPendingOrders(ctx context.Context) ([]models.PendingCustomOrder, error) {
	return s.repo.ListPending(ctx)
}

// ApproveCustomOrder confirms a pending custom order. notes is optional
// context for the customer/audit trail (e.g. an estimated ship date).
func (s *ApprovalService) ApproveCustomOrder(
	ctx context.Context,
	orderID, adminID uuid.UUID,
	notes *string,
) (*models.Order, error) {
	return s.decide(ctx, orderID, adminID, models.DecisionConfirmed, notes)
}

// DeclineCustomOrder rejects a pending custom order. Unlike approval, a
// reason is required — it's the only explanation the customer gets, and
// it lives on in the order_approvals audit trail.
func (s *ApprovalService) DeclineCustomOrder(
	ctx context.Context,
	orderID, adminID uuid.UUID,
	notes *string,
) (*models.Order, error) {
	if notes == nil || strings.TrimSpace(*notes) == "" {
		return nil, ErrInvalidInput{Field: "notes", Reason: "a reason is required when declining a custom order"}
	}
	return s.decide(ctx, orderID, adminID, models.DecisionDeclined, notes)
}

func (s *ApprovalService) decide(
	ctx context.Context,
	orderID, adminID uuid.UUID,
	decision models.ApprovalDecision,
	notes *string,
) (*models.Order, error) {
	result, err := s.repo.DecideCustomOrder(ctx, orderID, adminID, decision, notes)
	if err != nil {
		return nil, err // repository.ErrOrderNotPending bubbles up as-is
	}

	s.dispatchNotification(result.Customer, result.Order, decision, notes)

	return &result.Order, nil
}

// dispatchNotification fires the customer notification asynchronously so
// the HTTP response returns as soon as the DB transaction commits — the
// admin isn't kept waiting on an email/SMS provider's round trip
// (Section 5.2: "trigger point that automatically sends the notification
// ... once a decision is made").
//
// It deliberately does NOT reuse the request's context: r.Context() is
// cancelled the instant the HTTP handler returns, which would abort the
// notification mid-flight. A fresh background context with its own
// timeout keeps the goroutine alive independently of the request that
// spawned it. The recover() guards against a notifier bug taking down
// the process from inside a detached goroutine, where a panic would
// otherwise crash the whole server rather than just failing one request.
//
// This is a fire-and-forget goroutine, appropriate for this step's
// scope. A production system with delivery guarantees (retry on
// provider failure, surviving a server restart mid-send) would replace
// this with a durable outbox: write a notification_outbox row in the
// same DB transaction as the status update, and have a separate worker
// poll and send it.
func (s *ApprovalService) dispatchNotification(
	contact models.CustomerContact,
	order models.Order,
	decision models.ApprovalDecision,
	notes *string,
) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("approval: notifier goroutine panic recovered",
					"panic", rec, "order_id", order.ID)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var (
			content notifier.EmailContent
			err     error
		)
		if decision == models.DecisionConfirmed {
			content, err = notifier.RenderApprovalEmail(contact, order, notes)
		} else {
			reason := ""
			if notes != nil {
				reason = *notes
			}
			content, err = notifier.RenderDeclineEmail(contact, order, reason)
		}
		if err != nil {
			// A template-rendering failure is a programming error (a
			// malformed view model), not a transient provider issue —
			// but this is still a detached goroutine after the HTTP
			// response has already gone out, so there's nothing to do
			// but log it and move on, same as any other failure here.
			slog.Error("approval: failed to render decision email", "order_id", order.ID, "error", err)
			return
		}

		if err := s.notifier.SendEmail(ctx, contact.Email, content.Subject, content.HTML, content.Text); err != nil {
			slog.Error("approval: email notification failed",
				"order_id", order.ID, "error", err)
		}

		if contact.Phone != nil && strings.TrimSpace(*contact.Phone) != "" {
			smsBody := notifier.RenderDecisionSMS(order, decision)
			if err := s.notifier.SendSMS(ctx, *contact.Phone, smsBody); err != nil {
				slog.Error("approval: sms notification failed",
					"order_id", order.ID, "error", err)
			}
		}
	}()
}
