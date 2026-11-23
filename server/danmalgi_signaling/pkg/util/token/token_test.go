package token

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"
)

// signServiceToken 은 ValidateService 검증용 서비스 JWT 를 임의 claim 으로 서명한다.
func signServiceToken(t *testing.T, claims jwt.MapClaims, secret string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign service token: %v", err)
	}
	return tok
}

func TestValidateService_ReturnsServiceClaim(t *testing.T) {
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"service": "danmalgi_backend"}, "service-secret")

	got, err := ValidateService(tok)
	if err != nil {
		t.Fatalf("ValidateService returned error: %v", err)
	}
	if got != "danmalgi_backend" {
		t.Errorf("service = %q, want %q", got, "danmalgi_backend")
	}
}

// service claim 이 없으면 sub claim 으로 폴백한다.
func TestValidateService_FallsBackToSubClaim(t *testing.T) {
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"sub": "danmalgi_backend"}, "service-secret")

	got, err := ValidateService(tok)
	if err != nil {
		t.Fatalf("ValidateService returned error: %v", err)
	}
	if got != "danmalgi_backend" {
		t.Errorf("service = %q, want %q", got, "danmalgi_backend")
	}
}

// service claim 이 빈 문자열이면 식별자로 인정하지 않고 sub 로 넘어간다.
func TestValidateService_EmptyServiceClaimFallsBackToSub(t *testing.T) {
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"service": "", "sub": "danmalgi_backend"}, "service-secret")

	got, err := ValidateService(tok)
	if err != nil {
		t.Fatalf("ValidateService returned error: %v", err)
	}
	if got != "danmalgi_backend" {
		t.Errorf("service = %q, want %q", got, "danmalgi_backend")
	}
}

func TestValidateService_NoIdentifierClaimFails(t *testing.T) {
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"iss": "danmalgi"}, "service-secret")

	if _, err := ValidateService(tok); err == nil {
		t.Fatal("ValidateService should fail without a service/sub claim, got nil error")
	}
}

func TestValidateService_WrongSecretFails(t *testing.T) {
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"service": "danmalgi_backend"}, "another-secret")

	if _, err := ValidateService(tok); err == nil {
		t.Fatal("ValidateService should fail on signature mismatch, got nil error")
	}
}

// ValidateService 는 사용자 토큰용 JWT_SECRET 이 아니라 SERVICE_JWT_SECRET 으로 검증한다.
func TestValidateService_DoesNotUseUserSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "user-secret")
	t.Setenv(ServiceJWTSecretEnv, "service-secret")

	tok := signServiceToken(t, jwt.MapClaims{"service": "danmalgi_backend"}, "user-secret")

	if _, err := ValidateService(tok); err == nil {
		t.Fatal("ValidateService should not accept a token signed with JWT_SECRET, got nil error")
	}
}

func TestGenerateThenValidate_RoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")

	const wantUserID int64 = 42
	const wantDeviceID = "device-abc"

	tok, err := Generate(wantUserID, wantDeviceID)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if tok == "" {
		t.Fatal("Generate returned an empty token")
	}

	gotUserID, gotDeviceID, err := Validate(tok)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	if gotUserID == nil || *gotUserID != wantUserID {
		t.Errorf("userId = %v, want %d", gotUserID, wantUserID)
	}
	if gotDeviceID == nil || *gotDeviceID != wantDeviceID {
		t.Errorf("deviceId = %v, want %q", gotDeviceID, wantDeviceID)
	}
}

func TestValidate_WrongSecretFails(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	tok, err := Generate(1, "device-abc")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	t.Setenv("JWT_SECRET", "different-secret")
	if _, _, err := Validate(tok); err == nil {
		t.Fatal("Validate should fail when JWT_SECRET changes, got nil error")
	}
}
