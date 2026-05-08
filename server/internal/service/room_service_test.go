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
	createFn func(ctx context.Context, r *mysql.Room) error
}

func (s *roomTestStubRoomRepo) Create(ctx context.Context, r *mysql.Room) error {
	return s.createFn(ctx, r)
}

type roomTestStubRoomMemberRepo struct {
	roomExistsFn   func(ctx context.Context, roomID uint64) (bool, error)
	isUserInRoomFn func(ctx context.Context, userID uint64, roomID uint64) (bool, error)
	listMembersFn  func(ctx context.Context, roomID uint64) ([]uint64, error)
	createFn       func(ctx context.Context, m *mysql.RoomMember) error
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
	return s.createFn(ctx, m)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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
