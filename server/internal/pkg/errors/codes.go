package apperror

// 通用错误码（1xxx）—— V1接口设计 §3。
const (
	ErrUnauthorized        = 1001 // 未登录 / token 无效
	ErrInvalidParam        = 1002 // 参数错误
	ErrResourceNotFound    = 1003 // 资源不存在
	ErrPermissionDenied    = 1004 // 权限不足
	ErrTooManyRequests     = 1005 // 操作过于频繁
	ErrIllegalState        = 1006 // 状态不允许当前操作
	ErrConflict            = 1007 // 数据冲突
	ErrIdempotencyConflict = 1008 // 幂等冲突
	ErrServiceBusy         = 1009 // 服务繁忙（panic / 非 AppError 兜底）
)

// 认证 / 账号错误码（2xxx）。
const (
	ErrGuestAccountNotFound = 2001 // 游客账号不存在
	ErrWeChatBoundOther     = 2002 // 微信已绑定其他账号
	ErrAccountAlreadyBound  = 2003 // 当前账号已绑定微信
)

// 步数错误码（3xxx）。
const (
	ErrStepSyncInvalid   = 3001 // 步数同步数据异常
	ErrInsufficientSteps = 3002 // 可用步数不足
)

// 宝箱错误码（4xxx）。
const (
	ErrChestNotFound    = 4001 // 当前宝箱不存在
	ErrChestNotUnlocked = 4002 // 宝箱尚未解锁
	ErrChestNotOpenable = 4003 // 宝箱开启条件不满足
)

// 装扮 / 合成错误码（5xxx）。
const (
	ErrCosmeticNotFound        = 5001 // 道具不存在
	ErrCosmeticNotOwned        = 5002 // 道具不属于当前用户
	ErrCosmeticInvalidState    = 5003 // 道具状态不可用
	ErrCosmeticSlotMismatch    = 5004 // 装备槽位不匹配
	ErrComposeMaterialCount    = 5005 // 合成材料数量错误
	ErrComposeMaterialRarity   = 5006 // 合成材料品质不一致
	ErrComposeTargetIllegal    = 5007 // 合成目标品质不合法
	ErrCosmeticAlreadyEquipped = 5008 // 装扮已装备
)

// 房间错误码（6xxx）。
const (
	ErrRoomNotFound      = 6001 // 房间不存在
	ErrRoomFull          = 6002 // 房间已满
	ErrUserAlreadyInRoom = 6003 // 用户已在房间中
	ErrUserNotInRoom     = 6004 // 用户不在房间中
	ErrRoomInvalidState  = 6005 // 房间状态异常
)

// 表情 / WS 错误码（7xxx）。
const (
	ErrEmojiNotFound  = 7001 // 表情不存在
	ErrWSNotConnected = 7002 // WebSocket 未连接
)

// DefaultMessages 提供 code → 中文默认 message 的查表函数。
//
// 用法：
//   - 业务方习惯调 `apperror.New(code, "<具体上下文 msg>")` 显式传 message —— 让
//     文案与触发点上下文匹配；
//   - 兜底场景（如非 AppError 错误被 ErrorMappingMiddleware 翻译为 1009）可以
//     `apperror.New(ErrServiceBusy, DefaultMessages[ErrServiceBusy])` 拿默认串。
//
// 缺失的 code 返回零值（空串）。文案与 V1接口设计 §3 严格对齐。
var DefaultMessages = map[int]string{
	// 1xxx 通用
	ErrUnauthorized:        "未登录或 token 无效",
	ErrInvalidParam:        "参数错误",
	ErrResourceNotFound:    "资源不存在",
	ErrPermissionDenied:    "权限不足",
	ErrTooManyRequests:     "操作过于频繁",
	ErrIllegalState:        "状态不允许当前操作",
	ErrConflict:            "数据冲突",
	ErrIdempotencyConflict: "幂等冲突",
	ErrServiceBusy:         "服务繁忙",

	// 2xxx 认证
	ErrGuestAccountNotFound: "游客账号不存在",
	ErrWeChatBoundOther:     "微信已绑定其他账号",
	ErrAccountAlreadyBound:  "当前账号已绑定微信",

	// 3xxx 步数
	ErrStepSyncInvalid:   "步数同步数据异常",
	ErrInsufficientSteps: "可用步数不足",

	// 4xxx 宝箱
	ErrChestNotFound:    "当前宝箱不存在",
	ErrChestNotUnlocked: "宝箱尚未解锁",
	ErrChestNotOpenable: "宝箱开启条件不满足",

	// 5xxx 装扮 / 合成
	ErrCosmeticNotFound:        "道具不存在",
	ErrCosmeticNotOwned:        "道具不属于当前用户",
	ErrCosmeticInvalidState:    "道具状态不可用",
	ErrCosmeticSlotMismatch:    "装备槽位不匹配",
	ErrComposeMaterialCount:    "合成材料数量错误",
	ErrComposeMaterialRarity:   "合成材料品质不一致",
	ErrComposeTargetIllegal:    "合成目标品质不合法",
	ErrCosmeticAlreadyEquipped: "装扮已装备",

	// 6xxx 房间
	ErrRoomNotFound:      "房间不存在",
	ErrRoomFull:          "房间已满",
	ErrUserAlreadyInRoom: "用户已在房间中",
	ErrUserNotInRoom:     "用户不在房间中",
	ErrRoomInvalidState:  "房间状态异常",

	// 7xxx 表情 / WS
	ErrEmojiNotFound:  "表情不存在",
	ErrWSNotConnected: "WebSocket 未连接",
}
