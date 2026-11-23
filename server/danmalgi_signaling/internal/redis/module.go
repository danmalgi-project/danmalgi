package redis

import "go.uber.org/fx"

var Module fx.Option = fx.Options(
	fx.Provide(NewRedis),
)
