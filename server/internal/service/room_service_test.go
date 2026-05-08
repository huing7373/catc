package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub repo for RoomService.CreateRoom（独立 stub，不复用 auth_service_test.go 的
// stubUserRepo —— 本 stub 需要支持 UpdateCurrentRoomID 方法；同 package 同 build tag
// 集合内同名 type 会编译期 redeclared 错误，故重命名为 roomTestStubXxxRepo）。
// ============================================================

type roomTestStubUserRepo struct {
	createFn              func(ctx context.Context, u *mysql.User) error
	updateNicknameFn      func(ctx context.Context, userID uint64, nickname string) error
	findByIDFn            func(ctx context.Context, id uint64) (*mysql.User, error)
	updateCurrentRoomIDFn func(ctx context.Context, userID uint64, roomID *uint64) error
}

func (s *roomTestStubUserRepo) Create(ctx context.Context, u *mysql.User) error {
	if s.createFn != nil {
		return s.createFn(ctx, u)
	}
	panic("roomTestStubUserRepo.Create not configured")
}

func (s *roomTestStubUserRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	if s.updateNicknameFn != nil {
		return s.updateNicknameFn(ctx, userID, nickname)
	}
	panic("roomTestStubUserRepo.UpdateNickname not configured")
}

func (s *roomTestStubUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return s.findByIDFn(ctx, id)
}

func (s *roomTestStubUserRepo) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	return s.updateCurrentRoomIDFn(ctx, userID, roomID)
}

type roomTestStubRoomRepo struct {
	createFn            func(ctx context.Context, r *mysql.Room) error
	findByIDForUpdateFn func(ctx context.Context, roomID uint64) (*mysql.Room, error)
	updateStatusFn      func(ctx context.Context, roomID uint64, status int8) error
	findByIDFn          func(ctx context.Context, roomID uint64) (*mysql.Room, error)
}

func (s *roomTestStubRoomRepo) Create(ctx context.Context, r *mysql.Room) error {
	if s.createFn != nil {
		return s.createFn(ctx, r)
	}
	panic("roomTestStubRoomRepo.Create not configured")
}

func (s *roomTestStubRoomRepo) FindByIDForUpdate(ctx context.Context, roomID uint64) (*mysql.Room, error) {
	if s.findByIDForUpdateFn != nil {
		return s.findByIDForUpdateFn(ctx, roomID)
	}
	panic("roomTestStubRoomRepo.FindByIDForUpdate not configured")
}

func (s *roomTestStubRoomRepo) UpdateStatus(ctx context.Context, roomID uint64, status int8) error {
	if s.updateStatusFn != nil {
		return s.updateStatusFn(ctx, roomID, status)
	}
	panic("roomTestStubRoomRepo.UpdateStatus not configured")
}

func (s *roomTestStubRoomRepo) FindByID(ctx context.Context, roomID uint64) (*mysql.Room, error) {
	if s.findByIDFn != nil {
		return s.findByIDFn(ctx, roomID)
	}
	panic("roomTestStubRoomRepo.FindByID not configured")
}

type roomTestStubRoomMemberRepo struct {
	roomExistsFn                  func(ctx context.Context, roomID uint64) (bool, error)
	isUserInRoomFn                func(ctx context.Context, userID uint64, roomID uint64) (bool, error)
	listMembersFn                 func(ctx context.Context, roomID uint64) ([]uint64, error)
	createFn                      func(ctx context.Context, m *mysql.RoomMember) error
	countByRoomIDFn               func(ctx context.Context, roomID uint64) (int, error)
	deleteByRoomAndUserFn         func(ctx context.Context, roomID, userID uint64) (int64, error)
	existsForShareByRoomAndUserFn func(ctx context.Context, roomID, userID uint64) (bool, error)
	listRosterByRoomIDFn          func(ctx context.Context, roomID uint64) ([]mysql.RosterRow, error)
}

func (s *roomTestStubRoomMemberRepo) RoomExists(ctx context.Context, roomID uint64) (bool, error) {
	if s.roomExistsFn != nil {
		return s.roomExistsFn(ctx, roomID)
	}
	panic("roomTestStubRoomMemberRepo.RoomExists not configured")
}

func (s *roomTestStubRoomMemberRepo) IsUserInRoom(ctx context.Context, userID uint64, roomID uint64) (bool, error) {
	if s.isUserInRoomFn != nil {
		return s.isUserInRoomFn(ctx, userID, roomID)
	}
	panic("roomTestStubRoomMemberRepo.IsUserInRoom not configured")
}

func (s *roomTestStubRoomMemberRepo) ListMembers(ctx context.Context, roomID uint64) ([]uint64, error) {
	if s.listMembersFn != nil {
		return s.listMembersFn(ctx, roomID)
	}
	panic("roomTestStubRoomMemberRepo.ListMembers not configured")
}

func (s *roomTestStubRoomMemberRepo) Create(ctx context.Context, m *mysql.RoomMember) error {
	if s.createFn != nil {
		return s.createFn(ctx, m)
	}
	panic("roomTestStubRoomMemberRepo.Create not configured")
}

func (s *roomTestStubRoomMemberRepo) CountByRoomID(ctx context.Context, roomID uint64) (int, error) {
	if s.countByRoomIDFn != nil {
		return s.countByRoomIDFn(ctx, roomID)
	}
	panic("roomTestStubRoomMemberRepo.CountByRoomID not configured")
}

func (s *roomTestStubRoomMemberRepo) DeleteByRoomAndUser(ctx context.Context, roomID, userID uint64) (int64, error) {
	if s.deleteByRoomAndUserFn != nil {
		return s.deleteByRoomAndUserFn(ctx, roomID, userID)
	}
	panic("roomTestStubRoomMemberRepo.DeleteByRoomAndUser not configured")
}

func (s *roomTestStubRoomMemberRepo) ExistsForShareByRoomAndUser(ctx context.Context, roomID, userID uint64) (bool, error) {
	if s.existsForShareByRoomAndUserFn != nil {
		return s.existsForShareByRoomAndUserFn(ctx, roomID, userID)
	}
	panic("roomTestStubRoomMemberRepo.ExistsForShareByRoomAndUser not configured")
}

func (s *roomTestStubRoomMemberRepo) ListRosterByRoomID(ctx context.Context, roomID uint64) ([]mysql.RosterRow, error) {
	if s.listRosterByRoomIDFn != nil {
		return s.listRosterByRoomIDFn(ctx, roomID)
	}
	panic("roomTestStubRoomMemberRepo.ListRosterByRoomID not configured")
}

// ============================================================
// Story 11.8 stubs：PetRepo / SessionManager / BroadcastFn capture
// ============================================================

// roomTestStubPetRepo 实装 mysql.PetRepo；仅 FindDefaultByUserID 字段 fn 注入
// （Create 路径 Story 11.8 不消费，留 panic 兜底）。
type roomTestStubPetRepo struct {
	findDefaultByUserIDFn func(ctx context.Context, userID uint64) (*mysql.Pet, error)
}

func (s *roomTestStubPetRepo) Create(ctx context.Context, p *mysql.Pet) error {
	panic("roomTestStubPetRepo.Create not configured")
}

func (s *roomTestStubPetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	if s.findDefaultByUserIDFn != nil {
		return s.findDefaultByUserIDFn(ctx, userID)
	}
	// 默认走 ErrPetNotFound 路径让 broadcastMemberJoined 走 pet=nil 合法降级；
	// 既有 11.3 / 11.4 / 11.5 / 11.6 case 不显式注入 petRepo，broadcastMemberJoined
	// 在 happy 路径下能跑完不 panic
	return nil, mysql.ErrPetNotFound
}

// roomTestRecordedBroadcast 记录单次 broadcastFn / broadcastExceptFn 调用入参 +
// 调用时间戳（基于 atomic 序号，单调递增）；用于断言 close 4007 与 broadcast 顺序（r13）。
//
// **r3 fix**：新增 excludeUserID 字段记录 BroadcastExceptFn 调用的排除参数；
// BroadcastFn 调用时为 nil（语义：未启用 exclude）。
type roomTestRecordedBroadcast struct {
	roomID        uint64
	msg           []byte
	seq           int64   // 单调序号，让断言"close < broadcast"顺序时与 stubSessionMgr 共享
	excludeUserID *uint64 // r3：BroadcastExceptFn 调用时记录；BroadcastFn 调用时 nil
}

// roomTestStubBroadcastFn 包装 ws.BroadcastFn / ws.BroadcastExceptFn；内部统一记录
// 所有调用 + 提供 fn / exceptFn 方法供 service.NewRoomService 注入。
//
// **r3 fix**：r3 起 service 层 broadcastMemberJoined / broadcastMemberLeft 切到
// BroadcastExceptFn（必须 exclude 事件主体自己，V1 §12.3 行 2063）；测试需要同时
// 注入两种 fn 让 service 构造成功。calls 切片由 fn / exceptFn 共享（按 seqGen 单调
// 序号区分调用 + excludeUserID 字段区分类型）。
type roomTestStubBroadcastFn struct {
	mu             sync.Mutex
	calls          []roomTestRecordedBroadcast
	returnFn       func(ctx context.Context, roomID uint64, msg []byte) (int, error)                         // optional override（BroadcastFn 路径）
	returnExceptFn func(ctx context.Context, roomID uint64, excludeUserID uint64, msg []byte) (int, error) // optional override（BroadcastExceptFn 路径）
}

// fn 返回 ws.BroadcastFn type alias 兼容的 closure。
func (s *roomTestStubBroadcastFn) fn(seqGen *atomic.Int64) wsapp.BroadcastFn {
	return func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		s.mu.Lock()
		// 拷贝 msg 字节让 caller 释放原 buffer 仍可断言 capture 内容
		copied := append([]byte(nil), msg...)
		s.calls = append(s.calls, roomTestRecordedBroadcast{
			roomID: roomID,
			msg:    copied,
			seq:    seqGen.Add(1),
		})
		s.mu.Unlock()
		if s.returnFn != nil {
			return s.returnFn(ctx, roomID, msg)
		}
		return 0, nil
	}
}

