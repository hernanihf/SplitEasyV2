package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"spliteasy/internal/domain"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type fakeUserRepoForPush struct {
	lastSetUserID                                                     uint
	lastSetEnabled, lastSetExpenses, lastSetPayments, lastSetComments bool
}

func (f *fakeUserRepoForPush) Create(_ context.Context, _ *domain.User) error { return nil }
func (f *fakeUserRepoForPush) Update(_ context.Context, _ *domain.User) error { return nil }
func (f *fakeUserRepoForPush) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeUserRepoForPush) GetByID(_ context.Context, _ uint) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeUserRepoForPush) UpdatePushPreferences(_ context.Context, userID uint, enabled, expenses, payments, comments bool) error {
	f.lastSetUserID = userID
	f.lastSetEnabled = enabled
	f.lastSetExpenses = expenses
	f.lastSetPayments = payments
	f.lastSetComments = comments
	return nil
}

type fakePushSubRepo struct {
	count           int64
	subs            []domain.PushSubscription
	createdSub      *domain.PushSubscription
	deletedEndpoint string
	deletedGlobal   []string
	listedUserIDs   []uint
}

func (f *fakePushSubRepo) Create(_ context.Context, sub *domain.PushSubscription) error {
	f.createdSub = sub
	return nil
}
func (f *fakePushSubRepo) ListByUserIDs(_ context.Context, userIDs []uint) ([]domain.PushSubscription, error) {
	f.listedUserIDs = userIDs
	return f.subs, nil
}
func (f *fakePushSubRepo) DeleteByEndpoint(_ context.Context, _ uint, endpoint string) error {
	f.deletedEndpoint = endpoint
	return nil
}
func (f *fakePushSubRepo) DeleteByEndpointGlobal(_ context.Context, endpoint string) error {
	f.deletedGlobal = append(f.deletedGlobal, endpoint)
	return nil
}
func (f *fakePushSubRepo) CountByUserID(_ context.Context, _ uint) (int64, error) {
	return f.count, nil
}

