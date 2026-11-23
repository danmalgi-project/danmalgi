package domain

import "github.com/pion/webrtc/v4"

type UserTrack struct {
	User       *User
	TrackType  int
	TrackLocal *webrtc.TrackLocalStaticRTP
}
