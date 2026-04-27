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

type stubHomePetRepo struct {
	findDefaultByUserIDFn func(ctx context.Context, userID uint64) (*mysql.Pet, error)
}

func (s *stubHomePetRepo) Create(ctx context.Context, p *mysql.Pet) error { return nil }
func (s *stubHomePetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	return s.findDefaultByUserIDFn(ctx, userID)
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

type stubHomeChestRepo struct {
	findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.UserChest, error)
}

func (s *stubHomeChestRepo) Create(ctx context.Context, c *mysql.UserChest) error { return nil }
func (s *stubHomeChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	return s.findByUserIDFn(ctx, userID)
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