func TestSetPushPreferences_UpdatesUserRepo(t *testing.T) {
	userRepo := &fakeUserRepoForPush{}
	svc := NewPushService(&fakeGroupRepo{}, userRepo, &fakePushSubRepo{}, &fakeHTTPDoer{}, "pub", "priv", "sub")

	if err := svc.SetPushPreferences(context.Background(), 7, false, true, false, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userRepo.lastSetUserID != 7 ||
		userRepo.lastSetEnabled != false ||
		userRepo.lastSetExpenses != true ||
		userRepo.lastSetPayments != false ||
		userRepo.lastSetComments != true {
		t.Errorf("expected UpdatePushPreferences(7, false, true, false, true), got (%d, %v, %v, %v, %v)",
			userRepo.lastSetUserID, userRepo.lastSetEnabled, userRepo.lastSetExpenses, userRepo.lastSetPayments, userRepo.lastSetComments)
	}
}

func TestSubscribe_CreatesSubscription(t *testing.T) {
	subRepo := &fakePushSubRepo{}
	svc := NewPushService(&fakeGroupRepo{}, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	if err := svc.Subscribe(context.Background(), 3, "https://endpoint", "p256dh-key", "auth-key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subRepo.createdSub == nil || subRepo.createdSub.UserID != 3 || subRepo.createdSub.Endpoint != "https://endpoint" {
		t.Errorf("expected subscription created for user 3, got %+v", subRepo.createdSub)
	}
}

func TestSubscribe_RejectsWhenAtCap(t *testing.T) {
	subRepo := &fakePushSubRepo{count: MaxPushSubscriptionsPerUser}
	svc := NewPushService(&fakeGroupRepo{}, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	err := svc.Subscribe(context.Background(), 3, "https://endpoint", "p256dh-key", "auth-key")
	if err == nil {
		t.Error("expected error when at the subscription cap")
	}
	if subRepo.createdSub != nil {
		t.Error("expected no subscription to be created")
	}
}

func TestUnsubscribe_DeletesByEndpoint(t *testing.T) {
	subRepo := &fakePushSubRepo{}
	svc := NewPushService(&fakeGroupRepo{}, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	if err := svc.Unsubscribe(context.Background(), 3, "https://endpoint"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subRepo.deletedEndpoint != "https://endpoint" {
		t.Errorf("expected endpoint https://endpoint to be deleted, got %q", subRepo.deletedEndpoint)
	}
}

func TestNotifyGroupMembers_NoopWhenVAPIDNotConfigured(t *testing.T) {
	groupRepo := &fakeGroupRepo{group: &domain.Group{ID: 1, Name: "Trip"}}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, &fakePushSubRepo{}, &fakeHTTPDoer{}, "", "", "")

	err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// groupRepo.group being set but never fetched is the signal that this
	// returned early — a nil group would panic if GetByID were reached.
}

func TestNotifyGroupMembers_ExcludesActorAndDisabledMembers(t *testing.T) {
	group := &domain.Group{
		ID:   1,
		Name: "Trip",
		Members: []domain.User{
			{ID: 1, Name: "Alice", PushEnabled: true, PushExpensesEnabled: true},  // actor
			{ID: 2, Name: "Bob", PushEnabled: true, PushExpensesEnabled: true},    // recipient
			{ID: 3, Name: "Carol", PushEnabled: false, PushExpensesEnabled: true}, // disabled overall
		},
	}
	groupRepo := &fakeGroupRepo{group: group}
	subRepo := &fakePushSubRepo{}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	var gotActorName string
	err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(actorName string) string {
		gotActorName = actorName
		return "body"
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotActorName != "Alice" {
		t.Errorf("expected actor name Alice resolved from group members, got %q", gotActorName)
	}
	if len(subRepo.listedUserIDs) != 1 || subRepo.listedUserIDs[0] != 2 {
		t.Errorf("expected only Bob (id 2) as a recipient, got %+v", subRepo.listedUserIDs)
	}
}

func TestNotifyGroupMembers_ExcludesMembersWithCategoryDisabled(t *testing.T) {
	group := &domain.Group{
		ID:   1,
		Name: "Trip",
		Members: []domain.User{
			{ID: 1, Name: "Alice", PushEnabled: true, PushExpensesEnabled: true, PushPaymentsEnabled: true}, // actor
			{ID: 2, Name: "Bob", PushEnabled: true, PushExpensesEnabled: true, PushPaymentsEnabled: false},  // wants expenses, not payments
		},
	}
	groupRepo := &fakeGroupRepo{group: group}
	subRepo := &fakePushSubRepo{}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	// A payment notification should skip Bob...
	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryPayment, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subRepo.listedUserIDs != nil {
		t.Errorf("expected no recipients for a payment notification, got %+v", subRepo.listedUserIDs)
	}

	// ...but an expense notification should still reach him.
	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subRepo.listedUserIDs) != 1 || subRepo.listedUserIDs[0] != 2 {
		t.Errorf("expected Bob (id 2) as a recipient for an expense notification, got %+v", subRepo.listedUserIDs)
	}
}

func TestNotifyGroupMembers_NoopWhenNoEligibleRecipients(t *testing.T) {
	group := &domain.Group{
		ID:      1,
		Name:    "Trip",
		Members: []domain.User{{ID: 1, Name: "Alice", PushEnabled: true}}, // only the actor
	}
	groupRepo := &fakeGroupRepo{group: group}
	subRepo := &fakePushSubRepo{}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, &fakeHTTPDoer{}, "pub", "priv", "sub")

	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if subRepo.listedUserIDs != nil {
		t.Error("expected no subscription lookup when there are no eligible recipients")
	}
}

// realVAPIDKeysForTest generates a fresh, valid VAPID keypair so
// webpush-go's JWT signing succeeds — it validates the key shape before
// ever reaching the injected HTTP client.
func realVAPIDKeysForTest(t *testing.T) (string, string) {
	t.Helper()
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("failed to generate VAPID keys: %v", err)
	}
	return pub, priv
}

// fakeSubscriptionKeys returns well-formed (if meaningless) p256dh/auth
// values — webpush-go decodes and validates their shape (an uncompressed
// P-256 point / a 16-byte secret) before ever reaching the injected HTTP
// client, so arbitrary short strings like "p"/"a" fail before the fake
// transport is exercised.
func fakeSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	_, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("failed to generate a fake p256dh key: %v", err)
	}
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("failed to generate a fake auth secret: %v", err)
	}
	return pub, base64.RawURLEncoding.EncodeToString(authBytes)
}

