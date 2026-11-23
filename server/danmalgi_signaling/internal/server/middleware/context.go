package middleware

import (
	"context"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/pkg/errors"
)

func GetUserID(ctx context.Context) (int64, error) {
	userID, ok := ctx.Value(UID{}).(int64)
	if !ok {
		return 0, domain.NewErrInvalidToken(errors.WithStack(domain.ErrNotFindIdInToken))
	}
	return userID, nil
}

func GetDeviceID(ctx context.Context) (string, error) {
	deviceID, ok := ctx.Value(DID{}).(string)
	if !ok {
		return "", domain.NewErrInvalidToken(errors.WithStack(domain.ErrNotFindIdInToken))
	}
	return deviceID, nil
}

func GetUser(ctx context.Context) (*domain.User, error) {
	user, ok := ctx.Value(USER{}).(domain.User)
	if !ok {
		return nil, domain.NewErrInvalidToken(errors.WithStack(domain.ErrNotFindIdInToken))
	}
	return &user, nil
}

func GetToken(ctx context.Context) (string, bool) {
	tok, ok := ctx.Value(TOKEN{}).(string)
	return tok, ok
}
