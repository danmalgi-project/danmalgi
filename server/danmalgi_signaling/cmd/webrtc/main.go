package main

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/client"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/handler"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/manager"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/redis"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/signaling"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/log"
	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/rootdir"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

func main() {
	standaloneFlag := flag.Bool(
		"standalone",
		false,
		"외부 Redis·내부 API 서버 없이 인프로세스 fake로 기동한다 (로컬 검증 전용). env STANDALONE=true와 동일.",
	)
	flag.Parse()

	root := rootdir.Get()
	godotenv.Load(filepath.Join(root, ".env"))

	// .env 로드 이후에 판정해야 .env의 STANDALONE도 반영된다. 플래그가 지정되면 항상 우선.
	standalone := *standaloneFlag
	if !standalone {
		standalone, _ = strconv.ParseBool(os.Getenv("STANDALONE"))
	}

	var defaulted []string
	if standalone {
		defaulted = applyStandaloneEnvDefaults()
	}

	fx.New(
		appOptions(root, standalone),
		fx.Invoke(func(logger *slog.Logger) {
			if standalone {
				logStandaloneBanner(logger, defaulted)
			}
		}),
	).Run()
}

// appOptions는 애플리케이션의 fx 옵션 그래프를 구성한다.
// main()과 배선 검증 테스트가 동일한 정의를 공유하도록 분리한다.
//
// standalone이 true면 외부 의존성(Redis, 내부 API gRPC)만 인프로세스 fake로 바꾼다.
// 나머지 그래프(핸들러·유즈케이스·시그널링)는 프로덕션과 완전히 동일하게 유지해,
// 검증 경로가 실제 동작에서 갈라지지 않게 한다.
func appOptions(root string, standalone bool) fx.Option {
	externalDeps := fx.Options(client.Module, redis.Module)
	if standalone {
		externalDeps = fx.Options(client.FakeModule, redis.FakeModule)
	}

	return fx.Options(
		fx.Provide(func() *slog.Logger {
			logDir := os.Getenv("LOG_DIR")
			if logDir == "" {
				logDir = filepath.Join(root, "logs")
			}
			return log.NewLog(logDir)
		}),
		server.Module,
		externalDeps,
		manager.Module,
		handler.Module,
		signaling.Module,
		usecase.Module,
		fx.Invoke(registerGrpcService),
		fx.Invoke(server.RegisterHook),
	)
}

func registerGrpcService(
	server *grpc.Server,
	signalingHandler *handler.SignalingHandler,
) {
	signalingv1.RegisterSignalingServiceServer(server, signalingHandler)
}
