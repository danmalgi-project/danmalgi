package redis

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/fx/fxtest"
)

func newTestFakeRedis(t *testing.T) *goredis.Client {
	t.Helper()

	lc := fxtest.NewLifecycle(t)
	client, err := NewFakeRedis(lc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewFakeRedis 실패: %v", err)
	}
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	return client
}

// TestFakeRedisSupportsServiceOperations는 서비스 계층이 실제로 쓰는 연산만 골라 검증한다.
// UserService: GET user:%d / MGET
// SignalingService: HSET+EXPIRE 파이프라인 / HDEL (channel:%d:voice)
func TestFakeRedisSupportsServiceOperations(t *testing.T) {
	ctx := context.Background()
	client := newTestFakeRedis(t)

	// UserService: 캐시 미스는 redis.Nil이어야 내부 API 폴백 경로를 탄다.
	if err := client.Get(ctx, "user:1").Err(); err != goredis.Nil {
		t.Errorf("캐시 미스가 redis.Nil이 아니다: %v", err)
	}

	values, err := client.MGet(ctx, "user:1", "user:2").Result()
	if err != nil {
		t.Fatalf("MGET 실패: %v", err)
	}
	if len(values) != 2 || values[0] != nil || values[1] != nil {
		t.Errorf("미스인 MGET 결과가 nil이 아니다: %#v", values)
	}

	// SignalingService: presence 등록.
	key := "channel:1:voice"
	if _, err := client.Pipelined(ctx, func(p goredis.Pipeliner) error {
		p.HSet(ctx, key, "web-100", int64(100))
		p.Expire(ctx, key, 2*time.Minute)
		return nil
	}); err != nil {
		t.Fatalf("presence 등록 파이프라인 실패: %v", err)
	}

	got, err := client.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatalf("HGETALL 실패: %v", err)
	}
	if got["web-100"] != "100" {
		t.Errorf("presence가 저장되지 않았다: %#v", got)
	}

	// SignalingService: presence 해제.
	if err := client.HDel(ctx, key, "web-100").Err(); err != nil {
		t.Fatalf("HDEL 실패: %v", err)
	}
	if n, err := client.HLen(ctx, key).Result(); err != nil || n != 0 {
		t.Errorf("presence가 삭제되지 않았다: n=%d err=%v", n, err)
	}
}

// TestFakeRedisTTLExpires는 TTL이 실제 시간에 따라 만료되는지 확인한다.
// miniredis는 FastForward 호출로만 논리 시계가 흐르므로, 틱 goroutine이 빠지면
// presence 해시가 영원히 남아 실제 Redis와 동작이 갈린다 — 그 회귀를 잡는 테스트다.
func TestFakeRedisTTLExpires(t *testing.T) {
	ctx := context.Background()
	client := newTestFakeRedis(t)

	key := "channel:2:voice"
	if err := client.HSet(ctx, key, "web-1", int64(1)).Err(); err != nil {
		t.Fatalf("HSET 실패: %v", err)
	}
	if err := client.Expire(ctx, key, time.Second).Err(); err != nil {
		t.Fatalf("EXPIRE 실패: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		exists, err := client.Exists(ctx, key).Result()
		if err != nil {
			t.Fatalf("EXISTS 실패: %v", err)
		}
		if exists == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Errorf("TTL이 지났는데도 키가 만료되지 않았다 (FastForward 틱이 도는지 확인)")
}
