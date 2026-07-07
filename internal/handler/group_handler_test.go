package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"spliteasy/internal/domain"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type fakeGroupServiceForGroupHandler struct {
	deleteErr       error
	deletedGroupID  uint
	deletedCallerID uint
	group           *domain.Group
	updateErr       error
	updatedGroup    *domain.Group
	updatedName     *string
	updatedEmoji    *string
	previewErr      error
	previewResult   *domain.GroupPreview
	previewedToken  string
}

func (f *fakeGroupServiceForGroupHandler) CreateGroup(_ context.Context, _, _, _ string, _ uint) (*domain.Group, error) {
	return nil, nil
}
func (f *fakeGroupServiceForGroupHandler) GetGroup(_ context.Context, _ uint) (*domain.Group, error) {
	return f.group, nil
}
func (f *fakeGroupServiceForGroupHandler) ListGroupsForUser(_ context.Context, _ uint) ([]domain.Group, error) {
	return nil, nil
}
func (f *fakeGroupServiceForGroupHandler) GetInviteToken(_ context.Context, _, _ uint) (string, error) {
	return "", nil
}
func (f *fakeGroupServiceForGroupHandler) PreviewGroup(_ context.Context, token string) (*domain.GroupPreview, error) {
	f.previewedToken = token
	if f.previewErr != nil {
		return nil, f.previewErr
	}
	return f.previewResult, nil
}
func (f *fakeGroupServiceForGroupHandler) JoinGroup(_ context.Context, _ string, _ uint) (*domain.Group, error) {
	return nil, nil
}
func (f *fakeGroupServiceForGroupHandler) VerifyMembership(_ context.Context, _, _ uint) error {
	return nil
}
func (f *fakeGroupServiceForGroupHandler) UpdateGroup(_ context.Context, _, _ uint, name, emoji *string) (*domain.Group, error) {
	f.updatedName = name
	f.updatedEmoji = emoji
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updatedGroup, nil
}
func (f *fakeGroupServiceForGroupHandler) DeleteGroup(_ context.Context, groupID, callerID uint) error {
	f.deletedGroupID = groupID
	f.deletedCallerID = callerID
	return f.deleteErr
}

func deleteGroupRequest(t *testing.T, fake *fakeGroupServiceForGroupHandler, groupID string, authUserID uint) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", groupID)

	rec := httptest.NewRecorder()
	h := NewGroupHandler(fake, nil)
	h.DeleteGroup(rec, req)
	return rec
}

func updateGroupRequest(t *testing.T, fake *fakeGroupServiceForGroupHandler, groupID string, authUserID uint, body UpdateGroupRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID, bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", groupID)

	rec := httptest.NewRecorder()
	h := NewGroupHandler(fake, nil)
	h.UpdateGroup(rec, req)
	return rec
}

func strPtr(s string) *string { return &s }

func TestUpdateGroup_Success(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{updatedGroup: &domain.Group{ID: 1, Name: "New Name", Emoji: "🎉"}}
	rec := updateGroupRequest(t, fake, "1", 7, UpdateGroupRequest{Name: strPtr("New Name"), Emoji: strPtr("🎉")})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.updatedName == nil || *fake.updatedName != "New Name" {
		t.Errorf("expected name %q passed through, got %+v", "New Name", fake.updatedName)
	}
	if fake.updatedEmoji == nil || *fake.updatedEmoji != "🎉" {
		t.Errorf("expected emoji %q passed through, got %+v", "🎉", fake.updatedEmoji)
	}
}

func TestUpdateGroup_AllowsPartialUpdate(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{updatedGroup: &domain.Group{ID: 1, Name: "New Name"}}
	rec := updateGroupRequest(t, fake, "1", 7, UpdateGroupRequest{Name: strPtr("New Name")})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.updatedEmoji != nil {
		t.Errorf("expected emoji to stay nil (untouched) when omitted, got %+v", fake.updatedEmoji)
	}
}

