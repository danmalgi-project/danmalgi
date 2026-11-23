package middleware

import (
	"context"
	"log/slog"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
)

func LoggingMiddleware(logger *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		logger.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

// func LoggingInterceptor(l *slog.Logger) func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
// 	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
// 		l.Log(ctx, slog.Level(LevelInfo), "started call")

// 		response, err := handler(ctx, req)

// 		st, ok := status.FromError(err)
// 		if !ok {
// 			st = status.New(codes.Internal, "")
// 		}

// 		status.
// 		var level Level

// 		switch st.Code() {
// 		case codes.OK, codes.NotFound, codes.Canceled, codes.AlreadyExists, codes.InvalidArgument, codes.Unauthenticated:
// 			level = LevelInfo

// 		case codes.DeadlineExceeded, codes.PermissionDenied, codes.ResourceExhausted, codes.FailedPrecondition, codes.Aborted,
// 			codes.OutOfRange, codes.Unavailable:
// 			level = LevelWarn

// 		case codes.Unknown, codes.Unimplemented, codes.Internal, codes.DataLoss:
// 			level = LevelError

// 		default:
// 			level = LevelError
// 		}

// 		l.Log(ctx, slog.Level(level), "finished call", "grpc.service="+st.Proto().String(), "grpc.method")

// 		return response, err
// 	}
// }
