package client

import (
	"context"
	"fmt"

	device_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/device_store/v1"
	dm_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/dm_store/v1"
	user_storev1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/private/user_store/v1"
	userv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/user/v1"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

// standalone 모드용 내부 API 대체 구현.
//
// 서버가 내부 API에서 실제로 호출하는 것은 UserStoreService.GetUsers 하나뿐이다
// (DmStore/DeviceStore 클라이언트는 배선만 되어 있고 호출부가 없다).
// AuthMiddleware가 토큰의 userId로 GetUserByID를 호출해 실존 유저를 요구하므로,
// fake는 요청받은 모든 userId를 "실존하는 활성 유저"로 응답해야 연결이 성립한다
// — 어댑터가 연결마다 유니크한 userId를 발급하는 것과 맞물린다.

// FakeUser는 userId로부터 결정론적인 더미 유저를 만든다.
// 같은 userId는 언제나 같은 유저로 보이므로, 재연결 시 표시 이름이 흔들리지 않는다.
func FakeUser(id int64) *user_storev1.User {
	return &user_storev1.User{
		Id:              id,
		Email:           fmt.Sprintf("user%d@standalone.local", id),
		Name:            fmt.Sprintf("테스트유저%d", id),
		Tag:             fmt.Sprintf("%04d", id%10000),
		Status:          int32(userv1.UserStatus_USER_ACTIVE),
		ProfileImageUrl: "",
	}
}

// NewFakeUserStoreClient는 standalone용 유저 조회 대체 구현을 만든다.
// 서비스 계층 테스트가 이 조합을 직접 물려볼 수 있도록 노출한다.
func NewFakeUserStoreClient() user_storev1.UserStoreServiceClient {
	return fakeUserStoreClient{}
}

type fakeUserStoreClient struct{}

func (fakeUserStoreClient) GetUsers(
	ctx context.Context,
	in *user_storev1.GetUsersRequest,
	opts ...grpc.CallOption,
) (*user_storev1.GetUsersResponse, error) {
	users := make([]*user_storev1.User, 0, len(in.GetUserIds()))
	for _, id := range in.GetUserIds() {
		users = append(users, FakeUser(id))
	}
	return &user_storev1.GetUsersResponse{Users: users}, nil
}

// fakeDmStoreClient / fakeDeviceStoreClient는 호출부가 없는 의존성을 타입만 채워준다.
// 나중에 실제 호출이 생기면 여기서 빈 응답이 나가는 것을 보고 바로 알아채도록,
// nil을 provide하지 않고 명시적인 빈 응답 구현을 둔다.

type fakeDmStoreClient struct{}

func (fakeDmStoreClient) GetChannelNames(
	ctx context.Context,
	in *dm_storev1.GetChannelNamesRequest,
	opts ...grpc.CallOption,
) (*dm_storev1.GetChannelNamesResponse, error) {
	return &dm_storev1.GetChannelNamesResponse{}, nil
}

type fakeDeviceStoreClient struct{}

func (fakeDeviceStoreClient) GetChannelDevices(
	ctx context.Context,
	in *device_storev1.GetChannelDevicesRequest,
	opts ...grpc.CallOption,
) (*device_storev1.GetChannelDevicesResponse, error) {
	return &device_storev1.GetChannelDevicesResponse{}, nil
}

func (fakeDeviceStoreClient) GetDevices(
	ctx context.Context,
	in *device_storev1.GetDevicesRequest,
	opts ...grpc.CallOption,
) (*device_storev1.GetDevicesResponse, error) {
	return &device_storev1.GetDevicesResponse{}, nil
}

// FakeModule은 Module을 대체해 내부 API 연결 없이 클라이언트 인터페이스를 채운다(standalone 모드).
// 실제 gRPC 연결을 열지 않으므로 name:"api_client" *grpc.ClientConn은 제공하지 않는다.
var FakeModule = fx.Options(
	fx.Provide(
		NewFakeUserStoreClient,
		func() dm_storev1.DmStoreServiceClient { return fakeDmStoreClient{} },
		func() device_storev1.DeviceStoreServiceClient { return fakeDeviceStoreClient{} },
	),
)
