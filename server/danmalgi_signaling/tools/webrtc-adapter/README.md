# webrtc-adapter

브라우저 WebRTC 시그널링을 `danmalgi-webrtc-server`의 gRPC `SignalingService`에 물려
로컬에서 테스트하기 위한 **WebSocket ↔ gRPC 브릿지 + 웹 클라이언트**다.

- `main.go`: `:8888` HTTPS에서 정적 페이지(`index.html`)와 `/signaling`(WS↔gRPC 브릿지)을 제공한다.
- `index.html`: 마이크 입력을 캡처해 `wss://${location.host}/signaling`로 시그널링하는 최소 WebRTC 클라이언트.
- `cmd/gen-token`: 서버와 동일한 방식(`pkg/util/token`)으로 로컬 테스트용 JWT를 발급하는 CLI.

이 도구는 원래 `docs/webrtc-adapter`에 있었으나, 서버 proto 계약(`pkg/gen`)에 정합시키고
독립적으로 실행 가능하도록 `tools/webrtc-adapter`로 편입했다. 자체 proto 사본(`api/`)은
제거했고, `go.work`을 통해 저장소 루트의 `pkg/gen`을 직접 참조한다.

## 전제 조건

- Go 1.24 이상, `openssl` (Git for Windows에 포함된 것으로 충분).
- **standalone 모드(기본)에서는 외부 의존성이 필요 없다.** Redis도 내부 API 서버도 띄우지 않아도 되고,
  저장소 루트의 `.env`조차 없어도 된다. 이 저장소만 있으면 로컬 E2E 수동 검증이 된다.
- `-WithBackend`/`--with-backend`로 실제 백엔드에 붙일 때만 내부 API 서버·Redis가 미리 떠 있어야 하고,
  저장소 루트에 서버용 `.env`가 필요하다 (루트 `README.md`의 "시작하기" 참고).

## 빠른 시작 (원클릭)

```powershell
cd tools/webrtc-adapter
.\ssl.ps1   # 최초 1회: 로컬 self-signed 인증서 발급
.\run.ps1   # standalone 기동 -> 어댑터 기동 -> 접속 URL 안내
```

POSIX 셸(WSL/Git Bash/Linux/macOS)에서는 동일한 흐름을 `ssl.sh` / `run.sh`로 실행한다.

```sh
cd tools/webrtc-adapter
./ssl.sh
./run.sh
```

`run.ps1`/`run.sh`는 다음을 순서대로 수행한다.

1. **준비물 점검**: 어댑터 인증서(`server.crt`/`server.key`) 존재 여부를 확인한다. `-WithBackend`일 때만
   서버 `.env`의 필수 값과 내부 API/Redis 도달 여부까지 확인한다. 하나라도 충족되지 않으면 무엇이 문제인지
   명확한 메시지를 출력하고 종료한다.
2. **JWT 준비**: `tools/webrtc-adapter/.env`에 `JWT_TOKEN`이 없으면 `JWT_SECRET`으로 `cmd/gen-token`을
   실행해 새 토큰을 발급한다. 이 `JWT_SECRET`은 서버·어댑터 두 프로세스에 **같은 값으로 주입**된다.
3. **서버 기동**: 저장소 루트에서 `go run ./cmd/webrtc`(standalone이면 `--standalone`)를 실행하고
   `SERVER_PORT`가 열릴 때까지 대기한다.
4. **어댑터 기동**: `tools/webrtc-adapter`에서 `go run .`을 실행한다.
5. **접속 안내**: `https://localhost:<포트>/` 주소를 출력한다. 자체 서명 인증서 경고는 수락하면 된다.

종료하려면 스크립트가 실행 중인 터미널에서 `Ctrl+C`를 누른다 (서버·어댑터 프로세스를 함께 정리한다).

## standalone 모드 (기본값)

`run.ps1`/`run.sh`는 기본적으로 시그널링 서버를 `--standalone`으로 띄운다. 이 모드에서는 서버가
외부 의존성 두 개를 **인프로세스 fake로 대체**한다.

| 실제 의존성 | standalone 대체 | 비고 |
|---|---|---|
| Redis (voice presence) | 인프로세스 `miniredis` | 실제 Redis와 같은 프로토콜. TTL도 실제 시간에 맞춰 만료된다 |
| 내부 API `UserStoreService.GetUsers` | 요청받은 모든 `userId`를 활성 유저로 응답하는 fake | `AuthMiddleware`의 실존 유저 검증을 통과시킨다 |

