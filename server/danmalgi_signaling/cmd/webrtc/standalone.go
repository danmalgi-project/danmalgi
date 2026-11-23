package main

import (
	"log/slog"
	"os"
)

// StandaloneJWTSecret은 standalone 모드에서 JWT_SECRET이 비어 있을 때 쓰는 기본 시크릿이다.
//
// 이 값은 `tools/webrtc-adapter`의 run 스크립트와 반드시 일치해야 한다 —
// 어댑터가 다른 시크릿으로 서명하면 서버 AuthMiddleware 검증만 조용히 실패한다.
// 스크립트 쪽 기본값을 바꿀 일이 생기면 여기와 함께 고칠 것.
const StandaloneJWTSecret = "danmalgi-standalone-dev-secret"

// standaloneEnvDefaults는 standalone 모드에서 값이 비어 있을 때 채워 넣는 환경변수다.
//
// 여기 없는 키(REDIS_*, API_SERVER_*)는 standalone 모드에서 아예 쓰이지 않는다.
// 해당 의존성이 인프로세스 fake로 대체되기 때문이다.
var standaloneEnvDefaults = map[string]string{
	"SERVER_PORT": "8083",
	"HEALTH_PORT": "8084",
	"TLS":         "false",
	"NAT_IP":      "127.0.0.1",
	"JWT_SECRET":  StandaloneJWTSecret,
}

// applyStandaloneEnvDefaults는 .env 없이도 서버가 뜨도록 빠진 환경변수를 기본값으로 채운다.
// 이미 설정된 값은 건드리지 않으므로, .env가 있으면 그 값이 항상 우선한다.
// 채워 넣은 키 목록을 돌려준다(로그·테스트용).
func applyStandaloneEnvDefaults() []string {
	applied := make([]string, 0, len(standaloneEnvDefaults))
	for _, key := range standaloneEnvKeys() {
		if os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, standaloneEnvDefaults[key])
		applied = append(applied, key)
	}
	return applied
}

// standaloneEnvKeys는 로그 출력이 매 기동마다 흔들리지 않도록 고정된 순서를 준다.
func standaloneEnvKeys() []string {
	return []string{"SERVER_PORT", "HEALTH_PORT", "TLS", "NAT_IP", "JWT_SECRET"}
}

func logStandaloneBanner(logger *slog.Logger, applied []string) {
	logger.Warn(
		"standalone 모드로 기동합니다 — Redis와 내부 API가 인프로세스 fake로 대체됩니다. 로컬 검증 전용입니다.",
		"defaulted_env", applied,
		"server_port", os.Getenv("SERVER_PORT"),
	)
}
