package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// --- fakes ---

type fakeAcctUserRepo struct {
	// inputs recorded
	calls       []ids.UserID
	// configurable outputs
	user        *domain.User
	firstTime   bool
	err         error
	// ordering trace
	orderSink   *[]string
}

func (f *fakeAcctUserRepo) MarkDeletionRequested(_ context.Context, id ids.UserID) (*domain.User, bool, error) {
	f.calls = append(f.calls, id)
	if f.orderSink != nil {
		*f.orderSink = append(*f.orderSink, "mark")
	}
	return f.user, f.firstTime, f.err
}

type fakeAcctTokenRevoker struct {
	calls     []ids.UserID
	err       error
	orderSink *[]string
}

func (f *fakeAcctTokenRevoker) RevokeAllUserTokens(_ context.Context, id ids.UserID) error {
	f.calls = append(f.calls, id)
	if f.orderSink != nil {
		*f.orderSink = append(*f.orderSink, "revoke")
	}
	return f.err
}

type fakeAcctSessionDisconnector struct {
	gotUsers  []ws.UserID
	count     int
	err       error
	orderSink *[]string
}

func (f *fakeAcctSessionDisconnector) DisconnectUser(u ws.UserID) (int, error) {
	f.gotUsers = append(f.gotUsers, u)
	if f.orderSink != nil {
		*f.orderSink = append(*f.orderSink, "disconnect")
	}
	return f.count, f.err
}

type fakeAcctCacheInvalidator struct {
	calls     []string
	err       error
	orderSink *[]string
}

func (f *fakeAcctCacheInvalidator) Invalidate(_ context.Context, userID string) error {
	f.calls = append(f.calls, userID)
	if f.orderSink != nil {
		*f.orderSink = append(*f.orderSink, "invalidate")
	}
	return f.err
}

type fakeAcctAccessBlacklister struct {
	calls     []string
	ttls      []time.Duration
	err       error
	orderSink *[]string
}

func (f *fakeAcctAccessBlacklister) Add(_ context.Context, userID string, ttl time.Duration) error {
	f.calls = append(f.calls, userID)
	f.ttls = append(f.ttls, ttl)
	if f.orderSink != nil {
		*f.orderSink = append(*f.orderSink, "blacklist")
	}
	return f.err
}

// accessTokenTTLForTest is the per-test blacklist TTL. 15 minutes
// matches cfg.JWT.AccessExpirySec=900 from production config and
// the integration harnesses.
const accessTokenTTLForTest = 15 * time.Minute

// seeded fixed time — mirrors repository integration tests.
var fixedNow = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

func newUser(t time.Time) *domain.User {
	return &domain.User{
		ID:                  ids.UserID("u1"),
		AppleUserIDHash:     "hash:u1",
		DeletionRequested:   true,
		DeletionRequestedAt: &t,
	}
}

// runWithLoggedCtx builds a context carrying a zerolog logger that
// writes to buf. The service code calls logx.Ctx(ctx).Info() which
// picks up the context-bound logger via logx.Ctx → zerolog.Ctx.
func runWithLoggedCtx(t *testing.T, buf *bytes.Buffer) context.Context {
	t.Helper()
	lg := zerolog.New(buf).Level(zerolog.DebugLevel)
	return lg.WithContext(context.Background())
}

// compile-time sanity for logx import (silences unused imports if we
// tweak helpers later).
var _ = logx.Ctx

// --- tests ---

func TestAcctDel_HappyPath_FirstTime_AllStepsExecuted(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	order := []string{}
	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true, orderSink: &order}
	rev := &fakeAcctTokenRevoker{orderSink: &order}
	bl := &fakeAcctAccessBlacklister{orderSink: &order}
	dis := &fakeAcctSessionDisconnector{orderSink: &order}
	cache := &fakeAcctCacheInvalidator{orderSink: &order}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.False(t, got.WasAlreadyRequested)
	assert.Equal(t, fixedNow, got.RequestedAt)

	require.Len(t, repo.calls, 1)
	require.Len(t, rev.calls, 1)
	require.Len(t, bl.calls, 1)
	require.Len(t, dis.gotUsers, 1)
	require.Len(t, cache.calls, 1)

	assert.Equal(t, ids.UserID("u1"), repo.calls[0])
	assert.Equal(t, ids.UserID("u1"), rev.calls[0])
	assert.Equal(t, "u1", bl.calls[0])
	assert.Equal(t, accessTokenTTLForTest, bl.ttls[0],
		"blacklist TTL MUST equal configured access-token expiry (so entry auto-expires when the last valid access token naturally would)")
	assert.Equal(t, ws.UserID("u1"), dis.gotUsers[0])
	assert.Equal(t, "u1", cache.calls[0])
}

