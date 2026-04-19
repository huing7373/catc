# 错误码注册表

所有 MVP 错误码及其分类。此文件由 `internal/dto/error_codes_test.go` 校验与代码一致。

| Code | Category | HTTP Status | Message |
|---|---|---|---|
| `AUTH_INVALID_IDENTITY_TOKEN` | fatal | 401 | invalid identity token |
| `AUTH_TOKEN_EXPIRED` | fatal | 401 | token expired |
| `AUTH_REFRESH_TOKEN_REVOKED` | fatal | 401 | refresh token revoked |
| `FRIEND_ALREADY_EXISTS` | client_error | 409 | friend already exists |
| `FRIEND_LIMIT_REACHED` | client_error | 422 | friend limit reached |
| `FRIEND_INVITE_EXPIRED` | client_error | 410 | friend invite expired |
| `FRIEND_INVITE_USED` | client_error | 409 | friend invite already used |
| `FRIEND_BLOCKED` | client_error | 403 | user is blocked |
| `FRIEND_NOT_FOUND` | client_error | 404 | friend not found |
| `USER_NOT_FOUND` | client_error | 404 | user not found |
| `BLINDBOX_ALREADY_REDEEMED` | client_error | 409 | blindbox already redeemed |
| `BLINDBOX_INSUFFICIENT_STEPS` | client_error | 422 | insufficient steps |
| `BLINDBOX_NOT_FOUND` | client_error | 404 | blindbox not found |
| `SKIN_NOT_OWNED` | client_error | 403 | skin not owned |
| `RATE_LIMIT_EXCEEDED` | retry_after | 429 | rate limit exceeded |
| `EVENT_PROCESSING` | retry_after | 429 | event still processing |
| `DEVICE_BLACKLISTED` | fatal | 403 | device blacklisted |
| `INTERNAL_ERROR` | retryable | 500 | internal server error |
| `VALIDATION_ERROR` | client_error | 400 | validation error |
| `UNKNOWN_MESSAGE_TYPE` | client_error | 400 | unknown message type |
| `ROOM_FULL` | client_error | 409 | room is full |

## Category 说明

| Category | 客户端策略 |
|---|---|
| `retryable` | 指数退避自动重试 |
| `client_error` | 不重试，提示用户或调整请求 |
| `silent_drop` | 客户端无反应（fire-and-forget） |
| `retry_after` | 等待 Retry-After header 后重试 |
| `fatal` | 清理 token → 强制登出 |
