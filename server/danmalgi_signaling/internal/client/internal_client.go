package client

import (
	device_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/device_store/v1"
	dm_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/dm_store/v1"
	user_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/user_store/v1"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

type InternalConfig struct {
	fx.In
	Conn *grpc.ClientConn `name:"api_client"`
}

func NewInternalDmClient(i InternalConfig) dm_storev1.DmStoreServiceClient {
	return dm_storev1.NewDmStoreServiceClient(i.Conn)
}

func NewInternalDeviceClient(i InternalConfig) device_storev1.DeviceStoreServiceClient {
	return device_storev1.NewDeviceStoreServiceClient(i.Conn)
}

func NewInternalUserClient(i InternalConfig) user_storev1.UserStoreServiceClient {
	return user_storev1.NewUserStoreServiceClient(i.Conn)
}