핵심 효과: **어떤 userId를 입력해도 접속이 된다.** 실제 백엔드에서는 DB에 실존하는 userId만 통과하지만,
standalone에서는 참가자별로 서로 다른 정수를 아무거나 넣으면 그대로 별개 참가자로 잡힌다
(다중 접속 검증이 훨씬 쉬워진다).

대체되는 것은 외부 의존성뿐이고 핸들러·유즈케이스·시그널링 그래프는 프로덕션과 완전히 동일하므로,
검증 경로가 실제 동작에서 갈라지지 않는다.

실제 Redis/내부 API에 붙여 검증하려면:

```powershell
.\run.ps1 -WithBackend    # POSIX: ./run.sh --with-backend
```

서버를 직접 띄울 때도 동일하게 쓸 수 있다. `.env`가 없으면 `SERVER_PORT=8083`, `HEALTH_PORT=8084`,
`TLS=false`, `NAT_IP=127.0.0.1`, `JWT_SECRET`(로컬 전용 기본값)이 자동으로 채워진다.
이미 설정된 값은 덮어쓰지 않으므로 `.env`가 있으면 그 값이 항상 우선한다.

```sh
go run ./cmd/webrtc --standalone    # 또는 STANDALONE=true go run ./cmd/webrtc
```

> standalone은 **로컬 검증 전용**이다. 기동 시 경고 로그를 남기며, 운영 배포에서는 절대 켜지 않는다.

## 설정

값은 **flag > 환경변수 > 기본값** 순으로 적용된다. `.env.example`을 복사해 `tools/webrtc-adapter/.env`로
만들면 `run.ps1`/`run.sh`가 자동으로 읽어 어댑터 프로세스 환경변수로 반영한다.

| 값 | flag | env | 기본값 | 설명 |
|---|---|---|---|---|
| gRPC 서버 주소 | `--grpc-addr` | `GRPC_ADDR` | `localhost:8083` (run 스크립트는 서버 `.env`의 `SERVER_PORT`로 자동 유도) | signaling 서버 주소 |
| 리슨 주소 | `--listen-addr` | `LISTEN_ADDR` | `:8888` | 어댑터가 브라우저에 노출하는 HTTPS 주소 |
| 채널 ID | `--channel-id` | `CHANNEL_ID` | `0` | 시그널링 대상 채널 (정수) |
| JWT | `--token` | `JWT_TOKEN` | 선택 (없으면 userId 기반 발급 필요) | signaling 서버 인증에 쓰는 고정 fallback 토큰. userId 쿼리 없이 접속하는 연결이 사용 |
| JWT_SECRET | `--jwt-secret` | `JWT_SECRET` | 선택 (없으면 userId 기반 발급 불가) | userId 기반 연결별 토큰 서명 키. 서버 `.env`의 `JWT_SECRET`과 동일해야 함. `JWT_TOKEN`과 둘 중 하나는 필수 |
| TLS 인증서 | `--tls-cert` | `TLS_CERT` | `server.crt` | 어댑터 HTTPS 인증서 경로 |
| TLS 개인키 | `--tls-key` | `TLS_KEY` | `server.key` | 어댑터 HTTPS 개인키 경로 |

`index.html`은 `wss://${location.host}/signaling`으로 접속하므로 어댑터가 어떤 호스트/포트에서
떠 있든 별도 설정 없이 동작한다.

## 여러 명 접속 (userId 기반 연결별 토큰)

어댑터는 프로세스당 고정 토큰 하나만 쓰던 기존 방식 대신, `index.html`의 userId 입력창을 통해
**연결마다 즉석으로 JWT를 발급**해 하나의 어댑터 인스턴스로 여러 참가자를 동시에 접속시킬 수 있다.

- `index.html`에서 connect 전에 userId(정수)를 입력하면 WS URL이 `/signaling?userId=<입력값>`으로
  접속되고, 어댑터가 `pkg/util/token.Generate(userId, "web-<userId>")`로 연결별 토큰을 서명해
  gRPC 메타데이터에 싣는다. deviceId는 `web-<userId>`로 자동 생성된다.
- userId는 **서버에 실존하는 유저**여야 한다(서버 `AuthMiddleware`가 `GetUserByID`로 검증). 참가자별로
  서로 다른 userId를 입력해야 presence·track 라우팅이 충돌하지 않는다.
  단 standalone 모드에서는 fake가 모든 userId를 실존 유저로 응답하므로, 서로 다른 정수이기만 하면 된다.
- userId를 비운 채 connect하면 기존과 동일하게 `JWT_TOKEN`(고정 토큰)으로 접속한다(하위호환).
- 연결별 발급에는 어댑터에 `JWT_SECRET`이 설정돼 있어야 한다(아래 설정표 참고). `run.ps1`/`run.sh`는
  서버 `.env`의 `JWT_SECRET`을 자동으로 어댑터 프로세스에 주입한다.

