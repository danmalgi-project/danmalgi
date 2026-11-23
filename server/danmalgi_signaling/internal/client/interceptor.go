package client

import (
	"context"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func UnaryTokenForwardInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	return invoker(attachTokenToOutgoing(ctx), method, req, reply, cc, opts...)
}

func StreamTokenForwardInterceptor(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return streamer(attachTokenToOutgoing(ctx), desc, cc, method, opts...)
}

func attachTokenToOutgoing(ctx context.Context) context.Context {
	tok, ok := middleware.GetToken(ctx)
	if !ok || tok == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+tok)
}
