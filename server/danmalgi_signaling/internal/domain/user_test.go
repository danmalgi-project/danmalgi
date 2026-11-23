package domain

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// Redis 유저 캐시(user:<id>) 포맷. api 서버가 GenericJacksonJsonRedisSerializer(default typing)로
// 쓰므로 @class 타입 키가 함께 실린다.
const userCacheJSON = `{"@class":"com.danmalgi.backend.user.domain.model.User","id":1,"email":"a@example.com","name":"UserA","tag":"0001","deviceId":"device-1","status":1,"profileImageUrl":"https://cdn.example.com/a.webp"}`

func TestUser_UnmarshalBinary_CacheFormat(t *testing.T) {
	user := &User{}

	if err := user.UnmarshalBinary([]byte(userCacheJSON)); err != nil {
		t.Fatalf("UnmarshalBinary 실패: %v", err)
	}

	want := &User{
		ID:              1,
		Email:           "a@example.com",
		Name:            "UserA",
		Tag:             "0001",
		DeviceId:        "device-1",
		Status:          1,
		ProfileImageUrl: "https://cdn.example.com/a.webp",
	}
	if !reflect.DeepEqual(user, want) {
		t.Errorf("got %+v, want %+v", user, want)
	}
}

// 옛 포맷 캐시에 남아 있을 수 있는 oauthType/identifyId 키가 역직렬화를 깨뜨리지 않아야 한다.
func TestUser_UnmarshalBinary_IgnoresUnknownKeys(t *testing.T) {
	user := &User{}

	if err := user.UnmarshalBinary([]byte(`{"id":1,"oauthType":2,"identifyId":"x"}`)); err != nil {
		t.Fatalf("UnmarshalBinary 실패: %v", err)
	}

	if want := (&User{ID: 1}); !reflect.DeepEqual(user, want) {
		t.Errorf("got %+v, want %+v", user, want)
	}
}

// Unmarshal은 키 대소문자를 구분하지 않으므로, 태그 누락·오타는 Marshal 키 집합으로만 잡힌다.
func TestUser_MarshalBinary_KeysMatchCacheContract(t *testing.T) {
	b, err := (&User{ID: 1}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary 실패: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal 실패: %v", err)
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{"deviceId", "email", "id", "name", "profileImageUrl", "status", "tag"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("캐시 키 집합 불일치: got %v, want %v", keys, want)
	}
}
