package apperror_test

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// 静态断言：*AppError 实现 error 接口（编译期保证）。
var _ error = (*apperror.AppError)(nil)

// 自定义 sentinel error，用于 TestAppError_WrapPreservesCauseAndUnwrap 验证
// errors.Is / errors.As 穿透 AppError.Cause 链。
var errTypedSentinel = stderrors.New("typed sentinel cause")

// 自定义 typed error（带方法），用于 errors.As 穿透链路验证。
type typedCauseError struct{ tag string }

func (e *typedCauseError) Error() string { return "typed cause: " + e.tag }

func TestAppError_ErrorImplementsError(t *testing.T) {
	t.Run("Error() 无 Cause 时只打 code+msg", func(t *testing.T) {
		e := apperror.New(apperror.ErrInvalidParam, "x 必填")
		want := "code=1002 msg=x 必填"
		assert.Equal(t, want, e.Error())
	})

	t.Run("Error() 有 Cause 时附 cause", func(t *testing.T) {
		cause := stderrors.New("db down")
		e := apperror.Wrap(cause, apperror.ErrServiceBusy, "服务繁忙")
		want := "code=1009 msg=服务繁忙: db down"
		assert.Equal(t, want, e.Error())
	})

	// 同时也是包级静态断言的运行期再次确认
	t.Run("实现 error 接口", func(t *testing.T) {
		var e error = apperror.New(apperror.ErrUnauthorized, "x")
		require.Error(t, e)
	})
}

func TestAppError_WrapPreservesCauseAndUnwrap(t *testing.T) {
	wrapped := apperror.Wrap(errTypedSentinel, apperror.ErrServiceBusy, "msg")

	t.Run("errors.Is 穿透 Cause", func(t *testing.T) {
		assert.True(t, stderrors.Is(wrapped, errTypedSentinel),
			"errors.Is(wrapped, errTypedSentinel) 应为 true（Unwrap 链生效）")
	})

	t.Run("errors.As 穿透到 *AppError 自身", func(t *testing.T) {
		var ae *apperror.AppError
		require.True(t, stderrors.As(wrapped, &ae))
		assert.Equal(t, apperror.ErrServiceBusy, ae.Code)
	})

	t.Run("errors.As 穿透到 typed cause（深一层）", func(t *testing.T) {
		typedCause := &typedCauseError{tag: "inner"}
		w := apperror.Wrap(typedCause, apperror.ErrInvalidParam, "outer")
		var got *typedCauseError
		require.True(t, stderrors.As(w, &got))
		assert.Equal(t, "inner", got.tag)
	})
}

func TestAppError_AsExtractsAppError(t *testing.T) {
	t.Run("顶层就是 *AppError", func(t *testing.T) {
		base := apperror.New(apperror.ErrInvalidParam, "x")
		got, ok := apperror.As(base)
		require.True(t, ok)
		assert.Equal(t, apperror.ErrInvalidParam, got.Code)
	})

	t.Run("被 fmt.Errorf %w 包过一层", func(t *testing.T) {
		base := apperror.New(apperror.ErrUnauthorized, "未登录")
		wrapped := fmt.Errorf("handler context: %w", base)
		got, ok := apperror.As(wrapped)
		require.True(t, ok)
		assert.Equal(t, apperror.ErrUnauthorized, got.Code)
		assert.Equal(t, "未登录", got.Message)
	})

	t.Run("被 fmt.Errorf %w 多层包裹", func(t *testing.T) {
		base := apperror.New(apperror.ErrChestNotOpenable, "宝箱未到时间")
		wrapped := fmt.Errorf("layer3: %w", fmt.Errorf("layer2: %w", fmt.Errorf("layer1: %w", base)))
		got, ok := apperror.As(wrapped)
		require.True(t, ok)
		assert.Equal(t, apperror.ErrChestNotOpenable, got.Code)
	})

	t.Run("非 AppError 返回 (nil, false)", func(t *testing.T) {
		got, ok := apperror.As(stderrors.New("plain"))
		assert.False(t, ok)
		assert.Nil(t, got)
	})

	t.Run("nil 输入返回 (nil, false)", func(t *testing.T) {
		got, ok := apperror.As(nil)
		assert.False(t, ok)
		assert.Nil(t, got)
	})
}

func TestAppError_WrapNilReturnsNil(t *testing.T) {
	// edge case：epics.md AC 强制 "nil error 传入 Wrap → 不 panic，返回 nil"
	// 让 service 写 `return apperror.Wrap(s.repo.X(ctx), ...)` 一行搞定
	got := apperror.Wrap(nil, apperror.ErrServiceBusy, "msg")
	assert.Nil(t, got, "Wrap(nil, ...) 必须返回 nil（不返回 *AppError{Cause: nil}）")

	// 进一步验证：返回值能直接赋给 error interface 后仍是真 nil
	// （防止 (*AppError)(nil) 陷阱）
	var asErr error = got
	assert.Nil(t, asErr, "Wrap(nil, ...) 必须返回真 nil error，避免 (*AppError)(nil) 包成 non-nil interface")
}