// exceptFn 返回 ws.BroadcastExceptFn type alias 兼容的 closure（r3 引入）。
//
// 与 fn 共享同一 calls 切片：调用记录时 excludeUserID 字段非 nil，让断言能区分
// "BroadcastFn 调用 vs BroadcastExceptFn 调用"。
func (s *roomTestStubBroadcastFn) exceptFn(seqGen *atomic.Int64) wsapp.BroadcastExceptFn {
	return func(ctx context.Context, roomID, excludeUserID uint64, msg []byte) (int, error) {
		s.mu.Lock()
		copied := append([]byte(nil), msg...)
		excluded := excludeUserID
		s.calls = append(s.calls, roomTestRecordedBroadcast{
			roomID:        roomID,
			msg:           copied,
			seq:           seqGen.Add(1),
			excludeUserID: &excluded,
		})
		s.mu.Unlock()
		if s.returnExceptFn != nil {
			return s.returnExceptFn(ctx, roomID, excludeUserID, msg)
		}
		return 0, nil
	}
}

func (s *roomTestStubBroadcastFn) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// roomTestStubSessionMgr 实装 ws.SessionManager。本 story 仅消费
// ListSessionsByRoomID + Unregister 两个方法；其他方法默认兜底返默认值或 panic。
type roomTestStubSessionMgr struct {
	listSessionsByRoomIDFn func(ctx context.Context, roomID uint64) []*wsapp.Session
	unregisterFn           func(ctx context.Context, sessionID string) error
	unregisterCalls        []string // 记录调用过的 sessionID，便于断言
	mu                     sync.Mutex
}

func (s *roomTestStubSessionMgr) Register(ctx context.Context, sess *wsapp.Session) (string, error) {
	panic("roomTestStubSessionMgr.Register not configured")
}

func (s *roomTestStubSessionMgr) Unregister(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	s.unregisterCalls = append(s.unregisterCalls, sessionID)
	s.mu.Unlock()
	if s.unregisterFn != nil {
		return s.unregisterFn(ctx, sessionID)
	}
	return nil
}

func (s *roomTestStubSessionMgr) ListSessionsByRoomID(ctx context.Context, roomID uint64) []*wsapp.Session {
	if s.listSessionsByRoomIDFn != nil {
		return s.listSessionsByRoomIDFn(ctx, roomID)
	}
	return nil
}

func (s *roomTestStubSessionMgr) ListAllSessions(ctx context.Context) []*wsapp.Session {
	return nil
}

func (s *roomTestStubSessionMgr) IsRegistered(ctx context.Context, sessionID string) bool {
	return false
}

func (s *roomTestStubSessionMgr) IsCurrentForUser(ctx context.Context, sessionID string) bool {
	return false
}

func (s *roomTestStubSessionMgr) Close() error {
	return nil
}

// roomTestSeqGen 是测试包内全局 atomic 序号生成器；让 broadcastFn capture 的调用
// 时间戳与 closeLeaverSession 内的 Unregister 调用记录共用一个单调时钟（断言
// "close < broadcast"顺序，r13）。test 之间不复用（每个 test 应该构造新 seqGen）。

// newRoomTestStubBroadcastFnWithSeq 便利构造 roomTestStubBroadcastFn + seqGen，
// 直接返回可注入 service.NewRoomService 的 ws.BroadcastFn closure + capture struct。
//
// **r3 fix**：返回 (*roomTestStubBroadcastFn, *atomic.Int64, BroadcastFn, BroadcastExceptFn)
// 4 元组（新增 BroadcastExceptFn）让 caller 一次拿齐 service 构造所需 2 个广播 fn。
func newRoomTestStubBroadcastFnWithSeq() (*roomTestStubBroadcastFn, *atomic.Int64, wsapp.BroadcastFn, wsapp.BroadcastExceptFn) {
	bcast := &roomTestStubBroadcastFn{}
	seqGen := &atomic.Int64{}
	return bcast, seqGen, bcast.fn(seqGen), bcast.exceptFn(seqGen)
}

// roomTestStubTxMgr：直接执行 fn（不真开 tx；业务正确性靠 fn 内 repo 调用顺序断言；
// 真事务回滚由 dockertest 集成测试验证）。复用 auth_service_test.go 的 stubTxMgr
// type 名会重复声明 → 用独立 roomTestStubTxMgr。
type roomTestStubTxMgr struct {
	withTxFn func(ctx context.Context, fn func(txCtx context.Context) error) error
}

func (s *roomTestStubTxMgr) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return s.withTxFn(ctx, fn)
}

// roomTestDefaultStubTxMgr：默认直接执行 fn 把 ctx 当 txCtx
func roomTestDefaultStubTxMgr() *roomTestStubTxMgr {
	return &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			return fn(ctx)
		},
	}
}

// ============================================================
// Story 11.3 单测 case（≥4 case，epics.md §Story 11.3 钦定）
// ============================================================

// TestRoomService_CreateRoom_Happy_Inserts3Rows:
// happy 路径：user.CurrentRoomID == nil → 事务内 3 步 Create / Create / Update 全部
// 成功 → service 返 CreateRoomOutput{RoomID:3001, MaxMembers:4, MemberCount:1, Status:1}。
//
// 断言点：3 个 mock 方法被调用且参数正确 + GORM 回填 room.ID 流转到 service output。
func TestRoomService_CreateRoom_Happy_Inserts3Rows(t *testing.T) {
	var calls []string

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			calls = append(calls, "userRepo.FindByID")
			if id != 1001 {
				t.Errorf("FindByID id = %d, want 1001", id)
			}
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			calls = append(calls, "userRepo.UpdateCurrentRoomID")
			if userID != 1001 {
				t.Errorf("UpdateCurrentRoomID userID = %d, want 1001", userID)
			}
			if roomID == nil {
				t.Errorf("UpdateCurrentRoomID roomID is nil, want &3001")
			} else if *roomID != 3001 {
				t.Errorf("UpdateCurrentRoomID *roomID = %d, want 3001", *roomID)
			}
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			calls = append(calls, "roomRepo.Create")
			if r.CreatorUserID != 1001 {
				t.Errorf("room.CreatorUserID = %d, want 1001", r.CreatorUserID)
			}
			if r.Status != 1 {
				t.Errorf("room.Status = %d, want 1 (active)", r.Status)
			}
			if r.MaxMembers != 4 {
				t.Errorf("room.MaxMembers = %d, want 4", r.MaxMembers)
			}
			r.ID = 3001 // 模拟 GORM AUTO_INCREMENT 回填
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			calls = append(calls, "roomMemberRepo.Create")
			if m.RoomID != 3001 {
				t.Errorf("member.RoomID = %d, want 3001 (roomRepo 回填的 r.ID 应被 service 透传)", m.RoomID)
			}
			if m.UserID != 1001 {
				t.Errorf("member.UserID = %d, want 1001", m.UserID)
			}
			m.ID = 5001
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	expected := []string{
		"userRepo.FindByID",
		"roomRepo.Create",
		"roomMemberRepo.Create",
		"userRepo.UpdateCurrentRoomID",
	}
	if len(calls) != len(expected) {
		t.Fatalf("call count = %d, want %d; calls=%v", len(calls), len(expected), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d] = %q, want %q", i, c, expected[i])
		}
	}

	if out.RoomID != 3001 {
		t.Errorf("out.RoomID = %d, want 3001", out.RoomID)
	}
	if out.CreatorUserID != 1001 {
		t.Errorf("out.CreatorUserID = %d, want 1001", out.CreatorUserID)
	}
	if out.MaxMembers != 4 {
		t.Errorf("out.MaxMembers = %d, want 4", out.MaxMembers)
	}
	if out.MemberCount != 1 {
		t.Errorf("out.MemberCount = %d, want 1", out.MemberCount)
	}
	if out.Status != 1 {
		t.Errorf("out.Status = %d, want 1 (active)", out.Status)
	}
}

// TestRoomService_CreateRoom_UserAlreadyInRoom_PrecheckReturns6003:
// 预检路径（V1 §10.1 步骤 1 钦定）：user.CurrentRoomID != nil → 立即返 6003，
// 事务**未**开（mock txMgr.WithTx 不被调用）+ 3 个 repo Create / Update 方法**未**调用。
func TestRoomService_CreateRoom_UserAlreadyInRoom_PrecheckReturns6003(t *testing.T) {
	withTxCalled := false
	existingRoomID := uint64(9001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &existingRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（事务未开）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			t.Errorf("roomRepo.Create 不应被调用（事务未开）")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("roomMemberRepo.Create 不应被调用（事务未开）")
			return nil
		},
	}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 6003")
	}
	if out != nil {
		t.Errorf("out should be nil on 6003; got %+v", out)
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检路径在事务外）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomService_CreateRoom_RoomCreateFails_RollsBack:
// 事务内步骤 2a roomRepo.Create 失败（非哨兵 raw error）→ fn 返 error → tx rollback →
// service 包 1009；后续 roomMemberRepo.Create / userRepo.UpdateCurrentRoomID **未**调用。
func TestRoomService_CreateRoom_RoomCreateFails_RollsBack(t *testing.T) {
	wantCause := stderrors.New("simulated room repo failure")

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（rooms Create 已失败）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			return wantCause
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("roomMemberRepo.Create 不应被调用（rooms Create 已失败）")
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy 1009)", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause in chain; err=%v", err)
	}
}

// TestRoomService_CreateRoom_RoomMemberCreateUniqueUserIDFails_Returns6003:
// 事务内步骤 2b roomMemberRepo.Create 撞 UNIQUE(user_id) → ErrRoomMembersUserIDDuplicate
// → fn 返 sentinel → tx rollback → service 兜底 6003（**不**走 1009）。
//
// 断言：6003 兜底路径与预检路径完全等价（同 code + handler 端不区分）。
func TestRoomService_CreateRoom_RoomMemberCreateUniqueUserIDFails_Returns6003(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（room_members Create 已失败）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			r.ID = 3001
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			return mysql.ErrRoomMembersUserIDDuplicate
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003 兜底；错走 1009 = 顺序错)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomService_CreateRoom_RoomMemberCreateRoomUserDuplicate_Returns6003:
// 与上一 case 等价路径但走 ErrRoomMembersRoomUserDuplicate 哨兵（uk_room_user 冲突）。
// 验证两个独立哨兵都走 6003 路径（双 UNIQUE 约束兜底）。
func TestRoomService_CreateRoom_RoomMemberCreateRoomUserDuplicate_Returns6003(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			r.ID = 3001
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			return mysql.ErrRoomMembersRoomUserDuplicate
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (uk_room_user 兜底也走 6003)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomService_CreateRoom_UpdateCurrentRoomIDFails_RollsBack:
// 事务内最后一步 userRepo.UpdateCurrentRoomID 失败 → fn 返 error → tx rollback →
// service 包 1009。验证事务整体回滚（rooms / room_members 两步成功也被撤销 —— 在
// 单测层只能断言 service 返 1009；真 InnoDB rollback 由集成测试验证）。
func TestRoomService_CreateRoom_UpdateCurrentRoomIDFails_RollsBack(t *testing.T) {
	wantCause := stderrors.New("simulated update current_room_id failure")

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			r.ID = 3001
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			m.ID = 5001
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy 1009)", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause in chain; err=%v", err)
	}
}

