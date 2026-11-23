package middleware

import (
	"context"
	"strings"

	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"google.golang.org/grpc"
)

// ChatStoreServiceMethodPrefix 는 서비스 JWT 인증을 적용할 메서드의 FullMethod prefix 이다.
// 이 prefix 로 시작하는 메서드만 서비스 인증 경로로, 그 외는 기존 사용자 JWT 경로로 라우팅한다.
const ChatStoreServiceMethodPrefix = "/chat_store.v1.ChatStoreService/"

type authFunc = func(ctx context.Context) (context.Context, error)

// AuthDispatchUnaryInterceptor 는 FullMethod 를 기준으로 인증 함수를 분기하는 UnaryServerInterceptor 이다.
//   - chat_store.v1.ChatStoreService/* → serviceFn(서비스 JWT)
//   - 그 외 전부 → userFn(기존 사용자 JWT)
func AuthDispatchUnaryInterceptor(userFn, serviceFn authFunc) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		fn := userFn
		if strings.HasPrefix(info.FullMethod, ChatStoreServiceMethodPrefix) {
			fn = serviceFn
		}

		newCtx, err := fn(ctx)
		if err != nil {
			return nil, err
		}

		return handler(newCtx, req)
	}
}

// AuthDispatchStreamInterceptor 는 FullMethod 를 기준으로 인증 함수를 분기하는 StreamServerInterceptor 이다.
//   - chat_store.v1.ChatStoreService/* → serviceFn(서비스 JWT)
//   - 그 외 전부 → userFn(기존 사용자 JWT)
func AuthDispatchStreamInterceptor(userFn, serviceFn authFunc) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		fn := userFn
		if strings.HasPrefix(info.FullMethod, ChatStoreServiceMethodPrefix) {
			fn = serviceFn
		}

		newCtx, err := fn(stream.Context())
		if err != nil {
			return err
		}

		wrapped := grpcmiddleware.WrapServerStream(stream)
		wrapped.WrappedContext = newCtx
		return handler(srv, wrapped)
	}
}
