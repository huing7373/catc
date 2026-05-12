package service_test

import (
	"context"
	stderrors "errors"
	"testing"

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

// stubUserRepoForPetService: 本 story service 不调 userRepo（14.4 才用）；stub 全部
// 方法直接 panic 兜底（确保任何"误调用"在测试期立刻可见）。
type stubUserRepoForPetService struct{}

func (s *stubUserRepoForPetService) Create(ctx context.Context, u *mysql.User) error {
	panic("stubUserRepoForPetService.Create must not be called (pet service 不应调 userRepo)")
}
func (s *stubUserRepoForPetService) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	panic("stubUserRepoForPetService.UpdateNickname must not be called")
}
func (s *stubUserRepoForPetService) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	panic("stubUserRepoForPetService.FindByID must not be called (本 story service 不查 users 表；14.4 才用)")
}
func (s *stubUserRepoForPetService) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	panic("stubUserRepoForPetService.UpdateCurrentRoomID must not be called")
}

// buildPetService 用 stub repo 构造 PetService。sessionMgr / broadcastFn 全部传 nil
// —— 本 story 不广播；14.4 才 wire 真实实例。
//
// **断言**：service 在 nil sessionMgr / nil broadcastFn 下 SyncCurrentState 不 panic
// —— 这是 router.go wire 时（sessionMgr 可能为 nil HTTP-only fixture）的兼容性保障。
func buildPetService(petRepo mysql.PetRepo) service.PetService {
	return service.NewPetService(petRepo, &stubUserRepoForPetService{}, nil, nil)
}

// ============================================================
// 6 个 case（AC3 钦定 ≥4 case + 可选 case 5 / 6）
// ============================================================

// case 1 — happy state=2
// mock petRepo.FindDefaultByUserID 返 &Pet{ID: 100} / UpdateCurrentStateByID 返 nil
// → 调 SyncCurrentState({UserID: 10, State: 2}) → 验证返回 &SyncCurrentStateOutput{State: 2}
// + nil error + repo 调用次数（FindDefaultByUserID 1 次 / UpdateCurrentStateByID 1 次带 petID=100, state=2）。
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

// case 2 — pet-less noop（V1 §5.2 line 530-531 + r7 lessons）
// mock petRepo.FindDefaultByUserID 返 mysql.ErrPetNotFound → 调 SyncCurrentState
// ({UserID: 10, State: 3}) → 验证返回 &SyncCurrentStateOutput{State: 3} (回显入参) + nil error
// + repo 调用次数（FindDefaultByUserID 1 次 / UpdateCurrentStateByID **0 次** —— 这是 noop 路径关键）。
//
// **断言禁止**：不验证任何 errors.Is + apperror.Code，因为 pet-less 走 noop 路径 nil error
// （r7 lessons：pet-less **不**触发 1003 / ErrResourceNotFound）。
func TestPetService_SyncCurrentState_PetLess_Noop(t *testing.T) {
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound
		},
	}

	svc := buildPetService(repo)
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 3})

	// r7 锁定：pet-less 走 server-acknowledged noop 路径 → nil error
	if err != nil {
		t.Fatalf("SyncCurrentState pet-less: expected nil err, got %v", err)
	}
	// **断言不触发 1003**（r7 锁定）—— apperror.Code(err) 应为 0（nil err → 0）
	if got := apperror.Code(err); got != 0 {
		t.Errorf("apperror.Code(err) = %d, want 0 (pet-less 不触发 1003 / ErrResourceNotFound)", got)
	}
	// 回显入参 state
	if out == nil || out.State != 3 {
		t.Errorf("output = %+v, want &{State:3} (回显入参)", out)
	}
	// **关键校验**：UpdateCurrentStateByID 0 次调用（noop 跳 UPDATE）
	if repo.findDefaultCalls != 1 {
		t.Errorf("findDefaultCalls = %d, want 1", repo.findDefaultCalls)
	}
	if len(repo.updateCurrentStateArgs) != 0 {
		t.Errorf("updateCurrentStateArgs len = %d, want 0 (noop 路径必须跳 UPDATE)", len(repo.updateCurrentStateArgs))
	}
}

