package service_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub repo（每个 stub 必须实装完整 interface 以编译通过）
// 与 4.6 auth_service_test 同模式：通过 fn 字段让每个 case 自定义返回。
// ============================================================

type stubHomeUserRepo struct {
	findByIDFn func(ctx context.Context, id uint64) (*mysql.User, error)
}

func (s *stubHomeUserRepo) Create(ctx context.Context, u *mysql.User) error { return nil }
func (s *stubHomeUserRepo) UpdateNickname(ctx context.Context, id uint64, n string) error {
	return nil
}
func (s *stubHomeUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return s.findByIDFn(ctx, id)
}

// UpdateCurrentRoomID 兜底（home_service 不调；Story 11.3 加方法后编译需要）。
func (s *stubHomeUserRepo) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	return nil
}

type stubHomePetRepo struct {
	findDefaultByUserIDFn func(ctx context.Context, userID uint64) (*mysql.Pet, error)
}

func (s *stubHomePetRepo) Create(ctx context.Context, p *mysql.Pet) error { return nil }
func (s *stubHomePetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	return s.findDefaultByUserIDFn(ctx, userID)
}
// UpdateCurrentStateByID Story 14.2 加：home_service 不调本方法。
func (s *stubHomePetRepo) UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error {
	return nil
}

type stubHomeStepAccountRepo struct {
	findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.StepAccount, error)
}

func (s *stubHomeStepAccountRepo) Create(ctx context.Context, a *mysql.StepAccount) error {
	return nil
}
func (s *stubHomeStepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	return s.findByUserIDFn(ctx, userID)
}

// UpdateBalance Story 7.3 加：home_service 不调；保留以满足 interface。
func (s *stubHomeStepAccountRepo) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
	panic("stubHomeStepAccountRepo.UpdateBalance not configured (home_service should not call it)")
}

// FindByUserIDForUpdate Story 20.6 加：home_service 不调；保留以满足 interface。
func (s *stubHomeStepAccountRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	panic("stubHomeStepAccountRepo.FindByUserIDForUpdate not configured (home_service should not call it)")
}

// Spend Story 20.6 加：home_service 不调；保留以满足 interface。
func (s *stubHomeStepAccountRepo) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
	panic("stubHomeStepAccountRepo.Spend not configured (home_service should not call it)")
}

type stubHomeChestRepo struct {
	findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.UserChest, error)
}

func (s *stubHomeChestRepo) Create(ctx context.Context, c *mysql.UserChest) error { return nil }
func (s *stubHomeChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	return s.findByUserIDFn(ctx, userID)
}

// FindByUserIDForUpdate Story 20.6 加：home_service 不调；保留以满足 interface。
func (s *stubHomeChestRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	panic("stubHomeChestRepo.FindByUserIDForUpdate not configured (home_service should not call it)")
}

// Delete Story 20.6 加：home_service 不调；保留以满足 interface。
func (s *stubHomeChestRepo) Delete(ctx context.Context, id uint64) error {
	panic("stubHomeChestRepo.Delete not configured (home_service should not call it)")
}

// buildHomeService 用 4 个 stub repo 构造 HomeService。每个 case 独立设置 fn。
func buildHomeService(
	userFn func(ctx context.Context, id uint64) (*mysql.User, error),
	petFn func(ctx context.Context, userID uint64) (*mysql.Pet, error),
	stepFn func(ctx context.Context, userID uint64) (*mysql.StepAccount, error),
	chestFn func(ctx context.Context, userID uint64) (*mysql.UserChest, error),
) service.HomeService {
	return service.NewHomeService(
		&stubHomeUserRepo{findByIDFn: userFn},
		&stubHomePetRepo{findDefaultByUserIDFn: petFn},
		&stubHomeStepAccountRepo{findByUserIDFn: stepFn},
		&stubHomeChestRepo{findByUserIDFn: chestFn},
	)
}

// ============================================================
// 测试 case
// ============================================================