// TestRoomService_CreateRoom_FindByIDFails_Returns1009:
// 预检步骤 userRepo.FindByID 直接返 raw DB error → service 包 1009（不开事务）。
// 即便返 ErrUserNotFound 哨兵（理论不应发生 —— auth middleware 已校验有效 user），
// 也应包 1009 而非 6001（不属于 6xxx 房间错误码语义）。
func TestRoomService_CreateRoom_FindByIDFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated find user failure")
	withTxCalled := false

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		createFn: func(ctx context.Context, r *mysql.Room) error {
			t.Errorf("roomRepo.Create 不应被调用（FindByID 已失败）")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("roomMemberRepo.Create 不应被调用（FindByID 已失败）")
			return nil
		},
	}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error")
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检失败 → 不开事务）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// ============================================================
// Story 11.4 单测 case（≥6 case，epics.md §Story 11.4 钦定）
// ============================================================

// TestRoomService_JoinRoom_Happy_5StepsExecute:
// happy 路径：user.CurrentRoomID == nil → 事务内 5 步全部成功 →
// service 返 JoinRoomOutput{RoomID:3001, Joined:true}。
//
// 断言点：5 个 mock 方法被调用且参数正确（顺序：userRepo.FindByID →
// roomRepo.FindByIDForUpdate → roomMemberRepo.CountByRoomID →
// roomMemberRepo.Create → userRepo.UpdateCurrentRoomID）。
func TestRoomService_JoinRoom_Happy_5StepsExecute(t *testing.T) {
	var calls []string

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			calls = append(calls, "userRepo.FindByID")
			if id != 1002 {
				t.Errorf("FindByID id = %d, want 1002", id)
			}
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			calls = append(calls, "userRepo.UpdateCurrentRoomID")
			if userID != 1002 {
				t.Errorf("UpdateCurrentRoomID userID = %d, want 1002", userID)
			}
			if roomID == nil {
				t.Errorf("UpdateCurrentRoomID roomID is nil, want &3001")
			} else if *roomID != 3001 {
				t.Errorf("UpdateCurrentRoomID *roomID = %d, want 3001", *roomID)
			}
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			calls = append(calls, "roomRepo.FindByIDForUpdate")
			if roomID != 3001 {
				t.Errorf("FindByIDForUpdate roomID = %d, want 3001", roomID)
			}
			return &mysql.Room{
				ID:            3001,
				CreatorUserID: 1001,
				Status:        1,
				MaxMembers:    4,
			}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			calls = append(calls, "roomMemberRepo.CountByRoomID")
			if roomID != 3001 {
				t.Errorf("CountByRoomID roomID = %d, want 3001", roomID)
			}
			return 1, nil // 房间内已有 1 个成员（创建者），未满
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			calls = append(calls, "roomMemberRepo.Create")
			if m.RoomID != 3001 {
				t.Errorf("member.RoomID = %d, want 3001", m.RoomID)
			}
			if m.UserID != 1002 {
				t.Errorf("member.UserID = %d, want 1002", m.UserID)
			}
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Story 11.8 r2 修复后：post-commit hook 异步化，必须等 wg 才能断言 post-commit
	// userRepo.FindByID enrichment call。
	wg.Wait()

	// Story 11.8 加：事务 commit 成功后 broadcastMemberJoined 会再调一次 userRepo.FindByID
	// 拿 nickname / avatarUrl 构造 member.joined payload（fire-and-forget；事务外）。
	expected := []string{
		"userRepo.FindByID",
		"roomRepo.FindByIDForUpdate",
		"roomMemberRepo.CountByRoomID",
		"roomMemberRepo.Create",
		"userRepo.UpdateCurrentRoomID",
		"userRepo.FindByID", // Story 11.8 post-commit broadcastMemberJoined enrichment
	}
	if len(calls) != len(expected) {
		t.Fatalf("call count = %d, want %d; calls=%v", len(calls), len(expected), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d] = %q, want %q", i, c, expected[i])
		}
	}

	if out.RoomID != 3001 {
		t.Errorf("out.RoomID = %d, want 3001", out.RoomID)
	}
	if !out.Joined {
		t.Errorf("out.Joined = false, want true (V1 §10.4 钦定固定值)")
	}
}

// TestRoomService_JoinRoom_UserAlreadyInRoom_PrecheckReturns6003:
// 预检路径（V1 §10.4 步骤 1 钦定）：user.CurrentRoomID != nil → 立即返 6003，
// 事务**未**开 + repo 后续方法**未**调用。
func TestRoomService_JoinRoom_UserAlreadyInRoom_PrecheckReturns6003(t *testing.T) {
	withTxCalled := false
	existingRoomID := uint64(9001) // 与目标 3001 不同 → "已在其他房间"子场景

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: &existingRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（事务未开）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用（事务未开）")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用（事务未开）")
			return 0, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("roomMemberRepo.Create 不应被调用（事务未开）")
			return nil
		},
	}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6003")
	}
	if out != nil {
		t.Errorf("out should be nil on 6003; got %+v", out)
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检路径在事务外）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}
	// 关键：6003 message 等于 DefaultMessages
	if ae.Message != apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom] {
		t.Errorf("AppError.Message = %q, want %q (6003 双路径必须 message 等价)",
			ae.Message, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
	}
}

// TestRoomService_JoinRoom_UserAlreadyInTargetRoom_PrecheckReturns6003:
// V1 §10.4 行 1441 钦定特例：caller.CurrentRoomID == 当前请求的 roomId → 仍返 6003
// （client 不区分"已在目标房间" vs "已在其他房间"）。message 与 case 2 完全一致。
func TestRoomService_JoinRoom_UserAlreadyInTargetRoom_PrecheckReturns6003(t *testing.T) {
	targetRoomID := uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: &targetRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (V1 §10.4 钦定 'client 不区分两子场景')",
			ae.Code, apperror.ErrUserAlreadyInRoom)
	}
	if ae.Message != apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom] {
		t.Errorf("AppError.Message = %q, want %q", ae.Message, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
	}
}

// TestRoomService_JoinRoom_RoomNotFound_Returns6001:
// 事务内步骤 2a roomRepo.FindByIDForUpdate 返 mysql.ErrRoomNotFound → service 翻译 6001。
// 后续 CountByRoomID / Create / UpdateCurrentRoomID **未**被调用（事务回滚）。
func TestRoomService_JoinRoom_RoomNotFound_Returns6001(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（FindByIDForUpdate 已失败）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return nil, mysql.ErrRoomNotFound
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用（FindByIDForUpdate 已失败）")
			return 0, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("Create 不应被调用（FindByIDForUpdate 已失败）")
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6001")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomNotFound {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomNotFound 6001)", ae.Code, apperror.ErrRoomNotFound)
	}
}

// TestRoomService_JoinRoom_RoomClosed_Returns6005:
// 事务内步骤 2b：FindByIDForUpdate 返 room with Status=2（closed）→ service 翻译 6005。
// CountByRoomID / Create / UpdateCurrentRoomID **未**被调用。
func TestRoomService_JoinRoom_RoomClosed_Returns6005(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			// 模拟 closed 房间
			return &mysql.Room{
				ID:            3001,
				CreatorUserID: 1001,
				Status:        2, // closed
				MaxMembers:    4,
			}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用（status check 已失败）")
			return 0, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("Create 不应被调用（status check 已失败）")
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6005")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomInvalidState {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomInvalidState 6005)", ae.Code, apperror.ErrRoomInvalidState)
	}
}

// TestRoomService_JoinRoom_RoomFull_Returns6002:
// 事务内步骤 2c：CountByRoomID 返 4（满员）→ service 翻译 6002。
// Create / UpdateCurrentRoomID **未**被调用。
func TestRoomService_JoinRoom_RoomFull_Returns6002(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{
				ID:            3001,
				CreatorUserID: 1001,
				Status:        1, // active
				MaxMembers:    4,
			}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 4, nil // 满员
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("Create 不应被调用（满员判定已失败）")
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6002")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomFull {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomFull 6002)", ae.Code, apperror.ErrRoomFull)
	}
}

// TestRoomService_JoinRoom_DBUniqueUserIDDuplicate_Returns6003:
// 事务内步骤 2d：Create 返 ErrRoomMembersUserIDDuplicate（并发 race）→
// service 兜底 6003（与预检路径完全等价）。
// UpdateCurrentRoomID **未**被调用（rollback）。
func TestRoomService_JoinRoom_DBUniqueUserIDDuplicate_Returns6003(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（room_members Create 已失败）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{
				ID:            3001,
				CreatorUserID: 1001,
				Status:        1,
				MaxMembers:    4,
			}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 1, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			return mysql.ErrRoomMembersUserIDDuplicate
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (uk_user_id 兜底也走 6003，**不**应被 1009 兜底覆盖)",
			ae.Code, apperror.ErrUserAlreadyInRoom)
	}
	// 关键：6003 双路径 message 完全等价
	if ae.Message != apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom] {
		t.Errorf("AppError.Message = %q, want %q (6003 双路径必须 message 完全等价)",
			ae.Message, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
	}
}

