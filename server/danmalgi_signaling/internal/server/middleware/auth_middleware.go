package middleware

import (
	"context"
	"log/slog"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase"
	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/token"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UID struct{}
type DID struct{}
type USER struct{}
type TOKEN struct{}

func AuthMiddleware(logger *slog.Logger, userReader usecase.UserUsecase) func(ctx context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		authToken, err := auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			logger.Error("auth token not found", "error", err)
			return nil, status.Error(codes.Unauthenticated, "does not exist auth token")
		}
		ctx = context.WithValue(ctx, TOKEN{}, authToken)

		userID, deviceID, err := token.Validate(authToken)
		if err != nil {
			logger.Error("invalid auth token", "error", err)
			return nil, status.Errorf(codes.Unauthenticated, "invalid auth token: %v", err)
		}

		user, err := userReader.GetUserByID(ctx, *userID)
		if err != nil {
			logger.Error("failed to get user", "error", err)
			return nil, err
		}

		ctx = context.WithValue(ctx, UID{}, *userID)
		ctx = context.WithValue(ctx, DID{}, *deviceID)
		ctx = context.WithValue(ctx, USER{}, *user)
		return ctx, nil
	}
}
