// Package ws 内 snapshot.go：room.snapshot 构造与下发框架（Story 10.7 引入；
// Story 11.7 加 realSnapshotBuilder 替换 prod 路径）。
//
// 角色：
//   - SnapshotBuilder 接口：抽象 BuildSnapshot(ctx, roomID) (Snapshot, error)，
//     让 placeholder（Story 10.7）/ real（Story 11.7）实装可替换
//   - placeholderSnapshotBuilder：Story 10.7 默认实装；走 RoomMemberRepo.ListMembers
//     单表查询（不 JOIN users / pets）；丰富字段（nickname / pet.petId）降级空字符串、
//     pet.currentState 固定 1；Story 11.7 后仅测试路径保留作便捷 stub
//   - realSnapshotBuilder：Story 11.7 真实实装；走 RoomMemberRepo.ListRosterByRoomID
//     多表 JOIN 聚合（INNER JOIN users + LEFT JOIN pets + ORDER BY joined_at ASC，
//     复用 Story 11.6 已落地路径），nickname / petId 真实回填，pet-less 下发 null
//   - Snapshot struct：room.snapshot payload 的 Go 表示；与 V1 §12.3 字段表
//     going-forward 行严格 1:1 对齐（SnapshotMember.Pet 改 *SnapshotPet pointer 让
//     pet-less 序列化为 JSON null —— Story 11.7 r1 P1 关键决策）
//   - SendRoomSnapshot 函数：封装 build → marshal → conn 同步写入 + close 1011
//     失败处理路径；gateway.Handle 单点调用
//
// 设计原则：
//   - 接口形态先行：SnapshotBuilder 是 interface，placeholder 实装是 unexported
//     struct（与 SessionManager / RedisClient / PresenceRepo 同模式）
//   - ctx 传播严格（ADR-0007）：所有方法 / 函数首参数 ctx context.Context
//   - 错误语义内化：BuildSnapshot 抛 error → SendRoomSnapshot 走 close 1011 路径，
//     **不**透传到 caller / **不**走 server "推 type=error 消息" 路径（V1 §12.1 close
//     code 表 1011 行 + §12.3 末"snapshot 构建失败处理路径"注 三方钦定）
//   - 同步写入优先：snapshot 必须**同步**写入 *websocket.Conn（不走 Session.Send 异步
//     队列），因为握手成功 → 启动 readLoop / writeLoop 之前是关键窗口期，writeLoop
//     此时还没启动（V1 §12.1 握手第 4 步钦定；lesson
//     2026-05-06-ws-snapshot-startup-order-and-placeholder-r7.md）
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huing/cat/server/internal/repo/mysql"
)

// Snapshot 是 room.snapshot payload 的 Go 表示。
//
// 与 V1 §12.3 字段表完全 1:1 对齐：
//   - Room.{ID, MaxMembers, MemberCount}: room.{id, maxMembers, memberCount}
//   - Members[i].{UserID, Nickname, Pet.{PetID, CurrentState}}: members[i].{userId,
//     nickname, pet.{petId, currentState}}
//
// 不变量（V1 §12.3 末"不变量"小节钦定）：MemberCount == len(Members)
//
// 节点 4 阶段 placeholder（Story 10.7）/ Story 11.7 真实实装下发的 wire envelope 一致
// —— 字段值差异：placeholder 阶段 Nickname/PetID 空字符串 + Pet pointer 始终非 nil
// + CurrentState=1；真实阶段（Story 11.7）由 users.nickname / pets.id 真实回填 +
// pet-less 时 Pet pointer 为 nil → wire 下发 `"pet": null`；
// CurrentState 自 Story 14.3 起读真实 pets.current_state（placeholder 阶段仍硬编码 1）。
//
// 注意：本 struct 直接作为 serverEnvelope.Payload 的具体类型 —— **不**额外引入
// "内部 domain model + 外部 DTO 转换"层，YAGNI（与既有 inline 实装的 snapshotPayload
// 同模式）。
type Snapshot struct {
	Room    SnapshotRoom     `json:"room"`
	Members []SnapshotMember `json:"members"`
}

// SnapshotRoom 是 room.snapshot.payload.room 的 Go 表示。
//
// MaxMembers 节点 4 阶段固定 4；Story 11.x 真实实装仍是 4（房间最大成员数是
// 静态契约）—— 字段保留是为 future epic 灵活性，但节点 4 阶段值不变。
type SnapshotRoom struct {
	ID          string `json:"id"`
	MaxMembers  int    `json:"maxMembers"`
	MemberCount int    `json:"memberCount"`
}

