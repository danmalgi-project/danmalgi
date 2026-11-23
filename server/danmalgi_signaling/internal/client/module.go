package client

import "go.uber.org/fx"

var Module = fx.Options(
	fx.Provide(fx.Annotate(NewApiClient, fx.ResultTags(`name:"api_client"`))),
	fx.Provide(NewInternalDmClient, NewInternalDeviceClient, NewInternalUserClient),
)
