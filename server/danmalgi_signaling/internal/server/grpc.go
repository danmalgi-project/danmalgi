package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/pkg/errors"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	cert_path := os.Getenv("TLS_CERT")
	key_path := os.Getenv("TLS_KEY")

	// Load server's certificate and private key
	serverCert, err := tls.LoadX509KeyPair(cert_path, key_path)
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
}

func NewGrpc(logger *slog.Logger, userUsecase usecase.UserUsecase) (*grpc.Server, error) {
	opt := []grpc.ServerOption{
		// grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		// 	MinTime:             15 * time.Second, // ping을 15초 보다 더 자주 보내면 연결을 끊음
		// 	PermitWithoutStream: true,             // stream이 없어도 Ping을 허용 (idle 연결 감시)
		// }),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			// MaxConnectionIdle: , // 커넥션이 x초 만큼 사용하지 않으면 끊음
			// MaxConnectionAge: , // 커넥션 life cycle
			// MaxConnectionAgeGrace: , // 커넥션 끊을 시 유예 시간 (x초 후에 끊음)
			Time:    1 * time.Minute,  // ping을 보내는 주기
			Timeout: 30 * time.Second, // pong timeout
		}),
		grpc.ChainUnaryInterceptor(
			// chat_store.v1.ChatStoreService/* 는 서비스 JWT, 그 외는 기존 사용자 JWT 로 분기.
			middleware.AuthDispatchUnaryInterceptor(
				middleware.AuthMiddleware(logger, userUsecase),
				middleware.ServiceAuthMiddleware(logger),
			),
			logging.UnaryServerInterceptor(middleware.LoggingMiddleware(logger)),
			// grpc.UnaryServerInterceptor(middleware.UnaryErrorMiddleware()),
			recovery.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			// chat_store.v1.ChatStoreService/* 는 서비스 JWT, 그 외는 기존 사용자 JWT 로 분기.
			middleware.AuthDispatchStreamInterceptor(
				middleware.AuthMiddleware(logger, userUsecase),
				middleware.ServiceAuthMiddleware(logger),
			),
			logging.StreamServerInterceptor(middleware.LoggingMiddleware(logger)),
			// grpc.StreamServerInterceptor(middleware.StreamErrorMiddleware()),
			recovery.StreamServerInterceptor(),
		),
	}

	tls := os.Getenv("TLS")
	isTls, err := strconv.ParseBool(tls)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if isTls {
		tlsCredentials, err := loadTLSCredentials()
		if err != nil {
			return nil, errors.WithStack(err)
		}

		opt = append(opt, grpc.Creds(tlsCredentials))
	}

	server := grpc.NewServer(opt...)

	reflection.Register(server)

	return server, nil
}

func RegisterHook(lc fx.Lifecycle, logger *slog.Logger, server *grpc.Server) {
	healthServer := &http.Server{}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			port := fmt.Sprintf(":%s", os.Getenv("SERVER_PORT"))
			healthPort := fmt.Sprintf(":%s", os.Getenv("HEALTH_PORT"))

			// 리스너를 동기적으로 바인딩하여 포트 에러를 기동 단계에서 노출
			lis, err := net.Listen("tcp", port)
			if err != nil {
				return errors.WithStack(err)
			}

			healthLis, err := net.Listen("tcp", healthPort)
			if err != nil {
				lis.Close()
				return errors.WithStack(err)
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("success"))
			})
			healthServer.Handler = mux

			// grpc server
			go func() {
				if err := server.Serve(lis); err != nil {
					logger.Error("grpc server stopped", "error", err)
				}
			}()

			// health check server
			go func() {
				if err := healthServer.Serve(healthLis); err != nil && err != http.ErrServerClosed {
					logger.Error("health server stopped", "error", err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			server.GracefulStop()
			return healthServer.Shutdown(ctx)
		},
	})
}