// case 3 — DB 异常（FindDefaultByUserID 返其他 raw error）
// mock petRepo.FindDefaultByUserID 返 errors.New("connection refused") → 调 SyncCurrentState
// → 验证返回 nil output + apperror.Code(err) == ErrServiceBusy (1009)
// + UpdateCurrentStateByID **0 次调用**（未到达步骤 2）。
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
	// errors.Is 穿透校验：底层错误应可追溯
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链未保留 DB cause: %v", err)
	}
	if len(repo.updateCurrentStateArgs) != 0 {
		t.Errorf("updateCurrentStateArgs len = %d, want 0 (FindDefault 失败不应进入步骤 2)", len(repo.updateCurrentStateArgs))
	}
}

// case 4 — DB 异常（UpdateCurrentStateByID 返 raw error）
// mock petRepo.FindDefaultByUserID 返 &Pet{ID: 100} / UpdateCurrentStateByID 返
// errors.New("deadlock") → 调 SyncCurrentState → 验证返回 nil output + apperror.Code
// == ErrServiceBusy (1009) + FindDefaultByUserID 1 次 + UpdateCurrentStateByID 1 次
// （**确认调用发生但失败**，不被跳过）。
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
	// **关键校验**：UpdateCurrentStateByID **确实被调用了**（不被跳过），但失败 → 1009
	if len(repo.updateCurrentStateArgs) != 1 {
		t.Errorf("updateCurrentStateArgs len = %d, want 1 (调用发生但失败)", len(repo.updateCurrentStateArgs))
	}
}

// case 5（可选 ≥4 要求外）— 幂等同 state 重复上报（V1 §5.2 line 500 元信息表 +
// 服务端逻辑步骤 4 + r1 lessons）
// mock petRepo.UpdateCurrentStateByID 返 nil（即便业务上"同 user 同 state 重复上报"也 mock nil，
// **禁止**为该 case 让 mock 返 RowsAffected == 0 —— service 层不读 RowsAffected，case
// 与 case 1 行为完全等价）→ 调 SyncCurrentState({UserID: 10, State: 2}) 连续 2 次 →
// 两次都返 &SyncCurrentStateOutput{State: 2} + nil error。
func TestPetService_SyncCurrentState_IdempotentSameStateRepeated_Succeeds(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			// 不读 RowsAffected；两次调用都返 nil error（与 case 1 等价）
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
	// 两次都应触发 UPDATE（service 不缓存 / 不去重；DB 引擎自身处理"同值 UPDATE"幂等）
	if len(repo.updateCurrentStateArgs) != 2 {
		t.Errorf("updateCurrentStateArgs len = %d, want 2 (两次调用都进 UPDATE)", len(repo.updateCurrentStateArgs))
	}
}

// case 6（可选 ≥4 要求外）— 广播路径未被触发的 wire 占位
// 用 mock broadcastFn / mock sessionMgr 注入 NewPetService → 调 SyncCurrentState happy
// → 验证 broadcastFn / sessionMgr 任何方法 **0 次调用**（本 story 不广播；14.4 单测才覆盖广播路径）。
//
// **本 story 严格断言**：service 不调 broadcastFn closure（counter 始终 0）；service
// 不调 sessionMgr 任何方法（实现是"宁可 panic 也不静默" —— stubSessionMgr 所有方法 panic）。
func TestPetService_SyncCurrentState_BroadcastNotInvoked_PreWireForStory144(t *testing.T) {
	pet := &mysql.Pet{ID: 100, UserID: 10, CurrentState: 1, IsDefault: 1}
	repo := &stubPetRepoForPetService{
		findDefaultFn: func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return pet, nil
		},
		updateCurrentStateFn: func(ctx context.Context, petID uint64, state int8) error {
			return nil
		},
	}

	// broadcastFn counter：本 story service **不**调用 broadcastFn
	broadcastCalls := 0
	bcastFn := func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		broadcastCalls++
		return 0, nil
	}
	// sessionMgr nil —— 本 story service **不**调用任何 ws 包导出函数；与 router.go
	// HTTP-only fixture wire `nil sessionMgr` 兼容性一致
	svc := service.NewPetService(repo, &stubUserRepoForPetService{}, nil, bcastFn)

	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{UserID: 10, State: 2})
	if err != nil {
		t.Fatalf("SyncCurrentState: %v", err)
	}
	if out.State != 2 {
		t.Errorf("out.State = %d, want 2", out.State)
	}

	// **关键断言**：broadcastFn 必须为 0 次调用（14.4 才接管广播）
	if broadcastCalls != 0 {
		t.Errorf("broadcastCalls = %d, want 0 (本 story 不广播；14.4 才接管)", broadcastCalls)
	}
}