// AC6.1 happy: 4 repo 全成功 → HomeOutput 字段语义正确
func TestHomeService_LoadHome_AllReposOK_ReturnsCompleteOutput(t *testing.T) {
	unlockAt := time.Now().UTC().Add(10 * time.Minute)

	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			if id != 1001 {
				t.Errorf("FindByID id = %d, want 1001", id)
			}
			return &mysql.User{ID: 1001, Nickname: "用户1001", AvatarURL: ""}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 1001, PetType: 1, Name: "默认小猫", CurrentState: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 0, AvailableSteps: 0, ConsumedSteps: 0, Version: 0}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{ID: 5001, UserID: 1001, Status: 1, UnlockAt: unlockAt, OpenCostSteps: 1000, Version: 0}, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1001)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}
	if out.User.ID != 1001 {
		t.Errorf("User.ID = %d, want 1001", out.User.ID)
	}
	if out.User.Nickname != "用户1001" {
		t.Errorf("User.Nickname = %q, want 用户1001", out.User.Nickname)
	}
	if out.Pet == nil {
		t.Fatal("Pet should not be nil")
	}
	if out.Pet.ID != 2001 {
		t.Errorf("Pet.ID = %d, want 2001", out.Pet.ID)
	}
	if out.Pet.PetType != 1 {
		t.Errorf("Pet.PetType = %d, want 1", out.Pet.PetType)
	}
	if out.Pet.CurrentState != 1 {
		t.Errorf("Pet.CurrentState = %d, want 1", out.Pet.CurrentState)
	}
	if out.StepAccount.TotalSteps != 0 || out.StepAccount.AvailableSteps != 0 || out.StepAccount.ConsumedSteps != 0 {
		t.Errorf("StepAccount = %+v, want all 0", out.StepAccount)
	}
	if out.Chest.ID != 5001 {
		t.Errorf("Chest.ID = %d, want 5001", out.Chest.ID)
	}
	// 动态判定: unlockAt 在未来 10min → status=1 (counting)
	if out.Chest.Status != 1 {
		t.Errorf("Chest.Status = %d, want 1 (counting)", out.Chest.Status)
	}
	// remainingSeconds ~ 600 (容忍 ±5s 测试机抖动)
	if out.Chest.RemainingSeconds < 595 || out.Chest.RemainingSeconds > 600 {
		t.Errorf("Chest.RemainingSeconds = %d, want ~600", out.Chest.RemainingSeconds)
	}
	// UnlockAt 必须是 UTC
	if out.Chest.UnlockAt.Location().String() != "UTC" {
		t.Errorf("Chest.UnlockAt location = %q, want UTC", out.Chest.UnlockAt.Location().String())
	}
}

// AC6.2 chest unlockAt 已过 → 动态判定 Status=2 / RemainingSeconds=0
//
// **关键**：DB 原值 Status=1 (counting，登录初始化时写死)，但 unlockAt 已过
// → service 必须返 Status=2 (unlockable)，验证 chestStatusDynamic 逻辑。
func TestHomeService_LoadHome_ChestUnlocked_DynamicStatusIs2(t *testing.T) {
	pastUnlock := time.Now().UTC().Add(-1 * time.Minute)

	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1, Nickname: "u", AvatarURL: ""}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, Name: "p", CurrentState: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			// DB 原值 Status=1，但 unlockAt 已过
			return &mysql.UserChest{ID: 3, UserID: 1, Status: 1, UnlockAt: pastUnlock, OpenCostSteps: 1000}, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}
	if out.Chest.Status != 2 {
		t.Errorf("Chest.Status = %d, want 2 (unlockable)", out.Chest.Status)
	}
	if out.Chest.RemainingSeconds != 0 {
		t.Errorf("Chest.RemainingSeconds = %d, want 0 (unlockAt in past)", out.Chest.RemainingSeconds)
	}
}

// AC6.3 pet NotFound → HomeOutput.Pet == nil（不视为错误）
func TestHomeService_LoadHome_PetNotFound_PetIsNilNotError(t *testing.T) {
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1, Nickname: "u"}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{ID: 3, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v, want nil err", err)
	}
	if out.Pet != nil {
		t.Errorf("Pet = %+v, want nil (ErrPetNotFound 视为可空)", out.Pet)
	}
}

// AC6.4 user repo 失败 → 整体 1009
func TestHomeService_LoadHome_UserRepoFails_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, wantCause
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			t.Errorf("petRepo should NOT be called when userRepo fails")
			return nil, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			t.Errorf("stepAccountRepo should NOT be called")
			return nil, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			t.Errorf("chestRepo should NOT be called")
			return nil, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if out != nil {
		t.Errorf("out = %+v, want nil on userRepo error", out)
	}
	if err == nil {
		t.Fatal("err = nil, want *AppError(1009)")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code(err) = %d, want %d", got, apperror.ErrServiceBusy)
	}
	// cause 链穿透
	if !stderrors.Is(err, wantCause) {
		t.Errorf("err 链不含 wantCause；err = %v", err)
	}
}

// AC6.5 user NotFound → 1009（不视为可空；user 必须存在）
func TestHomeService_LoadHome_UserNotFound_Returns1009(t *testing.T) {
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, mysql.ErrUserNotFound
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) { return nil, nil },
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) { return nil, nil },
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) { return nil, nil },
	)

	_, err := svc.LoadHome(context.Background(), 1)
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code(err) = %d, want %d", got, apperror.ErrServiceBusy)
	}
}

// AC6.6 step_account NotFound → 整体 1009（不视为可空）
func TestHomeService_LoadHome_StepAccountNotFound_Returns1009(t *testing.T) {
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1, Nickname: "u"}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return nil, mysql.ErrStepAccountNotFound
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			t.Errorf("chestRepo should NOT be called when stepAccountRepo fails")
			return nil, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if out != nil {
		t.Errorf("out = %+v, want nil", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code(err) = %d, want 1009 (ErrServiceBusy)", got)
	}
}

