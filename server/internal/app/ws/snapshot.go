// Package ws 内 snapshot.go：room.snapshot 构造与下发框架（Story 10.7 引入）。
//
// 角色：
//   - SnapshotBuilder 接口：抽象 BuildSnapshot(ctx, roomID) (Snapshot, error)，
//     让 placeholder（本 story）/ real（Story 11.7）实装可替换
//   - placeholderSnapshotBuilder：本 story 的默认实装；走 RoomMemberRepo.ListMembers
//     单表查询（不 JOIN users / pets）；丰富字段（nickname / pet.petId）降级空字符串、
//     pet.currentState 固定 1
//   - Snapshot struct：room.snapshot payload 的 Go 表示；与 V1 §12.3 字段表
//     placeholder 行严格 1:1 对齐
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
// 节点 4 阶段 placeholder（Story 10.7）/ Story 11.7 真实实装下发的 wire 格式严格
// 一致 —— 仅丰富字段值不同（placeholder 阶段 Nickname/PetID 空字符串 + CurrentState=1；
// 真实阶段由 users.nickname / pets.id / pets.currentState 真实回填）。
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
//   - Nickname: placeholder 阶段空字符串 ""（V1 §12.3 字段表 placeholder 行钦定）
//   - Pet: 嵌套 SnapshotPet（不展平 —— 与 V1 §12.3 字段表 pet.* 嵌套结构一致）
type SnapshotMember struct {
	UserID   string      `json:"userId"`
	Nickname string      `json:"nickname"`
	Pet      SnapshotPet `json:"pet"`
}

// SnapshotPet 是 room.snapshot.payload.members[i].pet 的 Go 表示。
//
// 字段：
//   - PetID: placeholder 阶段空字符串 ""（V1 §12.3 字段表 placeholder 行钦定）
//   - CurrentState: 节点 4 阶段固定 1 (stationary_or_unknown)；Story 11.7 真实
//     实装亦固定 1（Epic 14 才真实驱动）—— V1 §12.3 字段表钦定
type SnapshotPet struct {
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// SnapshotBuilder 抽象 room.snapshot 构造路径。
//
// 节点 4 placeholder 实装：placeholderSnapshotBuilder（本文件
// NewPlaceholderSnapshotBuilder 构造）—— 走 RoomMemberRepo.ListMembers 单表查询。
//
// Story 11.7 真实实装：realSnapshotBuilder（在 Story 11.7 落地，本 story 不实装）
// —— 走 room_members JOIN users JOIN pets 聚合丰富字段；接口签名不变。
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
//   - 对每行构造 SnapshotMember{UserID: 字符串化, Nickname: "", Pet: {PetID: "",
//     CurrentState: 1}}
//   - room.id 字符串化、room.maxMembers=4、room.memberCount=len(members)
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
			Pet: SnapshotPet{
				PetID:        "",
				CurrentState: 1,
			},
		})
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
