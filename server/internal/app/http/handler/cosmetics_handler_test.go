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

// Story 23.3 — CosmeticsHandler.GetCatalog 单测（≥3 case stub service）
//
// 与 emojis_handler_test.go 同模式：gin TestMode + httptest.NewRecorder + stub
// service。复用 ErrorMappingMiddleware 让 c.Error(err) 走完整 envelope 路径。

// stubCosmeticService 用 fn 字段让每个 case 自定义返回。
type stubCosmeticService struct {
	listCatalogFn func(ctx context.Context) ([]service.CosmeticBrief, error)
}

func (s *stubCosmeticService) ListCatalog(ctx context.Context) ([]service.CosmeticBrief, error) {
	return s.listCatalogFn(ctx)
}

// buildCosmeticsHandlerRouter 构造 handler test router。
//
// 必挂中间件：ErrorMappingMiddleware（否则 c.Error 不写 envelope，断不到
// envelope.code）。
//
// **不**挂 Auth 中间件 / UserID 注入 —— 与 emojis_handler_test 同：cosmetics
// handler 不读 userID，service 也不需要 user 维度过滤，所以 route 测可以最小化
// （只测 handler 自身契约：service.ListCatalog 调用 + DTO 字段名 / 类型 + 错误
// 透传）。auth 行为由 router_test / 集成测试覆盖。
func buildCosmeticsHandlerRouter(svc service.CosmeticService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewCosmeticsHandler(svc)
	r.GET("/api/v1/cosmetics/catalog", h.GetCatalog)
	return r
}