// SnapshotMember 是 room.snapshot.payload.members[i] 的 Go 表示。
//
// 字段：
//   - UserID: BIGINT 字符串化（与 V1 §2.5 全局 BIGINT 字符串化约定一致）
//   - Nickname: placeholder 阶段空字符串 ""；真实阶段（Story 11.7）by users.nickname
//   - Pet: 嵌套 *SnapshotPet（pointer 让 nil → JSON null —— V1 §12.3 行 1881 钦定
//     LEFT JOIN pets pet-less 下发 `pet: null`；不展平字段，与 V1 §12.3 字段表
//     pet.* 嵌套结构一致）
//
// **关键变更**（Story 11.7 引入 / r1 P1 关键决策）：Pet 字段类型从值类型 SnapshotPet
// 改为 pointer 类型 *SnapshotPet —— 让 LEFT JOIN pets pet-less 时 GORM Scan
// RosterRow.PetID 为 nil → realSnapshotBuilder.BuildSnapshot 内构造 m.Pet 保持
// nil → JSON 序列化为 `"pet": null`（与 V1 §12.3 行 1881 r14 锁定的 going-forward
// 契约 + §10.3 五阶段过渡表 pet 整体行 placeholder / 真实两列同值"pet-less → null"
// 严格对齐）。
//
// **Story 10.7 落地的 placeholder 兼容形态**：placeholderSnapshotBuilder.BuildSnapshot
// 在所有 member 行均下发 `&SnapshotPet{PetID: "", CurrentState: 1}`（即 pointer
// 始终非 nil，PetID 空字符串）—— 与 V1 §12.3 行 1978 r14 锁定的"Story 10.7 落地
// 实装与 going-forward 契约的差异"段一致；client 按 client merge contract 空字符串
// 路径保留已有 petId，**不**清空真实值。本 story 不回工 placeholder 路径，仅切
// bootstrap prod 路径走 real builder。
//
// **Wire 兼容性影响**（本 story 引入 r1 P1 关键决策）：
//   - placeholder 阶段（Story 10.7）所有 member 下发 `pet ≠ null + petId: ""`，
//     即 wire 上 `"pet": {"petId": "", "currentState": 1}` —— 改 Pet 为 pointer
//     后 placeholder 调用方（NewPlaceholderSnapshotBuilder）必须同步改实装为
//     `m.Pet = &SnapshotPet{...}` pointer 构造（已同步）
//   - 真实阶段（本 story 引入）pet-less 下发 `"pet": null`、否则下发
//     `"pet": {"petId": "<id>", "currentState": 1}` —— 与 V1 §12.3 going-forward
//     契约严格对齐
//   - client 解析层（Story 12.3 落地）按 `Optional<MemberPetDTO>` 处理（iOS Codable
//     decodeIfPresent / Go json.Unmarshal *SnapshotPet 自然 nullable）—— 无需改
//     iOS / Go decoder 即可同时正确处理两个阶段
//
// **不**给 SnapshotMember 加 omitempty tag：V1 §12.3 行 1881 钦定"`payload.members[].pet`
// 必填（nullable）"—— pet-less 时 wire 显式下发 `"pet": null`，**不**省略 key。
type SnapshotMember struct {
	UserID   string       `json:"userId"`
	Nickname string       `json:"nickname"`
	Pet      *SnapshotPet `json:"pet"` // pointer：nil → JSON null（pet-less，real 阶段路径）
}