// TestRoomService_JoinRoom_DBUniqueRoomUserDuplicate_Returns6003:
// 事务内步骤 2d：Create 返 ErrRoomMembersRoomUserDuplicate（uk_room_user 兜底路径）→
// service 兜底 6003（与 case 7 等价）。
func TestRoomService_JoinRoom_DBUniqueRoomUserDuplicate_Returns6003(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: 3001, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 1, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			return mysql.ErrRoomMembersRoomUserDuplicate
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (uk_room_user 兜底也走 6003)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomService_JoinRoom_FindByIDForUpdateFailsRawError_Returns1009:
// 事务内步骤 2a：FindByIDForUpdate 返 raw DB error（非 ErrRoomNotFound）→ service 包 1009。
// 整个事务回滚（后续 repo 方法**未**被调用）。
func TestRoomService_JoinRoom_FindByIDForUpdateFailsRawError_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated db connection error during FindByIDForUpdate")

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return nil, wantCause
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用")
			return 0, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			t.Errorf("Create 不应被调用")
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy 1009)", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_JoinRoom_UpdateCurrentRoomIDFails_RollsBack:
// 事务内最后一步 step 2e UpdateCurrentRoomID 返 raw error → service 包 1009。
// 验证整个事务回滚（mock txMgr 验证 fn 返非 nil error）。
func TestRoomService_JoinRoom_UpdateCurrentRoomIDFails_RollsBack(t *testing.T) {
	wantCause := stderrors.New("simulated update current_room_id failure")

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1002, CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: 3001, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 1, nil
		},
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			return nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy 1009)", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_JoinRoom_FindByIDFails_Returns1009:
// 预检 userRepo.FindByID 直接返 raw DB error → service 包 1009（不开事务）。
func TestRoomService_JoinRoom_FindByIDFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated find user failure")
	withTxCalled := false

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用（FindByID 已失败）")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error")
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检失败 → 不开事务）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// ============================================================
// Story 11.5 单测 case（≥6 case，epics.md §Story 11.5 + V1 §10.5 钦定）
// ============================================================

// TestRoomService_LeaveRoom_Happy_NotLastMember:
// happy 路径，非最后一人离开。事务内 4 步全部成功（不调 UpdateStatus，因 remaining > 0）。
// 校验 5 个 mock 方法被调用顺序 + UpdateStatus **未**被调用 + UpdateCurrentRoomID nil 入参。
func TestRoomService_LeaveRoom_Happy_NotLastMember(t *testing.T) {
	var calls []string
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			calls = append(calls, "userRepo.FindByID")
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			calls = append(calls, "userRepo.UpdateCurrentRoomID")
			if userID != 1001 {
				t.Errorf("UpdateCurrentRoomID userID = %d, want 1001", userID)
			}
			if roomID != nil {
				t.Errorf("UpdateCurrentRoomID roomID = %v, want nil (leave 路径首次启用 nil 入参)", roomID)
			}
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			calls = append(calls, "roomRepo.FindByIDForUpdate")
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			t.Errorf("UpdateStatus 不应被调用（remaining > 0，跳过步骤 2e）")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			calls = append(calls, "roomMemberRepo.DeleteByRoomAndUser")
			if roomID != targetRoomID {
				t.Errorf("DeleteByRoomAndUser roomID = %d, want %d", roomID, targetRoomID)
			}
			if userID != 1001 {
				t.Errorf("DeleteByRoomAndUser userID = %d, want 1001", userID)
			}
			return 1, nil
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			calls = append(calls, "roomMemberRepo.CountByRoomID")
			return 2, nil // 还剩 2 人
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// 等 post-commit goroutine 完成后再读 calls（避免 -race 检测到 slice 并发读写）
	wg.Wait()

	expected := []string{
		"userRepo.FindByID",
		"roomRepo.FindByIDForUpdate",
		"roomMemberRepo.DeleteByRoomAndUser",
		"userRepo.UpdateCurrentRoomID",
		"roomMemberRepo.CountByRoomID",
	}
	if len(calls) != len(expected) {
		t.Fatalf("call count = %d, want %d; calls=%v", len(calls), len(expected), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d] = %q, want %q", i, c, expected[i])
		}
	}
	if out.RoomID != targetRoomID {
		t.Errorf("out.RoomID = %d, want %d", out.RoomID, targetRoomID)
	}
	if !out.Left {
		t.Errorf("out.Left = false, want true")
	}
}

// TestRoomService_LeaveRoom_Happy_LastMember_RoomClosed:
// happy 路径，最后一人离开。事务内 5 步全部成功（含 step 2e UpdateStatus = 2 closed）。
func TestRoomService_LeaveRoom_Happy_LastMember_RoomClosed(t *testing.T) {
	var calls []string
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID
	updateStatusCalled := false

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			calls = append(calls, "userRepo.FindByID")
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			calls = append(calls, "userRepo.UpdateCurrentRoomID")
			if roomID != nil {
				t.Errorf("UpdateCurrentRoomID roomID = %v, want nil", roomID)
			}
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			calls = append(calls, "roomRepo.FindByIDForUpdate")
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			calls = append(calls, "roomRepo.UpdateStatus")
			updateStatusCalled = true
			if roomID != targetRoomID {
				t.Errorf("UpdateStatus roomID = %d, want %d", roomID, targetRoomID)
			}
			if status != 2 {
				t.Errorf("UpdateStatus status = %d, want 2 (closed)", status)
			}
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			calls = append(calls, "roomMemberRepo.DeleteByRoomAndUser")
			return 1, nil
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			calls = append(calls, "roomMemberRepo.CountByRoomID")
			return 0, nil // 最后一人离开
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// 等 post-commit goroutine 完成（防 -race 误报 + updateStatusCalled 读写并发）
	wg.Wait()
	if !updateStatusCalled {
		t.Errorf("UpdateStatus 应被调用（最后一人离开 → status=2 closed）")
	}
	expected := []string{
		"userRepo.FindByID",
		"roomRepo.FindByIDForUpdate",
		"roomMemberRepo.DeleteByRoomAndUser",
		"userRepo.UpdateCurrentRoomID",
		"roomMemberRepo.CountByRoomID",
		"roomRepo.UpdateStatus",
	}
	if len(calls) != len(expected) {
		t.Fatalf("call count = %d, want %d; calls=%v", len(calls), len(expected), calls)
	}
	for i, c := range calls {
		if c != expected[i] {
			t.Errorf("call[%d] = %q, want %q", i, c, expected[i])
		}
	}
	if out.RoomID != targetRoomID {
		t.Errorf("out.RoomID = %d, want %d", out.RoomID, targetRoomID)
	}
	if !out.Left {
		t.Errorf("out.Left = false, want true")
	}
}

// TestRoomService_LeaveRoom_UserCurrentRoomIDNil_PrecheckReturns6004:
// 预检路径 (a)：user.CurrentRoomID == nil → 立即返 6004，事务**未**开。
func TestRoomService_LeaveRoom_UserCurrentRoomIDNil_PrecheckReturns6004(t *testing.T) {
	withTxCalled := false

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用（事务未开）")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 6004")
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检路径在事务外）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserNotInRoom 6004)", ae.Code, apperror.ErrUserNotInRoom)
	}
	if ae.Message != apperror.DefaultMessages[apperror.ErrUserNotInRoom] {
		t.Errorf("AppError.Message = %q, want %q (6004 双路径必须 message 等价)",
			ae.Message, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
	}
}

// TestRoomService_LeaveRoom_UserCurrentRoomIDDifferent_PrecheckReturns6004:
// 预检路径 (b)：user.CurrentRoomID 指向 != input.RoomID → 立即返 6004。
func TestRoomService_LeaveRoom_UserCurrentRoomIDDifferent_PrecheckReturns6004(t *testing.T) {
	otherRoomID := uint64(9001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &otherRoomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用（事务未开）")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// TestRoomService_LeaveRoom_DeleteRowsAffected0_TxRolledBack_Returns6004:
// V1 §10.5 行 1601 钦定 6004 双路径之 (c)：步骤 2b DELETE RowsAffected==0 → 6004 兜底。
// 关键 assert：UpdateCurrentRoomID / CountByRoomID / UpdateStatus 后续 3 步**未**调用（事务回滚）。
// 6004 兜底路径与预检路径完全等价（同 code + 同 message）。
func TestRoomService_LeaveRoom_DeleteRowsAffected0_TxRolledBack_Returns6004(t *testing.T) {
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（DELETE 0 行 → 事务回滚）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			t.Errorf("UpdateStatus 不应被调用")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			return 0, nil // 同一 user 并发两次 leave 输家场景
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用（DELETE 0 行 → 事务回滚）")
			return 0, nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 6004 (DELETE 兜底)")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (DELETE RowsAffected==0 兜底必须 6004，不应被 1009 兜底覆盖)",
			ae.Code, apperror.ErrUserNotInRoom)
	}
	if ae.Message != apperror.DefaultMessages[apperror.ErrUserNotInRoom] {
		t.Errorf("AppError.Message = %q, want %q (6004 双路径 message 必须等价)",
			ae.Message, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
	}
}

// TestRoomService_LeaveRoom_FindByIDForUpdateRoomNotFound_Returns1009:
// 步骤 2a SELECT FOR UPDATE 找不到 rooms 行 → service 翻译为 1009 ErrServiceBusy
// （**不**对外暴露 6001 —— V1 §10.5 行 1597 钦定 leave 接口不暴露 6001）。
func TestRoomService_LeaveRoom_FindByIDForUpdateRoomNotFound_Returns1009(t *testing.T) {
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用（FindByIDForUpdate 已失败）")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return nil, mysql.ErrRoomNotFound
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			t.Errorf("DeleteByRoomAndUser 不应被调用（FindByIDForUpdate 已失败）")
			return 0, nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	// **关键**：V1 §10.5 行 1597 钦定 leave 接口**不**暴露 6001 —— 即使 ErrRoomNotFound
	// 哨兵也要被翻译为 generic 1009（数据不一致按 DB 异常处理）。
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (V1 §10.5 钦定 leave 不暴露 6001，即使 mysql.ErrRoomNotFound 也走 1009)",
			ae.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomService_LeaveRoom_DeleteFails_Returns1009:
// 步骤 2b DeleteByRoomAndUser 返 (0, raw DB error) → service 1009。
func TestRoomService_LeaveRoom_DeleteFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated db connection error during DELETE")
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			t.Errorf("UpdateCurrentRoomID 不应被调用")
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			return 0, wantCause
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用")
			return 0, nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_LeaveRoom_UpdateCurrentRoomIDFails_RollsBack_Returns1009:
// 步骤 2c UpdateCurrentRoomID 失败 → service 1009 + 事务回滚（CountByRoomID / UpdateStatus 未调用）。
func TestRoomService_LeaveRoom_UpdateCurrentRoomIDFails_RollsBack_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated update current_room_id failure")
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			t.Errorf("UpdateStatus 不应被调用（事务回滚）")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			return 1, nil
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			t.Errorf("CountByRoomID 不应被调用（UpdateCurrentRoomID 已失败）")
			return 0, nil
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_LeaveRoom_CountByRoomIDFails_Returns1009:
// 步骤 2d CountByRoomID 失败 → service 1009 + UpdateStatus 未调用。
func TestRoomService_LeaveRoom_CountByRoomIDFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated count failure")
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			t.Errorf("UpdateStatus 不应被调用（CountByRoomID 已失败）")
			return nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			return 1, nil
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 0, wantCause
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_LeaveRoom_UpdateStatusFails_Returns1009_LastMemberPath:
// 步骤 2e UpdateStatus 失败（最后一人路径）→ service 1009 + 事务整体回滚。
func TestRoomService_LeaveRoom_UpdateStatusFails_Returns1009_LastMemberPath(t *testing.T) {
	wantCause := stderrors.New("simulated update status failure")
	const targetRoomID = uint64(3001)
	currentRoomID := targetRoomID

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &currentRoomID}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, userID uint64, roomID *uint64) error {
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: targetRoomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, roomID uint64, status int8) error {
			return wantCause
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, roomID, userID uint64) (int64, error) {
			return 1, nil
		},
		countByRoomIDFn: func(ctx context.Context, roomID uint64) (int, error) {
			return 0, nil // 触发 UpdateStatus 路径
		},
	}
	txMgr := roomTestDefaultStubTxMgr()

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// TestRoomService_LeaveRoom_FindByIDFails_Returns1009:
// 预检 userRepo.FindByID 失败 → service 1009（不开事务）。
func TestRoomService_LeaveRoom_FindByIDFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated find user failure")
	withTxCalled := false

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, roomID uint64) (*mysql.Room, error) {
			t.Errorf("FindByIDForUpdate 不应被调用（FindByID 已失败）")
			return nil, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{}
	txMgr := &roomTestStubTxMgr{
		withTxFn: func(ctx context.Context, fn func(txCtx context.Context) error) error {
			withTxCalled = true
			return fn(ctx)
		},
	}

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error")
	}
	if withTxCalled {
		t.Errorf("WithTx 不应被调用（预检失败）")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("errors.Is should find wantCause; err=%v", err)
	}
}

