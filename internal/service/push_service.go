package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// MaxPushSubscriptionsPerUser caps how many device subscriptions a single
// user can register. Not a real usage limit at this app's scale — just a
// guardrail against a client-side retry bug silently piling up rows forever.
const MaxPushSubscriptionsPerUser = 10

// pushNotificationTTL is how long a push service should hold a notification
// for an offline device before giving up.
const pushNotificationTTL = 24 * 60 * 60 // seconds

// PushCategory identifies which kind of group activity a notification is
// about, so it can be checked against the recipient's per-category
// preference (see domain.User's PushExpensesEnabled/PushPaymentsEnabled/
// PushCommentsEnabled) on top of their master PushEnabled switch.
type PushCategory string

const (
	PushCategoryExpense PushCategory = "expense"
	PushCategoryPayment PushCategory = "payment"
	PushCategoryComment PushCategory = "comment"
)

type PushService interface {
	SetPushPreferences(ctx context.Context, userID uint, enabled, expenses, payments, comments bool) error
	// Subscribe registers (or re-registers) a browser/device. Returns an
	// error if the user already has MaxPushSubscriptionsPerUser rows.
	Subscribe(ctx context.Context, userID uint, endpoint, p256dh, auth string) error
	Unsubscribe(ctx context.Context, userID uint, endpoint string) error
	// NotifyGroupMembers sends a push to every member of groupID except
	// actorID, skipping members who have disabled push overall or for this
	// category. The notification's title is always the group's name; its
	// body is produced by bodyFor, given the actor's display name — both
	// resolved from the single group fetch this method already does (no
	// extra query per caller). Per-subscription send failures are logged
	// and swallowed — one dead subscription must never stop the others from
	// being notified. Meant to be called from a goroutine the caller spawns
	// after writing the triggering request's response, using a context that
	// outlives the request (not r.Context(), which is canceled as soon as
	// the handler returns).
	NotifyGroupMembers(ctx context.Context, groupID, actorID uint, category PushCategory, bodyFor func(actorName string) string, data map[string]string) error
}

type pushService struct {
	groupRepo       repository.GroupRepository
	userRepo        repository.UserRepository
	subRepo         repository.PushSubscriptionRepository
	httpClient      httpDoer // nil means webpush-go's own default (*http.Client)
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string
}

func NewPushService(
	groupRepo repository.GroupRepository,
	userRepo repository.UserRepository,
	subRepo repository.PushSubscriptionRepository,
	httpClient httpDoer,
	vapidPublicKey, vapidPrivateKey, vapidSubject string,
) PushService {
	return &pushService{groupRepo, userRepo, subRepo, httpClient, vapidPublicKey, vapidPrivateKey, vapidSubject}
}

func (s *pushService) SetPushPreferences(ctx context.Context, userID uint, enabled, expenses, payments, comments bool) error {
	return s.userRepo.UpdatePushPreferences(ctx, userID, enabled, expenses, payments, comments)
}

// categoryEnabled checks a member's per-category flag for category on top of
// their master PushEnabled switch, which the caller already checked.
func categoryEnabled(m domain.User, category PushCategory) bool {
	switch category {
	case PushCategoryExpense:
		return m.PushExpensesEnabled
	case PushCategoryPayment:
		return m.PushPaymentsEnabled
	case PushCategoryComment:
		return m.PushCommentsEnabled
	default:
		return true
	}
}

func (s *pushService) Subscribe(ctx context.Context, userID uint, endpoint, p256dh, auth string) error {
	count, err := s.subRepo.CountByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if count >= MaxPushSubscriptionsPerUser {
		return fmt.Errorf("maximum of %d push subscriptions per user reached", MaxPushSubscriptionsPerUser)
	}
	return s.subRepo.Create(ctx, &domain.PushSubscription{
		UserID:   userID,
		Endpoint: endpoint,
		P256dh:   p256dh,
		Auth:     auth,
	})
}

func (s *pushService) Unsubscribe(ctx context.Context, userID uint, endpoint string) error {
	return s.subRepo.DeleteByEndpoint(ctx, userID, endpoint)
}

// pushPayload is the JSON the service worker receives in its "push" event
// (see public/sw.js in the frontend repo).
type pushPayload struct {
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
}

func (s *pushService) NotifyGroupMembers(ctx context.Context, groupID, actorID uint, category PushCategory, bodyFor func(actorName string) string, data map[string]string) error {
	if s.vapidPublicKey == "" || s.vapidPrivateKey == "" {
		return nil
	}

	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return err
	}

	actorName := ""
	var recipientIDs []uint
	for _, m := range group.Members {
		if m.ID == actorID {
			actorName = m.Name
			continue
		}
		if !m.PushEnabled || !categoryEnabled(m, category) {
			continue
		}
		recipientIDs = append(recipientIDs, m.ID)
	}
	if len(recipientIDs) == 0 {
		return nil
	}
	title := group.Name
	body := bodyFor(actorName)

	subs, err := s.subRepo.ListByUserIDs(ctx, recipientIDs)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(pushPayload{Title: title, Body: body, Data: data})
	if err != nil {
		return err
	}

	for _, sub := range subs {
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
		}, &webpush.Options{
			HTTPClient:      s.httpClient,
			Subscriber:      s.vapidSubject,
			VAPIDPublicKey:  s.vapidPublicKey,
			VAPIDPrivateKey: s.vapidPrivateKey,
			TTL:             pushNotificationTTL,
		})
		if err != nil {
			slog.Error("push send failed", "error", err, "subscription_id", sub.ID)
			continue
		}
		resp.Body.Close()

		// 404/410 is the Web Push standard's way of saying "this endpoint is
		// gone for good" (uninstalled, permission revoked, browser data
		// cleared) — without cleanup these rows would accumulate forever.
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			if delErr := s.subRepo.DeleteByEndpointGlobal(ctx, sub.Endpoint); delErr != nil {
				slog.Error("failed to clean up dead push subscription", "error", delErr)
			}
		}
	}

	return nil
}
