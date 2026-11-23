package server_test

// T5(unary 인터셉터 체인 활성화)의 회귀 가드. 인터셉터 블록이 다시 주석 처리되면
// 아래 인증 테스트가 반드시 실패해야 한다. 동시에 새 rpc GetChannelCallStartedAt을
// 프로덕션 진입점(server.NewGrpc + handler.NewSignalingHandler + RegisterSignalingServiceServer)과
// 동일한 등록 경로로 통과시켜 배선이 재구현으로 우회되지 않게 한다.

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/handler"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase/mocks"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/token"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeSignalingUsecase는 usecase.SignalingUsecase를 만족하는 최소 스텁이다.
// GetChannelCallStartedAt만 값을 돌려주고 나머지 메서드는 이 테스트가 다루는
// unary 경로에서 쓰이지 않으므로 zero value/nil을 반환한다.
type fakeSignalingUsecase struct {
	startedAt time.Time
	ok        bool
}

func (f *fakeSignalingUsecase) PreSignaling(ctx context.Context, userId, channelId int64, deviceId string) (*domain.PeerConnectionWrapper, error) {
	return nil, nil
}
func (f *fakeSignalingUsecase) PostSignaling(ctx context.Context, wrapper *domain.PeerConnectionWrapper, channelId int64) {
}
func (f *fakeSignalingUsecase) SendOffer(wrapper *domain.PeerConnectionWrapper) error { return nil }
func (f *fakeSignalingUsecase) SignalAnswer(sdpType, sdp string, wrapper *domain.PeerConnectionWrapper) error {
	return nil
}
func (f *fakeSignalingUsecase) SignalCandidate(candidate, sdpMid string, sdpMLineIndex int32, wrapper *domain.PeerConnectionWrapper) error {
	return nil
}
func (f *fakeSignalingUsecase) GetChannelCallStartedAt(channelId int64) (time.Time, bool) {
	return f.startedAt, f.ok
}

const bufSize = 1024 * 1024

// newTestServer는 프로덕션과 동일한 등록 경로(server.NewGrpc → RegisterSignalingServiceServer)로
// bufconn 위에 gRPC 서버를 띄우고, 클라이언트 conn과 정리 함수를 반환한다.
func newTestServer(t *testing.T, signalingUsecase *fakeSignalingUsecase) *grpc.ClientConn {
	t.Helper()

	t.Setenv("TLS", "false")
	t.Setenv("JWT_SECRET", "test-secret")

	mockUserUsecase := mocks.NewMockUserUsecase(t)
	mockUserUsecase.EXPECT().
		GetUserByID(mock.Anything, int64(1)).
		Return(&domain.User{ID: 1}, nil).
		Maybe()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := server.NewGrpc(logger, mockUserUsecase)
	if err != nil {
		t.Fatalf("server.NewGrpc failed: %v", err)
	}

	signalingv1.RegisterSignalingServiceServer(srv, handler.NewSignalingHandler(signalingUsecase))

	lis := bufconn.Listen(bufSize)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn
}

func TestGetChannelCallStartedAtRequiresAuth(t *testing.T) {
	conn := newTestServer(t, &fakeSignalingUsecase{})
	client := signalingv1.NewSignalingServiceClient(conn)

	_, err := client.GetChannelCallStartedAt(context.Background(), &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 1})

	if err == nil {
		t.Fatal("err = nil, want Unauthenticated (no bearer token attached)")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("err = %v, want status Unauthenticated", err)
	}
}

func TestGetChannelCallStartedAtReturnsStartedAt(t *testing.T) {
	startedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	conn := newTestServer(t, &fakeSignalingUsecase{startedAt: startedAt, ok: true})
	client := signalingv1.NewSignalingServiceClient(conn)

	tok, err := token.Generate(1, "device-1")
	if err != nil {
		t.Fatalf("token.Generate failed: %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)

	resp, err := client.GetChannelCallStartedAt(ctx, &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 1})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if resp.StartedAt == nil || !resp.StartedAt.AsTime().Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", resp.StartedAt, startedAt)
	}
}

func TestGetChannelCallStartedAtWhenNotInCall(t *testing.T) {
	conn := newTestServer(t, &fakeSignalingUsecase{ok: false})
	client := signalingv1.NewSignalingServiceClient(conn)

	tok, err := token.Generate(1, "device-1")
	if err != nil {
		t.Fatalf("token.Generate failed: %v", err)
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)

	resp, err := client.GetChannelCallStartedAt(ctx, &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 1})

	if err != nil {
		t.Fatalf("err = %v, want nil when not in call", err)
	}
	if resp.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil when not in call", resp.StartedAt)
	}
}