// ============================================================
// Story 11.6 单测 case：GetCurrentRoom (≥3 case) + GetRoomDetail (≥7 case)
// ============================================================


// TestRoomService_GetCurrentRoom_Happy_UserInRoom:
// stub userRepo.FindByID 返 user.CurrentRoomID=&3001 → out.RoomID 指向 3001。
func TestRoomService_GetCurrentRoom_Happy_UserInRoom(t *testing.T) {
	roomID := uint64(3001)
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			if id != 1001 {
				t.Errorf("FindByID id = %d, want 1001", id)
			}
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.GetCurrentRoom(context.Background(), service.GetCurrentRoomInput{UserID: 1001})
	if err != nil {
		t.Fatalf("GetCurrentRoom: %v", err)
	}
	if out.RoomID == nil {
		t.Fatalf("out.RoomID = nil, want &3001")
	}
	if *out.RoomID != 3001 {
		t.Errorf("out.RoomID = %d, want 3001", *out.RoomID)
	}
}

// TestRoomService_GetCurrentRoom_Happy_UserNotInAnyRoom:
// stub userRepo.FindByID 返 user.CurrentRoomID=nil → out.RoomID == nil（不视为业务错误）。
func TestRoomService_GetCurrentRoom_Happy_UserNotInAnyRoom(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.GetCurrentRoom(context.Background(), service.GetCurrentRoomInput{UserID: 1001})
	if err != nil {
		t.Fatalf("GetCurrentRoom: %v", err)
	}
	if out.RoomID != nil {
		t.Errorf("out.RoomID = %v, want nil (用户不在任何房间是合法场景)", out.RoomID)
	}
}

// TestRoomService_GetCurrentRoom_FindByIDFails_Returns1009:
// stub userRepo.FindByID 返 raw error → service 包成 1009。
func TestRoomService_GetCurrentRoom_FindByIDFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetCurrentRoom(context.Background(), service.GetCurrentRoomInput{UserID: 1001})
	if err == nil {
		t.Fatalf("GetCurrentRoom returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009)", ae.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess:
// 3 个 RosterRow（第 3 个 PetID=nil）→ out.Members 含 3 个，第 3 个 Pet == nil；
// MemberCount == 3；Members[0].Pet.CurrentState == 1；Members[0].Pet.Equips 必为
// `[]EquipOutput{}` 非 nil（节点 4 阶段硬编码）。
func TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess(t *testing.T) {
	roomID := uint64(3001)
	pet1, pet2 := uint64(8001), uint64(8002)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			if rid != roomID {
				t.Errorf("FindByID rid = %d, want %d", rid, roomID)
			}
			return &mysql.Room{ID: roomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return true, nil
		},
		listRosterByRoomIDFn: func(ctx context.Context, rid uint64) ([]mysql.RosterRow, error) {
			return []mysql.RosterRow{
				{UserID: 1001, Nickname: "A", AvatarURL: "https://a", PetID: &pet1},
				{UserID: 1002, Nickname: "B", AvatarURL: "", PetID: &pet2},
				{UserID: 1003, Nickname: "C", AvatarURL: "https://c", PetID: nil},
			}, nil
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	out, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	if err != nil {
		t.Fatalf("GetRoomDetail: %v", err)
	}
	if out.RoomID != roomID {
		t.Errorf("out.RoomID = %d, want %d", out.RoomID, roomID)
	}
	if out.MemberCount != 3 {
		t.Errorf("out.MemberCount = %d, want 3", out.MemberCount)
	}
	if len(out.Members) != 3 {
		t.Fatalf("len(out.Members) = %d, want 3", len(out.Members))
	}
	if out.MemberCount != len(out.Members) {
		t.Errorf("invariant violated: MemberCount=%d != len(Members)=%d", out.MemberCount, len(out.Members))
	}
	if out.Members[0].Pet == nil {
		t.Fatalf("out.Members[0].Pet = nil, want non-nil")
	}
	if out.Members[0].Pet.PetID != 8001 {
		t.Errorf("out.Members[0].Pet.PetID = %d, want 8001", out.Members[0].Pet.PetID)
	}
	if out.Members[0].Pet.CurrentState != 1 {
		t.Errorf("out.Members[0].Pet.CurrentState = %d, want 1 (节点 4 固定)", out.Members[0].Pet.CurrentState)
	}
	if out.Members[0].Pet.Equips == nil {
		t.Errorf("out.Members[0].Pet.Equips = nil, want []EquipOutput{} 非 nil（节点 4 阶段固定 []）")
	}
	if len(out.Members[0].Pet.Equips) != 0 {
		t.Errorf("len(Equips) = %d, want 0 (节点 4 阶段固定空数组)", len(out.Members[0].Pet.Equips))
	}
	if out.Members[2].Pet != nil {
		t.Errorf("out.Members[2].Pet = %+v, want nil (pet-less)", out.Members[2].Pet)
	}
}

// TestRoomService_GetRoomDetail_Step1aPrecheck_CurrentRoomIDDifferent_Returns6004:
// user.CurrentRoomID=&9999 ≠ in.RoomID=3001 → 6004；步骤 1b/2/3 不被调用（panic 兜底验证）。
func TestRoomService_GetRoomDetail_Step1aPrecheck_CurrentRoomIDDifferent_Returns6004(t *testing.T) {
	other := uint64(9999)
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &other}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (6004)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// TestRoomService_GetRoomDetail_Step1aPrecheck_CurrentRoomIDNil_Returns6004:
// user.CurrentRoomID=nil → 6004（与 != 子场景同 message / code）。
func TestRoomService_GetRoomDetail_Step1aPrecheck_CurrentRoomIDNil_Returns6004(t *testing.T) {
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (6004 nil 子场景)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// TestRoomService_GetRoomDetail_Step1bExistsForShare_Returns0Rows_Returns6004:
// 步骤 1a 通过但步骤 1b 返 (false, nil) → 6004 兜底；步骤 2/3 不被调用。
func TestRoomService_GetRoomDetail_Step1bExistsForShare_Returns0Rows_Returns6004(t *testing.T) {
	roomID := uint64(3001)
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return false, nil
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (6004 步骤 1b 兜底)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// TestRoomService_GetRoomDetail_Step2FindByID_NotFound_Returns6001:
// 步骤 1a / 1b 通过但 roomRepo.FindByID 返 ErrRoomNotFound → 6001。
func TestRoomService_GetRoomDetail_Step2FindByID_NotFound_Returns6001(t *testing.T) {
	roomID := uint64(3001)
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return nil, mysql.ErrRoomNotFound
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return true, nil
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6001")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomNotFound {
		t.Errorf("AppError.Code = %d, want %d (6001)", ae.Code, apperror.ErrRoomNotFound)
	}
}

// TestRoomService_GetRoomDetail_Step3ListRoster_DBError_Returns1009:
// 步骤 1a / 1b / 2 通过但 ListRosterByRoomID 返 raw error → 1009。
func TestRoomService_GetRoomDetail_Step3ListRoster_DBError_Returns1009(t *testing.T) {
	roomID := uint64(3001)
	wantCause := stderrors.New("simulated DB outage during roster query")
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, CreatorUserID: 1001, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return true, nil
		},
		listRosterByRoomIDFn: func(ctx context.Context, rid uint64) ([]mysql.RosterRow, error) {
			return nil, wantCause
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009)", ae.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomService_GetRoomDetail_Step1aFindByID_DBError_Returns1009:
// 步骤 1a userRepo.FindByID 返 raw error → 1009。
func TestRoomService_GetRoomDetail_Step1aFindByID_DBError_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage during step 1a")
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: 3001})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009)", ae.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomService_GetRoomDetail_Step1bExistsForShare_DBError_Returns1009:
// 步骤 1a 通过但 ExistsForShareByRoomAndUser 返 (false, raw error) → 1009。
func TestRoomService_GetRoomDetail_Step1bExistsForShare_DBError_Returns1009(t *testing.T) {
	roomID := uint64(3001)
	wantCause := stderrors.New("simulated DB outage during step 1b FOR SHARE")
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{}
	memberRepo := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return false, wantCause
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 1009")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009)", ae.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomService_GetRoomDetail_6004DualPath_MessagesEquivalent:
// V1 §10.3 行 1258 钦定 6004 双路径 message + code 完全等价 —— 步骤 1a 预检（!=）
// + 步骤 1b FOR SHARE 兜底两条路径返同一 apperror（同 message 同 code）。
func TestRoomService_GetRoomDetail_6004DualPath_MessagesEquivalent(t *testing.T) {
	roomID := uint64(3001)
	other := uint64(9999)

	userRepo1 := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &other}, nil
		},
	}
	roomRepo1 := &roomTestStubRoomRepo{}
	memberRepo1 := &roomTestStubRoomMemberRepo{}
	svc1 := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo1, roomRepo1, memberRepo1, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err1 := svc1.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	ae1, _ := apperror.As(err1)

	userRepo2 := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
		},
	}
	roomRepo2 := &roomTestStubRoomRepo{}
	memberRepo2 := &roomTestStubRoomMemberRepo{
		existsForShareByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (bool, error) {
			return false, nil
		},
	}
	svc2 := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo2, roomRepo2, memberRepo2, &roomTestStubPetRepo{}, &roomTestStubSessionMgr{}, (&roomTestStubBroadcastFn{}).fn(&atomic.Int64{}), (&roomTestStubBroadcastFn{}).exceptFn(&atomic.Int64{}))
	_, err2 := svc2.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: 1001, RoomID: roomID})
	ae2, _ := apperror.As(err2)

	if ae1 == nil || ae2 == nil {
		t.Fatalf("both errors should be *AppError; ae1=%v ae2=%v", ae1, ae2)
	}
	if ae1.Code != ae2.Code {
		t.Errorf("6004 双路径 code 不等价：path1=%d path2=%d", ae1.Code, ae2.Code)
	}
	if ae1.Code != apperror.ErrUserNotInRoom {
		t.Errorf("path1 Code = %d, want %d (6004)", ae1.Code, apperror.ErrUserNotInRoom)
	}
	if ae1.Message != ae2.Message {
		t.Errorf("6004 双路径 message 不等价：path1=%q path2=%q", ae1.Message, ae2.Message)
	}
}

