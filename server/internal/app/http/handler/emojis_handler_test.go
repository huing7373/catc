package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

// Story 17.4 — EmojisHandler.GetEmojis 单测（≥2 case stub service）
//
// 与 home_handler_test.go 同模式：gin TestMode + httptest.NewRecorder + stub service。
// 复用 ErrorMappingMiddleware 让 c.Error(err) 走完整 envelope 路径。

// stubEmojiService 用 fn 字段让每个 case 自定义返回。
//
// Story 17.5 加 validateCodeFn 占位字段（默认 nil；本 handler test 不调
// ValidateCode，但 service.EmojiService interface 已含该方法 → stub 必须实现以
// satisfy interface 编译）。
type stubEmojiService struct {
	listAvailableFn func(ctx context.Context) ([]service.EmojiBrief, error)
	validateCodeFn  func(ctx context.Context, code string) error
}

func (s *stubEmojiService) ListAvailable(ctx context.Context) ([]service.EmojiBrief, error) {
	return s.listAvailableFn(ctx)
}

func (s *stubEmojiService) ValidateCode(ctx context.Context, code string) error {
	if s.validateCodeFn == nil {
		// 本 handler test 不应调 ValidateCode；防御性 panic 让漂移暴露
		panic("stubEmojiService.ValidateCode not configured (emojis_handler_test 仅测 GET /emojis 路径，不期望走 WS emoji.send 路径)")
	}
	return s.validateCodeFn(ctx, code)
}

// buildEmojisHandlerRouter 构造 handler test router。
//
// 必挂中间件：ErrorMappingMiddleware（否则 c.Error 不写 envelope，断不到 envelope.code）。
//
// **不**挂 Auth 中间件 / UserID 注入 —— 与 home_handler_test 不同：emojis handler
// 不读 userID，service 也不需要 user 维度过滤，所以 route 测可以最小化（只测 handler
// 自身契约：service.ListAvailable 调用 + DTO 字段名 + 错误透传）。auth 行为由
// router_test / 集成测试覆盖。
func buildEmojisHandlerRouter(svc service.EmojiService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewEmojisHandler(svc)
	r.GET("/api/v1/emojis", h.GetEmojis)
	return r
}

// AC6.1 happy: service 返 4 个 EmojiBrief → envelope code=0 + items 长度=4 +
// 字段名严格 camelCase（assetUrl / sortOrder）+ **不**含 id / is_enabled / created_at / updated_at
func TestEmojisHandler_GetEmojis_HappyPath_4Rows(t *testing.T) {
	svc := &stubEmojiService{
		listAvailableFn: func(ctx context.Context) ([]service.EmojiBrief, error) {
			return []service.EmojiBrief{
				{Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1},
				{Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2},
				{Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3},
				{Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4},
			}, nil
		},
	}
	r := buildEmojisHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emojis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	bodyBytes := w.Body.Bytes()
	var body struct {
		Code int    `json:"code"`
		Data struct {
			Items []struct {
				Code      string `json:"code"`
				Name      string `json:"name"`
				AssetURL  string `json:"assetUrl"`
				SortOrder int    `json:"sortOrder"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body=%s", err, string(bodyBytes))
	}
	if body.Code != 0 {
		t.Errorf("body.code = %d, want 0", body.Code)
	}
	if len(body.Data.Items) != 4 {
		t.Fatalf("len(items) = %d, want 4", len(body.Data.Items))
	}
	if body.Data.Items[0].Code != "wave" || body.Data.Items[0].AssetURL != "https://x/wave" || body.Data.Items[0].SortOrder != 1 {
		t.Errorf("items[0] = %+v, want wave/https://x/wave/1", body.Data.Items[0])
	}
	if body.Data.Items[3].Code != "cry" || body.Data.Items[3].SortOrder != 4 {
		t.Errorf("items[3] = %+v, want cry/4", body.Data.Items[3])
	}

	// 防 snake_case 字段名 / 多余 client-不需要字段回归：用 raw JSON 解析顶层字段集合
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("raw json: %v", err)
	}
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &rawData); err != nil {
		t.Fatalf("raw data: %v", err)
	}
	// data 顶层应只含 "items"（**不**含 list / emojis / 等其他键）
	if _, ok := rawData["items"]; !ok {
		t.Errorf("data.items missing")
	}

	var rawItems []map[string]json.RawMessage
	if err := json.Unmarshal(rawData["items"], &rawItems); err != nil {
		t.Fatalf("raw items: %v", err)
	}
	if len(rawItems) != 4 {
		t.Fatalf("raw items len = %d, want 4", len(rawItems))
	}
	// 第 0 项字段集严格 = {code, name, assetUrl, sortOrder}（V1 §11.1 钦定）
	allowed := map[string]bool{"code": true, "name": true, "assetUrl": true, "sortOrder": true}
	for k := range rawItems[0] {
		if !allowed[k] {
			t.Errorf("items[0] 含 V1 §11.1 未声明字段 %q（**不应**下发 id / is_enabled / created_at / updated_at / asset_url snake_case）", k)
		}
	}
	for k := range allowed {
		if _, ok := rawItems[0][k]; !ok {
			t.Errorf("items[0] 缺字段 %q（V1 §11.1 钦定必有）", k)
		}
	}
}

// AC6.2 edge: service 返 1009 → envelope code=1009 + items 不存在
//
// ErrorMappingMiddleware 把 *AppError 1009 翻译到 HTTP 响应（具体 status / message
// 由 mapping 决定）；本断言只锁 envelope.code 业务码字段，与 home_handler_test 同模式。
func TestEmojisHandler_GetEmojis_ServiceError_Returns1009(t *testing.T) {
	svc := &stubEmojiService{
		listAvailableFn: func(ctx context.Context) ([]service.EmojiBrief, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := buildEmojisHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emojis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("json.Decode: %v; body=%s", err, w.Body.String())
	}
	if body.Code != apperror.ErrServiceBusy {
		t.Errorf("body.code = %d, want %d (1009)", body.Code, apperror.ErrServiceBusy)
	}
}

// AC6.3 edge: service 返 0 行 → envelope code=0 + items=[] **非 null**
//
// V1 §11.1 行 1833 钦定 items: [] 与 items: null 语义不同；防 service.ListAvailable
// 返 nil slice 或 handler emojiResponseDTO 兜底失效导致 wire 下发 null。
func TestEmojisHandler_GetEmojis_EmptyList_ReturnsEmptyArrayNotNull(t *testing.T) {
	svc := &stubEmojiService{
		listAvailableFn: func(ctx context.Context) ([]service.EmojiBrief, error) {
			return []service.EmojiBrief{}, nil
		},
	}
	r := buildEmojisHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emojis", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// 解析 data.items 的 raw JSON，验证为 `[]` 而非 `null`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("raw: %v", err)
	}
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &rawData); err != nil {
		t.Fatalf("data: %v", err)
	}
	got := string(rawData["items"])
	if got == "null" {
		t.Errorf("data.items = null, want [] (V1 §11.1 行 1833 钦定)")
	}
	if got != "[]" {
		t.Errorf("data.items = %q, want []", got)
	}
}