// AC6.7 chest NotFound → 整体 1009（不视为可空）
func TestHomeService_LoadHome_ChestNotFound_Returns1009(t *testing.T) {
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1, Nickname: "u"}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return nil, mysql.ErrChestNotFound
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if out != nil {
		t.Errorf("out = %+v, want nil", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code(err) = %d, want 1009 (ErrServiceBusy)", got)
	}
}

// ============================================================
// Story 11.10: GET /home 扩展 - room.currentRoomId 真实数据
//
// 节点 4 阶段（11.10 落地）service 层透传 user.CurrentRoomID 到
// HomeOutput.Room.CurrentRoomID。本组 case 验证 service 层
// **零额外 repo 调用** + **不做** rooms.status cross-check。
// ============================================================

// AC11.10.1 happy: 用户在房间 → HomeOutput.Room.CurrentRoomID = &roomID
//
// **关键**：节点 4 阶段（11.10 落地后）user.CurrentRoomID 不再被 service 层强制
// 视为 nil；service 直接透传 mysql.User.CurrentRoomID 字段值到 HomeOutput.Room.CurrentRoomID。
func TestHomeService_LoadHome_UserInRoom_RoomCurrentRoomIDIsRoomID(t *testing.T) {
	roomID := uint64(3001)
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{
				ID: 1, Nickname: "u", AvatarURL: "",
				CurrentRoomID: &roomID, // 用户在房间 3001
			}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
		},
	)
	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}
	if out.Room.CurrentRoomID == nil {
		t.Fatal("Room.CurrentRoomID = nil, want &3001")
	}
	if *out.Room.CurrentRoomID != 3001 {
		t.Errorf("*Room.CurrentRoomID = %d, want 3001", *out.Room.CurrentRoomID)
	}
}

// AC11.10.2 happy: 用户不在任何房间 → HomeOutput.Room.CurrentRoomID = nil
//
// users.current_room_id IS NULL 在 GORM 解析为 *uint64 nil；service 透传到
// HomeOutput.Room.CurrentRoomID = nil；handler 把 nil 序列化为 JSON null。
func TestHomeService_LoadHome_UserNotInAnyRoom_RoomCurrentRoomIDIsNil(t *testing.T) {
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{
				ID: 1, Nickname: "u", AvatarURL: "",
				CurrentRoomID: nil, // 用户不在任何房间
			}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
		},
	)
	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}
	if out.Room.CurrentRoomID != nil {
		t.Errorf("Room.CurrentRoomID = %v, want nil", *out.Room.CurrentRoomID)
	}
}

// AC11.10.3 edge: users.current_room_id 指向已 closed 的房间（理论不该）→ 仍返回该 id
//
// epics.md §Story 11.10 行 2040 钦定：service 层**不做** rooms 表 cross-check；client
// 在拿到 currentRoomId 后调 /rooms/{id} 时由 11.6 ACL 走 6004 / 6005 自行处理。
//
// **rationale**：home 是聚合接口性能敏感，强制 cross-check rooms.status 会引入额外 1 次
// rooms 表查询 + 与房间业务耦合；user.current_room_id 的"幻象"由 11.5 退出房间事务的
// `UPDATE users SET current_room_id = NULL` 步骤兜底，正常情况下不会指向 closed room。
func TestHomeService_LoadHome_CurrentRoomIDPointsToClosedRoom_StillReturnsID(t *testing.T) {
	closedRoomID := uint64(9999) // 假设房间 9999 在 DB 中 status=2 closed（service 不知晓）
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			// service 层只看 user.CurrentRoomID 字段值，**不**查 rooms.status
			return &mysql.User{
				ID: 1, Nickname: "u",
				CurrentRoomID: &closedRoomID,
			}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
		},
	)
	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v, want nil err (即便 currentRoomID 指向 closed 房间也不报错)", err)
	}
	if out.Room.CurrentRoomID == nil {
		t.Fatal("Room.CurrentRoomID = nil, want &9999 (即便指向已 closed 房间也透传)")
	}
	if *out.Room.CurrentRoomID != 9999 {
		t.Errorf("*Room.CurrentRoomID = %d, want 9999", *out.Room.CurrentRoomID)
	}
}

// AC6.8 pet repo 非 NotFound 错误 → 1009（**不**被错认为可空 nil pet）
func TestHomeService_LoadHome_PetRepoOtherError_Returns1009(t *testing.T) {
	wantCause := stderrors.New("connection lost")
	svc := buildHomeService(
		func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: 1, Nickname: "u"}, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
			return nil, wantCause
		},
		func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			t.Errorf("stepAccountRepo should NOT be called when pet repo fails (non-NotFound)")
			return nil, nil
		},
		func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			t.Errorf("chestRepo should NOT be called")
			return nil, nil
		},
	)

	out, err := svc.LoadHome(context.Background(), 1)
	if out != nil {
		t.Errorf("out = %+v, want nil (pet 非 NotFound 错误必须中断)", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code(err) = %d, want %d", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, wantCause) {
		t.Errorf("err 链不含 wantCause; err = %v", err)
	}
}
