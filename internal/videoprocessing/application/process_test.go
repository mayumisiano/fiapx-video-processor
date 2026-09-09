package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	identitydomain "video-processor/internal/identity/domain"
	"video-processor/internal/videoprocessing/application"
	"video-processor/internal/videoprocessing/domain"
)

type fakeUserRepo struct {
	byID map[uuid.UUID]*identitydomain.User
}

func (r *fakeUserRepo) Create(ctx context.Context, user *identitydomain.User) error { return nil }
func (r *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*identitydomain.User, error) {
	return nil, identitydomain.ErrUserNotFound
}
func (r *fakeUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*identitydomain.User, error) {
	if u, ok := r.byID[id]; ok {
		return u, nil
	}
	return nil, identitydomain.ErrUserNotFound
}

type notification struct {
	to, fileName, reason string
	failed               bool
}

type fakeNotifier struct {
	sent []notification
	err  error
}

func (n *fakeNotifier) NotifyCompleted(ctx context.Context, to, fileName string) error {
	n.sent = append(n.sent, notification{to: to, fileName: fileName})
	return n.err
}
func (n *fakeNotifier) NotifyFailed(ctx context.Context, to, fileName, reason string) error {
	n.sent = append(n.sent, notification{to: to, fileName: fileName, reason: reason, failed: true})
	return n.err
}

func pendingRequestForProcessing(userID uuid.UUID) *domain.ProcessingRequest {
	return domain.NewProcessingRequest(userID, domain.VideoMetadata{OriginalName: "movie.mp4", Format: "mp4"}, "videos/movie.mp4")
}

func TestProcess_HappyPath_CompletesAndNotifiesSuccess(t *testing.T) {
	user := &identitydomain.User{ID: uuid.New(), Email: "ada@example.com"}
	req := pendingRequestForProcessing(user.ID)
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	users := &fakeUserRepo{byID: map[uuid.UUID]*identitydomain.User{user.ID: user}}
	notifier := &fakeNotifier{}
	extractor := &fakeExtractor{extractCount: 3}
	processor := application.NewProcessor(repo, &fakeStorage{}, extractor, users, notifier)

	err := processor.Process(context.Background(), req.ID.String())

	if err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
	stored := repo.byID[req.ID]
	if stored.Status != domain.StatusCompleted {
		t.Errorf("Status = %q, want %q", stored.Status, domain.StatusCompleted)
	}
	if stored.Result == nil || stored.Result.FrameCount != 3 {
		t.Errorf("Result = %+v, want FrameCount 3", stored.Result)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].failed {
		t.Errorf("notifier.sent = %+v, want one successful notification", notifier.sent)
	}
}

func TestProcess_ExtractionFails_RecordsFailureAndNotifies(t *testing.T) {
	user := &identitydomain.User{ID: uuid.New(), Email: "ada@example.com"}
	req := pendingRequestForProcessing(user.ID)
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	users := &fakeUserRepo{byID: map[uuid.UUID]*identitydomain.User{user.ID: user}}
	notifier := &fakeNotifier{}
	extractor := &fakeExtractor{extractCount: 0, extractErr: errors.New("ffmpeg: exit status 1")}
	processor := application.NewProcessor(repo, &fakeStorage{}, extractor, users, notifier)

	err := processor.Process(context.Background(), req.ID.String())

	if err != nil {
		t.Fatalf("Process() error = %v, want nil (business failure is not an infra error)", err)
	}
	stored := repo.byID[req.ID]
	if stored.Status != domain.StatusFailed {
		t.Errorf("Status = %q, want %q", stored.Status, domain.StatusFailed)
	}
	if stored.FailureReason != domain.FailureCorruptedFile {
		t.Errorf("FailureReason = %q, want %q", stored.FailureReason, domain.FailureCorruptedFile)
	}
	if len(notifier.sent) != 1 || !notifier.sent[0].failed {
		t.Errorf("notifier.sent = %+v, want one failure notification", notifier.sent)
	}
}

func TestProcess_NotificationFailure_DoesNotAffectPersistedState(t *testing.T) {
	user := &identitydomain.User{ID: uuid.New(), Email: "ada@example.com"}
	req := pendingRequestForProcessing(user.ID)
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	users := &fakeUserRepo{byID: map[uuid.UUID]*identitydomain.User{user.ID: user}}
	notifier := &fakeNotifier{err: errors.New("smtp: connection refused")}
	extractor := &fakeExtractor{extractCount: 3}
	processor := application.NewProcessor(repo, &fakeStorage{}, extractor, users, notifier)

	err := processor.Process(context.Background(), req.ID.String())

	if err != nil {
		t.Fatalf("Process() error = %v, want nil (notification failures must be best-effort)", err)
	}
	if repo.byID[req.ID].Status != domain.StatusCompleted {
		t.Errorf("Status = %q, want %q despite notification failure", repo.byID[req.ID].Status, domain.StatusCompleted)
	}
}

func TestProcess_UnknownRequestID_ReturnsInfraError(t *testing.T) {
	repo := newFakeRepo()
	users := &fakeUserRepo{byID: map[uuid.UUID]*identitydomain.User{}}
	processor := application.NewProcessor(repo, &fakeStorage{}, &fakeExtractor{}, users, &fakeNotifier{})

	err := processor.Process(context.Background(), uuid.New().String())

	if err == nil {
		t.Fatalf("Process() error = nil, want an error for a request the repository cannot find")
	}
}

func TestProcess_AlreadyProcessing_ReturnsNilWithoutChangingState(t *testing.T) {
	user := &identitydomain.User{ID: uuid.New(), Email: "ada@example.com"}
	req := pendingRequestForProcessing(user.ID)
	if err := req.StartProcessing(); err != nil {
		t.Fatalf("setup StartProcessing() error = %v", err)
	}
	repo := newFakeRepo()
	repo.byID[req.ID] = req
	users := &fakeUserRepo{byID: map[uuid.UUID]*identitydomain.User{user.ID: user}}
	notifier := &fakeNotifier{}
	processor := application.NewProcessor(repo, &fakeStorage{}, &fakeExtractor{extractCount: 3}, users, notifier)

	err := processor.Process(context.Background(), req.ID.String())

	if err != nil {
		t.Fatalf("Process() error = %v, want nil for an invalid transition that is not an infra failure", err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("notifier.sent = %+v, want no notification when the message is not reprocessed", notifier.sent)
	}
}