func TestNotifyGroupMembers_SendsToEachSubscription(t *testing.T) {
	pub, priv := realVAPIDKeysForTest(t)
	group := &domain.Group{
		ID:   1,
		Name: "Trip",
		Members: []domain.User{
			{ID: 1, Name: "Alice", PushEnabled: true, PushExpensesEnabled: true},
			{ID: 2, Name: "Bob", PushEnabled: true, PushExpensesEnabled: true},
		},
	}
	groupRepo := &fakeGroupRepo{group: group}
	p256dh, auth := fakeSubscriptionKeys(t)
	subRepo := &fakePushSubRepo{subs: []domain.PushSubscription{
		{ID: 10, UserID: 2, Endpoint: "https://push.example/a", P256dh: p256dh, Auth: auth},
	}}
	doer := &fakeHTTPDoer{response: &http.Response{StatusCode: http.StatusCreated, Body: http.NoBody}}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, doer, pub, priv, "mailto:test@example.com")

	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doer.lastReq == nil {
		t.Fatal("expected a push request to be sent")
	}
	if doer.lastReq.URL.String() != "https://push.example/a" {
		t.Errorf("expected the request to target the subscription's endpoint, got %q", doer.lastReq.URL.String())
	}
}

func TestNotifyGroupMembers_CleansUpGoneSubscription(t *testing.T) {
	pub, priv := realVAPIDKeysForTest(t)
	group := &domain.Group{
		ID:   1,
		Name: "Trip",
		Members: []domain.User{
			{ID: 1, Name: "Alice", PushEnabled: true, PushExpensesEnabled: true},
			{ID: 2, Name: "Bob", PushEnabled: true, PushExpensesEnabled: true},
		},
	}
	groupRepo := &fakeGroupRepo{group: group}
	p256dh, auth := fakeSubscriptionKeys(t)
	subRepo := &fakePushSubRepo{subs: []domain.PushSubscription{
		{ID: 10, UserID: 2, Endpoint: "https://push.example/dead", P256dh: p256dh, Auth: auth},
	}}
	doer := &fakeHTTPDoer{response: &http.Response{StatusCode: http.StatusGone, Body: http.NoBody}}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, doer, pub, priv, "mailto:test@example.com")

	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subRepo.deletedGlobal) != 1 || subRepo.deletedGlobal[0] != "https://push.example/dead" {
		t.Errorf("expected the gone subscription to be cleaned up, got %+v", subRepo.deletedGlobal)
	}
}

func TestNotifyGroupMembers_OneFailedSendDoesNotStopOthers(t *testing.T) {
	pub, priv := realVAPIDKeysForTest(t)
	group := &domain.Group{
		ID:   1,
		Name: "Trip",
		Members: []domain.User{
			{ID: 1, Name: "Alice", PushEnabled: true, PushExpensesEnabled: true},
			{ID: 2, Name: "Bob", PushEnabled: true, PushExpensesEnabled: true},
		},
	}
	groupRepo := &fakeGroupRepo{group: group}
	p256dh, auth := fakeSubscriptionKeys(t)
	subRepo := &fakePushSubRepo{subs: []domain.PushSubscription{
		{ID: 10, UserID: 2, Endpoint: "https://push.example/a", P256dh: p256dh, Auth: auth},
		{ID: 11, UserID: 2, Endpoint: "https://push.example/b", P256dh: p256dh, Auth: auth},
	}}
	// A doer that errors on every call still lets NotifyGroupMembers finish
	// without returning an error itself — per-subscription failures are
	// logged and swallowed, not surfaced to the caller.
	doer := &fakeHTTPDoer{err: errors.New("network error")}
	svc := NewPushService(groupRepo, &fakeUserRepoForPush{}, subRepo, doer, pub, priv, "mailto:test@example.com")

	if err := svc.NotifyGroupMembers(context.Background(), 1, 1, PushCategoryExpense, func(string) string { return "body" }, nil); err != nil {
		t.Fatalf("expected NotifyGroupMembers to swallow send errors, got: %v", err)
	}
}