func TestAcctDel_HappyPath_AlreadyRequested_StillRunsAllSideEffects(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	originalStamp := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	repo := &fakeAcctUserRepo{user: newUser(originalStamp), firstTime: false}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)

	assert.True(t, got.WasAlreadyRequested, "§21.8 #5: wasAlreadyRequested must be true on repeat")
	assert.Equal(t, originalStamp, got.RequestedAt,
		"§21.8 #1: RequestedAt preserves the FIRST-call stamp, not the current time")

	// All five steps must still run (§21.8 #3 — idempotent path must
	// re-run side effects so Redis / WS state converge even if the
	// first call left orphans). This is load-bearing for the access
	// blacklist: a re-DELETE after a Redis outage during the first
	// attempt MUST re-write the blacklist entry.
	assert.Len(t, rev.calls, 1)
	assert.Len(t, bl.calls, 1, "blacklist re-write on idempotent repeat — round-1 fix convergence")
	assert.Len(t, dis.gotUsers, 1)
	assert.Len(t, cache.calls, 1)
}

func TestAcctDel_Step1_UserNotFound_Returns404_NoSideEffects(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{err: repository.ErrUserNotFound}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	_, err := svc.RequestAccountDeletion(ctx, ids.UserID("ghost"))
	require.Error(t, err)

	var appErr *dto.AppError
	require.True(t, errors.As(err, &appErr), "service must wrap ErrUserNotFound in AppError")
	assert.Equal(t, "USER_NOT_FOUND", appErr.Code)

	assert.Zero(t, len(rev.calls), "§21.8 #3 fail-closed: Step 2 MUST NOT run after Step 1 failure")
	assert.Zero(t, len(bl.calls), "§21.8 #3 fail-closed: Step 3 (blacklist) MUST NOT run after Step 1 failure")
	assert.Zero(t, len(dis.gotUsers), "§21.8 #3 fail-closed: Step 4 MUST NOT run after Step 1 failure")
	assert.Zero(t, len(cache.calls), "§21.8 #3 fail-closed: Step 5 MUST NOT run after Step 1 failure")
}

func TestAcctDel_Step1_GenericError_Wraps500_NoSideEffects(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{err: fmt.Errorf("mongo: connection reset")}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	_, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))

	var appErr *dto.AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "INTERNAL_ERROR", appErr.Code)

	assert.Zero(t, len(rev.calls))
	assert.Zero(t, len(bl.calls))
	assert.Zero(t, len(dis.gotUsers))
	assert.Zero(t, len(cache.calls))
}

func TestAcctDel_Step2_RevokeError_FailOpen_ContinuesToStep3_4_5(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true}
	rev := &fakeAcctTokenRevoker{err: errors.New("blacklist: redis down")}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err, "§21.3: Step 2 is fail-open — main response still 202")
	require.NotNil(t, got)

	assert.Len(t, bl.calls, 1, "Step 3 (blacklist) must run after Step 2 fail-open")
	assert.Len(t, dis.gotUsers, 1, "Step 4 must run after Step 2 fail-open")
	assert.Len(t, cache.calls, 1, "Step 5 must run after Step 2 fail-open")

	// Warn log MUST be emitted so ops sees the partial state.
	assert.Contains(t, buf.String(), "account_deletion_revoke_partial",
		"warn audit log `account_deletion_revoke_partial` must land in buf — fail-open is observable, not silent (feedback_no_backup_fallback)")
}

func TestAcctDel_Step3_BlacklistError_FailOpen_ContinuesToStep4_5(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{err: errors.New("blacklist: redis down")}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err, "§21.3: Step 3 (blacklist) is fail-open — main response still 202")
	require.NotNil(t, got)

	assert.Len(t, dis.gotUsers, 1, "Step 4 must still run after Step 3 fail-open")
	assert.Len(t, cache.calls, 1, "Step 5 must still run after Step 3 fail-open")

	// Warn log makes the partial state observable to ops (a Redis
	// outage during blacklist write leaves a small window where the
	// still-valid access token works — process_deletion_queue + Epic
	// 1.x may add a safety sweep later).
	assert.Contains(t, buf.String(), "account_deletion_access_blacklist_error",
		"warn log `account_deletion_access_blacklist_error` must land in buf")
}

func TestAcctDel_Step4_DisconnectError_FailOpen_ContinuesToStep5(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{err: errors.New("hub: internal state")}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Len(t, cache.calls, 1, "Step 5 must still run after Step 4 fail-open")
	assert.Contains(t, buf.String(), "account_deletion_disconnect_error")
}

func TestAcctDel_Step5_InvalidateError_FailOpen_Response202(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{err: errors.New("redis del failed")}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	got, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	require.NotNil(t, got, "§21.3: Step 5 is fail-open — main response still 202")

	assert.Contains(t, buf.String(), "account_deletion_resume_invalidate_error")
}

