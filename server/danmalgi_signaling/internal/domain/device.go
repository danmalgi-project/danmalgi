package domain

import "encoding/json"

type Device struct {
	ID       int64
	UserID   int64
	DeviceID string  `gorm:"unique:size:255;not null"`
	FcmToken *string `gorm:"unique;size:255"`
}

type DeviceCache struct {
	UserID   int64
	FcmToken string
}

func (d *DeviceCache) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, d)
}

func (d *DeviceCache) MarshalBinary() ([]byte, error) {
	return json.Marshal(d)
}