func TestAppError_CodeFromNonAppErrorReturnsZero(t *testing.T) {
	t.Run("plain error 返回 0", func(t *testing.T) {
		assert.Equal(t, 0, apperror.Code(stderrors.New("plain")))
	})

	t.Run("nil 返回 0", func(t *testing.T) {
		assert.Equal(t, 0, apperror.Code(nil))
	})

	t.Run("AppError 返回 Code", func(t *testing.T) {
		ae := apperror.New(apperror.ErrInvalidParam, "x")
		assert.Equal(t, apperror.ErrInvalidParam, apperror.Code(ae))
	})

	t.Run("被 fmt.Errorf %w 包过的 AppError 仍能取出 Code", func(t *testing.T) {
		base := apperror.New(apperror.ErrRoomFull, "满了")
		wrapped := fmt.Errorf("ctx: %w", base)
		assert.Equal(t, apperror.ErrRoomFull, apperror.Code(wrapped))
	})
}

// TestAppError_AllCodesMatchV1Spec：26 码 table-driven，
// 防止后续 dev 改错码值 / 漏改与 V1接口设计 §3 同步。
// 数值来源：docs/宠物互动App_V1接口设计.md §3。
func TestAppError_AllCodesMatchV1Spec(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		// 通用 1xxx
		{"ErrUnauthorized", apperror.ErrUnauthorized, 1001},
		{"ErrInvalidParam", apperror.ErrInvalidParam, 1002},
		{"ErrResourceNotFound", apperror.ErrResourceNotFound, 1003},
		{"ErrPermissionDenied", apperror.ErrPermissionDenied, 1004},
		{"ErrTooManyRequests", apperror.ErrTooManyRequests, 1005},
		{"ErrIllegalState", apperror.ErrIllegalState, 1006},
		{"ErrConflict", apperror.ErrConflict, 1007},
		{"ErrIdempotencyConflict", apperror.ErrIdempotencyConflict, 1008},
		{"ErrServiceBusy", apperror.ErrServiceBusy, 1009},

		// 认证 2xxx
		{"ErrGuestAccountNotFound", apperror.ErrGuestAccountNotFound, 2001},
		{"ErrWeChatBoundOther", apperror.ErrWeChatBoundOther, 2002},
		{"ErrAccountAlreadyBound", apperror.ErrAccountAlreadyBound, 2003},

		// 步数 3xxx
		{"ErrStepSyncInvalid", apperror.ErrStepSyncInvalid, 3001},
		{"ErrInsufficientSteps", apperror.ErrInsufficientSteps, 3002},

		// 宝箱 4xxx
		{"ErrChestNotFound", apperror.ErrChestNotFound, 4001},
		{"ErrChestNotUnlocked", apperror.ErrChestNotUnlocked, 4002},
		{"ErrChestNotOpenable", apperror.ErrChestNotOpenable, 4003},

		// 装扮 / 合成 5xxx
		{"ErrCosmeticNotFound", apperror.ErrCosmeticNotFound, 5001},
		{"ErrCosmeticNotOwned", apperror.ErrCosmeticNotOwned, 5002},
		{"ErrCosmeticInvalidState", apperror.ErrCosmeticInvalidState, 5003},
		{"ErrCosmeticSlotMismatch", apperror.ErrCosmeticSlotMismatch, 5004},
		{"ErrComposeMaterialCount", apperror.ErrComposeMaterialCount, 5005},
		{"ErrComposeMaterialRarity", apperror.ErrComposeMaterialRarity, 5006},
		{"ErrComposeTargetIllegal", apperror.ErrComposeTargetIllegal, 5007},
		{"ErrCosmeticAlreadyEquipped", apperror.ErrCosmeticAlreadyEquipped, 5008},

		// 房间 6xxx
		{"ErrRoomNotFound", apperror.ErrRoomNotFound, 6001},
		{"ErrRoomFull", apperror.ErrRoomFull, 6002},
		{"ErrUserAlreadyInRoom", apperror.ErrUserAlreadyInRoom, 6003},
		{"ErrUserNotInRoom", apperror.ErrUserNotInRoom, 6004},
		{"ErrRoomInvalidState", apperror.ErrRoomInvalidState, 6005},

		// 表情 / WS 7xxx
		{"ErrEmojiNotFound", apperror.ErrEmojiNotFound, 7001},
		{"ErrWSNotConnected", apperror.ErrWSNotConnected, 7002},
	}

	// V1 §3 共 32 码（不含 0=成功）：1xxx(9) + 2xxx(3) + 3xxx(2) + 4xxx(3) +
	// 5xxx(8) + 6xxx(5) + 7xxx(2) = 32。本测试列表必须 32 条，少一条说明漏码。
	// （epics.md 提到 "26 码" 是误计；以 V1接口设计 §3 为准。）
	require.Len(t, cases, 32, "32 码必须全列；少一条说明 codes.go 缺常量或本测试漏列")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got, "%s 码值与 V1接口设计 §3 不一致", tc.name)
		})
	}

	// 同时验证 DefaultMessages 至少包含 26 条 entry（防止 codes.go 添了常量
	// 但忘了同步 message map）
	for _, tc := range cases {
		_, ok := apperror.DefaultMessages[tc.want]
		assert.True(t, ok, "DefaultMessages 缺 %s (code=%d) 的默认 message", tc.name, tc.want)
	}
}

func TestAppError_WithMetadataAccumulates(t *testing.T) {
	e := apperror.New(apperror.ErrInvalidParam, "x").
		WithMetadata("user_id", "u-123").
		WithMetadata("attempt", 3)

	require.NotNil(t, e.Metadata)
	assert.Equal(t, "u-123", e.Metadata["user_id"])
	assert.Equal(t, 3, e.Metadata["attempt"])

	// 同 key 后写覆盖
	e2 := e.WithMetadata("attempt", 5)
	assert.Equal(t, 5, e2.Metadata["attempt"])
}
