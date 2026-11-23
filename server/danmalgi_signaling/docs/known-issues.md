# 알려진 이슈 / 후속 작업

재발성·구조적 위험이지만 별도 사이클로 다뤄야 하는 항목을 기록한다. dev-workflow 사이클의 compound 단계에서 도출된 후속 후보가 여기로 승격된다.

---

## [우선] 서버측: 존재하지 않는 userId → `GetUsers` 빈 응답에서 인덱스 패닉

- **위치:** `internal/usecase/service/user_service.go:44` — `proto := response.Users[0]`
- **증상:** `GetUserByID`가 Redis 캐시 미스 시 내부 `user_store` gRPC의 `GetUsers`를 호출하고, 응답의 `response.Users[0]`을 무조건 인덱싱한다. 조회한 userId가 실존하지 않아 `Users`가 빈 슬라이스로 오면 **index out of range 패닉**이 난다.
- **호출 경로(패닉 도달점):**
  - `AuthMiddleware`(`internal/server/middleware/auth_interceptor.go:34`) — 모든 signaling 연결이 토큰의 userId로 `GetUserByID`를 거친다.
  - `OnTrack` 이벤트(`internal/signaling/event.go:46`) — 트랙 수신 시에도 호출.
- **왜 이제 위험이 커졌나:** `adapter-per-connection-token` 사이클(2026-07-19)로 `tools/webrtc-adapter`가 **웹에서 받은 임의 userId로 JWT를 즉석 서명**해 서버로 보낸다. 서명은 유효하므로 `token.Validate`는 통과하고, 실존하지 않는 userId가 곧장 `GetUserByID`까지 도달한다 → 잘못된 userId 입력만으로 서버 패닉을 유발할 수 있는 표면이 생겼다.
- **제안 방어(구현은 별도 사이클):**
  1. `user_service.go:44`에서 `len(response.Users) == 0`을 먼저 검사해 도메인 에러(예: `ErrUserNotFound`)를 반환하고, `AuthMiddleware`가 이를 `codes.Unauthenticated`/`NotFound`로 매핑하도록 한다.
  2. gRPC recovery interceptor가 이 패닉을 잡는지 확인(별도 goroutine에서 터지는 `OnTrack` 경로는 못 잡을 수 있음 — signaling 다중 sender 채널 규칙과 동일한 함정 계열).
  3. 회귀 테스트: 빈 `Users` 응답에 대해 패닉 대신 에러를 반환하는지 검증.
- **테스트 유지 지침:** 이 방어를 구현할 때 `user_service` 단위 테스트를 함께 추가/수정한다(CLAUDE.md).

---

## 기타 후속 후보 (낮은 우선순위)

- **index.html connect 재클릭 시 리소스 누적:** `tools/webrtc-adapter/index.html`의 connect 버튼을 다시 누르면 이전 `RTCPeerConnection`·이벤트 리스너가 정리되지 않고 누적된다. 동작 자체는 유지되나, 반복 테스트 시 정리 로직 추가 여지. (출처: `adapter-per-connection-token` 사이클 리뷰 Nit.)
- **run 스크립트 JWT_SECRET 주입 라이브 검증:** `run.sh`/`run.ps1`이 서버 `.env`의 JWT_SECRET을 어댑터 프로세스에 주입하는 경로를, 전제 환경(로컬 API/Redis 기동)이 갖춰진 상태에서 end-to-end로 실검증. (자동 테스트 범위 밖이라 사이클 내 미실행.)