// AC5.6 happy: service 返 N 个 CosmeticBrief → envelope code=0 + items 长度 N +
// cosmeticItemId 是 **string** + slot/rarity 是 **number** + 字段名严格
// camelCase（cosmeticItemId / iconUrl / assetUrl）+ **不**含 dropWeight /
// isEnabled / createdAt / updatedAt / snake_case。
func TestCosmeticsHandler_GetCatalog_HappyPath_3Rows(t *testing.T) {
	svc := &stubCosmeticService{
		listCatalogFn: func(ctx context.Context) ([]service.CosmeticBrief, error) {
			return []service.CosmeticBrief{
				{CosmeticItemID: 1, Code: "hat_yellow", Name: "小黄帽", Slot: 1, Rarity: 1, IconURL: "https://x/i1", AssetURL: "https://x/a1"},
				{CosmeticItemID: 2, Code: "hat_red", Name: "小红帽", Slot: 1, Rarity: 1, IconURL: "https://x/i2", AssetURL: "https://x/a2"},
				{CosmeticItemID: 13, Code: "hat_crown", Name: "金王冠", Slot: 1, Rarity: 3, IconURL: "https://x/i3", AssetURL: "https://x/a3"},
			}, nil
		},
	}
	r := buildCosmeticsHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/catalog", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	bodyBytes := w.Body.Bytes()
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				CosmeticItemID string `json:"cosmeticItemId"`
				Code           string `json:"code"`
				Name           string `json:"name"`
				Slot           int    `json:"slot"`
				Rarity         int    `json:"rarity"`
				IconURL        string `json:"iconUrl"`
				AssetURL       string `json:"assetUrl"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body=%s", err, string(bodyBytes))
	}
	if body.Code != 0 {
		t.Errorf("body.code = %d, want 0", body.Code)
	}
	if len(body.Data.Items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(body.Data.Items))
	}
	it0 := body.Data.Items[0]
	if it0.CosmeticItemID != "1" || it0.Code != "hat_yellow" || it0.Name != "小黄帽" ||
		it0.Slot != 1 || it0.Rarity != 1 || it0.IconURL != "https://x/i1" || it0.AssetURL != "https://x/a1" {
		t.Errorf("items[0] = %+v, want cosmeticItemId=\"1\" hat_yellow 小黄帽 slot=1 rarity=1 ...", it0)
	}
	if body.Data.Items[2].CosmeticItemID != "13" || body.Data.Items[2].Rarity != 3 {
		t.Errorf("items[2] = %+v, want cosmeticItemId=\"13\" rarity=3", body.Data.Items[2])
	}

	// 防 cosmeticItemId 被序列化成 number / snake_case 字段名 / 多余字段回归：
	// 用 raw JSON 解析校验 cosmeticItemId 是 JSON string（带引号）+ slot/rarity
	// 是 JSON number（无引号）+ 字段集严格 = §8.1 钦定 7 字段。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("raw json: %v", err)
	}
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &rawData); err != nil {
		t.Fatalf("raw data: %v", err)
	}
	if _, ok := rawData["items"]; !ok {
		t.Errorf("data.items missing")
	}
	var rawItems []map[string]json.RawMessage
	if err := json.Unmarshal(rawData["items"], &rawItems); err != nil {
		t.Fatalf("raw items: %v", err)
	}
	if len(rawItems) != 3 {
		t.Fatalf("raw items len = %d, want 3", len(rawItems))
	}
	// cosmeticItemId 必须是 JSON string（带引号），**不**是 number
	if got := string(rawItems[0]["cosmeticItemId"]); got != `"1"` {
		t.Errorf("rawItems[0].cosmeticItemId = %s, want \"1\" (string，§8.1 行 1262 BIGINT 字符串化；不可序列化成 number)", got)
	}
	// slot / rarity 必须是 JSON number（无引号），**不**字符串化
	if got := string(rawItems[0]["slot"]); got != "1" {
		t.Errorf("rawItems[0].slot = %s, want 1 (int，§8.1 行 1265 不字符串化)", got)
	}
	if got := string(rawItems[0]["rarity"]); got != "1" {
		t.Errorf("rawItems[0].rarity = %s, want 1 (int，§8.1 行 1266 不字符串化)", got)
	}
	// 字段集严格 = §8.1 钦定 7 字段（无多余 / 无缺失 / 全 camelCase）
	allowed := map[string]bool{
		"cosmeticItemId": true, "code": true, "name": true,
		"slot": true, "rarity": true, "iconUrl": true, "assetUrl": true,
	}
	for k := range rawItems[0] {
		if !allowed[k] {
			t.Errorf("items[0] 含 §8.1 未声明字段 %q（**不应**下发 dropWeight / isEnabled / createdAt / updatedAt / snake_case）", k)
		}
	}
	for k := range allowed {
		if _, ok := rawItems[0][k]; !ok {
			t.Errorf("items[0] 缺字段 %q（§8.1 钦定必有）", k)
		}
	}
}

// AC5.7 edge: service 返 0 行 → envelope code=0 + items=[] **非 null**
// （§8.1 行 1301 钦定 catalog 为空返 {items:[]} code=0 不报错；防 service
// 返 nil slice 或 handler catalogResponseDTO 兜底失效导致 wire 下发 null）。
func TestCosmeticsHandler_GetCatalog_EmptyList_ReturnsEmptyArrayNotNull(t *testing.T) {
	svc := &stubCosmeticService{
		listCatalogFn: func(ctx context.Context) ([]service.CosmeticBrief, error) {
			return []service.CosmeticBrief{}, nil
		},
	}
	r := buildCosmeticsHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/catalog", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body=%s", err, w.Body.String())
	}
	if body.Code != 0 {
		t.Errorf("body.code = %d, want 0 (空 catalog 非 error)", body.Code)
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
		t.Errorf("data.items = null, want [] (§8.1 行 1301 钦定)")
	}
	if got != "[]" {
		t.Errorf("data.items = %q, want []", got)
	}
}

// AC5.8 edge: service 返 1009 → handler c.Error 透传 → envelope code=1009
//
// ErrorMappingMiddleware 把 *AppError 1009 翻译到 HTTP 响应；本断言只锁
// envelope.code 业务码字段，与 emojis_handler_test 同模式。
func TestCosmeticsHandler_GetCatalog_ServiceError_Returns1009(t *testing.T) {
	svc := &stubCosmeticService{
		listCatalogFn: func(ctx context.Context) ([]service.CosmeticBrief, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := buildCosmeticsHandlerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/catalog", nil)
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
