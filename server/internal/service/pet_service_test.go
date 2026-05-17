package service_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub PetRepo / stub UserRepo（与 4.6 / 4.8 / 7.3 stub 同模式：fn 字段自定义返回）
// ============================================================

type stubPetRepoForPetService struct {
	findDefaultFn          func(ctx context.Context, userID uint64) (*mysql.Pet, error)
	updateCurrentStateFn   func(ctx context.Context, petID uint64, state int8) error
	findDefaultCalls       int
	updateCurrentStateArgs []petUpdateArg
}

type petUpdateArg struct {
	petID uint64
	state int8
}

func (s *stubPetRepoForPetService) Create(ctx context.Context, p *mysql.Pet) error {
	panic("stubPetRepoForPetService.Create not configured (pet_service.SyncCurrentState should not call it)")
}

func (s *stubPetRepoForPetService) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	s.findDefaultCalls++
	if s.findDefaultFn == nil {
		return nil, stderrors.New("stub: findDefaultFn not set")
	}
	return s.findDefaultFn(ctx, userID)
}

func (s *stubPetRepoForPetService) UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error {
	s.updateCurrentStateArgs = append(s.updateCurrentStateArgs, petUpdateArg{petID: petID, state: state})
	if s.updateCurrentStateFn == nil {
		return nil
	}
	return s.updateCurrentStateFn(ctx, petID, state)
}

// FindByID Story 26.3 加到 PetRepo interface（equip 步骤 6 用）；pet_service
// 不调本方法 —— 兜底 panic 让"误调"在测试期立刻可见。
func (s *stubPetRepoForPetService) FindByID(ctx context.Context, petID uint64) (*mysql.Pet, error) {
	panic("stubPetRepoForPetService.FindByID not configured (pet_service should not call it)")
}

// stubUserRepoForPetService: 自 Story 14.4 起 service 层在 SyncCurrentState UPDATE
// 成功路径会调 FindByID 查 users.current_room_id；其他方法（Create/UpdateNickname/
// UpdateCurrentRoomID）pet service 永不调用，保持 panic 兜底以让"误调用"在测试期
// 立刻可见。
//
// **14-4 r1 后 FindByID 调用点切换到 detached goroutine 内**：单测无法用主线程
// 直接断言 findByIDCalls；optional `findByIDDone *sync.WaitGroup` 钩子让单测在
// FindByID 返回时拿到同步信号。用法：
//   - case 7 / 10 / 12（FindByID 成功 + 广播被调）：可不设 findByIDDone，靠
//     broadcastRecorder.wg 间接同步（broadcastFn 调用前 FindByID 必已返回）
//   - case 8（CurrentRoomID nil，broadcast 0 次） / case 11（FindByID err，
//     broadcast 0 次）：broadcastRecorder.wg 不会 Done，必须设 findByIDDone +
//     wg.Add(1) + Wait 等 FindByID 跑完才能断言 findByIDCalls
type stubUserRepoForPetService struct {
	findByIDFn    func(ctx context.Context, id uint64) (*mysql.User, error)
	findByIDCalls int32
	findByIDDone  *sync.WaitGroup // optional：FindByID 返回前 wg.Done() 同步信号
}

func (s *stubUserRepoForPetService) Create(ctx context.Context, u *mysql.User) error {
	panic("stubUserRepoForPetService.Create must not be called (pet service 不应调 userRepo.Create)")
}
func (s *stubUserRepoForPetService) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	panic("stubUserRepoForPetService.UpdateNickname must not be called")
}
func (s *stubUserRepoForPetService) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	atomic.AddInt32(&s.findByIDCalls, 1)
	if s.findByIDDone != nil {
		defer s.findByIDDone.Done()
	}
	if s.findByIDFn == nil {
		// Story 14.4 缺省：CurrentRoomID 为 nil（用户不在房间），不触发广播
		return &mysql.User{ID: id, CurrentRoomID: nil}, nil
	}
	return s.findByIDFn(ctx, id)
}
func (s *stubUserRepoForPetService) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	panic("stubUserRepoForPetService.UpdateCurrentRoomID must not be called")
}

