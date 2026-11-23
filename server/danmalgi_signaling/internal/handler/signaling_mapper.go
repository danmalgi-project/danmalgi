package handler

import (
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	userv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/user/v1"
	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func convertOfferResponse(offer domain.Offer) *signalingv1.SignalingResponse {
	// joined_user_tracks는 델타가 아니라 이 수신자를 뺀 채널 전체 명단 스냅샷이다.
	// 그래서 명단이 비어도 필드를 반드시 채워 보낸다 — "필드 부재"와 "빈 명단"이
	// 구분되면 클라이언트가 마지막 참가자의 퇴장을 반영할 수 없다.
	joinedUserTracks := &signalingv1.UserTracks{
		UserTracks: make([]*signalingv1.UserTrack, 0, len(offer.UserTracks)),
	}
	for _, userTrack := range offer.UserTracks {
		joinedUserTracks.UserTracks = append(joinedUserTracks.UserTracks, &signalingv1.UserTrack{
			User: &userv1.User{
				Id:              userTrack.User.ID,
				Name:            userTrack.User.Name,
				Email:           userTrack.User.Email,
				Tag:             userTrack.User.Tag,
				Status:          userv1.UserStatus(userTrack.User.Status),
				ProfileImageUrl: userTrack.User.ProfileImageUrl,
			},
			TrackId:   userTrack.TrackLocal.ID(),
			TrackType: signalingv1.TrackType(userTrack.TrackType),
		})
	}

	return &signalingv1.SignalingResponse{
		SignalingResponseInfo: &signalingv1.SignalingResponseInfo{
			Message: &signalingv1.SignalingResponseInfo_Offer{
				Offer: &signalingv1.Offer{
					Sdp: &signalingv1.SessionDescription{
						SdpType: offer.SessionDescription.Type.String(),
						SdpData: offer.SessionDescription.SDP,
					},
					JoinedUserTracks: joinedUserTracks,
				},
			},
		},
	}
}

func convertCandidateResponse(candidate *webrtc.ICECandidate) *signalingv1.SignalingResponse {
	return &signalingv1.SignalingResponse{
		SignalingResponseInfo: &signalingv1.SignalingResponseInfo{
			Message: &signalingv1.SignalingResponseInfo_Candidate{
				Candidate: &signalingv1.Candidate{
					Candidate:     candidate.ToJSON().Candidate,
					SdpMid:        candidate.SDPMid,
					SdpMlineIndex: int32(candidate.SDPMLineIndex),
				},
			},
		},
	}
}

// convertChannelCallStartedAtResponse는 통화 시작 시각 조회 결과를 응답 메시지로
// 변환한다. ok == false(통화 중 아님)면 StartedAt 필드를 채우지 않는다 - "필드
// 부재"와 "시각 0값"을 클라이언트가 구분할 수 있어야 하기 때문이다.
func convertChannelCallStartedAtResponse(startedAt time.Time, ok bool) *signalingv1.GetChannelCallStartedAtResponse {
	if !ok {
		return &signalingv1.GetChannelCallStartedAtResponse{}
	}
	return &signalingv1.GetChannelCallStartedAtResponse{
		StartedAt: timestamppb.New(startedAt),
	}
}

// func convertTrackResponse(track domain.Track) *signalingv1.SignalingResponse {
// 	return &signalingv1.SignalingResponse{
// 		SignalingResponseInfo: &signalingv1.SignalingResponseInfo{
// 			Message: &signalingv1.SignalingResponseInfo_Track{
// 				Track: &signalingv1.Track{
// 					UserId:    track.UserId,
// 					TrackId:   track.TrackId,
// 					TrackType: signalingv1.TrackType(track.TrackType),
// 				},
// 			},
// 		},
// 	}
// }
