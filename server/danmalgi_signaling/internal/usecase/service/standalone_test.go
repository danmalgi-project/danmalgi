package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	fakeredis "github.com/danmalgi-project/danmalgi-webrtc-server/internal/redis"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx/fxtest"
)

func newStandaloneRedis(t *testing.T) *goredis.Client {
	t.Helper()

	lc := fxtest.NewLifecycle(t)
	redisClient, err := fakeredis.NewFakeRedis(lc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewFakeRedis 실패: %v", err)
	}
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	return redisClient
}

// TestSignalingPresenceRoundTripInStandalone은 presence 저장/해제가
// 인프로세스 miniredis에서도 실제 Redis와 같은 키 구조로 동작하는지 확인한다.
// (SignalingService.PreSignaling/PostSignaling이 쓰는 channel:%d:voice 해시)
func TestSignalingPresenceRoundTripInStandalone(t *testing.T) {
	ctx := context.Background()
	redisClient := newStandaloneRedis(t)
	service := &SignalingService{redis: redisClient}

	const channelID = int64(7)

	if err := service.saveJoinedUser(ctx, 100, channelID, "web-100"); err != nil {
		t.Fatalf("presence 저장 실패: %v", err)
	}
	if err := service.saveJoinedUser(ctx, 200, channelID, "web-200"); err != nil {
		t.Fatalf("presence 저장 실패: %v", err)
	}

	// deviceId가 유니크하면 두 참가자가 각각 한 칸씩 차지해야 한다.
	presence, err := redisClient.HGetAll(ctx, "channel:7:voice").Result()
	if err != nil {
		t.Fatalf("presence 조회 실패: %v", err)
	}
	if len(presence) != 2 || presence["web-100"] != "100" || presence["web-200"] != "200" {
		t.Fatalf("presence가 참가자별로 분리되지 않았다: %#v", presence)
	}

	// 한쪽 종료가 상대 presence를 지우면 안 된다.
	if err := redisClient.HDel(ctx, "channel:7:voice", "web-100").Err(); err != nil {
		t.Fatalf("presence 해제 실패: %v", err)
	}
	presence, err = redisClient.HGetAll(ctx, "channel:7:voice").Result()
	if err != nil {
		t.Fatalf("presence 조회 실패: %v", err)
	}
	if len(presence) != 1 || presence["web-200"] != "200" {
		t.Errorf("한쪽 종료가 상대 presence까지 지웠다: %#v", presence)
	}
}