func TestAcctDel_AuditLog_EmitsActionWithUserIDAndFlag(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	originalStamp := time.Date(2026, 3, 1, 4, 5, 6, 0, time.UTC)
	repo := &fakeAcctUserRepo{user: newUser(originalStamp), firstTime: false}
	svc := service.NewAccountDeletionService(
		repo, &fakeAcctTokenRevoker{}, &fakeAcctAccessBlacklister{}, &fakeAcctSessionDisconnector{}, &fakeAcctCacheInvalidator{}, accessTokenTTLForTest,
	)
	_, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)

	// Audit log exists, with action + userId + wasAlreadyRequested.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var auditFields map[string]any
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if act, ok := m["action"].(string); ok && act == "account_deletion_request" {
			auditFields = m
			break
		}
	}
	require.NotNil(t, auditFields, "audit log with action=account_deletion_request must exist")
	assert.Equal(t, "u1", auditFields["userId"])
	assert.Equal(t, true, auditFields["wasAlreadyRequested"], "audit correctly reflects idempotent path")
}

func TestAcctDel_AuditLog_DoesNotIncludePII(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	// Seed a user with PII-ish fields set; they must never appear in
	// the captured audit log bytes.
	displayName := "Alice-SECRET-PII-TOKEN"
	u := newUser(fixedNow)
	u.DisplayName = &displayName
	tz := "Asia/Shanghai"
	u.Timezone = &tz
	u.AppleUserIDHash = "hash:leaked-sub-hash"

	repo := &fakeAcctUserRepo{user: u, firstTime: true}
	svc := service.NewAccountDeletionService(
		repo, &fakeAcctTokenRevoker{}, &fakeAcctAccessBlacklister{}, &fakeAcctSessionDisconnector{}, &fakeAcctCacheInvalidator{}, accessTokenTTLForTest,
	)
	_, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)

	logs := buf.String()
	assert.NotContains(t, logs, "Alice-SECRET-PII-TOKEN", "§M13: displayName MUST NOT leak into audit log")
	assert.NotContains(t, logs, "leaked-sub-hash", "§NFR-SEC-6: apple_user_id_hash MUST NOT leak")
	assert.NotContains(t, logs, "Asia/Shanghai", "timezone not in audit scope (§NFR-SEC-10 minimization)")
}

func TestAcctDel_StrictOrder_MarkRevokeBlacklistDisconnectInvalidate(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	order := []string{}
	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true, orderSink: &order}
	rev := &fakeAcctTokenRevoker{orderSink: &order}
	bl := &fakeAcctAccessBlacklister{orderSink: &order}
	dis := &fakeAcctSessionDisconnector{orderSink: &order}
	cache := &fakeAcctCacheInvalidator{orderSink: &order}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	_, err := svc.RequestAccountDeletion(ctx, ids.UserID("u1"))
	require.NoError(t, err)

	assert.Equal(t, []string{"mark", "revoke", "blacklist", "disconnect", "invalidate"}, order,
		"§21.8 #2 (round-1 extended): order MUST be strictly mark → revoke → blacklist → disconnect → invalidate; blacklist MUST precede disconnect so a client reopen between disconnect-frame and upgrade cannot sneak through")
}

func TestAcctDel_WSUserIDTypeConversion(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{user: newUser(fixedNow), firstTime: true}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	uid := ids.UserID("00000000-0000-4000-8000-abcdef012345")
	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	_, err := svc.RequestAccountDeletion(ctx, uid)
	require.NoError(t, err)

	require.Len(t, dis.gotUsers, 1)
	assert.Equal(t, string(uid), string(dis.gotUsers[0]),
		"ws.UserID(string(uid)) conversion must preserve the lexical value byte-for-byte")
}

func TestAcctDel_EmptyUserID_RejectsBeforeSideEffects(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	ctx := runWithLoggedCtx(t, buf)

	repo := &fakeAcctUserRepo{}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	svc := service.NewAccountDeletionService(repo, rev, bl, dis, cache, accessTokenTTLForTest)
	_, err := svc.RequestAccountDeletion(ctx, "")
	require.Error(t, err)

	var appErr *dto.AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)

	assert.Zero(t, len(repo.calls), "must not hit repo when userID is empty")
}

func TestAcctDel_NilCollaborator_Panics(t *testing.T) {
	t.Parallel()

	repo := &fakeAcctUserRepo{}
	rev := &fakeAcctTokenRevoker{}
	bl := &fakeAcctAccessBlacklister{}
	dis := &fakeAcctSessionDisconnector{}
	cache := &fakeAcctCacheInvalidator{}

	assert.Panics(t, func() {
		service.NewAccountDeletionService(nil, rev, bl, dis, cache, accessTokenTTLForTest)
	})
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, nil, bl, dis, cache, accessTokenTTLForTest)
	})
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, rev, nil, dis, cache, accessTokenTTLForTest)
	})
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, rev, bl, nil, cache, accessTokenTTLForTest)
	})
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, rev, bl, dis, nil, accessTokenTTLForTest)
	})
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, rev, bl, dis, cache, 0)
	}, "non-positive accessTokenTTL must panic at construction")
	assert.Panics(t, func() {
		service.NewAccountDeletionService(repo, rev, bl, dis, cache, -1*time.Second)
	}, "negative accessTokenTTL must panic at construction")
}
