package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	user_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/user_store/v1"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type UserService struct {
	redis              *redis.Client
	internalUserClient user_storev1.UserStoreServiceClient
}

func NewUserService(
	redis *redis.Client,
	internalUserClient user_storev1.UserStoreServiceClient,
) *UserService {
	return &UserService{
		redis:              redis,
		internalUserClient: internalUserClient,
	}
}

func (u *UserService) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	user := &domain.User{}
	err := u.redis.Get(ctx, fmt.Sprintf("user:%d", userID)).Scan(user)
	if err == nil {
		return user, nil
	}
	if err != redis.Nil {
		return nil, errors.WithStack(err)
	}

	response, err := u.internalUserClient.GetUsers(ctx, &user_storev1.GetUsersRequest{UserIds: []int64{userID}})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	proto := response.Users[0]
	return &domain.User{
		ID:              proto.Id,
		Email:           proto.Email,
		Name:            proto.Name,
		Tag:             proto.Tag,
		Status:          int(proto.Status),
		ProfileImageUrl: proto.ProfileImageUrl,
	}, nil
}

// GetUsersByIDs는 Redis 캐시에서 유저 정보를 조회하고, 캐시 미스 시 내부 API에서 가져온다.
func (u *UserService) GetUsersByIDs(ctx context.Context, userIDs []string) (map[string]*domain.User, error) {
	userMap := make(map[string]*domain.User)
	missedIDs := make([]int64, 0)

	values, err := u.redis.MGet(ctx, userIDs...).Result()
	if err != nil {
		return nil, err
	}

	for i, v := range values {
		if v == nil {
			id, _ := strconv.ParseInt(userIDs[i], 10, 64)
			missedIDs = append(missedIDs, id)
			continue
		}

		user := &domain.User{}
		if err := user.UnmarshalBinary([]byte(v.(string))); err != nil {
			id, _ := strconv.ParseInt(userIDs[i], 10, 64)
			missedIDs = append(missedIDs, id)
			continue
		}
		userMap[userIDs[i]] = user
	}

	if len(missedIDs) == 0 {
		return userMap, nil
	}

	response, err := u.internalUserClient.GetUsers(ctx, &user_storev1.GetUsersRequest{UserIds: missedIDs})
	if err != nil {
		return nil, err
	}

	for _, proto := range response.Users {
		userMap[fmt.Sprintf("%d", proto.Id)] = &domain.User{
			ID:              proto.Id,
			Email:           proto.Email,
			Name:            proto.Name,
			Tag:             proto.Tag,
			Status:          int(proto.Status),
			ProfileImageUrl: proto.ProfileImageUrl,
		}
	}

	return userMap, nil
}