## 인증서 (TLS)

브라우저가 마이크 권한을 요구하려면 HTTPS가 필요해 어댑터 자체가 self-signed 인증서로 `:8888`을
서빙한다. `san.cnf`는 `localhost`/`127.0.0.1`을 SAN으로 포함한다.

```powershell
.\ssl.ps1   # 또는 ./ssl.sh
```

생성된 `server.key`/`server.crt`는 로컬 전용이며 `.gitignore`에 포함되어 커밋되지 않는다.
(이전에 저장소에 커밋되어 있던 `server.key`/`server.crt`/`server.csr`/`cert.pem`/`key.pem`은
제거했다. 필요하면 위 스크립트로 새로 발급한다.)

## JWT 발급

서버의 `AuthMiddleware`는 `pkg/util/token.Validate`로 토큰을 검증한다. 이 토큰은
`userId`(number) + `deviceId`(string) claim을 `JWT_SECRET`(서버 `.env`와 동일한 값)으로
HS256 서명한 것이어야 한다. `cmd/gen-token`이 동일한 규칙(`pkg/util/token.Generate`)으로 토큰을 만든다.

```sh
go run ./cmd/gen-token --jwt-secret "<서버 .env의 JWT_SECRET>" --user-id 1 --device-id local-test-device
```

`run.ps1`/`run.sh`는 `tools/webrtc-adapter/.env`에 `JWT_TOKEN`이 없을 때 이 CLI를 자동으로 호출한다.

## 수동 실행 (원클릭 스크립트 없이)

```sh
# 1) 저장소 루트에서 서버 기동
go run ./cmd/webrtc

# 2) tools/webrtc-adapter에서 어댑터 기동
cd tools/webrtc-adapter
go run . --token "<gen-token으로 발급한 JWT>"
```

## 트러블슈팅

| 증상 | 원인 | 조치 |
|---|---|---|
| `Redis에 연결할 수 없습니다` / `내부 API 서버에 연결할 수 없습니다` | `-WithBackend`로 실행했는데 로컬 백엔드 미기동 | `-WithBackend` 없이 실행해 standalone으로 검증하거나, 서버 `.env`의 `REDIS_HOST/PORT`, `API_SERVER_HOST/PORT`가 실제로 로컬에서 응답하는지 확인 |
| `어댑터 인증서가 없습니다` | `ssl.ps1`/`ssl.sh` 미실행 | 먼저 인증서 발급 스크립트를 실행 |
| `JWT_TOKEN(고정 토큰) 또는 JWT_SECRET...중 하나가 필요합니다` | `--token`/`JWT_TOKEN`, `--jwt-secret`/`JWT_SECRET` 모두 미설정 | 둘 중 하나를 설정(`cmd/gen-token`으로 고정 토큰 발급 또는 `JWT_SECRET` 지정) |
| gRPC `Unauthenticated` | 토큰의 `JWT_SECRET`이 서버 `.env`와 다름 | 고정 토큰이면 `gen-token --jwt-secret`에, userId 기반이면 어댑터 `JWT_SECRET`에 서버와 동일한 값을 지정 |
| userId 입력했는데 `Unauthenticated`/연결 실패 | 실존하지 않는 userId를 입력했거나, 어댑터 `JWT_SECRET`이 서버 `.env`와 불일치 | 서버에 존재하는 userId인지 확인하고, `JWT_SECRET` 값을 서버 `.env`와 맞춤(run 스크립트는 자동 주입) |
| 브라우저가 마이크 권한을 요구하지 않음 / 접속 실패 | `https://`가 아닌 `http://`로 접속 | 반드시 `https://localhost:<포트>/`로 접속하고 인증서 경고를 수락 |

## 개발 시 테스트

WS Message ↔ `signalingv1` 변환 로직(`buildSignalingRequest`/`buildWSMessage`)은 순수 함수로 분리되어
있어 네트워크 없이 단위 테스트한다.

```sh
cd tools/webrtc-adapter
go build ./...
go vet ./...
go test ./...
```

> **주의 — `go mod tidy` 금지:** 이 모듈은 `go.work`로 저장소 루트의 `pkg/gen`을 참조하며 메인 모듈에 대한 explicit `require`가 없다. 현재 툴체인에서 `go mod tidy`를 실행하면 워크스페이스 멤버에 대한 원격 조회(`git ls-remote`)가 시도돼 실패한다. 검증은 `go mod tidy` 없이 위 `build`/`vet`/`test` 세 명령으로만 한다.
