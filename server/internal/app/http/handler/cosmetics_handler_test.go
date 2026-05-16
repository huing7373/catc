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
	// Story 23.4 加：GetInventory 路径 stub（否则 stub 不满足扩展后的
	// CosmeticService interface → 既有 GetCatalog 测试编译红）。
	listInventoryFn func(ctx context.Context, userID uint64) ([]service.InventoryGroup, error)
}

func (s *stubCosmeticService) ListCatalog(ctx context.Context) ([]service.CosmeticBrief, error) {
	return s.listCatalogFn(ctx)
}

func (s *stubCosmeticService) ListInventory(ctx context.Context, userID uint64) ([]service.InventoryGroup, error) {
	if s.listInventoryFn == nil {
		panic("stubCosmeticService.ListInventory not configured (本 case 走 GetCatalog 路径，不期望走 GET /cosmetics/inventory)")
	}
	return s.listInventoryFn(ctx, userID)
}

// buildCosmeticsInventoryHandlerRouter 构造 GetInventory test router。
//
// 与 newChestHandlerRouter 同模式：挂 ErrorMappingMiddleware（否则 c.Error 不写
// envelope）+ 可选注入 userID 到 c.Keys（mockUserID=nil 不挂 → 测 unreachable
// userID 缺失分支）。GetInventory **读** userID（与 GetCatalog 不同），故需
// userID 注入模式（参照 chest_handler_test.go 行 59-75）。
func buildCosmeticsInventoryHandlerRouter(svc service.CosmeticService, mockUserID *uint64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	if mockUserID != nil {
		uid := *mockUserID
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, uid)
			c.Next()
		})
	}
	h := handler.NewCosmeticsHandler(svc)
	r.GET("/api/v1/cosmetics/inventory", h.GetInventory)
	return r
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

// ============================================================================
// Story 23.4 — CosmeticsHandler.GetInventory 单测（≥4 case stub service）
// ============================================================================

