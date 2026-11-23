package handler

import (
	"context"
	"io"
	"strconv"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/server/middleware"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type SignalingHandler struct {
	signalingv1.UnimplementedSignalingServiceServer
	signalingUsecase usecase.SignalingUsecase
}

func NewSignalingHandler(signalingUsecase usecase.SignalingUsecase) *SignalingHandler {
	return &SignalingHandler{
		signalingUsecase: signalingUsecase,
	}
}

func (s *SignalingHandler) Signaling(stream signalingv1.SignalingService_SignalingServer) error {
	ctx := stream.Context()

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return domain.NewErrInvalidToken(errors.WithStack(domain.ErrNotFindIdInToken))
	}

	channelIdStrings := md["channel_id"]
	channelId, err := strconv.ParseInt(channelIdStrings[0], 10, 64)
	if err != nil {
		return errors.WithStack(err)
	}

	userId := ctx.Value(middleware.UID{}).(int64)
	deviceId := ctx.Value(middleware.DID{}).(string)

	wrapper, err := s.signalingUsecase.PreSignaling(ctx, userId, channelId, deviceId)
	if err != nil {
		return err
	}
	defer s.signalingUsecase.PostSignaling(ctx, wrapper, channelId)

	done := make(chan error)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				request, err := stream.Recv()
				if err != nil {
					if err == io.EOF {
						return
					}
					continue
				}

				switch message := request.SignalingRequestInfo.Message.(type) {
				case *signalingv1.SignalingRequestInfo_Answer:
					err = s.signalingUsecase.SignalAnswer(message.Answer.Sdp.SdpType, message.Answer.Sdp.SdpData, wrapper)
				case *signalingv1.SignalingRequestInfo_Candidate:
					err = s.signalingUsecase.SignalCandidate(message.Candidate.Candidate, message.Candidate.SdpMid, message.Candidate.SdpMlineIndex, wrapper)
				default:
					err = errors.New("invalid error")
				}

				if err != nil {
					wrapper.EnqueueErr(err)
					return
				}
			}
		}
	}()

	go func() {
		for {
			var response *signalingv1.SignalingResponse
			select {
			case offer := <-wrapper.Offer:
				response = convertOfferResponse(offer)
			case candidate := <-wrapper.Candidate:
				response = convertCandidateResponse(candidate)
			case err := <-wrapper.Err:
				done <- err
				return
			case <-ctx.Done():
				return
			}

			if err := stream.Send(response); err != nil {
				wrapper.EnqueueErr(err)
				return
			}
		}
	}()

	// 초기 offer가 실패하면 클라이언트는 아무것도 받지 못하므로 스트림을 끝낸다.
	if err := s.signalingUsecase.SendOffer(wrapper); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	case err := <-done:
		return err
	}
}

// GetChannelCallStartedAt은 채널 통화방의 통화 시작 시각을 조회한다. 통화 중이
// 아닌 경우는 에러가 아니다 - 빈 응답(StartedAt 부재) + OK를 반환한다. 채널
// 멤버십 인가는 서버가 멤버십을 모르므로 범위 밖이며, 인증은 인터셉터가 담당하므로
// middleware.GetUserID를 호출하지 않는다.
func (s *SignalingHandler) GetChannelCallStartedAt(
	ctx context.Context,
	req *signalingv1.GetChannelCallStartedAtRequest,
) (*signalingv1.GetChannelCallStartedAtResponse, error) {
	if req.GetChannelId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid channel_id")
	}

	startedAt, ok := s.signalingUsecase.GetChannelCallStartedAt(req.GetChannelId())
	return convertChannelCallStartedAtResponse(startedAt, ok), nil
}