// uint64Ptr 工具便利。
func uint64Ptr(v uint64) *uint64 { return &v }

// broadcastRecorder 捕获 BroadcastFn 调用入参 + 调用次数，配合 wg.Wait 让单测主
// 线程等 broadcast goroutine 跑完再断言。
type broadcastRecorder struct {
	mu         sync.Mutex
	calls      []broadcastCall
	returnErr  error
	returnSent int
	wg         *sync.WaitGroup // optional：调用后 wg.Done() 让单测主线程同步等待
}

type broadcastCall struct {
	roomID uint64
	msg    []byte
}

func (r *broadcastRecorder) fn(ctx context.Context, roomID uint64, msg []byte) (int, error) {
	r.mu.Lock()
	// **defensive copy**：caller 可能在 broadcastFn return 后 mutate buffer
	cp := make([]byte, len(msg))
	copy(cp, msg)
	r.calls = append(r.calls, broadcastCall{roomID: roomID, msg: cp})
	r.mu.Unlock()
	if r.wg != nil {
		r.wg.Done()
	}
	return r.returnSent, r.returnErr
}

func (r *broadcastRecorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *broadcastRecorder) lastCall() (broadcastCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return broadcastCall{}, false
	}
	return r.calls[len(r.calls)-1], true
}

// petEnvelopeForTest 是 ws.serverEnvelope 的测试本地 mirror（unmarshal 用）。
type petEnvelopeForTest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload"`
	Ts        int64           `json:"ts"`
}

type petStateChangedPayloadForTest struct {
	UserID       string `json:"userId"`
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// buildPetService 用 stub repo 构造 PetService。sessionMgr / broadcastFn 全部传 nil
// —— 与既有 case 1-5 兼容（不广播路径下行为不变）。
func buildPetService(petRepo mysql.PetRepo) service.PetService {
	return service.NewPetService(petRepo, &stubUserRepoForPetService{}, nil, nil)
}

// buildPetServiceWithBroadcast 注入 stubUserRepo + broadcastRecorder（Story 14.4
// 新增 case 7-12 用）。
func buildPetServiceWithBroadcast(
	petRepo mysql.PetRepo,
	userRepo mysql.UserRepo,
	bcast *broadcastRecorder,
) service.PetService {
	return service.NewPetService(petRepo, userRepo, nil, bcast.fn)
}

// ============================================================
// case 1 — happy state=2（既有；自 Story 14.4 起 user.CurrentRoomID nil → 不广播）
// ============================================================
func TestPetService_SyncCurrentState_Happy_State2(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			if userID != 10 {
				t.Errorf("userID = %d, want 10", userID)
			}
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}

	svc := buildPetService(repo)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})

	if err != nil {
		t.Fatalf("SyncCurrentState: %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	if repo.findDefaultCalls != 1 {
		t.Errorf("findDefaultCalls = %d, want 1", repo.findDefaultCalls)
	}
	if len(repo.updateCurrentStateArgs) != 1 {
		t.Fatalf("updateCurrentStateArgs len = %d, want 1", len(repo.updateCurrentStateArgs))
	}
	if repo.updateCurrentStateArgs[0] != (petUpdateArg{petID: 100, state: 2}) {
		t.Errorf("updateCurrentStateArgs[0] = %+v, want {petID:100,state:2}", repo.updateCurrentStateArgs[0])
	}
}

