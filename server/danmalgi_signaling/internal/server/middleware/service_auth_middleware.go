package middleware

import (
	"context"
	"log/slog"

	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/token"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ServiceAuthMiddleware 는 서버 간(private) 호출에 실린 서비스 JWT 를 검증하는 AuthFunc 이다.
// 사용자 조회/컨텍스트 주입은 하지 않는다(서비스 호출에는 사용자 레코드가 없다).
func ServiceAuthMiddleware(logger *slog.Logger) func(ctx context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		authToken, err := auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			logger.Error("service auth token not found", "error", err)
			return nil, status.Error(codes.Unauthenticated, "does not exist service auth token")
		}

		if _, err := token.ValidateService(authToken); err != nil {
			logger.Error("invalid service auth token", "error", err)
			return nil, status.Errorf(codes.Unauthenticated, "invalid service auth token: %v", err)
		}

		return ctx, nil
	}
}
