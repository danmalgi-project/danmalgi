package main

import (
	"os"
	"testing"

	"go.uber.org/fx"
)

// TestAppWiring은 main()이 사용하는 fx DI 그래프가 온전하게 연결되는지 검증한다.
// ValidateApp은 생성자를 실행하지 않고 의존성 그래프의 타입만 검사하므로
// Redis 연결이나 포트 바인딩, .env 없이 실행할 수 있다.
// registerGrpcService / server.RegisterHook 배선이 누락되거나
// 의존성이 끊기면 이 테스트가 실패한다.
func TestAppWiring(t *testing.T) {
	if err := fx.ValidateApp(fx.NopLogger, appOptions(t.TempDir(), false)); err != nil {
		t.Fatalf("fx 의존성 그래프 검증 실패: %v", err)
	}
}

// TestAppWiringStandalone은 standalone 모드에서 외부 의존성을 fake로 바꿔도
// 그래프가 그대로 성립하는지 검증한다. fake 모듈이 제공 타입을 하나라도 빠뜨리면
// (예: DmStore/DeviceStore 클라이언트) 여기서 잡힌다.
func TestAppWiringStandalone(t *testing.T) {
	if err := fx.ValidateApp(fx.NopLogger, appOptions(t.TempDir(), true)); err != nil {
		t.Fatalf("standalone fx 의존성 그래프 검증 실패: %v", err)
	}
}

// TestApplyStandaloneEnvDefaults는 비어 있는 키만 채우는지 검증한다.
func TestApplyStandaloneEnvDefaults(t *testing.T) {
	for _, key := range standaloneEnvKeys() {
		t.Setenv(key, "")
	}
	t.Setenv("SERVER_PORT", "19999")

	applied := applyStandaloneEnvDefaults()

	if got := os.Getenv("SERVER_PORT"); got != "19999" {
		t.Errorf("이미 설정된 SERVER_PORT를 덮어썼다: %q", got)
	}
	for _, key := range applied {
		if key == "SERVER_PORT" {
			t.Errorf("이미 설정된 SERVER_PORT가 채워진 키 목록에 들어갔다")
		}
	}

	// 나머지 키는 기본값으로 채워져야 한다.
	for _, key := range standaloneEnvKeys() {
		if key == "SERVER_PORT" {
			continue
		}
		if got := os.Getenv(key); got != standaloneEnvDefaults[key] {
			t.Errorf("%s 기본값이 채워지지 않았다: got %q, want %q", key, got, standaloneEnvDefaults[key])
		}
	}

	// TLS는 grpc.NewGrpc가 ParseBool로 읽으므로 반드시 파싱 가능한 값이어야 한다.
	if os.Getenv("TLS") != "false" {
		t.Errorf("TLS 기본값이 false가 아니다: %q", os.Getenv("TLS"))
	}
}

// TestStandaloneEnvKeysCoverDefaults는 순서 목록과 기본값 맵이 어긋나지 않게 한다.
// 한쪽에만 키를 추가하면 조용히 누락되므로 여기서 잡는다.
func TestStandaloneEnvKeysCoverDefaults(t *testing.T) {
	keys := standaloneEnvKeys()
	if len(keys) != len(standaloneEnvDefaults) {
		t.Fatalf("키 목록(%d)과 기본값 맵(%d)의 크기가 다르다", len(keys), len(standaloneEnvDefaults))
	}
	for _, key := range keys {
		if _, ok := standaloneEnvDefaults[key]; !ok {
			t.Errorf("%s가 기본값 맵에 없다", key)
		}
	}
}