// ============================================================
// case 2 — pet-less noop（V1 §5.2 line 530-531 + r7 lessons）
// ============================================================
func TestPetService_SyncCurrentState_PetLess_Noop(t *testing.T) {
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound
		},
	}

	svc := buildPetService(repo)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 3})

	if err != nil {
		t.Fatalf("SyncCurrentState pet-less: expected nil err, got %v", err)
	}
	if got := apperror.Code(err); got != 0 {
		t.Errorf("apperror.Code(err) = %d, want 0 (pet-less 不触发 1003 / ErrResourceNotFound)", got)
	}
	if out == nil || out.State != 3 {
		t.Errorf("output = %+v, want &{State:3} (回显入参)", out)
	}
	if repo.findDefaultCalls != 1 {
		t.Errorf("findDefaultCalls = %d, want 1", repo.findDefaultCalls)
	}
	if len(repo.updateCurrentStateArgs) != 0 {
		t.Errorf("updateCurrentStateArgs len = %d, want 0 (noop 路径必须跳 UPDATE)", len(repo.updateCurrentStateArgs))
	}
}

// ============================================================
// case 3 — DB 异常（FindDefaultByUserID 返其他 raw error）
// ============================================================
func TestPetService_SyncCurrentState_FindDefault_DBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("connection refused")
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return nil, dbErr
		},
	}

	svc := buildPetService(repo)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})

	if out != nil {
		t.Errorf("output = %+v, want nil", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链未保留 DB cause: %v", err)
	}
	if len(repo.updateCurrentStateArgs) != 0 {
		t.Errorf("updateCurrentStateArgs len = %d, want 0 (FindDefault 失败不应进入步骤 2)", len(repo.updateCurrentStateArgs))
	}
}

// ============================================================
// case 4 — DB 异常（UpdateCurrentStateByID 返 raw error）
// ============================================================
func TestPetService_SyncCurrentState_Update_DBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("deadlock")
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return dbErr
		},
	}

	svc := buildPetService(repo)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})

	if out != nil {
		t.Errorf("output = %+v, want nil", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链未保留 DB cause: %v", err)
	}
	if repo.findDefaultCalls != 1 {
		t.Errorf("findDefaultCalls = %d, want 1", repo.findDefaultCalls)
	}
	if len(repo.updateCurrentStateArgs) != 1 {
		t.Errorf("updateCurrentStateArgs len = %d, want 1 (调用发生但失败)", len(repo.updateCurrentStateArgs))
	}
}

// ============================================================
// case 5 — 幂等同 state 重复上报（V1 §5.2 + r1 lessons）
// ============================================================
func TestPetService_SyncCurrentState_IdempotentSameStateRepeated_Succeeds(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}

	svc := buildPetService(repo)
	in := service.SyncCurrentStateInput{UserID: 10, State: 2}

	out1, err1 := svc.SyncCurrentState(context.Background(), in)
	out2, err2 := svc.SyncCurrentState(context.Background(), in)

	if err1 != nil || err2 != nil {
		t.Fatalf("err1 = %v, err2 = %v; expected both nil", err1, err2)
	}
	if out1 == nil || out2 == nil || out1.State != 2 || out2.State != 2 {
		t.Errorf("out1 = %+v, out2 = %+v; expected both &{State:2}", out1, out2)
	}
	if len(repo.updateCurrentStateArgs) != 2 {
		t.Errorf("updateCurrentStateArgs len = %d, want 2 (两次调用都进 UPDATE)", len(repo.updateCurrentStateArgs))
	}
}

