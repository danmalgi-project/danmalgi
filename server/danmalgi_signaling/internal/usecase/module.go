package usecase

import (
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/signaling"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase/service"
	"go.uber.org/fx"
)

var Module fx.Option = fx.Options(
	fx.Provide(fx.Annotate(
		service.NewUserService,
		fx.As(new(UserUsecase)),
		fx.As(new(signaling.UserUsecase)),
	)),
	fx.Provide(fx.Annotate(service.NewSignalingService, fx.As(new(SignalingUsecase)))),
)
