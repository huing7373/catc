package dto

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
)

// TestSignInWithAppleRequest_Validator drives go-playground/validator via
// Gin's Binding mechanism so the test exercises exactly the path the
// handler takes (binding:"required,uuid,oneof=...,min,max").
func TestSignInWithAppleRequest_Validator(t *testing.T) {
	t.Parallel()

	dev := uuid.NewString()

	cases := []struct {
		name    string
		body    SignInWithAppleRequest
		wantErr bool
	}{
		{
			name: "happy path",
			body: SignInWithAppleRequest{
				IdentityToken: "header.payload.sig",
				DeviceID:      dev,
				Platform:      "watch",
				Nonce:         "12345678",
			},
			wantErr: false,
		},
		{
			name: "missing identityToken",
			body: SignInWithAppleRequest{
				DeviceID: dev, Platform: "watch", Nonce: "12345678",
			},
			wantErr: true,
		},
		{
			name: "identityToken oversized (>8192)",
			body: SignInWithAppleRequest{
				IdentityToken: strings.Repeat("a", 8193),
				DeviceID:      dev, Platform: "watch", Nonce: "12345678",
			},
			wantErr: true,
		},
		{
			name: "deviceId not uuid",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: "not-a-uuid", Platform: "watch", Nonce: "12345678",
			},
			wantErr: true,
		},
		{
			name: "deviceId empty",
			body: SignInWithAppleRequest{
				IdentityToken: "x", Platform: "watch", Nonce: "12345678",
			},
			wantErr: true,
		},
		{
			name: "platform not in enum",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "android", Nonce: "12345678",
			},
			wantErr: true,
		},
		{
			name: "platform iphone is valid",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "iphone", Nonce: "12345678",
			},
			wantErr: false,
		},
		{
			name: "nonce too short",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "watch", Nonce: "short",
			},
			wantErr: true,
		},
		{
			name: "nonce too long",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "watch", Nonce: strings.Repeat("n", 129),
			},
			wantErr: true,
		},
		{
			name: "authorizationCode optional and bounded",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "watch", Nonce: "12345678",
				AuthorizationCode: strings.Repeat("c", 1024),
			},
			wantErr: false,
		},
		{
			name: "authorizationCode oversized",
			body: SignInWithAppleRequest{
				IdentityToken: "x", DeviceID: dev, Platform: "watch", Nonce: "12345678",
				AuthorizationCode: strings.Repeat("c", 1025),
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := binding.Validator.ValidateStruct(&tc.body)
			if tc.wantErr {
				require.Error(t, err, "expected validator to reject %#v", tc.body)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRefreshTokenRequest_Validator exercises the same binding path the
// handler uses.
func TestRefreshTokenRequest_Validator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    RefreshTokenRequest
		wantErr bool
	}{
		{name: "empty", body: RefreshTokenRequest{}, wantErr: true},
		{name: "too short (<16)", body: RefreshTokenRequest{RefreshToken: "header.p.s"}, wantErr: true},
		{name: "oversized (>8192)", body: RefreshTokenRequest{RefreshToken: strings.Repeat("a", 8193)}, wantErr: true},
		{
			name: "legal jwt-shaped value",
			body: RefreshTokenRequest{RefreshToken: strings.Repeat("a", 128) + "." + strings.Repeat("b", 256) + "." + strings.Repeat("c", 256)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := binding.Validator.ValidateStruct(&tc.body)
			if tc.wantErr {
				require.Error(t, err, "expected validator to reject %#v", tc.body)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserPublicFromDomain_OmitsNilFields(t *testing.T) {
	t.Parallel()
	pub := UserPublicFromDomain(&domain.User{ID: "u1"})
	assert.Equal(t, "u1", pub.ID)
	assert.Nil(t, pub.DisplayName)
	assert.Nil(t, pub.Timezone)
}

func TestUserPublicFromDomain_PassesThroughOptionalFields(t *testing.T) {
	t.Parallel()
	dn := "kuachan"
	tz := "Asia/Shanghai"
	pub := UserPublicFromDomain(&domain.User{ID: "u2", DisplayName: &dn, Timezone: &tz})
	assert.Equal(t, "u2", pub.ID)
	require.NotNil(t, pub.DisplayName)
	assert.Equal(t, "kuachan", *pub.DisplayName)
	require.NotNil(t, pub.Timezone)
	assert.Equal(t, "Asia/Shanghai", *pub.Timezone)
}
