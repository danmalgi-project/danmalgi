// gen-token은 webrtc-adapter 로컬 테스트에 쓸 JWT를 발급하는 CLI다.
// pkg/util/token.Generate가 서명하는 토큰은 서버의 token.Validate와
// 동일한 claim(userId, deviceId)·서명(JWT_SECRET, HS256)을 사용하므로
// 서버의 AuthMiddleware를 그대로 통과한다.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/token"
)

func main() {
	userID := flag.Int64("user-id", 1, "토큰에 담을 userId")
	deviceID := flag.String("device-id", "local-test-device", "토큰에 담을 deviceId")
	jwtSecret := flag.String("jwt-secret", "", "서명에 쓸 JWT_SECRET (미지정 시 환경변수 JWT_SECRET 사용)")
	flag.Parse()

	if *jwtSecret != "" {
		if err := os.Setenv("JWT_SECRET", *jwtSecret); err != nil {
			log.Fatal(err)
		}
	}

	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET이 필요합니다: --jwt-secret 플래그 또는 JWT_SECRET 환경변수를 설정하세요 (서버 .env와 동일한 값)")
	}

	tok, err := token.Generate(*userID, *deviceID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(tok)
}
