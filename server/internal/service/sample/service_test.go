package sample_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/service/sample"
)

// MockSampleRepo 是 sample.SampleRepo 的 testify/mock 手写实装。
//
// 本包是 Story 1.5 交付的 service-层 mock 模板。未来 Epic 4+ 新增 service 时，
// 复制本文件 → 把 "Sample" / "FindByID" / DTO 类型统一 rename，即可得到新
// service 的 mock 骨架。
//
// ADR 0001-test-stack.md §3.4 明示：repo mock **手写**，不走 mockgen / mockery。
type MockSampleRepo struct {
	mock.Mock
}

func (m *MockSampleRepo) FindByID(ctx context.Context, id string) (*sample.SampleDTO, error) {
	args := m.Called(ctx, id)
	// 第一个返回值可能为 nil *SampleDTO；args.Get 拿到 interface{}(nil) 后
	// 用 comma-ok 做类型断言，避免 nil 强转 panic（见 story 1.5 Dev Notes §常见陷阱 #2）。
	dto, _ := args.Get(0).(*sample.SampleDTO)
	return dto, args.Error(1)
}

type testCtxKey struct{}

func TestSampleService_GetValue(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		ctxBuilder func() context.Context
		setup      func(*MockSampleRepo)
		wantValue  int
		wantErr    error  // 用 errors.Is 断言
		wantErrMsg string // 用字面量断言；与 wantErr 二选一
		notCalled  bool
	}{
		{
			name: "happy: 正常查询返回 value",
			id:   "abc",
			setup: func(m *MockSampleRepo) {
				m.On("FindByID", mock.Anything, "abc").
					Return(&sample.SampleDTO{ID: "abc", Value: 42}, nil).Once()
			},
			wantValue: 42,
		},
		{
			name: "edge: id 空串预校验失败，repo 不被调用",
			id:   "",
			setup: func(m *MockSampleRepo) {
				// 不设置任何期望 —— 如果 repo 被调用，AssertExpectations 会兜底失败；
				// 同时 notCalled=true 会显式断言 FindByID 未被调用。
			},
			wantValue: 0,
			wantErr:   sample.ErrSampleNotFound,
			notCalled: true,
		},
		{
			name: "edge: repo 返回错误原样向上传",
			id:   "x",
			setup: func(m *MockSampleRepo) {
				m.On("FindByID", mock.Anything, "x").
					Return(nil, errors.New("db down")).Once()
			},
			wantValue:  0,
			wantErrMsg: "db down",
		},
		{
			name: "happy: ctx 正确传递到 repo",
			id:   "abc",
			ctxBuilder: func() context.Context {
				return context.WithValue(context.Background(), testCtxKey{}, "propagated")
			},
			setup: func(m *MockSampleRepo) {
				m.On("FindByID",
					mock.MatchedBy(func(ctx context.Context) bool {
						return ctx != nil && ctx.Value(testCtxKey{}) == "propagated"
					}),
					"abc",
				).Return(&sample.SampleDTO{ID: "abc", Value: 7}, nil).Once()
			},
			wantValue: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &MockSampleRepo{}
			tc.setup(repo)

			svc := sample.NewSampleService(repo)

			ctx := context.Background()
			if tc.ctxBuilder != nil {
				ctx = tc.ctxBuilder()
			}

			got, err := svc.GetValue(ctx, tc.id)

			assert.Equal(t, tc.wantValue, got)
			switch {
			case tc.wantErr != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
			case tc.wantErrMsg != "":
				require.Error(t, err)
				assert.EqualError(t, err, tc.wantErrMsg)
			default:
				require.NoError(t, err)
			}

			if tc.notCalled {
				repo.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}
