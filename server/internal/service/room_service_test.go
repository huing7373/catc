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
	createFn            func(ctx context.Context, r *mysql.Room) error
	findByIDForUpdateFn func(ctx context.Context, roomID uint64) (*mysql.Room, error)
	updateStatusFn      func(ctx context.Context, roomID uint64, status int8) error
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

type roomTestStubRoomMemberRepo struct {
	roomExistsFn          func(ctx context.Context, roomID uint64) (bool, error)
	isUserInRoomFn        func(ctx context.Context, userID uint64, roomID uint64) (bool, error)
	listMembersFn         func(ctx context.Context, roomID uint64) ([]uint64, error)
	createFn              func(ctx context.Context, m *mysql.RoomMember) error
	countByRoomIDFn       func(ctx context.Context, roomID uint64) (int, error)
	deleteByRoomAndUserFn func(ctx context.Context, roomID, userID uint64) (int64, error)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
	out, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: 1002, RoomID: 3001})
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	expected := []string{
		"userRepo.FindByID",
		"roomRepo.FindByIDForUpdate",
		"roomMemberRepo.CountByRoomID",
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: 1001, RoomID: targetRoomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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

	svc := service.NewRoomService(txMgr, userRepo, roomRepo, memberRepo)
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
