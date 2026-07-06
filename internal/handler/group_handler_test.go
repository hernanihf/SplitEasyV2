package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
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
func (f *fakeGroupServiceForGroupHandler) JoinGroup(_ context.Context, _ string, _ uint) (*domain.Group, error) {
	return nil, nil
}
func (f *fakeGroupServiceForGroupHandler) VerifyMembership(_ context.Context, _, _ uint) error {
	return nil
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
