# danmalgi_webrtc_server

`danmalgi_webrtc_server`는 Go로 구현된 고성능 WebRTC 서버입니다. 시그널링 및 기타 상호작용을 위해 gRPC를 사용하여 실시간 통신 기능을 제공합니다.

## 기능

*   **WebRTC 시그널링:** WebRTC 연결 설정을 위해 필요한 정보 교환을 처리합니다.
*   **사용자 인증:** 안전한 사용자 로그인 및 인증.
*   **실시간 채팅:** 방 내에서 그룹 채팅 기능.
*   **다이렉트 메시징:** 일대일 비공개 메시징.
*   **친구 시스템:** 친구 목록 및 관계 관리.
*   **방:** 통신 방 생성 및 관리.
*   **사용자 관리:** 기본적인 사용자 프로필 및 상태 관리.

## 기술 스택

*   **언어:** [Go](https://golang.org/)
*   **API:** [gRPC](https://grpc.io/)
*   **DI:** [uber-go/fx](https://github.com/uber-go/fx)
*   **WebRTC:** [pion/webrtc](https://github.com/pion/webrtc)
*   **캐시/브로커:** [Redis](https://redis.io/)
*   **컨테이너화:** [Docker](https://www.docker.com/)

## 프로젝트 구조

```
.
├── cmd/webrtc/       # 주 애플리케이션 진입점 (fx 부트스트랩)
├── internal/         # 비공개 애플리케이션 로직
│   ├── client/       #   내부 API(gRPC) 클라이언트
│   ├── domain/       #   도메인 모델 및 에러
│   ├── handler/      #   gRPC 핸들러 (진입 계층)
│   ├── manager/      #   PeerConnection 세션 관리
│   ├── redis/        #   Redis 커넥션
│   ├── server/       #   gRPC 서버 및 미들웨어(인터셉터)
│   ├── signaling/    #   WebRTC 시그널링 이벤트 로직
│   └── usecase/      #   유스케이스 인터페이스 및 service 구현
├── pkg/              # 공유 라이브러리 및 유틸리티
│   ├── gen/          #   proto에서 생성된 Go 코드 (public/private)
│   ├── log/          #   로깅
│   └── util/         #   공용 유틸 (ctxutil, rootdir, token)
├── proto/            # proto 정의 (git submodule: public / private)
├── tools/
│   └── webrtc-adapter/  # 로컬 브라우저 시그널링 테스트용 WS↔gRPC 브릿지 (독립 Go 모듈, 자세한 내용은 해당 README 참고)
├── Dockerfile        # 컨테이너 빌드 지침
└── go.mod            # Go 모듈 정의
```

## 시작하기

### 전제 조건

*   Go 1.24 이상
*   Redis 및 내부 API 서버 (standalone 모드로 실행하면 필요 없습니다 — 아래 "standalone 모드" 참고)
*   Protocol Buffers 컴파일러 (`protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`)

### 설치

1.  **리포지토리 복제 (submodule 포함):**
    ```sh
    git clone --recurse-submodules https://github.com/danmalgi-project/danmalgi_webrtc_server.git
    cd danmalgi_webrtc_server
    ```
    이미 복제한 경우 submodule을 초기화합니다.
    ```sh
    git submodule update --init --recursive
    ```

2.  **종속성 설치:**
    ```sh
    go mod tidy
    ```

3.  **gRPC 코드 생성:**
    proto 정의는 submodule로 `proto/public/`, `proto/private/`에 있습니다. Claude Code에서 `/proto-build` 명령을 실행하면 `.proto` 정의로부터 `pkg/gen/`에 Go 코드를 생성합니다.

4.  **환경 설정:**
    루트 디렉토리에 `.env` 파일을 생성합니다. Redis 연결 세부 정보 및 서버 포트와 같은 환경 변수를 지정해야 합니다.

5.  **서버 실행:**
    ```sh
    go run ./cmd/webrtc
    ```

### standalone 모드 (로컬 검증 전용)

외부 Redis와 내부 API 서버 없이 이 저장소만으로 서버를 띄울 수 있습니다.
`--standalone`을 주면 두 외부 의존성이 **인프로세스 fake로 대체**됩니다.

```sh
go run ./cmd/webrtc --standalone    # 또는 .env에 STANDALONE=true
```

| 실제 의존성 | standalone 대체 |
|---|---|
| Redis (voice presence) | 인프로세스 `miniredis` (TTL 포함 동일 동작) |
| 내부 API `UserStoreService.GetUsers` | 요청받은 모든 `userId`를 활성 유저로 응답하는 fake |

`.env`가 없어도 `SERVER_PORT=8083`, `HEALTH_PORT=8084`, `TLS=false`, `NAT_IP=127.0.0.1`,
`JWT_SECRET`(로컬 전용 기본값)이 자동으로 채워집니다. 이미 설정된 값은 덮어쓰지 않으므로
`.env`가 있으면 그 값이 항상 우선합니다.

대체되는 것은 외부 의존성뿐이고 핸들러·유즈케이스·시그널링 그래프는 프로덕션과 동일합니다.
**로컬 검증 전용이며 운영 배포에서는 켜지 않습니다** (기동 시 경고 로그를 남깁니다).

### Docker

Docker를 사용하여 서버를 빌드하고 실행할 수도 있습니다.
```sh
docker build -t danmalgi_webrtc_server .
docker run -p [HOST_PORT]:[CONTAINER_PORT] --env-file .env danmalgi_webrtc_server
```

## 구성

서버는 환경 변수를 사용하여 구성됩니다. 루트 디렉토리에 `.env` 파일을 생성하고 다음 변수를 설정합니다.

| 변수               | 설명                                                                   | 기본값    |
| ----------------- | ---------------------------------------------------------------------- | --------- |
| `SERVER_PORT`     | gRPC 서버가 수신할 포트.                                               |           |
| `HEALTH_PORT`     | health check(`/health`) HTTP 서버가 수신할 포트.                       |           |
| `TLS`             | 보안 연결을 위한 TLS를 켜거나 끕니다. (`true`/`false`)                 | `false`   |
| `TLS_CERT`        | TLS 인증서 파일 경로. (`TLS=true`일 때 필요)                          |           |
| `TLS_KEY`         | TLS 개인 키 파일 경로. (`TLS=true`일 때 필요)                         |           |
| `NAT_IP`          | WebRTC의 NAT 순회를 위한 서버의 공용 IP 주소.                        |           |
| `JWT_SECRET`      | JSON 웹 토큰(JWT) 서명 및 확인에 사용되는 비밀 키.                   |           |
| `REDIS_HOST`      | Redis 서버의 호스트 이름 또는 IP 주소.                               |           |
| `REDIS_PORT`      | Redis 서버가 실행 중인 포트.                                         | `6379`    |
| `API_SERVER_HOST` | 이 서버가 통신하는 내부 API 서버의 호스트 이름.                      | `localhost` |
| `API_SERVER_PORT` | 내부 API 서버의 포트.                                                | `8080`    |
| `LOG_DIR`         | 로그 파일이 기록될 디렉토리. (미설정 시 `<root>/logs`)               |           |
| `STANDALONE`      | `true`면 Redis·내부 API를 인프로세스 fake로 대체 (로컬 검증 전용). `--standalone` 플래그와 동일. | `false`   |

## API

서버의 API는 gRPC로 정의되며, proto 정의는 submodule로 관리됩니다. 생성된 Go 코드는 `pkg/gen/`에 있습니다.

*   **`proto/public/`** — 클라이언트가 사용하는 공개 API
    *   `signaling/v1/signaling.proto`: WebRTC 시그널링 서비스 (이 서버가 제공)
    *   `auth/v1/auth.proto`, `user/v1/user.proto`, `chat/v1/chat.proto`, `dm/v1/dm.proto`, `friend/v1/friend.proto`, `relationship/v1/relationship.proto`
*   **`proto/private/`** — 내부 서비스 간 통신용 API (이 서버가 클라이언트로 호출)
    *   `user_store/v1`, `device_store/v1`, `dm_store/v1`

이 서버가 직접 제공하는 gRPC 서비스는 `SignalingService`이며, 나머지 내부(`*_store`) 서비스는 `API_SERVER_HOST`의 내부 API 서버로 호출합니다.

## 로컬 브라우저 시그널링 테스트

브라우저에서 실제 WebRTC 시그널링을 눈으로 확인하려면 `tools/webrtc-adapter`(WS↔gRPC 브릿지 + 웹 클라이언트, 원클릭 실행 스크립트 포함)를 사용하세요. 자세한 사용법은 [`tools/webrtc-adapter/README.md`](tools/webrtc-adapter/README.md)를 참고하세요.