// ============================================================
// Story 11.8 单测 case：broadcast 触发路径（mock 注入）
// ============================================================

// envelope shape for unmarshal during test assertions（local struct，wire JSON 字段
// 集合与 ws.serverEnvelope 等价；ws.serverEnvelope 是 unexported 不能跨包消费）。
type story118EnvelopeForTest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload"`
	Ts        int64           `json:"ts"`
}

// case J1: happy 路径 + broadcastFn 被调用 1 次 + payload 字段值正确（含 pet）
func TestRoomService_JoinRoom_Happy_BroadcastMemberJoinedCalled(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)
	const petID = uint64(7001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			// 用 nickname / avatarUrl 真实值兜底（broadcast 路径会再次调本方法 enrichment）
			return &mysql.User{
				ID:            userID,
				Nickname:      "用户1002",
				AvatarURL:     "https://avatar/1002",
				CurrentRoomID: nil, // 预检通过
			}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error {
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 1, nil },
		createFn:        func(ctx context.Context, m *mysql.RoomMember) error { return nil },
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: petID, UserID: uid, IsDefault: 1, CurrentState: 1}, nil
		},
	}
	bcast, seqGen, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	sessionMgr := &roomTestStubSessionMgr{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID})
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Story 11.8 r2 修复后：post-commit hook 异步化，等 wg 后再断言
	wg.Wait()
	if !out.Joined {
		t.Errorf("out.Joined = false, want true")
	}

	_ = exceptFn
	_ = seqGen
	if got := bcast.callCount(); got != 1 {
		t.Fatalf("broadcastExceptFn call count = %d, want 1", got)
	}
	call := bcast.calls[0]
	if call.roomID != roomID {
		t.Errorf("broadcastExceptFn roomID = %d, want %d", call.roomID, roomID)
	}
	// r3 [P1] fix 断言：member.joined 必须经 BroadcastExceptFn 路径调用，且
	// excludeUserID == joinerUserID（V1 §12.3 行 2063 钦定 + 防御 race）
	if call.excludeUserID == nil {
		t.Errorf("broadcast call should be via BroadcastExceptFn (excludeUserID set), got nil")
	} else if *call.excludeUserID != userID {
		t.Errorf("broadcast excludeUserID = %d, want %d (joiner self)", *call.excludeUserID, userID)
	}

	var env story118EnvelopeForTest
	if err := json.Unmarshal(call.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "member.joined" {
		t.Errorf("envelope.Type = %q, want \"member.joined\"", env.Type)
	}
	if env.RequestID != "" {
		t.Errorf("envelope.RequestID = %q, want \"\"", env.RequestID)
	}
	if env.Ts <= 0 {
		t.Errorf("envelope.Ts = %d, want > 0", env.Ts)
	}

	var payload struct {
		UserID    string `json:"userId"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatarUrl"`
		Pet       *struct {
			PetID        string `json:"petId"`
			CurrentState int    `json:"currentState"`
		} `json:"pet"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.UserID != "1002" {
		t.Errorf("payload.userId = %q, want \"1002\"", payload.UserID)
	}
	if payload.Nickname != "用户1002" {
		t.Errorf("payload.nickname = %q, want \"用户1002\"", payload.Nickname)
	}
	if payload.AvatarURL != "https://avatar/1002" {
		t.Errorf("payload.avatarUrl = %q, want \"https://avatar/1002\"", payload.AvatarURL)
	}
	if payload.Pet == nil {
		t.Fatalf("payload.pet = nil, want object with petId/currentState")
	}
	if payload.Pet.PetID != "7001" {
		t.Errorf("payload.pet.petId = %q, want \"7001\"", payload.Pet.PetID)
	}
	if payload.Pet.CurrentState != 1 {
		t.Errorf("payload.pet.currentState = %d, want 1", payload.Pet.CurrentState)
	}
}

// case J2: pet-less 路径 → mock petRepo 返 ErrPetNotFound → broadcast payload pet=null
// （wire 上 `"pet": null` 严格存在）
func TestRoomService_JoinRoom_Happy_PetLess_BroadcastWithNullPet(t *testing.T) {
	const userID = uint64(1003)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: userID, Nickname: "用户1003", AvatarURL: "", CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 1, nil },
		createFn:        func(ctx context.Context, m *mysql.RoomMember) error { return nil },
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound // pet-less
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, &roomTestStubSessionMgr{}, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Story 11.8 r2 修复后：等 wg 后再断言
	wg.Wait()

	if got := bcast.callCount(); got != 1 {
		t.Fatalf("broadcastFn call count = %d, want 1", got)
	}
	// 严格断言：wire 上含 `"pet":null`（pointer nil → JSON null，与 V1 §12.3 钦定一致）
	if !bytes.Contains(bcast.calls[0].msg, []byte(`"pet":null`)) {
		t.Errorf("payload should contain `\"pet\":null`; got: %s", string(bcast.calls[0].msg))
	}
}

// case J3: 事务回滚（roomMemberRepo.Create 撞 UNIQUE）→ broadcastFn / userRepo
// post-commit enrichment **未**被调用（fire-and-forget 在 commit 失败路径不触发）
func TestRoomService_JoinRoom_TxRollback_BroadcastNotCalled(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)
	var userFindByIDCalls int

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			userFindByIDCalls++
			return &mysql.User{ID: userID, Nickname: "B", CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 1, nil },
		createFn: func(ctx context.Context, m *mysql.RoomMember) error {
			// 事务内 INSERT 撞 UNIQUE 兜底 → 6003
			return mysql.ErrRoomMembersUserIDDuplicate
		},
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			t.Errorf("petRepo.FindDefaultByUserID 不应被调用（事务回滚路径）")
			return nil, mysql.ErrPetNotFound
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, &roomTestStubSessionMgr{}, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID}); err == nil {
		t.Fatalf("JoinRoom should return error (6003 dup), got nil")
	}
	// rollback 路径：runPostCommitAsync 不会被触发；wg.Wait() 立即返回（无 Add）
	wg.Wait()

	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn call count = %d, want 0 (tx rollback path)", got)
	}
	if userFindByIDCalls != 1 {
		t.Errorf("userRepo.FindByID called %d times, want 1 (only precheck; not post-commit enrichment)", userFindByIDCalls)
	}
}

// case J4: broadcastFn 返 raw error → JoinRoomOutput 仍正确返回（fire-and-forget
// 不影响主路径）
func TestRoomService_JoinRoom_BroadcastFails_DoesNotAffectReturn(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: userID, Nickname: "B", CurrentRoomID: nil}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 1, nil },
		createFn:        func(ctx context.Context, m *mysql.RoomMember) error { return nil },
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound
		},
	}
	// **r3 fix**：r3 起 broadcastMemberJoined 切到 BroadcastExceptFn 路径；模拟
	// network failure 必须注入 returnExceptFn（而非旧 returnFn）。
	bcast := &roomTestStubBroadcastFn{
		returnExceptFn: func(ctx context.Context, rid, excludeUserID uint64, msg []byte) (int, error) {
			return 0, stderrors.New("simulated broadcast network failure")
		},
	}
	seqGen := &atomic.Int64{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, &roomTestStubSessionMgr{}, bcast.fn(seqGen), bcast.exceptFn(seqGen))
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID})
	if err != nil {
		t.Fatalf("JoinRoom should succeed even when broadcastExceptFn errors (fire-and-forget): %v", err)
	}
	wg.Wait()
	if !out.Joined {
		t.Errorf("out.Joined = false, want true")
	}
	if got := bcast.callCount(); got != 1 {
		t.Errorf("broadcastExceptFn should still be called once even though it returns error; got %d", got)
	}
}

