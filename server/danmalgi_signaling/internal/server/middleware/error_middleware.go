package middleware

// var statusMap = map[int]struct {
// 	grpcCode codes.Code
// 	pbCode   chatv1.ErrorCode
// }{
// 	domain.INVALID_TOKEN:     {codes.Unauthenticated, chatv1.ErrorCode_INVALID_TOKEN},
// 	domain.INVALID_INPUT:     {codes.InvalidArgument, chatv1.ErrorCode_INVALID_INPUT},
// 	domain.DUPLICATED_DATA:   {codes.AlreadyExists, chatv1.ErrorCode_DUPLICATED_DATA},
// 	domain.OAUTH_ERROR:       {codes.PermissionDenied, chatv1.ErrorCode_OAUTH_ERROR},
// 	domain.RECORD_NOT_FOUND:  {codes.NotFound, chatv1.ErrorCode_RECORD_NOT_FOUND},
// 	domain.PERSISTENCE_ERROR: {codes.Internal, chatv1.ErrorCode_PERSISTENCE_ERROR},
// }

// func UnaryErrorMiddleware() grpc.UnaryServerInterceptor {
// 	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
// 		resp, err := handler(ctx, req)
// 		if err != nil {
// 			customErr := toCustomError(err)

// 			log.Log.Error(fmt.Sprintf("%+v", customErr.BaseErr))

// 			errorStatus, ok := statusMap[customErr.StatusCode]
// 			if !ok {
// 				return nil, status.Error(codes.Internal, "internal error")
// 			}

// 			st := status.New(errorStatus.grpcCode, customErr.Message)
// 			ds, err := st.WithDetails(&chatv1.ErrorDetail{
// 				ErrorCode: errorStatus.pbCode,
// 			})

// 			if err != nil {
// 				return nil, st.Err()
// 			}

// 			return nil, ds.Err()
// 		}

// 		return resp, nil
// 	}
// }

// func StreamErrorMiddleware() grpc.StreamServerInterceptor {
// 	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
// 		err := handler(srv, ss)
// 		if err != nil {
// 			customErr := toCustomError(err)

// 			log.Log.Error(fmt.Sprintf("%+v", customErr.BaseErr))

// 			errorStatus, ok := statusMap[customErr.StatusCode]
// 			if !ok {
// 				return status.Error(codes.Internal, "internal error")
// 			}

// 			st := status.New(errorStatus.grpcCode, customErr.Message)
// 			ds, err := st.WithDetails(&chatv1.ErrorDetail{
// 				ErrorCode: errorStatus.pbCode,
// 			})

// 			if err != nil {
// 				return st.Err()
// 			}

// 			return ds.Err()
// 		}

// 		return nil
// 	}
// }

// func toCustomError(err error) *domain.CustomError {
// 	var customErr *domain.CustomError
// 	if !errors.As(err, &customErr) {
// 		customErr = &domain.CustomError{
// 			BaseErr:    err,
// 			StatusCode: domain.INTERNAL,
// 			Message:    err.Error(),
// 		}
// 	}

// 	return customErr
// }
