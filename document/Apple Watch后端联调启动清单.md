# Apple Watch 后端联调启动清单

这份文档用于把 watch 端和当前后端联机跑起来，目标是先验证：

- `GET /v1/platform/ws-registry`
- `debug.echo`
- `room.join`
- `action.update`
- `action.broadcast`

相关背景说明可以一起参考：

- [Apple Watch 后端接入说明](/Users/boommice/catc/document/Apple%20Watch后端接入说明.md)
- [联机同步设计稿](/Users/boommice/catc/document/联机同步设计稿.md)
- [联调 MVP 客户端对接指南](/Users/boommice/catc/docs/api/integration-mvp-client-guide.md)

---

## 1. 先确认后端入口

当前仓库里真正的后端入口是：

- [server/cmd/cat/main.go](/Users/boommice/catc/server/cmd/cat/main.go)

不是旧 README 里写的 `cmd/server`。

后端联调时，服务端必须跑在 `debug` 模式，否则 watch 端请求 `ws-registry` 时看不到 `room.join / action.update / action.broadcast`。

---

## 2. 启动基础设施

先起 MongoDB 和 Redis：

```bash
cd /Users/boommice/catc/server
make docker-up
```

关闭时用：

```bash
cd /Users/boommice/catc/server
make docker-down
```

`make docker-up` 对应的是 [server/Makefile](/Users/boommice/catc/server/Makefile) 里的这段：

- `docker compose -f deploy/docker-compose.yml up -d`

---

## 3. 准备本地配置

建议直接以当前的 [server/config/default.toml](/Users/boommice/catc/server/config/default.toml) 为基础复制一份本地配置：

```bash
cd /Users/boommice/catc/server
cp config/default.toml config/local.toml
```

然后把下面这些值改掉：

- `[server].mode = "debug"`
- `[server].port = 8080` 或你自己的端口
- `[server].host = "127.0.0.1"` 或留空
- `[mongo].uri`
- `[mongo].db`
- `[redis].addr`
- `[redis].db`

`local.toml.example` 也能参考，但它的字段骨架比较旧，和当前 `Config` 结构不完全一致，所以更推荐从 `default.toml` 复制。

### 3.1 本地配置样例

下面是一份可以直接参考的 `config/local.toml` 样例：

```toml
[server]
host = "127.0.0.1"
port = 8080
tls = false
mode = "debug"

[log]
level = "debug"
format = "json"
output = ""

[mongo]
uri = "mongodb://127.0.0.1:27017"
db = "cat"
timeout_sec = 10

[redis]
addr = "127.0.0.1:6379"
db = 0

[jwt]
private_key_path = ""
private_key_path_old = ""
active_kid = ""
old_kid = ""
issuer = ""
access_expiry_sec = 900
refresh_expiry_sec = 2592000

[ws]
max_connections = 10000
ping_interval_sec = 30
pong_timeout_sec = 60
send_buf_size = 256
dedup_ttl_sec = 300
connect_rate_per_window = 5
connect_rate_window_sec = 60
blacklist_default_ttl_sec = 86400
resume_cache_ttl_sec = 60

[apns]
enabled = false
key_id = ""
team_id = ""
bundle_id = ""
key_path = ""
watch_topic = ""
iphone_topic = ""
stream_key = "apns:queue"
dlq_key = "apns:dlq"
retry_zset_key = "apns:retry"
consumer_group = "apns_workers"
worker_count = 2
idem_ttl_sec = 300
read_block_ms = 1000
read_count = 10
retry_backoffs_ms = [1000, 3000, 9000]
max_attempts = 4
token_expiry_days = 30

[cdn]
base_url = ""
```

---

## 4. 启动后端

### 4.1 直接运行

```bash
cd /Users/boommice/catc/server
go run ./cmd/cat -config config/local.toml
```

### 4.2 先构建再运行

先在仓库根目录构建：

```bash
cd /Users/boommice/catc
bash scripts/build.sh
```

然后启动二进制：

```bash
cd /Users/boommice/catc/server
../build/catserver -config config/local.toml
```

`scripts/build.sh` 产物是：

- `/Users/boommice/catc/build/catserver`

---

## 5. 验证后端是不是 debug 模式

启动成功后，先访问 registry：

```bash
curl http://127.0.0.1:8080/v1/platform/ws-registry
```

如果后端已经是 debug 模式，你应该能在 `messages` 里看到这些类型：

- `debug.echo`
- `room.join`
- `action.update`
- `action.broadcast`

如果这里返回空数组，通常说明：

- 你还在跑 `release` 模式
- 或者你连错了服务端地址

---

## 6. watch 端需要怎么配

watch 端现在读取的环境变量名是：

- `CAT_BACKEND_HTTP_URL`
- `CAT_BACKEND_WS_URL`
- `CAT_BACKEND_NAME`
- `CAT_BACKEND_TOKEN`
- `CAT_BACKEND_ROOM_ID`

对应代码在：

- [BackendConfig.swift](/Users/boommice/catc/ios/CatWatch/App/Sync/BackendConfig.swift)

### 6.1 本地联调示例

如果你在 watch 模拟器上跑，并且后端也在本机：

```text
CAT_BACKEND_HTTP_URL=http://127.0.0.1:8080
CAT_BACKEND_WS_URL=ws://127.0.0.1:8080/ws
CAT_BACKEND_NAME=local-debug
CAT_BACKEND_TOKEN=watch-alice
CAT_BACKEND_ROOM_ID=test-room
```

如果是真机 watch 联调：

- `HTTP_URL` / `WS_URL` 要换成你 Mac 的局域网 IP
- `ws://` 在真机上通常会碰到 ATS 限制
- 真机优先考虑 `wss://`，或者临时给 debug build 加 ATS 豁免

---

## 7. 联机测试顺序

watch 端当前已经按这个顺序在跑：

1. 请求 `GET /v1/platform/ws-registry`
2. 连接 `/ws`
3. 发送 `debug.echo`
4. 收到 `debug.echo.result`
5. 自动发送 `room.join`
6. 收到 `room.join.result`
7. 本地状态变化时发送 `action.update`
8. 收到 `action.broadcast` 后更新好友猫状态

所以你可以按下面的观察顺序测：

- watch 首页是否能显示 `debug.echo 已跑通`
- `room.join` 是否成功
- 右侧 / 四宫格是否出现好友猫
- 本地猫状态变化后，好友猫是否同步变化

---

## 8. 常见问题

### 8.1 registry 是空的

大概率是后端没跑在 `debug` 模式。

### 8.2 `debug.echo` 成功但 `room.join` 没反应

先确认：

- `room.join` 是否真的出现在 registry 里
- watch 端的 `CAT_BACKEND_ROOM_ID` 是否有值
- `Authorization` 是否带上了非空 token

### 8.3 watch 端连不上本机

先确认：

- 你的 `CAT_BACKEND_WS_URL` 和 `CAT_BACKEND_HTTP_URL` 指向的是可达地址
- 真机不要直接指向 `127.0.0.1`
- 真机优先用局域网 IP 或 `wss://`

### 8.4 README 里命令和这里不一样

以当前仓库真实入口为准：

- `server/cmd/cat/main.go`
- `go run ./cmd/cat -config config/local.toml`

不是旧的 `cmd/server`。