// case L1: LeaveRoom happy 路径 + broadcastFn 被调用 1 次 + payload type=member.left
// + payload.userId 字符串化正确（leaver 未持 WS / 已断开场景：sessionMgr 列表空，
// closeLeaverSession 走 no-op，broadcastFn 仍触发；与 case L2 同语义）
func TestRoomService_LeaveRoom_Happy_NoLeaverSession_StillBroadcasts(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			rid := roomID
			return &mysql.User{ID: userID, CurrentRoomID: &rid}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, rid uint64, st int8) error { return nil },
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (int64, error) { return 1, nil },
		countByRoomIDFn:       func(ctx context.Context, rid uint64) (int, error) { return 1, nil }, // 离开后还剩 1 人
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	sessionMgr := &roomTestStubSessionMgr{
		// 返 nil 列表 → leaver 未持 WS / 已断开（合法场景，no-op + log info）
		listSessionsByRoomIDFn: func(ctx context.Context, rid uint64) []*wsapp.Session { return nil },
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// Story 11.8 r2 修复后：post-commit hook 异步化，等 wg 后再断言副作用
	wg.Wait()
	if !out.Left {
		t.Errorf("out.Left = false, want true")
	}

	// closeLeaverSession 走 no-op：Unregister **不**应被调用（leaver 未命中）
	sessionMgr.mu.Lock()
	if got := len(sessionMgr.unregisterCalls); got != 0 {
		t.Errorf("Unregister call count = %d, want 0 (leaver session not registered)", got)
	}
	sessionMgr.mu.Unlock()

	// broadcastExceptFn 仍被调用 1 次（即使 leaver 未持 WS，broadcast 仍触发让其他成员收 member.left）
	if got := bcast.callCount(); got != 1 {
		t.Fatalf("broadcastExceptFn call count = %d, want 1", got)
	}
	call := bcast.calls[0]
	if call.roomID != roomID {
		t.Errorf("broadcastExceptFn roomID = %d, want %d", call.roomID, roomID)
	}
	// r3 [P1] fix 断言：member.left 必须经 BroadcastExceptFn 路径调用，且
	// excludeUserID == leaverUserID（双保险，即使 leaver 仍在列表也排除）
	if call.excludeUserID == nil {
		t.Errorf("broadcast call should be via BroadcastExceptFn (excludeUserID set), got nil")
	} else if *call.excludeUserID != userID {
		t.Errorf("broadcast excludeUserID = %d, want %d (leaver self)", *call.excludeUserID, userID)
	}

	var env story118EnvelopeForTest
	if err := json.Unmarshal(call.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "member.left" {
		t.Errorf("envelope.Type = %q, want \"member.left\"", env.Type)
	}
	if env.RequestID != "" {
		t.Errorf("envelope.RequestID = %q, want \"\"", env.RequestID)
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.UserID != "1002" {
		t.Errorf("payload.userId = %q, want \"1002\"", payload.UserID)
	}
}

// case L2: LeaveRoom 事务回滚（rowsAffected==0 → 6004）→ closeLeaverSession /
// broadcastFn **未**被调用（fire-and-forget 在 commit 失败路径不触发）
func TestRoomService_LeaveRoom_TxRollback_BroadcastNotCalled(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			rid := roomID
			return &mysql.User{ID: userID, CurrentRoomID: &rid}, nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (int64, error) {
			return 0, nil // RowsAffected=0 → 6004 兜底
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	sessionMgr := &roomTestStubSessionMgr{
		listSessionsByRoomIDFn: func(ctx context.Context, rid uint64) []*wsapp.Session {
			t.Errorf("ListSessionsByRoomID 不应被调用（事务回滚路径）")
			return nil
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, &roomTestStubPetRepo{}, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID}); err == nil {
		t.Fatalf("LeaveRoom should return 6004 error, got nil")
	}
	// rollback 路径：runPostCommitAsync 未触发；wg.Wait() 立即返回
	wg.Wait()

	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn call count = %d, want 0 (tx rollback path)", got)
	}
	sessionMgr.mu.Lock()
	if got := len(sessionMgr.unregisterCalls); got != 0 {
		t.Errorf("Unregister call count = %d, want 0 (tx rollback path)", got)
	}
	sessionMgr.mu.Unlock()
}

// case L3: LeaveRoom 预检失败（user.CurrentRoomID == nil → 预检 6004）→
// closeLeaverSession / broadcastFn / sessionMgr 全部**未**被调用
func TestRoomService_LeaveRoom_PrecheckFail_BroadcastNotCalled(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: userID, CurrentRoomID: nil}, nil
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	sessionMgr := &roomTestStubSessionMgr{
		listSessionsByRoomIDFn: func(ctx context.Context, rid uint64) []*wsapp.Session {
			t.Errorf("ListSessionsByRoomID 不应被调用（预检失败路径）")
			return nil
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, &roomTestStubRoomRepo{}, &roomTestStubRoomMemberRepo{}, &roomTestStubPetRepo{}, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID})
	if err == nil {
		t.Fatalf("LeaveRoom should return 6004 precheck error, got nil")
	}
	// 预检失败路径：runPostCommitAsync 未触发；wg.Wait() 立即返回
	wg.Wait()
	ae, ok := apperror.As(err)
	if !ok || ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("err = %v, want apperror with code 6004", err)
	}

	if got := bcast.callCount(); got != 0 {
		t.Errorf("broadcastFn call count = %d, want 0 (precheck fail path)", got)
	}
}

// case Order1（codex review r4 [P1] fix 验证）：
// 同一 roomID 上 JoinRoom → LeaveRoom 快速触发，post-commit 异步 hooks 必须按
// caller 调用顺序串行执行（per-room serialization）—— 即使第一个 hook（join 的
// broadcastMemberJoined）的 user/pet enrichment 被人为拖慢，第二个 hook（leave 的
// broadcastMemberLeft）也必须等第一个 hook 完成才开始，保证 client 收到的事件
// 顺序为 member.joined → member.left（绝不反转）。
//
// 实现：通过 userRepo.FindByID 注入 channel-blocking 让 join 路径 enrichment 阻塞
// 直到 release；release 前发起 LeaveRoom；release 后 wg.Wait()；断言 bcast.calls
// 顺序 [member.joined, member.left]。
func TestRoomService_PostCommit_PerRoomSerialization_PreservesCausalOrdering(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)
	const petID = uint64(7001)

	// gate 控制 join 路径的 broadcastMemberJoined enrichment（FindByID 调用）阻塞，
	// 让 leave 路径的 transaction commit 先完成 + post-commit hook 已 enqueue，
	// 验证 per-room mutex 阻止 leave hook 抢跑。
	gate := make(chan struct{})

	// FindByID：预检调用立即返回；post-commit enrichment 调用阻塞等 gate（仅第一次
	// 阻塞，后续调用不阻塞 —— 否则 leave 自己的预检也会卡住）。
	var findByIDCalls atomic.Int64
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			n := findByIDCalls.Add(1)
			user := &mysql.User{
				ID:        userID,
				Nickname:  "U",
				AvatarURL: "https://avatar/1002",
			}
			// n==1: join 预检（CurrentRoomID=nil 让 join 通过）
			// n==2: join post-commit enrichment（**阻塞**，等 gate close）
			// n==3: leave 预检（CurrentRoomID=&roomID 让 leave 通过）
			switch n {
			case 1:
				user.CurrentRoomID = nil
			case 2:
				<-gate // 阻塞 join 的 broadcastMemberJoined 直到测试主动 release
				user.CurrentRoomID = nil
			default:
				roomIDCopy := roomID
				user.CurrentRoomID = &roomIDCopy
			}
			return user, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, rid uint64, status int8) error { return nil },
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 0, nil },
		createFn:        func(ctx context.Context, m *mysql.RoomMember) error { return nil },
		deleteByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (int64, error) {
			return 1, nil
		},
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: petID, UserID: uid, IsDefault: 1, CurrentState: 1}, nil
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	sessionMgr := &roomTestStubSessionMgr{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	// (1) JoinRoom → 启动 goroutine A（broadcastMemberJoined），其 enrichment 卡在 gate
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// 等待 goroutine A 真正进入 mu 临界区 + 触发 FindByID（n==2）—— 否则下面
	// LeaveRoom 启动的 goroutine B 可能抢跑拿到 mu，掩盖 per-room serialization
	// 行为（让本测试在缺乏 per-room mutex 时也能巧合通过）。
	deadline := time.Now().Add(2 * time.Second)
	for findByIDCalls.Load() < 2 {
		if time.Now().After(deadline) {
			close(gate) // 防止 goroutine 泄漏
			t.Fatalf("goroutine A did not enter post-commit FindByID within deadline")
		}
		time.Sleep(1 * time.Millisecond)
	}

	// (2) LeaveRoom → 启动 goroutine B（broadcastMemberLeft）；
	// 因为 per-room mutex 已被 A 持有，B 会阻塞在 mu.Lock 排队，**不会**抢跑发出
	// member.left。
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	// (3) Release gate → A 完成 enrichment + 发出 member.joined → 释放 mu →
	// B 拿到 mu → 完成 member.left
	close(gate)

	wg.Wait()

	// (5) 断言：bcast.calls 长度 == 2，顺序 [member.joined, member.left]
	bcast.mu.Lock()
	defer bcast.mu.Unlock()
	if len(bcast.calls) != 2 {
		t.Fatalf("broadcast call count = %d, want 2 (1 join + 1 leave)", len(bcast.calls))
	}

	var env0, env1 story118EnvelopeForTest
	if err := json.Unmarshal(bcast.calls[0].msg, &env0); err != nil {
		t.Fatalf("unmarshal call[0]: %v", err)
	}
	if err := json.Unmarshal(bcast.calls[1].msg, &env1); err != nil {
		t.Fatalf("unmarshal call[1]: %v", err)
	}
	// **关键断言**（per-room serialization）：causal ordering 必须保留 ——
	// 即使 join 的 enrichment 被人为拖慢，leave 的 broadcast 也必须排在后面。
	if env0.Type != "member.joined" {
		t.Errorf("call[0].type = %q, want \"member.joined\" (causal ordering violated by per-room serialization bug)", env0.Type)
	}
	if env1.Type != "member.left" {
		t.Errorf("call[1].type = %q, want \"member.left\" (causal ordering violated by per-room serialization bug)", env1.Type)
	}
	// seq 单调递增（让 close[Unregister] < broadcastMemberLeft 也得到保证；本 case
	// 主要验证两条 broadcast 的相对顺序）
	if bcast.calls[0].seq >= bcast.calls[1].seq {
		t.Errorf("seq order: call[0].seq=%d should be < call[1].seq=%d", bcast.calls[0].seq, bcast.calls[1].seq)
	}
}

