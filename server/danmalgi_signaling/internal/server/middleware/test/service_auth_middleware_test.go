package test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testServiceSecret = "test-service-secret"

func makeServiceToken(service string, secret string) string {
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"service": service,
		"iat":     time.Now().Unix(),
	}).SignedString([]byte(secret))
	return tok
}

func TestServiceAuthMiddleware_ValidToken(t *testing.T) {
	os.Setenv("SERVICE_JWT_SECRET", testServiceSecret)
	defer os.Unsetenv("SERVICE_JWT_SECRET")

	fn := middleware.ServiceAuthMiddleware(slog.Default())
	ctx := ctxWithBearer(makeServiceToken("danmalgi_backend", testServiceSecret))

	resultCtx, err := fn(ctx)
	require.NoError(t, err)
	assert.NotNil(t, resultCtx)
}

func TestServiceAuthMiddleware_NoToken(t *testing.T) {
	fn := middleware.ServiceAuthMiddleware(slog.Default())

	_, err := fn(context.Background())
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestServiceAuthMiddleware_WrongSignature(t *testing.T) {
	os.Setenv("SERVICE_JWT_SECRET", testServiceSecret)
	defer os.Unsetenv("SERVICE_JWT_SECRET")

	fn := middleware.ServiceAuthMiddleware(slog.Default())
	// 다른 secret 으로 서명 → 서명 불일치
	ctx := ctxWithBearer(makeServiceToken("danmalgi_backend", "another-secret"))

	_, err := fn(ctx)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestServiceAuthMiddleware_MissingServiceClaim(t *testing.T) {
	os.Setenv("SERVICE_JWT_SECRET", testServiceSecret)
	defer os.Unsetenv("SERVICE_JWT_SECRET")

	// service / sub claim 이 없는 토큰
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": time.Now().Unix(),
	}).SignedString([]byte(testServiceSecret))

	fn := middleware.ServiceAuthMiddleware(slog.Default())
	ctx := ctxWithBearer(tok)

	_, err := fn(ctx)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}