// ============================================================
// case 7 — happy 用户在房间 → broadcastFn 调用 1 次 + payload 字段正确
//
// **关键点**：
//   - 用 sync.WaitGroup 同步 broadcast goroutine（broadcastRecorder.wg + Add(1)）
//   - 验证 broadcastFn 调用次数 1 + roomID == 500
//   - unmarshal msg bytes 验证 envelope 完整字段
//
// 这是 Story 14.4 AC3 case 7 的实装（取代既有 case 6 的"广播路径未被触发占位"）。
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_UserInRoom_BroadcastOnce(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}
	userRepo := &stubUserRepoForPetService{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			if id != 10 {
				t.Errorf("FindByID userID = %d, want 10", id)
			}
			return &mysql.User{ID: 10, CurrentRoomID: uint64Ptr(500)}, nil
		},
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	bcast := &broadcastRecorder{wg: wg, returnSent: 1, returnErr: nil}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})
	if err != nil {
		t.Fatalf("SyncCurrentState: %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	// 等 broadcast goroutine 跑完
	waitWithTimeout(t, wg, 2*time.Second, "broadcast goroutine did not complete")

	if got := bcast.callCount(); got != 1 {
		t.Fatalf("broadcastFn callCount = %d, want 1", got)
	}
	last, ok := bcast.lastCall()
	if !ok {
		t.Fatal("no broadcast call recorded")
	}
	if last.roomID != 500 {
		t.Errorf("broadcast roomID = %d, want 500", last.roomID)
	}

	// unmarshal envelope + payload 完整断言
	var env petEnvelopeForTest
	if err := json.Unmarshal(last.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v; msg=%s", err, string(last.msg))
	}
	if env.Type != "pet.state.changed" {
		t.Errorf("envelope.Type = %q, want \"pet.state.changed\"", env.Type)
	}
	if env.RequestID != "" {
		t.Errorf("envelope.RequestID = %q, want \"\"", env.RequestID)
	}
	if env.Ts <= 0 {
		t.Errorf("envelope.Ts = %d, want > 0", env.Ts)
	}

	var p petStateChangedPayloadForTest
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.UserID != "10" {
		t.Errorf("payload.userId = %q, want \"10\"", p.UserID)
	}
	if p.PetID != "100" {
		t.Errorf("payload.petId = %q, want \"100\"", p.PetID)
	}
	if p.CurrentState != 2 {
		t.Errorf("payload.currentState = %d, want 2", p.CurrentState)
	}

	// FindByID 调用次数 1（主路径同步 lookup）
	if got := atomic.LoadInt32(&userRepo.findByIDCalls); got != 1 {
		t.Errorf("userRepo.FindByID calls = %d, want 1", got)
	}
}

// ============================================================
// case 8 — 用户不在房间（CurrentRoomID nil）→ broadcastFn 调用 0 次
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_UserNotInRoom_NoBroadcast(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}
	userWG := &sync.WaitGroup{}
	userWG.Add(1)
	userRepo := &stubUserRepoForPetService{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 10, CurrentRoomID: nil}, nil
		},
		findByIDDone: userWG,
	}
	bcast := &broadcastRecorder{}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})
	if err != nil {
		t.Fatalf("SyncCurrentState: %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	// 14-4 r1 后 FindByID 在 detached goroutine 内执行 —— 用 findByIDDone wg 等
	// goroutine 跑到 FindByID 返回；50ms 兜底等 goroutine 走完 nil-CurrentRoomID 分支
	waitWithTimeout(t, userWG, 2*time.Second, "FindByID goroutine did not complete")
	time.Sleep(50 * time.Millisecond)
	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn callCount = %d, want 0 (CurrentRoomID nil 不广播)", got)
	}
	if got := atomic.LoadInt32(&userRepo.findByIDCalls); got != 1 {
		t.Errorf("userRepo.FindByID calls = %d, want 1", got)
	}
}

// ============================================================
// case 9 — UpdateCurrentStateByID err → broadcastFn 调用 0 次
// UPDATE 失败路径在 broadcast 触发之前 return error
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_UpdateErr_NoBroadcast(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return stderrors.New("deadlock")
		},
	}
	// 关键：UPDATE 失败时**不应**调到 FindByID（findByIDFn 不设 → 缺省返 CurrentRoomID nil）
	userRepo := &stubUserRepoForPetService{}
	bcast := &broadcastRecorder{}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})
	if out != nil {
		t.Errorf("output = %+v, want nil", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}

	time.Sleep(50 * time.Millisecond)
	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn callCount = %d, want 0 (UPDATE 失败不广播)", got)
	}
	if got := atomic.LoadInt32(&userRepo.findByIDCalls); got != 0 {
		t.Errorf("userRepo.FindByID calls = %d, want 0 (UPDATE 失败前不应调 FindByID)", got)
	}
}