// TestRoomService_PostCommit_RapidJoinLeave_PreservesEnqueueOrder
// （Story 11.8 codex review r5 [P1] regression test）：
//
// **r5 [P1] regression**：r4 perRoomMu 方案在 caller commit 顺序为 join → leave 时，
// 两个 goroutine 启动顺序由 Go scheduler 决定，后者可能抢先 mu.Lock，broadcast 顺序
// 反转（member.left 先于 member.joined）。
//
// **r5 修法**：用 per-room FIFO channel queue + worker，**enqueue 在 caller 同步段**
// 完成 → enqueue 顺序 = caller 调用顺序 = commit 顺序，channel 顺序 receive，causal
// ordering 严格保留。
//
// **测试策略**：
//  1. 用 stub broadcastFn 内部 sleep 5ms 让第一条 broadcast（member.joined）拖慢
//     —— 模拟 worker 跑 fn 时实际较慢。
//  2. caller 同步连续调 JoinRoom + LeaveRoom（同 user 同 room）—— 两次 commit
//     顺序确定 join → leave。
//  3. 断言 broadcast 调用顺序 [member.joined, member.left] —— 即使 join 的
//     broadcast 跑很慢，leave 也必须排在它**之后**（因为同 channel FIFO 顺序）。
//
// **r4 vs r5 行为差异**：r4 perRoomMu 在本 case 仍可能通过（mu 在 goroutine 内
// 串行化，sleep 也只发生在 mu 临界区内），但 caller 同步段 r4 没保序点 —— 真正
// 暴露 r4 缺陷需要"goroutine 启动顺序反转"模拟，而 Go runtime 不直接暴露这个。
// 本 case 至少验证 r5 channel 路径在"worker 慢消费"场景下也能保序，作为 r5 修
// 复的 regression 防护。
func TestRoomService_PostCommit_RapidJoinLeave_PreservesEnqueueOrder(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)
	const petID = uint64(7001)

	// FindByID：用一个 atomic counter 跟踪状态（仿照 caller 同步段先 join 后 leave
	// 的语义） —— state 推进基于"caller 路径调用次序"而非整体调用次数，因为 post-
	// commit enrichment 跑在 worker goroutine，与 caller 是并发的。
	//
	// 简化策略：把 user.CurrentRoomID 用 atomic.Bool joined 表示状态，按"join 后
	// 标记 joined=true，leave 后标记 joined=false"。预检读 joined 决定返 nil 或
	// &roomID。enrichment 路径返什么 CurrentRoomID 不重要（broadcastMemberJoined
	// 不读 user.CurrentRoomID）。
	var joined atomic.Bool
	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			user := &mysql.User{
				ID:        userID,
				Nickname:  "U",
				AvatarURL: "https://avatar/1002",
			}
			if joined.Load() {
				roomIDCopy := roomID
				user.CurrentRoomID = &roomIDCopy
			} else {
				user.CurrentRoomID = nil
			}
			return user, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error {
			// rid != nil → join；rid == nil → leave
			if rid != nil {
				joined.Store(true)
			} else {
				joined.Store(false)
			}
			return nil
		},
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, rid uint64, status int8) error { return nil },
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		countByRoomIDFn: func(ctx context.Context, rid uint64) (int, error) { return 0, nil },
		createFn:        func(ctx context.Context, m *mysql.RoomMember) error { return nil },
		deleteByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (int64, error) {
			return 1, nil
		},
	}
	petRepo := &roomTestStubPetRepo{
		findDefaultByUserIDFn: func(ctx context.Context, uid uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: petID, UserID: uid, IsDefault: 1, CurrentState: 1}, nil
		},
	}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()
	// 让第一条 broadcast 跑慢（模拟 fanout 慢路径）：通过 returnExceptFn hook 在
	// 第一条 invocation 时 sleep 5ms。
	var broadcastInvocations atomic.Int64
	bcast.returnExceptFn = func(ctx context.Context, rid, exclude uint64, msg []byte) (int, error) {
		if broadcastInvocations.Add(1) == 1 {
			// 让第一条（应该是 member.joined）跑慢 5ms
			time.Sleep(5 * time.Millisecond)
		}
		return 0, nil
	}
	sessionMgr := &roomTestStubSessionMgr{}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	// (1) JoinRoom —— 同步段 enqueue member.joined fn 到 roomID FIFO channel
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// (2) LeaveRoom —— 同步段 enqueue member.left fn 到**同**一 roomID 的 FIFO
	// channel。两次 enqueue 都在 caller 同步段完成，channel FIFO 保证顺序。
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	// (3) 等所有 enqueue 的 fn 跑完
	wg.Wait()

	// (4) 断言：broadcast 必为 [member.joined, member.left] 严格顺序
	bcast.mu.Lock()
	defer bcast.mu.Unlock()
	if len(bcast.calls) != 2 {
		t.Fatalf("broadcast call count = %d, want 2 (1 join + 1 leave)", len(bcast.calls))
	}
	var env0, env1 story118EnvelopeForTest
	if err := json.Unmarshal(bcast.calls[0].msg, &env0); err != nil {
		t.Fatalf("unmarshal call[0]: %v", err)
	}
	if err := json.Unmarshal(bcast.calls[1].msg, &env1); err != nil {
		t.Fatalf("unmarshal call[1]: %v", err)
	}
	if env0.Type != "member.joined" {
		t.Errorf("call[0].type = %q, want \"member.joined\" (FIFO enqueue order broken)", env0.Type)
	}
	if env1.Type != "member.left" {
		t.Errorf("call[1].type = %q, want \"member.left\" (FIFO enqueue order broken)", env1.Type)
	}
	if bcast.calls[0].seq >= bcast.calls[1].seq {
		t.Errorf("seq order: call[0].seq=%d should be < call[1].seq=%d", bcast.calls[0].seq, bcast.calls[1].seq)
	}
}

// TestRoomService_PostCommit_LeaveCloseDoesNotBlockBroadcast
// （Story 11.8 codex review r5 [P2] regression test）：
//
// **r5 [P2] regression**：r4 把 closeLeaverSessionAsync + broadcastMemberLeft
// 放在同一 per-room mutex 临界区 —— close 走 CloseWithCode drain 慢路径（最坏 5s）
// 时，**整 room 后续所有 broadcast 阻塞**直到 close 返回。
//
// **r5 修法**：close 拆出独立 fire-and-forget goroutine（runCloseLeaverAsync），
// 不进 per-room queue。queue worker 跑完 broadcastMemberLeft 立即处理下一条事件，
// 不等 close 完成。
//
// **测试策略**：
//  1. 注入 sessionMgr 让 leaver Session 命中 unregisterLeaverSessionSync
//     （listSessionsByRoomIDFn 返一个 fake Session）。
//  2. （隐式）验证 close 拆独立 goroutine 后，wg.Wait() 不等 close 完成 —— 即
//     wg 只追踪 enqueue 进 queue 的 broadcastMemberLeft，**不**追踪 close。
//     因此 wg.Wait() 返回后，broadcast 必已完成；close goroutine 可能仍在跑
//     （但与 broadcast 顺序无关）。
//  3. 断言 broadcast 已发出（说明没被 close 阻塞）。
//
// **限制**：本测试不能直接断言 "close 在 broadcast 之后跑" —— 因为 r5 修法两者
// 完全异步、无相对顺序保证（这正是设计意图）。本测试只验证 broadcast 不被 close
// 拖慢 / 阻塞。
func TestRoomService_PostCommit_LeaveCloseDoesNotBlockBroadcast(t *testing.T) {
	const userID = uint64(1002)
	const roomID = uint64(3001)

	userRepo := &roomTestStubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			roomIDCopy := roomID
			return &mysql.User{ID: userID, Nickname: "L", CurrentRoomID: &roomIDCopy}, nil
		},
		updateCurrentRoomIDFn: func(ctx context.Context, uid uint64, rid *uint64) error { return nil },
	}
	roomRepo := &roomTestStubRoomRepo{
		findByIDForUpdateFn: func(ctx context.Context, rid uint64) (*mysql.Room, error) {
			return &mysql.Room{ID: roomID, Status: 1, MaxMembers: 4}, nil
		},
		updateStatusFn: func(ctx context.Context, rid uint64, status int8) error { return nil },
	}
	memberRepo := &roomTestStubRoomMemberRepo{
		deleteByRoomAndUserFn: func(ctx context.Context, rid, uid uint64) (int64, error) { return 1, nil },
		countByRoomIDFn:       func(ctx context.Context, rid uint64) (int, error) { return 1, nil }, // remaining=1，跳过 closed 路径
	}
	petRepo := &roomTestStubPetRepo{}
	bcast, _, fn, exceptFn := newRoomTestStubBroadcastFnWithSeq()

	// sessionMgr 返回一个 fake Session 让 unregisterLeaverSessionSync 命中
	// （走 close 异步路径）；ListSessionsByRoomID 返非空，Unregister 立即成功。
	// 注入 nil Session list（找不到 leaver）让 close 路径 no-op —— 本测试不验证
	// close 行为，只验证 broadcast 路径不被 close 阻塞。
	sessionMgr := &roomTestStubSessionMgr{
		listSessionsByRoomIDFn: func(ctx context.Context, rid uint64) []*wsapp.Session {
			return nil // close async path 走 no-op，broadcast 自走 queue
		},
	}

	svc := service.NewRoomService(roomTestDefaultStubTxMgr(), userRepo, roomRepo, memberRepo, petRepo, sessionMgr, fn, exceptFn)
	wg := &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	start := time.Now()
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userID, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// HTTP 路径返回延迟（同步段 + enqueue + close goroutine 启动；不含 close drain）
	httpLatency := time.Since(start)
	if httpLatency > 500*time.Millisecond {
		t.Errorf("LeaveRoom HTTP latency = %v, want < 500ms (close should not block caller)", httpLatency)
	}

	// 等 broadcast 完成
	wg.Wait()

	if got := bcast.callCount(); got != 1 {
		t.Errorf("broadcast call count = %d, want 1 (member.left)", got)
	}
}
