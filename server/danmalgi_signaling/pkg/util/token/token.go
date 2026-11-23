package token

import (
	"os"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
)

// Generate는 userId와 deviceId를 claim으로 담아 JWT_SECRET(HS256)으로 서명한
// 액세스 토큰을 발급한다. Validate가 요구하는 claim과 1:1로 대응한다.
func Generate(userId int64, deviceId string) (string, error) {
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      "Danmalgi",
		"userId":   userId,
		"deviceId": deviceId,
	}).SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return "", errors.WithStack(err)
	}

	return accessToken, nil
}

// --- 서비스 간(server-to-server) JWT 검증 ---
//
// ⚠️ 와이어 포맷(secret env 키, 서명 알고리즘, claim 이름, metadata 스킴)은 backend 계약과
// 확정되지 않았다. 아래 기본값으로 구현하되, backend 팀 확정 시 이 상수만 조정하면 되도록
// 검증 로직을 여기 한 곳에 격리한다.
const (
	// ServiceJWTSecretEnv 는 서비스 JWT 서명 검증에 쓰는 대칭키 env 키.
	ServiceJWTSecretEnv = "SERVICE_JWT_SECRET"
	// serviceClaimKey / subjectClaimKey 는 서비스 식별자를 담는 claim 이름 후보.
	serviceClaimKey = "service"
	subjectClaimKey = "sub"
)

// ValidateService 는 서버 간 호출에 실린 서비스 JWT 를 검증하고 서비스 식별자를 반환한다.
// HMAC 서명(SERVICE_JWT_SECRET)과 서비스 식별 claim(service 또는 sub) 존재를 확인한다.
func ValidateService(token string) (string, error) {
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return []byte(os.Getenv(ServiceJWTSecretEnv)), nil
	})
	if err != nil {
		return "", errors.WithStack(err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		return "", errors.New("invalid service token claims")
	}

	if service, ok := claims[serviceClaimKey].(string); ok && service != "" {
		return service, nil
	}
	if subject, ok := claims[subjectClaimKey].(string); ok && subject != "" {
		return subject, nil
	}

	return "", errors.New("service identifier claim not found in token")
}

func Validate(token string) (*int64, *string, error) {
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}

		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	claims := parsedToken.Claims.(jwt.MapClaims)
	userId, ok := claims["userId"]
	if !ok {
		return nil, nil, errors.New("not exist subject in token")
	}

	id, ok := userId.(float64)
	if !ok {
		return nil, nil, errors.New("can not cast id to uint16")
	}

	device, ok := claims["deviceId"]
	if !ok {
		return nil, nil, errors.New("not exist deivce in token")
	}

	deviceID, ok := device.(string)
	if !ok {
		return nil, nil, errors.New("can not cast device id to string")
	}

	userID := int64(id)
	return &userID, &deviceID, nil
}
