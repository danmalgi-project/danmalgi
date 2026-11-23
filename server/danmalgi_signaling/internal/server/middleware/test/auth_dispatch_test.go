package test

import (
	"context"
	"testing"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestAuthDispatch_RoutesChatStoreToServiceFn(t *testing.T) {
	var userCalled, serviceCalled bool

	userFn := func(ctx context.Context) (context.Context, error) {
		userCalled = true
		return ctx, nil
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		serviceCalled = true
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchUnaryInterceptor(userFn, serviceFn)

	handlerCalled := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/chat_store.v1.ChatStoreService/GetLastMessages"}
	resp, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, serviceCalled)
	assert.False(t, userCalled)
	assert.True(t, handlerCalled)
}

func TestAuthDispatch_RoutesOtherMethodsToUserFn(t *testing.T) {
	var userCalled, serviceCalled bool

	userFn := func(ctx context.Context) (context.Context, error) {
		userCalled = true
		return ctx, nil
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		serviceCalled = true
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchUnaryInterceptor(userFn, serviceFn)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.True(t, userCalled)
	assert.False(t, serviceCalled)
}

func TestAuthDispatch_AuthFailurePropagates(t *testing.T) {
	authErr := context.Canceled

	userFn := func(ctx context.Context) (context.Context, error) {
		return nil, authErr
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchUnaryInterceptor(userFn, serviceFn)

	handlerCalled := false
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}
	_, err := interceptor(context.Background(), nil, info, handler)

	require.Error(t, err)
	assert.Equal(t, authErr, err)
	assert.False(t, handlerCalled, "인증 실패 시 handler 는 호출되지 않아야 함")
}

// stubServerStream 은 grpc.ServerStream 을 만족하는 최소 더미 구현이다.
type stubServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stubServerStream) Context() context.Context {
	return s.ctx
}

func TestAuthDispatchStream_RoutesChatStoreToServiceFn(t *testing.T) {
	var userCalled, serviceCalled bool

	userFn := func(ctx context.Context) (context.Context, error) {
		userCalled = true
		return ctx, nil
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		serviceCalled = true
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchStreamInterceptor(userFn, serviceFn)

	handlerCalled := false
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/chat_store.v1.ChatStoreService/GetLastMessages"}
	stream := &stubServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, info, handler)

	require.NoError(t, err)
	assert.True(t, serviceCalled)
	assert.False(t, userCalled)
	assert.True(t, handlerCalled)
}

func TestAuthDispatchStream_RoutesOtherMethodsToUserFn(t *testing.T) {
	var userCalled, serviceCalled bool

	userFn := func(ctx context.Context) (context.Context, error) {
		userCalled = true
		return ctx, nil
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		serviceCalled = true
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchStreamInterceptor(userFn, serviceFn)

	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}
	stream := &stubServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, info, handler)

	require.NoError(t, err)
	assert.True(t, userCalled)
	assert.False(t, serviceCalled)
}

func TestAuthDispatchStream_AuthFailurePropagates(t *testing.T) {
	authErr := context.Canceled

	userFn := func(ctx context.Context) (context.Context, error) {
		return nil, authErr
	}
	serviceFn := func(ctx context.Context) (context.Context, error) {
		return ctx, nil
	}

	interceptor := middleware.AuthDispatchStreamInterceptor(userFn, serviceFn)

	handlerCalled := false
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/chat.v1.ChatService/SendMessage"}
	stream := &stubServerStream{ctx: context.Background()}
	err := interceptor(nil, stream, info, handler)

	require.Error(t, err)
	assert.Equal(t, authErr, err)
	assert.False(t, handlerCalled, "인증 실패 시 handler 는 호출되지 않아야 함")
}
