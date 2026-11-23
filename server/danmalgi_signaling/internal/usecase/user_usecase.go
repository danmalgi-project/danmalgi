package usecase

import (
	"context"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
)

type UserUsecase interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	GetUsersByIDs(ctx context.Context, userIDs []string) (map[string]*domain.User, error)
}