func TestUpdateGroup_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{updateErr: service.ErrGroupNotFound}
	rec := updateGroupRequest(t, fake, "1", 7, UpdateGroupRequest{Name: strPtr("New Name")})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGroup_MapsNotMemberTo403(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{updateErr: service.ErrNotGroupMember}
	rec := updateGroupRequest(t, fake, "1", 99, UpdateGroupRequest{Name: strPtr("New Name")})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGroup_RejectsUnauthenticated(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/1", nil)
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h := NewGroupHandler(fake, nil)
	h.UpdateGroup(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestUpdateGroup_RejectsInvalidID(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}
	rec := updateGroupRequest(t, fake, "not-a-number", 7, UpdateGroupRequest{Name: strPtr("New Name")})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpdateGroup_RejectsNameOverMaxLen(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}
	longName := make([]byte, maxNameLen+1)
	for i := range longName {
		longName[i] = 'a'
	}
	rec := updateGroupRequest(t, fake, "1", 7, UpdateGroupRequest{Name: strPtr(string(longName))})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func previewGroupRequest(t *testing.T, fake *fakeGroupServiceForGroupHandler, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/preview?token="+url.QueryEscape(token), nil)
	rec := httptest.NewRecorder()
	h := NewGroupHandler(fake, nil)
	h.PreviewGroup(rec, req)
	return rec
}

func TestPreviewGroup_Success(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{
		previewResult: &domain.GroupPreview{Name: "Trip to BA", Emoji: "🏔️", Currency: "ARS", MemberCount: 3},
	}
	rec := previewGroupRequest(t, fake, "valid-token")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.previewedToken != "valid-token" {
		t.Errorf("expected token %q passed through, got %q", "valid-token", fake.previewedToken)
	}
	var body domain.GroupPreview
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body != *fake.previewResult {
		t.Errorf("expected preview %+v, got %+v", *fake.previewResult, body)
	}
}

// PreviewGroup requires no Authorization header at all — this is the whole
// point (previewing before signing in) — so there's no "unauthenticated"
// rejection test here, unlike every other group endpoint.
func TestPreviewGroup_DoesNotRequireAuth(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{previewResult: &domain.GroupPreview{Name: "No Auth Needed"}}
	rec := previewGroupRequest(t, fake, "valid-token")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no auth header, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreviewGroup_RejectsInvalidToken(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{previewErr: errors.New("invalid or expired invite link")}
	rec := previewGroupRequest(t, fake, "bogus")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreviewGroup_RejectsOversizedToken(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}
	longToken := make([]byte, maxTokenLen+1)
	for i := range longToken {
		longToken[i] = 'a'
	}
	rec := previewGroupRequest(t, fake, string(longToken))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// The oversized token must be rejected before it ever reaches the
	// service — confirms there's no way to use this endpoint to push an
	// unbounded string into a query executed against the DB.
	if fake.previewedToken != "" {
		t.Errorf("expected the service to never be called, got token %q", fake.previewedToken)
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}
	rec := deleteGroupRequest(t, fake, "1", 7)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.deletedGroupID != 1 || fake.deletedCallerID != 7 {
		t.Errorf("expected DeleteGroup(1, 7), got (%d, %d)", fake.deletedGroupID, fake.deletedCallerID)
	}
}

func TestDeleteGroup_MapsNotFoundTo404(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{deleteErr: service.ErrGroupNotFound}
	rec := deleteGroupRequest(t, fake, "1", 7)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteGroup_MapsNotCreatorTo403(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{deleteErr: service.ErrNotGroupCreator}
	rec := deleteGroupRequest(t, fake, "1", 99)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteGroup_RejectsUnauthenticated(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/1", nil)
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h := NewGroupHandler(fake, nil)
	h.DeleteGroup(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDeleteGroup_RejectsInvalidID(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{}
	rec := deleteGroupRequest(t, fake, "not-a-number", 7)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteGroup_RecordsAuditEntry(t *testing.T) {
	fake := &fakeGroupServiceForGroupHandler{group: &domain.Group{ID: 1, Name: "Trip to BA"}}
	audit := &fakeAuditService{}
	h := NewGroupHandler(fake, audit)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/1", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(7)))
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h.DeleteGroup(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(audit.records) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(audit.records))
	}
	entry := audit.records[0]
	if entry.groupID != 1 || entry.actorID != 7 || entry.action != domain.AuditActionGroupDeleted || entry.detail != "Trip to BA" {
		t.Errorf("unexpected audit entry: %+v", entry)
	}
}
