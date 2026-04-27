// Package service 收纳 server 端的业务 service 层。
//
// 本包是节点 2 §Story 4.6 起的真实业务 service 落地点（auth_service.go
// 是第一个）；后续 Epic 4.8 home_service / Epic 7 step_service / 等等同包平级。
//
// # 分层职责（设计文档 §5.2）
//
//   - service 层是**业务规则归属层**（业务常量在此定义）+ **事务边界**控制者
//     （调 txMgr.WithTx 包多 repo 写入）+ **跨 repo 编排**（不同 repo 的 happy 链 / 错误链）
//   - service **不**直接依赖 *gorm.DB —— 仅 import repo interface + repo 哨兵 error +
//     tx.Manager interface
//   - service 层的错误：repo raw error 或 sentinel → service apperror.Wrap → handler c.Error
//     → middleware envelope（ADR-0006 三层映射）
package service

import (
	"context"
	stderrors "errors"
	"strconv"
	"time"

	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// 业务常量（数据库设计 §5 / V1 §4.1 钦定）。在 service 包定义的理由：service 是
// 业务规则归属层（设计文档 §5.2）；handler 不应硬编码业务值（handler 只做 wire），
// repo 不应承载业务规则（repo 只做 CRUD）。
//
// 这些常量与 internal/repo/mysql 包内的 AuthTypeGuest 常量同源（值都是 1），
// 但两包独立定义避免双向 import 循环。
const (
	authTypeGuest         = 1                // user_auth_bindings.auth_type 游客取值
	petTypeDefault        = 1                // pets.pet_type 默认（节点 2 阶段固定 1=猫）
	petCurrentStateRest   = 1                // pets.current_state 默认（1=rest）
	petIsDefaultYes       = 1                // pets.is_default 标识默认宠物
	petNameDefault        = "默认小猫"          // V1 §4.1 行 187 钦定首次创建宠物名
	userStatusActive      = 1                // users.status 1=active
	chestStatusCounting   = 1                // user_chests.status 1=counting（倒计时中）
	chestOpenCostStepsDef = 1000             // user_chests.open_cost_steps 默认 1000（数据库设计 §5.6）
	chestUnlockDelay      = 10 * time.Minute // user_chests.unlock_at = now()+10min（V1 §4.1 + 数据库设计 §6.7）
)

// AuthService 是 auth handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**的理由：handler 单测时注入 stub struct（实现本 interface），
// 不引入第三方 mock 框架（gomock / testify mock）；与 4.5 中间件单测同模式。
type AuthService interface {
	// GuestLogin: 复用或创建游客身份。
	//
	// 流程：
	//  1. authBindingRepo.FindByGuestUID(ctx, guestUid) 查 binding（**事务外**普通查询）
	//  2. **命中**（err == nil）：调 reuseLogin（不开事务，加载 user + pet + 签 token）
	//  3. **未命中**（errors.Is(err, repo.ErrAuthBindingNotFound)）：调 firstTimeLogin
	//     （开事务初始化 5 行 + 签 token）
	//  4. **其他 err**（DB 异常）：apperror.Wrap(err, ErrServiceBusy) → 1009
	//
	// 错误约定：handler 透传，由 ErrorMappingMiddleware 写 envelope（ADR-0006）。
	// service 层永远只产出 *AppError，不返 raw error。
	GuestLogin(ctx context.Context, in GuestLoginInput) (*GuestLoginOutput, error)
}

// GuestLoginInput 是 service 层 DTO（**不是** wire DTO，handler 转换）。
//
// 节点 2 阶段：Platform / AppVersion / DeviceModel 字段不消费业务逻辑（只做契约
// 占位 + future audit log 用）；service 层不校验它们 —— 校验由 handler 在前面做。
type GuestLoginInput struct {
	GuestUID    string
	Platform    string
	AppVersion  string
	DeviceModel string
}

// GuestLoginOutput 是 service 层 DTO；由 handler 翻译成 V1 §4.1 钦定的 wire DTO
// （BIGINT id 转 string / petType 保 number / hasBoundWechat 节点 2 永远 false 等）。
type GuestLoginOutput struct {
	Token          string
	UserID         uint64
	Nickname       string
	AvatarURL      string
	HasBoundWechat bool
	PetID          uint64
	PetType        int
	PetName        string
}

// authServiceImpl 是 AuthService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - txMgr: 事务管理器（4.2 落地）
//   - signer: JWT 签发（4.4 落地）
//   - userRepo / authBindingRepo / petRepo / stepAccountRepo / chestRepo: 5 个 mysql repo
type authServiceImpl struct {
	txMgr            tx.Manager
	signer           *auth.Signer
	userRepo         mysql.UserRepo
	authBindingRepo  mysql.AuthBindingRepo
	petRepo          mysql.PetRepo
	stepAccountRepo  mysql.StepAccountRepo
	chestRepo        mysql.ChestRepo
}

// NewAuthService 构造 AuthService。
//
// 全部依赖通过参数显式注入；不引入 wire / fx 框架（与 4.2 / 4.4 / 4.5 同模式）。
func NewAuthService(
	txMgr tx.Manager,
	signer *auth.Signer,
	userRepo mysql.UserRepo,
	authBindingRepo mysql.AuthBindingRepo,
	petRepo mysql.PetRepo,
	stepAccountRepo mysql.StepAccountRepo,
	chestRepo mysql.ChestRepo,
) AuthService {
	return &authServiceImpl{
		txMgr:           txMgr,
		signer:          signer,
		userRepo:        userRepo,
		authBindingRepo: authBindingRepo,
		petRepo:         petRepo,
		stepAccountRepo: stepAccountRepo,
		chestRepo:       chestRepo,
	}
}

// GuestLogin 是 AuthService.GuestLogin 的实装。详见 interface godoc。
func (s *authServiceImpl) GuestLogin(ctx context.Context, in GuestLoginInput) (*GuestLoginOutput, error) {
	// (1) 查 binding（**事务外**，普通连接池查询；不需要事务包裹）
	binding, err := s.authBindingRepo.FindByGuestUID(ctx, in.GuestUID)
	if err != nil && !stderrors.Is(err, mysql.ErrAuthBindingNotFound) {
		// DB 异常 → 包成 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (2) 命中分支：复用
	if binding != nil {
		return s.reuseLogin(ctx, binding.UserID)
	}

	// (3) 未命中分支：开事务初始化 5 行
	return s.firstTimeLogin(ctx, in.GuestUID)
}

// reuseLogin 加载已有 user + 默认 pet → 签 token；**不**开事务（只读 + 签 token，
// 没有 multi-row 写入；开事务反而 overhead）。
//
// HasBoundWechat 节点 2 阶段游客永远 false（V1 §4.1 行 184）；future bind-wechat
// epic 实装时改成查 user_auth_bindings WHERE auth_type=2（wechat）。
func (s *authServiceImpl) reuseLogin(ctx context.Context, userID uint64) (*GuestLoginOutput, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		// 理论不应发生（binding 存在但 user 不存在 → 数据脏）；包成 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	pet, err := s.petRepo.FindDefaultByUserID(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	token, err := s.signer.Sign(user.ID, 0) // 0 = 用 default expire（4.4 钦定）
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	return &GuestLoginOutput{
		Token:          token,
		UserID:         user.ID,
		Nickname:       user.Nickname,
		AvatarURL:      user.AvatarURL,
		HasBoundWechat: false, // 节点 2 阶段游客永远 false（V1 §4.1 行 184）
		PetID:          pet.ID,
		PetType:        int(pet.PetType),
		PetName:        pet.Name,
	}, nil
}

// firstTimeLogin 实装 5 行初始化事务（数据库设计 §8.1）。
//
// # 事务内顺序（必须按此顺序）
//
//  1. users 插入 → 拿到 user.ID（AUTO_INCREMENT 由 GORM 回填）
//  2. users.UpdateNickname：写真实昵称 "用户{id}"（需要 user.ID 故必须紧跟在 1 之后）
//  3. user_auth_bindings 插入（type=1, identifier=guestUid, user_id=user.ID）
//  4. pets 插入（user_id=user.ID, pet_type=1, is_default=1, name="默认小猫"）
//  5. user_step_accounts 插入（user_id=user.ID, 全 0）
//  6. user_chests 插入（user_id=user.ID, status=1, unlock_at=now+10min UTC）
//
// 后 4 行依赖 user.ID（外键关联），所以**必须**先 INSERT users 拿 ID 再依次写。
//
// # nickname 两步写
//
// AUTO_INCREMENT 语义约束：必须先 INSERT 才知道 id；不能在 INSERT 前预填 nickname。
// 本 story 选 INSERT users（nickname 占位空串）→ UPDATE users.nickname = "用户{id}"
// 两步路径 —— 这是 V1 §4.1 行 182 钦定 "自动生成 nickname=用户{id}" 的唯一可行实装
// （不用 trigger 是为了让业务规则归属 service 层而非 DB 层，符合设计文档 §5.2）。
//
// # 并发幂等：ER_DUP_ENTRY → reuseLogin（覆盖两条 race 路径）
//
// firstTimeLogin 内有**两个**唯一约束可能在并发场景被打破，事务必须穷举：
//
//   - users.uk_guest_uid（**最早**的冲突点 —— 步骤 1 INSERT users）
//   - user_auth_bindings.uk_auth_type_identifier（步骤 3 INSERT binding）
//
// 这两个唯一索引在不同表上，但都是"同 guestUid 不能落两条记录"语义的实施点。
// 不同表 → repo 层产出**不同**哨兵 error（ErrUsersGuestUIDDuplicate /
// ErrAuthBindingDuplicate）—— 不能合并成一个，因为底层失败位置不同。**但** service
// 层对它们的反应**必须一致**（都是"先入者赢，我退到 reuseLogin"），否则会出现：
// Tx A 先 commit → Tx B INSERT users 1062 → 当前代码若只识别 binding-dup，
// users-dup 会落入 generic error → 1009 → 客户端误以为"服务异常"重试 …
// 违反 V1 §4.1 钦定的 "同一 guestUid 重复调用 → 同一 user_id" 幂等语义。
//
// 并发场景全展开（两条 Tx 同 guestUid，A 先到）：
//
// ## 路径 1：A 已 commit 整个事务 → B 在 users 阶段就被拒
//
//  1. A 的 firstTimeLogin: INSERT users → INSERT binding → INSERT pet/step/chest → COMMIT ✓
//  2. B 的 FindByGuestUID 在 A commit **之前**抓到的快照 → 返 NotFound → 进 firstTimeLogin
//  3. B 的 INSERT users 触发 uk_guest_uid 冲突 → repo 翻译为 ErrUsersGuestUIDDuplicate
//  4. 事务 rollback → service 捕获哨兵 → 重新 FindByGuestUID（这次拿到 A 的 binding）
//     → 调 reuseLogin → 返回 A 的 user_id + B 自己签的 token
//
// ## 路径 2：A 还在事务中（已 INSERT users 但未 commit）→ B 在 binding 阶段被拒
//
//  1. A 的 firstTimeLogin: INSERT users 成功 → INSERT binding 成功（事务中）
//  2. B 的 firstTimeLogin: INSERT users 成功（B 拿到不同 user_id；users.uk_guest_uid
//     在 A 未 commit 时不会 block B —— InnoDB 唯一索引在 INSERT 时**不**做行锁，是
//     **insert intent gap lock**；具体行为取决于事务隔离级别 + B 的 INSERT 时间点）
//  3. B 的 INSERT binding 触发 uk_auth_type_identifier 冲突 → 翻译为 ErrAuthBindingDuplicate
//  4. 同上回退 reuseLogin
//
// 实务里 InnoDB 的具体行为更复杂（B 的 users INSERT 可能也会被 A 的事务 block 直到
// A commit/rollback —— 取决于隔离级别 + uk_guest_uid 的 insert intent gap lock 行为）。
// 但**业务侧**只需保证**两条路径都走 reuseLogin** —— 不去赌哪条路径更常见，两条都覆盖。
//
// # 失败回滚 → 1009
//
// 任一步抛非 ErrAuthBindingDuplicate 的 error → tx.WithTx 自动 rollback → fn
// 返 error → service 包装为 ErrServiceBusy(1009)。client 侧由 Story 5.4
// SilentReloginUseCase 处理重试。
//
// # ctx 用法（ADR-0007 §2.4）
//
// fn 内的所有 repo 调用必须用 **txCtx** 而非外层 ctx —— txCtx 携带 tx 句柄，
// repo 通过 tx.FromContext(txCtx, fallback) 取到 tx；用错 ctx 会绕过 tx 走 db pool
// 新连接，该调用脱离事务，业务语义错乱。
func (s *authServiceImpl) firstTimeLogin(ctx context.Context, guestUID string) (*GuestLoginOutput, error) {
	var (
		user *mysql.User
		pet  *mysql.Pet
	)

	err := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (1) users 插入；GORM 回填 u.ID（AUTO_INCREMENT）
		u := &mysql.User{
			GuestUID:  guestUID,
			Nickname:  "", // 占位，下面 UpdateNickname 填真实昵称
			AvatarURL: "",
			Status:    userStatusActive,
		}
		if err := s.userRepo.Create(txCtx, u); err != nil {
			return err
		}
		user = u

		// (2) UPDATE users.nickname = "用户{id}"
		// nickname 长度："用户" 2 字符 + uint64 max 20 字符 = 最长 22 < 64（VARCHAR 限制），
		// 永远不会超长，无需校验长度（详见 story Dev Notes "nickname 编码 / 长度"）。
		nickname := "用户" + strconv.FormatUint(u.ID, 10)
		if err := s.userRepo.UpdateNickname(txCtx, u.ID, nickname); err != nil {
			return err
		}
		u.Nickname = nickname

		// (3) user_auth_bindings 插入；冲突 → ErrAuthBindingDuplicate
		if err := s.authBindingRepo.Create(txCtx, &mysql.AuthBinding{
			UserID:         u.ID,
			AuthType:       authTypeGuest,
			AuthIdentifier: guestUID,
		}); err != nil {
			return err
		}

		// (4) pets 插入（默认猫）
		p := &mysql.Pet{
			UserID:       u.ID,
			PetType:      petTypeDefault,
			Name:         petNameDefault,
			CurrentState: petCurrentStateRest,
			IsDefault:    petIsDefaultYes,
		}
		if err := s.petRepo.Create(txCtx, p); err != nil {
			return err
		}
		pet = p

		// (5) user_step_accounts 插入（全 0）
		if err := s.stepAccountRepo.Create(txCtx, &mysql.StepAccount{
			UserID:         u.ID,
			TotalSteps:     0,
			AvailableSteps: 0,
			ConsumedSteps:  0,
			Version:        0,
		}); err != nil {
			return err
		}

		// (6) user_chests 插入（counting + unlock_at = now + 10min UTC）
		// time.Now().UTC() 必须 —— V1 §2.5 钦定时间字段 ISO 8601 UTC；不能用本地时区。
		if err := s.chestRepo.Create(txCtx, &mysql.UserChest{
			UserID:        u.ID,
			Status:        chestStatusCounting,
			UnlockAt:      time.Now().UTC().Add(chestUnlockDelay),
			OpenCostSteps: chestOpenCostStepsDef,
			Version:       0,
		}); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// 检查是否是**两条 race 路径**（users.uk_guest_uid / user_auth_bindings.uk_auth_type_identifier）
		// 任一冲突 → 走复用分支。两条都必须覆盖，详见函数顶部 "# 并发幂等" 一节。
		//
		// **关键**：两个不同表的唯一约束 → 两个独立的 sentinel error；不能省略其中任一
		// 检查（任何一条漏判都会让"同 guestUid 并发"在某个 timing 下退化为 1009）。
		if stderrors.Is(err, mysql.ErrAuthBindingDuplicate) || stderrors.Is(err, mysql.ErrUsersGuestUIDDuplicate) {
			binding, ferr := s.authBindingRepo.FindByGuestUID(ctx, guestUID)
			if ferr != nil || binding == nil {
				// 极罕见：刚抛 duplicate，紧接 FindByGuestUID 又查不到 —— 数据脏 / 异常路径
				return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
			}
			return s.reuseLogin(ctx, binding.UserID)
		}
		// 其他失败 → 1009（事务已 rollback）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 事务 commit 成功 → 签 token + 返回（**事务外**签，HMAC 不应拉长事务持续时间）
	token, err := s.signer.Sign(user.ID, 0)
	if err != nil {
		// 极罕见：事务已 commit 但 sign 失败 —— **不**回滚（用户已创建落库）；
		// 直接返 1009，client 的 SilentRelogin 会重新调一次（走 reuseLogin 拿新 token）。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	return &GuestLoginOutput{
		Token:          token,
		UserID:         user.ID,
		Nickname:       user.Nickname,
		AvatarURL:      user.AvatarURL,
		HasBoundWechat: false, // 节点 2 阶段游客永远 false
		PetID:          pet.ID,
		PetType:        int(pet.PetType),
		PetName:        pet.Name,
	}, nil
}
