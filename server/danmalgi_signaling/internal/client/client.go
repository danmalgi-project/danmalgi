package client

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewApiClient(lc fx.Lifecycle) (*grpc.ClientConn, error) {
	url := fmt.Sprintf("%s:%s", os.Getenv("API_SERVER_HOST"), os.Getenv("API_SERVER_PORT"))
	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// grpc.WithChainUnaryInterceptor(UnaryTokenForwardInterceptor),
		// grpc.WithChainStreamInterceptor(StreamTokenForwardInterceptor),
	)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return conn.Close()
		},
	})

	return conn, nil
}
