// 이 파일만 외부 테스트 패키지(service_test)에 둔다.
// internal/client는 middleware -> usecase -> service로 이어지므로,
// package service 안에서 import하면 테스트 시 import cycle이 된다.
package service_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/client"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	fakeredis "github.com/danmalgi-project/danmalgi-webrtc-server/internal/redis"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase/service"
	"go.uber.org/fx/fxtest"
)

// TestUserServiceGetUserByIDInStandalone은 standalone 조합(인프로세스 miniredis + fake 내부 API)에서
// AuthMiddleware가 타는 경로가 실제로 통과하는지 검증한다.
//
// AuthMiddleware는 토큰의 userId로 GetUserByID를 호출하고, 실패하면 연결을 거부한다.
// standalone에서는 Redis에 user:%d 캐시가 절대 채워지지 않으므로(서버는 이 키를 쓰지 않는다)
// 항상 캐시 미스 -> 내부 API 폴백 경로를 탄다. 그 폴백이 유저를 돌려줘야 연결이 성립한다.
func TestUserServiceGetUserByIDInStandalone(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	redisClient, err := fakeredis.NewFakeRedis(lc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewFakeRedis 실패: %v", err)
	}
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	userService := service.NewUserService(redisClient, client.NewFakeUserStoreClient())

	// 어댑터는 연결마다 유니크한 userId를 발급하므로, 임의의 userId가 모두 통과해야 한다.
	for _, userID := range []int64{1, 2, 999} {
		user, err := userService.GetUserByID(context.Background(), userID)
		if err != nil {
			t.Fatalf("userId=%d 조회 실패 — standalone에서 연결이 거부된다: %v", userID, err)
		}
		if user.ID != userID {
			t.Errorf("userId=%d 조회 결과의 ID가 다르다: got %d", userID, user.ID)
		}
		if user.Name == "" {
			t.Errorf("userId=%d 조회 결과에 표시 이름이 없다", userID)
		}
	}
}

// TestUserServiceGetUserByIDCacheHit은 api 서버가 쓰는 실제 user:<id> 캐시 포맷을 리터럴로 넣고,
// 캐시 히트 경로가 그 값을 그대로 domain.User로 읽는지 검증한다.
// fake 내부 API와 구별되는 값을 넣어 반환값이 캐시에서 왔음을 증명한다.
func TestUserServiceGetUserByIDCacheHit(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	redisClient, err := fakeredis.NewFakeRedis(lc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewFakeRedis 실패: %v", err)
	}
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	ctx := context.Background()
	cached := `{"@class":"com.danmalgi.backend.user.domain.model.User","id":5,"email":"cached@example.com","name":"CachedUser","tag":"0001","deviceId":"device-1","status":1,"profileImageUrl":"https://cdn.example.com/cached.webp"}`
	if err := redisClient.Set(ctx, "user:5", cached, 0).Err(); err != nil {
		t.Fatalf("캐시 세팅 실패: %v", err)
	}

	userService := service.NewUserService(redisClient, client.NewFakeUserStoreClient())

	user, err := userService.GetUserByID(ctx, 5)
	if err != nil {
		t.Fatalf("GetUserByID 실패: %v", err)
	}

	want := domain.User{
		ID:              5,
		Email:           "cached@example.com",
		Name:            "CachedUser",
		Tag:             "0001",
		DeviceId:        "device-1",
		Status:          1,
		ProfileImageUrl: "https://cdn.example.com/cached.webp",
	}
	if *user != want {
		t.Errorf("got %+v, want %+v", *user, want)
	}
}