// AC7.9 happy: service 返 N 组（含 instances）→ envelope code=0 + groups 长度 N
// + cosmeticItemId / userCosmeticItemId 是 **string** + slot/rarity/status/count
// 是 **number** + 全 camelCase + **无 code 字段** + HTTP 200。
func TestCosmeticsHandler_GetInventory_HappyPath(t *testing.T) {
	uid := uint64(42)
	svc := &stubCosmeticService{
		listInventoryFn: func(ctx context.Context, userID uint64) ([]service.InventoryGroup, error) {
			if userID != 42 {
				t.Errorf("service 收到 userID = %d, want 42 (从 c.Get(UserIDKey) 取)", userID)
			}
			return []service.InventoryGroup{
				{
					CosmeticItemID: 12, Name: "小黄帽", Slot: 1, Rarity: 1,
					IconURL: "https://x/i12", AssetURL: "https://x/a12", Count: 2,
					Instances: []service.InventoryInstance{
						{UserCosmeticItemID: 90001, Status: 1},
						{UserCosmeticItemID: 90008, Status: 2},
					},
				},
				{
					CosmeticItemID: 24, Name: "星星围巾", Slot: 4, Rarity: 2,
					IconURL: "https://x/i24", AssetURL: "https://x/a24", Count: 1,
					Instances: []service.InventoryInstance{
						{UserCosmeticItemID: 90010, Status: 1},
					},
				},
			}, nil
		},
	}
	r := buildCosmeticsInventoryHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/inventory", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	bodyBytes := w.Body.Bytes()
	var body struct {
		Code int `json:"code"`
		Data struct {
			Groups []struct {
				CosmeticItemID string `json:"cosmeticItemId"`
				Name           string `json:"name"`
				Slot           int    `json:"slot"`
				Rarity         int    `json:"rarity"`
				IconURL        string `json:"iconUrl"`
				AssetURL       string `json:"assetUrl"`
				Count          int    `json:"count"`
				Instances      []struct {
					UserCosmeticItemID string `json:"userCosmeticItemId"`
					Status             int    `json:"status"`
				} `json:"instances"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body=%s", err, string(bodyBytes))
	}
	if body.Code != 0 {
		t.Errorf("body.code = %d, want 0", body.Code)
	}
	if len(body.Data.Groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(body.Data.Groups))
	}
	g0 := body.Data.Groups[0]
	if g0.CosmeticItemID != "12" || g0.Name != "小黄帽" || g0.Slot != 1 || g0.Rarity != 1 ||
		g0.IconURL != "https://x/i12" || g0.AssetURL != "https://x/a12" || g0.Count != 2 {
		t.Errorf("groups[0] = %+v, want cosmeticItemId=\"12\" 小黄帽 slot=1 rarity=1 count=2", g0)
	}
	if len(g0.Instances) != 2 || g0.Instances[0].UserCosmeticItemID != "90001" || g0.Instances[0].Status != 1 ||
		g0.Instances[1].UserCosmeticItemID != "90008" || g0.Instances[1].Status != 2 {
		t.Errorf("groups[0].instances = %+v, want [{90001,1},{90008,2}]", g0.Instances)
	}

	// raw JSON 断言：cosmeticItemId / userCosmeticItemId 是 string（带引号）+
	// slot/rarity/status/count 是 number（无引号）+ 字段集严格 = §8.2 钦定（无
	// code 字段，全 camelCase）。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("raw json: %v", err)
	}
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &rawData); err != nil {
		t.Fatalf("raw data: %v", err)
	}
	var rawGroups []map[string]json.RawMessage
	if err := json.Unmarshal(rawData["groups"], &rawGroups); err != nil {
		t.Fatalf("raw groups: %v", err)
	}
	if len(rawGroups) != 2 {
		t.Fatalf("raw groups len = %d, want 2", len(rawGroups))
	}
	if got := string(rawGroups[0]["cosmeticItemId"]); got != `"12"` {
		t.Errorf("rawGroups[0].cosmeticItemId = %s, want \"12\" (string，§8.2 行 1368 BIGINT 字符串化)", got)
	}
	if got := string(rawGroups[0]["slot"]); got != "1" {
		t.Errorf("rawGroups[0].slot = %s, want 1 (int，§8.2 行 1370 不字符串化)", got)
	}
	if got := string(rawGroups[0]["count"]); got != "2" {
		t.Errorf("rawGroups[0].count = %s, want 2 (int，§8.2 行 1374)", got)
	}
	var rawInst []map[string]json.RawMessage
	if err := json.Unmarshal(rawGroups[0]["instances"], &rawInst); err != nil {
		t.Fatalf("raw instances: %v", err)
	}
	if got := string(rawInst[0]["userCosmeticItemId"]); got != `"90001"` {
		t.Errorf("rawInst[0].userCosmeticItemId = %s, want \"90001\" (string，§8.2 行 1376)", got)
	}
	if got := string(rawInst[0]["status"]); got != "1" {
		t.Errorf("rawInst[0].status = %s, want 1 (int，§8.2 行 1377)", got)
	}
	// 字段集严格 = §8.2 钦定 8 字段（无 code / 无多余 / 全 camelCase）
	allowed := map[string]bool{
		"cosmeticItemId": true, "name": true, "slot": true, "rarity": true,
		"iconUrl": true, "assetUrl": true, "count": true, "instances": true,
	}
	for k := range rawGroups[0] {
		if !allowed[k] {
			t.Errorf("groups[0] 含 §8.2 未声明字段 %q（**不应**下发 code / snake_case；§8.2 groups[] 无 code，与 §8.1 catalog 不同）", k)
		}
	}
	for k := range allowed {
		if _, ok := rawGroups[0][k]; !ok {
			t.Errorf("groups[0] 缺字段 %q（§8.2 钦定必有）", k)
		}
	}
}

// AC7.10 edge 空背包: service 返 []InventoryGroup{} → data.groups 是 `[]`
// 而非 `null`（§8.2 行 1440 防 Swift Codable 解析 nil）。
func TestCosmeticsHandler_GetInventory_EmptyBag_ReturnsEmptyArrayNotNull(t *testing.T) {
	uid := uint64(42)
	svc := &stubCosmeticService{
		listInventoryFn: func(ctx context.Context, userID uint64) ([]service.InventoryGroup, error) {
			return []service.InventoryGroup{}, nil
		},
	}
	r := buildCosmeticsInventoryHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/inventory", nil)
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
		t.Errorf("body.code = %d, want 0 (空背包非 error，§8.2 行 1432)", body.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("raw: %v", err)
	}
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(raw["data"], &rawData); err != nil {
		t.Fatalf("data: %v", err)
	}
	got := string(rawData["groups"])
	if got == "null" {
		t.Errorf("data.groups = null, want [] (§8.2 行 1440 钦定)")
	}
	if got != "[]" {
		t.Errorf("data.groups = %q, want []", got)
	}
}

// AC7.11 edge 缺 userID in context → 1009 unreachable 兜底
// （参照 chest_handler_test 缺 userID case；不注入 userID router）。
func TestCosmeticsHandler_GetInventory_MissingUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubCosmeticService{
		listInventoryFn: func(ctx context.Context, userID uint64) ([]service.InventoryGroup, error) {
			t.Fatal("service 不应被调用（userID 缺失应在 handler 早返 1009）")
			return nil, nil
		},
	}
	r := buildCosmeticsInventoryHandlerRouter(svc, nil) // 不注入 userID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/inventory", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body=%s", err, w.Body.String())
	}
	if body.Code != apperror.ErrServiceBusy {
		t.Errorf("body.code = %d, want %d (1009 unreachable 兜底)", body.Code, apperror.ErrServiceBusy)
	}
}

// AC7.12 edge service 返 1009 → handler c.Error 透传 → envelope code=1009。
func TestCosmeticsHandler_GetInventory_ServiceError_Returns1009(t *testing.T) {
	uid := uint64(42)
	svc := &stubCosmeticService{
		listInventoryFn: func(ctx context.Context, userID uint64) ([]service.InventoryGroup, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := buildCosmeticsInventoryHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cosmetics/inventory", nil)
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
