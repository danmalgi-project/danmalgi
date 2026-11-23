package redis

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

// fakeRedisTickInterval은 miniredis의 논리 시계를 실제 시간에 맞춰 전진시키는 주기다.
// miniredis는 벽시계로 키를 만료시키지 않고 FastForward 호출로만 TTL이 흐르므로,
// 이 틱이 없으면 presence 해시(channel:%d:voice)의 2분 TTL이 영원히 만료되지 않아
// 실제 Redis와 동작이 갈린다.
const fakeRedisTickInterval = time.Second

// NewFakeRedis는 인프로세스 miniredis를 띄우고 거기에 연결된 클라이언트를 돌려준다.
// standalone 모드 전용 대체 구현으로, 외부 Redis 없이 로컬 E2E 검증을 가능하게 한다.
// 반환 타입이 NewRedis와 동일하므로 소비자(UserService/SignalingService)는 손댈 필요가 없다.
func NewFakeRedis(lc fx.Lifecycle, logger *slog.Logger) (*redis.Client, error) {
	server, err := miniredis.Run()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(fakeRedisTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				server.FastForward(fakeRedisTickInterval)
			case <-done:
				return
			}
		}
	}()

	logger.Info("standalone: 인프로세스 miniredis 기동", "addr", server.Addr())

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// 틱 goroutine을 먼저 완전히 세운 뒤 서버를 닫는다.
			// 순서가 바뀌면 FastForward가 이미 닫힌 서버를 건드릴 수 있다.
			close(done)
			wg.Wait()

			err := client.Close()
			server.Close()
			return err
		},
	})

	return client, nil
}

// FakeModule은 Module을 대체해 인프로세스 miniredis를 제공한다(standalone 모드).
var FakeModule fx.Option = fx.Options(
	fx.Provide(NewFakeRedis),
)
