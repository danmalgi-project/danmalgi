package domain

import (
	"encoding/json"
)

// User의 json 태그는 Redis 유저 캐시(user:<id>) 포맷과 일치해야 한다.
type User struct {
	ID              int64  `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	Tag             string `json:"tag"`
	DeviceId        string `json:"deviceId"`
	Status          int    `json:"status"`
	ProfileImageUrl string `json:"profileImageUrl"`
}

func (u *User) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, u)
}

func (u *User) MarshalBinary() ([]byte, error) {
	return json.Marshal(u)
}