// ============================================================
// case 10 — broadcastFn 自身 return error → SyncCurrentState 仍返 HTTP 200 ack
// （fire-and-forget 严格语义）
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_BroadcastFnErr_HTTP200(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}
	userRepo := &stubUserRepoForPetService{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 10, CurrentRoomID: uint64Ptr(500)}, nil
		},
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	bcast := &broadcastRecorder{
		wg:         wg,
		returnSent: 0,
		returnErr:  stderrors.New("session manager dead"),
	}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})

	// **关键断言**：fire-and-forget 严格语义 —— broadcast 失败 **不影响** HTTP 200 ack
	if err != nil {
		t.Fatalf("SyncCurrentState: expected nil err (fire-and-forget), got %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	waitWithTimeout(t, wg, 2*time.Second, "broadcast goroutine did not complete")
	if got := bcast.callCount(); got != 1 {
		t.Errorf("broadcastFn callCount = %d, want 1 (broadcastFn 被调用了，只是返 error)", got)
	}
}

// ============================================================
// case 11 — FindByID err → log warn + 不 broadcast + HTTP 200
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_FindByIDErr_HTTP200_NoBroadcast(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}
	userWG := &sync.WaitGroup{}
	userWG.Add(1)
	userRepo := &stubUserRepoForPetService{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, stderrors.New("connection refused")
		},
		findByIDDone: userWG,
	}
	bcast := &broadcastRecorder{}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})

	// FindByID 失败也不影响 HTTP 200 ack（fire-and-forget）
	if err != nil {
		t.Fatalf("SyncCurrentState: expected nil err, got %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	// 14-4 r1 后 FindByID 在 detached goroutine 内执行 —— 用 findByIDDone wg 等
	// goroutine 跑到 FindByID 返回；50ms 兜底等 goroutine 走完 err 分支
	waitWithTimeout(t, userWG, 2*time.Second, "FindByID goroutine did not complete")
	time.Sleep(50 * time.Millisecond)
	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn callCount = %d, want 0 (FindByID 失败不应启 goroutine 广播)", got)
	}
	if got := atomic.LoadInt32(&userRepo.findByIDCalls); got != 1 {
		t.Errorf("userRepo.FindByID calls = %d, want 1", got)
	}
}

// ============================================================
// case 12 — 同一秒多次 state-sync 每次都广播（V1 §12.3 line 2253 钦定）
// ============================================================
func TestPetService_SyncCurrentState_Story144_Broadcast_RepeatedSameSecond_BroadcastsEach(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}
	userRepo := &stubUserRepoForPetService{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 10, CurrentRoomID: uint64Ptr(500)}, nil
		},
	}
	wg := &sync.WaitGroup{}
	wg.Add(3)
	bcast := &broadcastRecorder{wg: wg, returnSent: 1}

	svc := buildPetServiceWithBroadcast(repo, userRepo, bcast)
	for i := 0; i < 3; i++ {
		_, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})
		if err != nil {
			t.Fatalf("SyncCurrentState iter %d: %v", i, err)
		}
	}

	waitWithTimeout(t, wg, 2*time.Second, "broadcast goroutines did not complete")
	if got := bcast.callCount(); got != 3 {
		t.Errorf("broadcastFn callCount = %d, want 3 (每次都广播，不去重)", got)
	}
}

// waitWithTimeout 等 wg.Wait() 在 timeout 内完成，否则 t.Fatalf。
func waitWithTimeout(t *testing.T, wg *sync.WaitGroup, timeout time.Duration, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s (timeout %v)", msg, timeout)
	}
}
