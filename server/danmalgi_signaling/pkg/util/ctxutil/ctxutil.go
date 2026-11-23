package ctxutil

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc/metadata"
)

// WithBearerToken extracts the Authorization Bearer token from the incoming
// gRPC metadata of ctx and returns a new background context with that token
// attached as outgoing metadata. Returns the original ctx if no token is found.
func WithBearerToken(ctx context.Context) context.Context {
	tok, err := auth.AuthFromMD(ctx, "bearer")
	if err != nil || tok == "" {
		return ctx
	}
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "bearer "+tok))
}