// SnapshotPet 是 room.snapshot.payload.members[i].pet 的 Go 表示。
//
// 字段：
//   - PetID: placeholder 阶段空字符串 ""（V1 §12.3 字段表 placeholder 行钦定）
//   - CurrentState: placeholder 阶段（Story 10.7 r14 锁定）固定 1 (rest)；
//     Story 11.7 真实实装阶段亦固定 1；自 Story 14.3 起 realSnapshotBuilder
//     从 pets.current_state 读真实值（1=rest / 2=walk / 3=run，V1 §12.3
//     line 1988 + §10.3 line 1389 钦定），placeholder builder 仍硬编码 1
type SnapshotPet struct {
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// SnapshotBuilder 抽象 room.snapshot 构造路径。
//
// 节点 4 placeholder 实装：placeholderSnapshotBuilder（本文件
// NewPlaceholderSnapshotBuilder 构造）—— 走 RoomMemberRepo.ListMembers 单表查询；
// Story 11.7 后仅由测试路径保留作便捷 stub（prod 路径切到 real builder）。
//
// Story 11.7 真实实装：realSnapshotBuilder（本文件 NewRealSnapshotBuilder 构造）
// —— 走 room_members INNER JOIN users + LEFT JOIN pets 聚合丰富字段（复用 Story
// 11.6 ListRosterByRoomID 单 SQL 路径）；接口签名不变（10.7 r10 P1 钦定形态冻结）。
//
// 接口边界：BuildSnapshot 仅参与 server → client 推送路径；**不**参与 REST GET
// /rooms/{roomId}（REST handler 走 service 层 + 自己的 DTO，与 ws snapshot 字段
// 集合允许 drift —— V1 §10 REST 字段表与 §12.3 WS 字段表本来就独立锚定）。
type SnapshotBuilder interface {
	// BuildSnapshot 构造给定 roomID 的 Snapshot。
	//
	// 错误语义：
	//   - room 不存在 / DB 异常 → 返 error；caller（SendRoomSnapshot）走 close 1011
	//   - room 内 0 成员 → 返 Snapshot{Members: []}（节点 4 阶段不会出现，因为
	//     gateway.Handle 第 5 步已校验当前用户在 room_members；但接口语义上接受）
	//
	// 不抛 panic：所有底层异常用 error 透传；调用方走 close 1011 让客户端按 transient
	// network failure 重连，避免 panic 影响其他 session。
	//
	// ctx：透传给底层 ListMembers 调用；ctx cancel 时返 ctx.Err 包装的 error。
	BuildSnapshot(ctx context.Context, roomID uint64) (Snapshot, error)
}

// NewPlaceholderSnapshotBuilder 构造节点 4 阶段 placeholder 实装。
//
// 参数：
//   - roomMember: RoomMemberRepo（已在 main.go bootstrap 期 wire；与 gateway 共享同一
//     实例，避免重复构造）
//
// 调用方：bootstrap/router.go 把构造产物注入 wsapp.NewGateway(... , builder)。
//
// Story 11.7 切换路径：把 NewPlaceholderSnapshotBuilder 替换为 NewRealSnapshotBuilder
// （Story 11.7 提供）—— gateway 层完全不感知。
func NewPlaceholderSnapshotBuilder(roomMember mysql.RoomMemberRepo) SnapshotBuilder {
	return &placeholderSnapshotBuilder{roomMember: roomMember}
}

// placeholderSnapshotBuilder 是节点 4 阶段 SnapshotBuilder 的默认实装。
//
// 唯一依赖：RoomMemberRepo.ListMembers —— 单表查询（不 JOIN users / pets）。
//
// 行为（V1 §12.3 字段表 placeholder 行 + lesson
// 2026-05-06-ws-snapshot-placeholder-full-roster-and-disconnect-broadcast-r8.md
// 钦定）：
//   - 调 roomMember.ListMembers(ctx, roomID) 拿全部 user_id（**禁止**只取当前握手
//     用户 —— 房间已有 ≥2 成员时漏返其他成员会让 client 把 snapshot 当
//     authoritative state 错误清空已加载的 roster）
//   - 对每行构造 SnapshotMember{UserID: 字符串化, Nickname: "", Pet: &{PetID: "",
//     CurrentState: 1}}
//   - room.id 字符串化、room.maxMembers=4、room.memberCount=len(members)
//
// **r14 锁定的 placeholder 兼容形态**：所有 member 行下发 `pet ≠ null + petId: ""`
// （即 pointer 始终非 nil，PetID 空字符串），与 V1 §12.3 行 1978 r14 锁定的
// "Story 10.7 落地实装与 going-forward 契约的差异"段一致；Story 11.7 r1 P1 关键
// 决策把 SnapshotMember.Pet 改为 pointer 类型让 real 阶段 pet-less 下发 null，
// placeholder 实装层同步为 pointer 构造但语义不变（pointer 始终非 nil + petId 空字符串）。
//
// **测试便利保留**：bootstrap/router.go 在 Story 11.7 后切到 NewRealSnapshotBuilder，
// placeholder 仅由测试路径（ws_test.go / snapshot_test.go / ws_integration_test.go）
// 保留作便捷 stub，让既有 case 字段降级语义不需要 fixture 变动。
type placeholderSnapshotBuilder struct {
	roomMember mysql.RoomMemberRepo
}

// BuildSnapshot 实装 SnapshotBuilder.BuildSnapshot 接口。
func (b *placeholderSnapshotBuilder) BuildSnapshot(ctx context.Context, roomID uint64) (Snapshot, error) {
	members, err := b.roomMember.ListMembers(ctx, roomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("ws snapshot: list members for room %d: %w", roomID, err)
	}
	out := Snapshot{
		Room: SnapshotRoom{
			ID:          strconv.FormatUint(roomID, 10),
			MaxMembers:  4,
			MemberCount: len(members),
		},
		Members: make([]SnapshotMember, 0, len(members)),
	}
	for _, uid := range members {
		out.Members = append(out.Members, SnapshotMember{
			UserID:   strconv.FormatUint(uid, 10),
			Nickname: "",
			Pet: &SnapshotPet{ // pointer：与 V1 §12.3 行 1978 r14 锁定的 Story 10.7 落地形态一致：所有 member 下发 `pet ≠ null + petId: ""`
				PetID:        "",
				CurrentState: 1,
			},
		})
	}
	return out, nil
}

// NewRealSnapshotBuilder 构造节点 4 阶段真实 SnapshotBuilder 实装（Story 11.7 引入）。
//
// 与 NewPlaceholderSnapshotBuilder 的差异（V1 §12.3 行 1878 + 行 1976 + 行 1978
// r14 锁定的 going-forward 契约）：
//   - placeholder（Story 10.7 落地）：走 ListMembers 单表查询（不 JOIN users / pets），
//     所有 member 一律下发 `pet ≠ null + petId: ""` + nickname 空字符串 ""
//   - real（Story 11.7 落地，going-forward 契约；Story 14.3 落地 currentState 真值）：
//     走 ListRosterByRoomID 多表 JOIN 聚合（INNER JOIN users + LEFT JOIN pets ON
//     pets.is_default=1 + ORDER BY joined_at ASC，复用 Story 11.6 已落地路径），
//     nickname / petId 真实回填，pet-less 下发 `pet: null`，pet.currentState
//     **自 Story 14.3 起**从 RosterRow.CurrentState 读真实值（pets.current_state；
//     V1 §12.3 line 1988 / §10.3 line 1389 钦定）
//
// 参数：
//   - roomMember: RoomMemberRepo（已在 main.go bootstrap 期 wire；与 gateway / placeholder
//     共享同一实例，避免重复构造；与 Story 10.7 placeholder 同模式）
//
// 调用方：bootstrap/router.go 把构造产物注入 wsapp.NewGateway(... , builder)；
// 与 NewPlaceholderSnapshotBuilder 调用模式一致，仅替换构造函数名。
//
// 测试便利保留：placeholderSnapshotBuilder / NewPlaceholderSnapshotBuilder 在
// 本 story 不删除（既有 ws_test.go / snapshot_test.go / ws_integration_test.go
// 大量调用点保留作便捷 stub —— 本 story 仅切 prod 路径走 real builder）。
func NewRealSnapshotBuilder(roomMember mysql.RoomMemberRepo) SnapshotBuilder {
	return &realSnapshotBuilder{roomMember: roomMember}
}

// realSnapshotBuilder 是节点 4 阶段 SnapshotBuilder 的真实实装（Story 11.7 引入）。
//
// 唯一依赖：RoomMemberRepo.ListRosterByRoomID —— Story 11.6 已落地的多表 JOIN
// 聚合方法（INNER JOIN users + LEFT JOIN pets + ORDER BY joined_at ASC，单 SQL）。
//
// 行为（V1 §12.3 字段表 going-forward 行 + §10.3 五阶段过渡表节点 4 真实列钦定）：
//   - 调 roomMember.ListRosterByRoomID(ctx, roomID) 拿 []mysql.RosterRow 全 roster
//     （ORDER BY joined_at ASC 稳定排序；0 行 → []）
//   - 对每行 RosterRow 构造 SnapshotMember{
//       UserID:   strconv.FormatUint(r.UserID, 10),  // BIGINT 字符串化（V1 §2.5）
//       Nickname: r.Nickname,                          // 真实值（INNER JOIN users.nickname）
//       Pet:      <如下分流>,
//     }
//     - r.PetID == nil（pet-less，LEFT JOIN pets 行 NULL）→ m.Pet 保持 nil →
//       JSON 序列化为 `"pet": null`
//     - r.PetID != nil → SnapshotMember.Pet = &SnapshotPet{
//         PetID:        strconv.FormatUint(*r.PetID, 10), // BIGINT 字符串化
//         CurrentState: int(*r.CurrentState), // Story 14.3 落地：读 RosterRow.CurrentState（pets.current_state；schema §6.4 NOT NULL DEFAULT 1 → r.PetID != nil 分支内 *r.CurrentState 必非 nil）
//       }
//   - room.id 字符串化（与 placeholder 一致）
//   - room.maxMembers 固定 4（V1 §12.3 钦定，与 placeholder 一致）
//   - room.memberCount = len(Members)（不变量：== Members[]，与 placeholder 一致）
//
// **不**消费 PresenceRepo / 不下发 isOnline 字段（V1 §12.3 going-forward 契约不区分
// 在线 / 离线，roster = room_members 全行 view）；与 epics.md 行 1964 旧描述存在
// drift，本 story 严格按 V1 §12.3 r14 锁定的 going-forward 契约。
//
// **不**下发 avatarUrl 字段（本 story 范围红线 + r1 关键决策；client merge contract
// "未出现字段保留 client 已有值"路径已锁定，§15.6 推荐流程要求 client 先 GET
// /rooms/{roomId} 拿真实 avatarUrl）；future 如需扩展 SnapshotMember 字段集合再单开 story。
//
// **不**下发 pet.equips 字段（节点 4 阶段固定 []，本 story 范围红线 + V1 §12.3
// 行 1889 钦定 Epic 26 / Story 26.6 落地后再扩展）。
//
// ctx：透传给底层 ListRosterByRoomID 调用；ctx cancel 时返 ctx.Err 包装的 error。
//
// 错误：query 失败 → 返 (Snapshot{}, err 包装)；caller（SendRoomSnapshot）走
// close 1011 reason "snapshot build failed"（与 placeholder 错误处理路径一致）。
type realSnapshotBuilder struct {
	roomMember mysql.RoomMemberRepo
}

// BuildSnapshot 实装 SnapshotBuilder.BuildSnapshot 接口（real 路径）。
func (b *realSnapshotBuilder) BuildSnapshot(ctx context.Context, roomID uint64) (Snapshot, error) {
	roster, err := b.roomMember.ListRosterByRoomID(ctx, roomID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("ws snapshot: list roster for room %d: %w", roomID, err)
	}
	out := Snapshot{
		Room: SnapshotRoom{
			ID:          strconv.FormatUint(roomID, 10),
			MaxMembers:  4,
			MemberCount: len(roster),
		},
		Members: make([]SnapshotMember, 0, len(roster)),
	}
	for _, r := range roster {
		m := SnapshotMember{
			UserID:   strconv.FormatUint(r.UserID, 10),
			Nickname: r.Nickname,
			// Pet 字段填充见下：pet-less → nil；非 pet-less → &SnapshotPet{...}
		}
		if r.PetID != nil {
			// Story 14.3 落地：从 RosterRow.CurrentState 读真实值（pets.current_state；
			// schema §6.4 NOT NULL DEFAULT 1 → r.PetID != nil 分支内 *r.CurrentState 通常非 nil）
			// r1 fix：仍加 nil 守卫防御 schema 不变量损坏（future migration / 旧 binary 跑新 schema）
			// → 兜底默认值 1（与 §6.4 NOT NULL DEFAULT 1 一致），避免 panic 整个 snapshot 路径
			currentState := 1
			if r.CurrentState != nil {
				currentState = int(*r.CurrentState)
			}
			m.Pet = &SnapshotPet{
				PetID:        strconv.FormatUint(*r.PetID, 10),
				CurrentState: currentState,
			}
		}
		// r.PetID == nil → m.Pet 保持 nil → JSON 序列化为 `"pet": null`
		out.Members = append(out.Members, m)
	}
	return out, nil
}

// SendRoomSnapshot 把房间 snapshot 同步写入到 conn（gateway.Handle 调用点）。
//
// **必须同步写入**（不走 Session.Send 异步队列）—— 因为本函数在 readLoop / writeLoop
// 启动**之前**调用（V1 §12.1 握手第 3 步钦定 snapshot 同步写入；Story 10.3 r10 P1
// 修后顺序为 snapshot → Register → 启动 readLoop/writeLoop）；writeLoop 此时还没
// 启动，async 入队消息将永不被发送（lesson
// 2026-05-06-ws-snapshot-startup-order-and-placeholder-r7.md 钦定）。
//
// 错误处理：
//   - builder.BuildSnapshot 抛 error → 返 error；caller 走 close 1011 reason
//     "snapshot build failed"（V1 §12.1 1011 + §12.3 末"snapshot 构建失败处理
//     路径"钦定，**禁止**走 "推 type=error 消息" 路径，6005 错误码保留给 Epic
//     11.x / 14.x 业务流程的运行时状态错误推送；lesson
//     2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md 钦定）
//   - json.Marshal 抛 error（理论不可能，Snapshot struct 全是 marshalable 字段）
//     → 返 error；caller 同走 close 1011
//   - conn.WriteMessage 抛 error → 返 error；caller 同走 close 1011
//
// 写超时：调用方传入 writeTimeout > 0 时本函数 SetWriteDeadline；与既有 inline
// 实装语义一致。writeTimeout <= 0 表示不设 deadline（与既有兜底一致）。
//
// 注意签名：本函数签名是 (ctx, conn, roomID, builder, writeTimeout)；不传 *Session
// —— 因为本函数在 Session 进入 manager **之前**调用，session 此时是"裸"的没有
// sessionID（gateway.Handle 第 6.1 步：newSession("", userID, roomID, conn, ...)）；
// 直接传 conn 简化依赖。
//
// 节点 4 阶段调用点：仅 gateway.Handle 第 6.2 步（一次性握手期）；不参与 Session
// 生命周期内的重复推送（V1 §12.3 钦定服务端**可选**重复推送，但节点 4 不实装）。
//
// future：Story 11.7 真实实装替换 builder；Epic 14 / Epic 11 future story 如需
// "成员变化触发 snapshot 重发" 可在 Session.Send 路径调本函数（届时本函数若需
// 走 Session.Send 异步队列，再加 SendRoomSnapshotAsync 平行函数；当前不做）。
func SendRoomSnapshot(
	ctx context.Context,
	conn *websocket.Conn,
	roomID uint64,
	builder SnapshotBuilder,
	writeTimeout time.Duration,
) error {
	snap, err := builder.BuildSnapshot(ctx, roomID)
	if err != nil {
		return fmt.Errorf("ws snapshot: build for room %d: %w", roomID, err)
	}
	env := serverEnvelope{
		Type:      "room.snapshot",
		RequestID: "", // V1 §12.3 主动推送类固定 ""
		Payload:   snap,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		// json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap
		return fmt.Errorf("ws snapshot: marshal envelope for room %d: %w", roomID, err)
	}
	if writeTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	if err := conn.WriteMessage(websocket.TextMessage, bytes); err != nil {
		return fmt.Errorf("ws snapshot: write to conn for room %d: %w", roomID, err)
	}
	slog.Default().Debug("ws snapshot sent",
		slog.Uint64("roomID", roomID),
		slog.Int("memberCount", snap.Room.MemberCount),
	)
	return nil
}

// ============================================================================
// Story 11.8 — member.joined / member.left payload + envelope helpers
// ============================================================================

// MemberJoinedPayload 是 member.joined 消息的 payload（Story 11.8 引入）。
//
// 与 V1 §12.3 行 2003-2013 字段表完全 1:1 对齐：
//   - UserID:    BIGINT 字符串化（V1 §2.5 全局约定）
//   - Nickname:  来自 users.nickname；必非空字符串（节点 2 阶段首次创建时 server 写入 "用户{id}"）
//   - AvatarURL: 来自 users.avatar_url；可空字符串 ""（首次创建用户时为空），**不**为 null
//   - Pet:       *SnapshotPet pointer：nil → JSON null（pet-less 路径），与 SnapshotMember.Pet
//     同模式（Story 11.7 r1 P1 钦定 pointer 类型支持 JSON null 序列化）
//
// **关键决策**：本 struct **不**复用 SnapshotMember struct 自身 —— SnapshotMember 不含
// avatarUrl 字段（11.7 r1 P1 范围红线已锁定不回工 SnapshotMember 字段集合）；本 story
// 走"两个独立 struct，字段集合刻意 drift"路径，与 V1 §12.3 placeholder vs real vs
// member.joined 三阶段独立锚定的设计哲学一致。
//
// **Pet 字段类型**：用 *SnapshotPet pointer 类型（不展平字段；与 SnapshotMember.Pet
// 同模式），nil → JSON `"pet": null` + 非 nil → JSON object（含 petId / currentState）。
// **不**新增 MemberJoinedPet struct 重复 SnapshotPet 字段集合（YAGNI；两个 payload
// struct 共用 SnapshotPet 节省维护成本）。
type MemberJoinedPayload struct {
	UserID    string       `json:"userId"`
	Nickname  string       `json:"nickname"`
	AvatarURL string       `json:"avatarUrl"`
	Pet       *SnapshotPet `json:"pet"` // pointer：nil → JSON null（pet-less 路径）
}

// MemberLeftPayload 是 member.left 消息的 payload（Story 11.8 引入）。
//
// 与 V1 §12.3 行 2073-2080 字段表完全 1:1 对齐 —— 仅 1 字段 userId（V1 §12.3 行 2097
// 钦定 leave 事件 client UX 不需要显示昵称，payload 精简；client 从已有 roster 查
// nickname，UX 文案降级为"有人离开"也可接受，减少 server 加载压力）。
type MemberLeftPayload struct {
	UserID string `json:"userId"`
}

// BuildMemberJoinedEnvelope wrap MemberJoinedPayload 进 serverEnvelope + json.Marshal
// 返 ([]byte, error)（Story 11.8 引入）。
//
// 用途：service 层 RoomService.JoinRoom 在事务 commit 成功后调用本 helper 拿到
// []byte 后调 BroadcastFn 推送给该房间内其他在线 Session；隐藏 ws 包内部
// serverEnvelope struct，让 service 层只 import payload 类型 + helper 函数。
//
// envelope 字段值（V1 §12.3 通用信封 + 行 1840 钦定）：
//   - Type:      "member.joined"
//   - RequestID: ""（主动推送类消息固定 ""）
//   - Payload:   入参 payload
//   - Ts:        time.Now().UnixMilli()（服务端发送时间戳 ms）
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap（与
// SendRoomSnapshot 同模式）。caller 收到 error 时 log warn 不重试（与 broadcast
// 失败同 fire-and-forget 语义）。
func BuildMemberJoinedEnvelope(payload MemberJoinedPayload) ([]byte, error) {
	env := serverEnvelope{
		Type:      "member.joined",
		RequestID: "", // V1 §12.3 主动推送类消息固定 ""
		Payload:   payload,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal member.joined: %w", err)
	}
	return bytes, nil
}

// BuildMemberLeftEnvelope wrap MemberLeftPayload 进 serverEnvelope + json.Marshal
// 返 ([]byte, error)（Story 11.8 引入；与 BuildMemberJoinedEnvelope 同模式,
// Type = "member.left"）。
//
// envelope 字段值（V1 §12.3 通用信封）：
//   - Type:      "member.left"
//   - RequestID: ""
//   - Payload:   入参 payload（仅 userId 字段，V1 §12.3 钦定 leave 事件 payload 精简）
//   - Ts:        time.Now().UnixMilli()
func BuildMemberLeftEnvelope(payload MemberLeftPayload) ([]byte, error) {
	env := serverEnvelope{
		Type:      "member.left",
		RequestID: "",
		Payload:   payload,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal member.left: %w", err)
	}
	return bytes, nil
}

// ============================================================================
// Story 14.4 — pet.state.changed payload + envelope helper
// ============================================================================

// PetStateChangedPayload 是 pet.state.changed 消息的 payload（Story 14.4 引入）。
//
// 与 V1 §12.3 `### 宠物状态变更（pet.state.changed）` 字段表完全 1:1 对齐
// （V1 line 2223-2230 字段表）：
//   - UserID:       BIGINT 字符串化（V1 §2.5 全局约定）；状态变更的 user 主键，
//     来自 POST /pets/current/state-sync 当前 user.id
//   - PetID:        BIGINT 字符串化（V1 §2.5 全局约定）；状态变更的 pet 主键，
//     来自 service 层 FindDefaultByUserID 查到的 pets.id
//   - CurrentState: number (int) 必填；变更后宠物当前状态枚举（1=rest / 2=walk
//     / 3=run，与数据库 §6.4 pets.current_state 同义；与 §10.3 / §12.3 room.snapshot
//     / §12.3 member.joined 同语义；与 §5.2 request state 等价 —— 都是入参回显）
//
// **payload 字段集合严格只 3 字段**（V1 §12.3 行 2257 future fields 注 +
// 本 story 范围红线钦定）：不含 nickname / avatarUrl / equips / equips[].renderConfig
// 等任何其他字段；装备变更广播由独立路径（Epic 27 / 30 等）触发，**不**扩展本 payload。
//
// **关键约束**（V1 §12.3 line 2250 钦定）：3 字段都必填（**禁止** payload 为 `{}`
// 或缺任一字段）；缺字段视为契约违反，client 解析层走"安全忽略 + log warn"路径。
// Go struct 层不显式 omitempty（与 SnapshotMember / MemberJoinedPayload 同模式），
// 所有字段一律 JSON marshal 输出。
type PetStateChangedPayload struct {
	UserID       string `json:"userId"`
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// BuildPetStateChangedEnvelope wrap PetStateChangedPayload 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 14.4 引入；与 BuildMemberJoinedEnvelope
// / BuildMemberLeftEnvelope 同模式）。
//
// 用途：service 层 PetService.SyncCurrentState 在 UPDATE pets.current_state
// 成功后调用本 helper 拿到 []byte 后调 BroadcastFn 推送给该房间内所有在线
// Session（**包含**发起者自己 —— 与 member.joined / member.left 排除发起者不同
// 语义，详见 V1 §12.3 line 2249 "广播范围"段）；隐藏 ws 包内部 serverEnvelope
// struct，让 service 层只 import payload 类型 + helper 函数。
//
// envelope 字段值（V1 §12.3 通用信封 + 行 2225-2230 钦定）：
//   - Type:      "pet.state.changed"
//   - RequestID: ""（主动推送类消息固定 ""）
//   - Payload:   入参 payload
//   - Ts:        time.Now().UnixMilli()（服务端发送时间戳 ms；与 member.joined /
//     member.left ts 字段语义一致 —— 仅作日志关联 + UI 辅助展示，**禁止**用作业务
//     排序 / 状态新旧判定，V1 §12.3 line 2255 + line 1961 钦定）
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap（与
// SendRoomSnapshot / BuildMemberJoinedEnvelope 同模式）。caller 收到 error 时
// log warn 不重试（与 broadcast 失败同 fire-and-forget 语义，V1 §12.3 line 2254）。
func BuildPetStateChangedEnvelope(payload PetStateChangedPayload) ([]byte, error) {
	env := serverEnvelope{
		Type:      "pet.state.changed",
		RequestID: "", // V1 §12.3 主动推送类消息固定 ""
		Payload:   payload,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal pet.state.changed: %w", err)
	}
	return bytes, nil
}

// ============================================================================
// Story 17.5 — emoji.received payload + envelope helper
// ============================================================================

// EmojiReceivedPayload 是 emoji.received 消息的 payload（Story 17.5 引入）。
//
// 与 V1 §12.3 `### 收到表情广播（emoji.received）` 字段表完全 1:1 对齐：
//   - UserID:    BIGINT 字符串化（V1 §2.5 全局约定）；表情发起者的 user 主键；
//                来自 emoji.send 当前 user.id（即 WS 握手 token 解码后的 user.id）
//   - EmojiCode: 客户端发送的表情业务标识符；server 已在 §12.2 服务端逻辑步骤 4
//                校验过该 emojiCode 必然存在于 emoji_configs 且 is_enabled=1
//                （client 收到 emoji.received 时**无需**再次校验 —— server 端为
//                single source of truth）；与 §11.1 data.items[].code 同语义
//
// **payload 字段集合严格只 2 字段**（V1 §12.3 future fields 注 + 本 story 范围
// 红线钦定）：不含 ts（envelope 顶层已含）/ assetUrl（client 从 §11.1 缓存列表
// 查得）/ name（同上）/ 任何其他字段；client 用 emojiCode 作 key 在 §11.1 缓存中
// 定位 assetUrl / name 等渲染所需字段，**不**靠 broadcast 携带。
//
// **关键约束**（V1 §12.3 钦定）：2 字段都必填；缺字段视为契约违反，client 解析
// 层走"安全忽略 + log warn"路径。Go struct 层不显式 omitempty（与
// PetStateChangedPayload / MemberJoinedPayload 同模式），所有字段一律 JSON
// marshal 输出。
type EmojiReceivedPayload struct {
	UserID    string `json:"userId"`
	EmojiCode string `json:"emojiCode"`
}

// BuildEmojiReceivedEnvelope wrap EmojiReceivedPayload 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 17.5 引入；与 BuildPetStateChangedEnvelope
// / BuildMemberJoinedEnvelope / BuildMemberLeftEnvelope 同模式）。
//
// 用途：handler 层（emoji_handler.HandleEmojiSend）在 §12.2 服务端逻辑步骤 5
// 校验全通过后调用本 helper 拿到 []byte 后调 BroadcastFn 推送给该房间内所有
// 在线 Session（**包含**发起者自己 —— 与 member.joined / member.left 排除发起者
// 不同语义，与 pet.state.changed 同语义；详见 V1 §12.3）。
//
// envelope 字段值（V1 §12.3 通用信封 + 钦定）：
//   - Type:      "emoji.received"
//   - RequestID: ""（主动推送类消息固定 ""；V1 §12.3 钦定 ——
//                **不**回带 emoji.send.requestId，广播 fanout 给房间内所有 Session，
//                server 端无法对所有接收者都"配对" emoji.send.requestId；
//                发起者自己的 self-broadcast 也走广播路径，故 requestId 同样固定 ""）
//   - Payload:   入参 payload
//   - Ts:        time.Now().UnixMilli()（与 pet.state.changed / member.joined 同
//                语义，仅作日志关联 + UI 辅助展示，**禁止**用作业务排序）
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap。caller 收到
// error 时 log warn 不重试（与 broadcast 失败同 fire-and-forget 语义）。
func BuildEmojiReceivedEnvelope(payload EmojiReceivedPayload) ([]byte, error) {
	env := serverEnvelope{
		Type:      "emoji.received",
		RequestID: "", // V1 §12.3 主动推送类消息固定 ""
		Payload:   payload,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal emoji.received: %w", err)
	}
	return bytes, nil
}

// ============================================================================
// Story 17.5 — error payload + envelope helper（**首次**落地 WS error 路径）
// ============================================================================

// ErrorPayload 是 error 消息的 payload（Story 17.5 引入；V1 §12.3 `### error` 字段表）。
//
// 字段：
//   - Code:    错误码，复用 §3 全局错误码定义（本 story 用到的：1002 参数错误 /
//              1009 服务繁忙 / 6004 用户不在房间中 / 7001 emoji not found）
//   - Message: 错误描述，可读字符串（不做国际化，与 §3 message 字段一致）
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// BuildErrorEnvelope wrap (requestID, code, message) 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 17.5 引入；**首次**落地 WS error
// envelope helper —— 节点 4 / 5 既有路径都没用过 WS error 消息：snapshot 失败走
// close 1011；pet.state.changed / member.joined / member.left 都是 broadcast 类
// 消息失败仅 log，不回 error）。
//
// 用途：emoji_handler.HandleEmojiSend 在校验失败（1002 / 1009 / 6004 / 7001）时
// 调用本 helper 拿 []byte 后调 Session.SendPriority 直接推回发起者 Session
// （**不**走 broadcast —— V1 §12.2 钦定"错误响应通过 §12.3 error 消息回送 /
// requestId 回带 emoji.send.requestId"，是单 Session 响应而非房间广播）。
//
// envelope 字段值（V1 §12.3 通用信封 + 钦定）：
//   - Type:      "error"
//   - RequestID: caller 入参 requestID（响应类 error 回带 client 请求的 requestId；
//                server 主动错误传 "" 即可，本 story 只调用响应类路径）
//   - Payload:   ErrorPayload{Code, Message}
//   - Ts:        time.Now().UnixMilli()
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap（与既有
// envelope helpers 同模式）。
func BuildErrorEnvelope(requestID string, code int, message string) ([]byte, error) {
	env := serverEnvelope{
		Type:      "error",
		RequestID: requestID, // 响应类 error 回带 client 请求 requestId
		Payload:   ErrorPayload{Code: code, Message: message},
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal error: %w", err)
	}
	return bytes, nil
}
