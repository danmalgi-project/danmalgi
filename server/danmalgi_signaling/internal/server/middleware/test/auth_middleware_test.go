package test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase/mocks"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testJWTSecret = "test-secret"

func makeToken(userID int64, deviceID string) string {
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":      "danmalgi",
		"userId":   float64(userID),
		"deviceId": deviceID,
		"iat":      time.Now().Unix(),
	}).SignedString([]byte(testJWTSecret))
	return tok
}

func ctxWithBearer(token string) context.Context {
	md := metadata.Pairs("authorization", "bearer "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelError})
	return slog.New(handler), buf
}

func containsLogMessage(buf *bytes.Buffer, message string) bool {
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if msg, ok := entry["msg"].(string); ok && msg == message {
			return true
		}
	}
	return false
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	userID := int64(42)
	deviceID := "device-abc"
	expectedUser := &domain.User{ID: userID, Name: "testuser"}

	mockUsecase := mocks.NewMockUserUsecase(t)
	mockUsecase.EXPECT().
		GetUserByID(mock.Anything, userID).
		Return(expectedUser, nil)

	fn := middleware.AuthMiddleware(slog.Default(), mockUsecase)
	ctx := ctxWithBearer(makeToken(userID, deviceID))

	resultCtx, err := fn(ctx)

	require.NoError(t, err)

	uid, ok := resultCtx.Value(middleware.UID{}).(int64)
	assert.True(t, ok)
	assert.Equal(t, userID, uid)

	did, ok := resultCtx.Value(middleware.DID{}).(string)
	assert.True(t, ok)
	assert.Equal(t, deviceID, did)

	user, ok := resultCtx.Value(middleware.USER{}).(domain.User)
	assert.True(t, ok)
	assert.Equal(t, *expectedUser, user)
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	mockUsecase := mocks.NewMockUserUsecase(t)
	fn := middleware.AuthMiddleware(slog.Default(), mockUsecase)

	_, err := fn(context.Background())

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	mockUsecase := mocks.NewMockUserUsecase(t)
	fn := middleware.AuthMiddleware(slog.Default(), mockUsecase)

	ctx := ctxWithBearer("invalid.token.value")

	_, err := fn(ctx)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestAuthMiddleware_GetUserByIDFails(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	userID := int64(42)
	deviceID := "device-abc"
	dbErr := errors.New("db connection failed")

	mockUsecase := mocks.NewMockUserUsecase(t)
	mockUsecase.EXPECT().
		GetUserByID(mock.Anything, userID).
		Return(nil, dbErr)

	fn := middleware.AuthMiddleware(slog.Default(), mockUsecase)
	ctx := ctxWithBearer(makeToken(userID, deviceID))

	_, err := fn(ctx)

	require.Error(t, err)
	assert.Equal(t, dbErr, err)
}

func TestAuthMiddleware_StoresRawToken(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	userID := int64(42)
	deviceID := "device-abc"
	expectedUser := &domain.User{ID: userID, Name: "testuser"}
	rawToken := makeToken(userID, deviceID)

	mockUsecase := mocks.NewMockUserUsecase(t)
	mockUsecase.EXPECT().
		GetUserByID(mock.Anything, userID).
		Return(expectedUser, nil)

	fn := middleware.AuthMiddleware(slog.Default(), mockUsecase)
	ctx := ctxWithBearer(rawToken)

	resultCtx, err := fn(ctx)

	require.NoError(t, err)

	tok, ok := middleware.GetToken(resultCtx)
	assert.True(t, ok)
	assert.Equal(t, rawToken, tok)
}

func TestAuthMiddleware_NoToken_LogsError(t *testing.T) {
	logger, buf := newTestLogger()
	mockUsecase := mocks.NewMockUserUsecase(t)
	fn := middleware.AuthMiddleware(logger, mockUsecase)

	_, _ = fn(context.Background())

	assert.True(t, containsLogMessage(buf, "auth token not found"))
}

func TestAuthMiddleware_InvalidToken_LogsError(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	logger, buf := newTestLogger()
	mockUsecase := mocks.NewMockUserUsecase(t)
	fn := middleware.AuthMiddleware(logger, mockUsecase)

	ctx := ctxWithBearer("invalid.token.value")
	_, _ = fn(ctx)

	assert.True(t, containsLogMessage(buf, "invalid auth token"))
}

func TestAuthMiddleware_GetUserByIDFails_LogsError(t *testing.T) {
	os.Setenv("JWT_SECRET", testJWTSecret)
	defer os.Unsetenv("JWT_SECRET")

	userID := int64(42)
	deviceID := "device-abc"
	dbErr := errors.New("db connection failed")

	logger, buf := newTestLogger()
	mockUsecase := mocks.NewMockUserUsecase(t)
	mockUsecase.EXPECT().
		GetUserByID(mock.Anything, userID).
		Return(nil, dbErr)

	fn := middleware.AuthMiddleware(logger, mockUsecase)
	ctx := ctxWithBearer(makeToken(userID, deviceID))
	_, _ = fn(ctx)

	assert.True(t, containsLogMessage(buf, "failed to get user"))
}
