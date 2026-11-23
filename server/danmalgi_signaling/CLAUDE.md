# 모델 사용 지침

- **설계 및 복잡한 작업**: opus 모델을 사용한다. (아키텍처 설계, 복잡한 문제 해결, 여러 요소를 고려해야 하는 판단 등)
- **구현 위주 및 비교적 가벼운 작업**: sonnet 모델을 사용한다. (단순 코드 구현, 가벼운 수정, 반복적인 작업 등)

# 테스트 코드 유지 지침

- 기능의 **수정/제거/추가**가 발생하면 관련 **테스트 코드도 반드시 함께 업데이트**한다.
  - **기능 추가**: 새 기능을 검증하는 테스트 코드를 함께 작성한다.
  - **기능 수정**: 변경된 동작에 맞게 기존 테스트 코드를 수정한다.
  - **기능 제거**: 더 이상 유효하지 않은 테스트 코드를 함께 제거한다.
- 기능 변경과 테스트 코드 업데이트는 **하나의 작업 단위로 처리**하며, 테스트 업데이트를 누락한 채 작업을 완료하지 않는다.

# 프로젝트 규칙 및 함정

## 보조 도구(`tools/`) — gRPC 계약 소비

- **proto 단일 소스:** 서버의 gRPC 계약을 소비하는 보조 도구·모듈은 자체 proto 사본을 두지 않고 프로젝트 `pkg/gen`만 참조한다. 사본을 두면 서버 계약과 어긋나 연결 자체가 불가능해진다(경계면은 항상 서버 계약 단일 소스에 정렬).
- **워크스페이스 참조:** 보조 도구를 별도 Go 모듈로 둘 때는 `go.work`의 `use`에 저장소 루트를 포함해 `pkg/gen`을 참조하고, 메인 모듈에 대한 explicit `require`는 추가하지 않는다. 현재 툴체인에서는 require를 추가하거나 `go mod tidy`를 실행하면 워크스페이스 멤버에 대한 원격 조회가 시도돼 빌드가 깨진다 — 이런 모듈은 `go mod tidy` 대신 `go build`/`go vet`/`go test`로만 검증한다.

## 보조 도구(`tools/`) — 서버 신원·인증 경계

- **식별 축(identity axis) 정렬:** 서버는 세 개의 독립된 식별 축을 쓰며, signaling 연결을 여는 어댑터/클라이언트 도구는 셋을 함께 맞춰야 한다 — (1) **신원 검증**: `AuthMiddleware`가 토큰의 `userId`로 `GetUserByID`를 호출하므로 userId는 **반드시 실존 유저**여야 한다(없으면 연결 거부). (2) **presence**: Redis 해시 `channel:%d:voice`에 **field=deviceId**로 저장되므로 두 연결의 deviceId가 같으면 presence가 한 칸으로 충돌하고 한쪽 종료가 상대 presence까지 삭제한다 → deviceId는 **연결별 유니크**. (3) **track 라우팅**: `track.User.ID == wrapper.UserId`로 소유자를 판정하므로 같은 userId 두 연결은 서로의 트랙을 자기 것으로 오인한다 → 참가자별 **userId 유니크**. ⇒ 새 클라이언트 경로를 만들 때 userId(실존·유니크)·deviceId(유니크, 예 `web-<userId>`) 세 축을 한꺼번에 점검한다.
- **`token.Generate`의 env 의존:** `pkg/util/token`의 `Generate`/`Validate`는 호출 시점에 `os.Getenv("JWT_SECRET")`을 읽는다. 이 함수를 쓰는 도구는 서명·검증 이전에 JWT_SECRET이 **프로세스 env에 존재**해야 하며, 플래그(`--jwt-secret`)로 받은 값은 `main()` 초입에서 `os.Setenv("JWT_SECRET", ...)`로 정규화한다(안 하면 빈 시크릿으로 서명돼 서버 검증만 조용히 실패). 서명 시점(연결 핸들러)이 항상 정규화 이후임을 보장할 것.
- **run 스크립트 env 주입 루프의 drift:** `run.sh`/`run.ps1`은 명시된 키 집합만 어댑터 프로세스에 주입한다(`run.sh`의 `GRPC_ADDR LISTEN_ADDR CHANNEL_ID JWT_TOKEN TLS_CERT TLS_KEY` 루프). 어댑터가 새 env(예: JWT_SECRET)에 의존하게 되면 이 루프나 별도 `export`도 함께 갱신해야 한다 — 안 그러면 수동 실행은 되는데 스크립트 기동만 조용히 실패한다. 값의 **권위 소스는 서버 `.env`**로 두고 거기서 읽어 주입해 drift를 막는다.

## signaling 채널 수명주기 — 다중 sender 채널

- **소유자가 데이터 채널을 close하지 않는다.** `PeerConnectionWrapper`의 `Offer`/`Candidate`/`Err`는 여러 goroutine(Pion 콜백·handler·usecase)이 동시에 send하는 채널이다. Go 관례상 다중 sender 채널을 receiver/소유자가 close하면, close 이후 다른 goroutine의 send가 `send on closed channel` panic을 낸다 — 이 panic은 별도 goroutine에서 터져 gRPC recovery interceptor로도 못 잡고 **프로세스 전체를 죽인다.** 수명주기 종료는 데이터 채널 close가 아니라 `wrapper.Close()`(= `close(done)`)로만 신호한다.
- **send는 반드시 `EnqueueOffer`/`EnqueueCandidate`/`EnqueueErr`를 쓰고 직접 `ch <-` 를 쓰지 않는다.** 이 메서드들이 `select { case ch <- v: case <-done: }`로 감싸 close 이후 send를 panic 없이 drop하고(반환 `false`), receiver 소멸 후 blocking send의 goroutine 누수도 막는다. 새 send 지점이나 새 채널을 추가할 때도 동일 규칙을 적용한다(원리: 다중 concurrent sender 채널 = owner-close 금지 + done 방어 send).

## signaling 재협상 — wrapper 단위 직렬화와 스냅샷 계약

- **재협상은 wrapper 단위로 직렬화하고, 두 임계구역을 같은 뮤텍스로 묶는다.** `SignalingState() == stable` 확인 + `CreateOffer` + `SetLocalDescription`이 한 임계구역(`Negotiate`), `SetRemoteDescription` + pending 플래그 소거가 다른 임계구역(`ApplyAnswer`)이며 **둘은 반드시 같은 락**(`negotiationMu`)이어야 한다. 한쪽이라도 락 밖에 있으면 lost-wakeup 창이 열린다 — `ApplyAnswer`가 pending을 소비한 직후 `Negotiate`가 non-stable을 관측해 pending을 다시 세우면 이를 깨울 answer가 영영 오지 않아 **그 참가자의 갱신이 영구 정지**한다(에러도 panic도 없이 조용히 멈추므로 사후 진단이 어렵다). 새 offer 발행 경로를 만들 때 `PeerConnection`을 직접 만지지 말고 `Negotiate`/`ApplyAnswer`를 경유한다.
- **언버퍼드 채널 send는 락 밖에서 한다.** `EnqueueOffer` 등 `Enqueue*` 호출은 `Negotiate`가 반환한 **뒤**에 한다. 락을 쥔 채 언버퍼드 채널에 send하면 팬아웃 goroutine이 클라이언트 읽기 속도에 묶여 그 wrapper의 재협상 전체가 정체된다(위 다중 sender 채널 규칙과 짝: send 자체는 항상 `Enqueue*` 경유).
- **팬아웃 페이로드는 델타가 아니라 전송 시점 스냅샷으로 싣는다.** 전달이 실패·지연·유실될 수 있는 경로(재협상 glare, 닫힌 wrapper, 로그만 남는 실패)에서 "이번에 바뀐 것"만 실으면 **한 번의 실패 = 영구 유실**이라 수신자 상태가 되돌릴 방법 없이 어긋난다. 전송 직전에 권위 소스(예: `signalingManager.GetTracks`)에서 전체 상태를 다시 떠 replace 계약으로 보내면 실패한 전송을 다음 성공 전송이 자동 복구한다. 이때 **빈 상태도 반드시 전송**한다 — 빈 목록과 필드 부재가 구분되면 "마지막 항목 제거"를 표현할 수 없으므로 mapper는 항상 non-nil로 매핑한다.

## signaling 채널 상태 — 생성 부작용이 있는 shard 컨테이너

- **조회 경로는 컨테이너를 만들지 않는 접근자를 쓴다.** `SignalingManager.getOrCreateShard`는 읽기·teardown 경로(`Gets`/`GetTracks`/`RemoveTrack`)에서도 호출되며 없으면 `ChannelShard`를 **새로 만든다.** 외부 입력(`channel_id`)으로 조회하는 새 rpc·메서드가 이 함수를 타면 임의 ID 조회마다 shard가 쌓여 `shards` map이 무한 성장한다(메모리 누수 + DoS 표면) → `s.mu.RLock()`으로 map을 직접 조회하고 부재 시 "값 없음"을 반환하는 읽기 전용 접근자를 따로 만든다(예: `GetCallStartedAt`).
- **세션 경계 값은 컨테이너 생성 시점이 아니라 0→1 전이 시점에 기록한다.** 컨테이너가 읽기 부작용으로도 생기므로 "shard가 생겼다 = 통화가 시작됐다"가 성립하지 않는다. `callStartedAt` 같은 세션 시작 값은 `Save`의 `len(shard.wrappers) == 0` 분기(= 첫 참가자 입장)에서만 기록하고, 길이 판정과 대입을 `shard.mu.Lock()` **한 임계구역 안**에 둔다(락 순서는 `s.mu` → `shard.mu`로 통일 — 역순 획득 금지).
- **"컨테이너 존재"를 "상태 있음"으로 읽지 않는다 — 참가자 수로 판정한다.** 마지막 참가자가 나가면 `Delete`가 shard를 map에서 지우지만, 직후 pion teardown 콜백이 `RemoveTrack`/`Gets`를 호출해 **빈 shard를 되살린다**(부활한 빈 shard를 회수하는 주체는 현재 없다). 따라서 조회는 `len(shard.wrappers) == 0`을 반드시 검사해야 한다 — 빠뜨리면 zero time을 유효 값으로 반환해 "약 2000년 통화 중" 같은 **에러도 로그도 없는 조용한 오답**이 난다. 이 가드는 편의적 이중 안전장치가 아니라 도달 가능 경로의 **필수 가드**이며, 지울 때는 부활 경로가 사라졌음을 먼저 증명해야 한다.

## Windows 개발환경

- 한글 등 비ASCII 문자열을 포함한 PowerShell 스크립트(`.ps1`)는 **UTF-8 BOM 포함**으로 저장한다. BOM이 없으면 PowerShell 5.1 파서가 시스템 코드페이지로 오인해 파싱에 실패한다.
- **`-race` 등 cgo 의존 명령은 PowerShell 툴로 실행한다.** Bash 툴 sandbox가 `-race`가 필요로 하는 `cgo.exe` 자식 프로세스 스폰을 차단해 `runtime/cgo: ...cgo.exe: exit status 2`로 실패한다(코드 결함이 아니라 sandbox 제약). 동일 명령이 PowerShell 툴에서는 바로 성공한다. `-race`/cgo 명령이 Bash에서 이 에러로 죽으면 코드를 의심하지 말고 PowerShell 툴로 재실행하라.
- **포맷 판정은 CRLF 노이즈를 "제거한 뒤" 한다 — 노이즈를 이유로 신호를 기각하지 않는다.** 저장소가 `core.autocrlf=true`로 CRLF 체크아웃되어 있어 (1) `gofmt -l ./...`은 이번에 손대지 않은 파일까지 전부 flag하고, (2) CRLF 파일에 `gofmt -d`를 걸면 **파일 전체가 재작성 형태로 출력돼 그 안에 섞인 실제 위반(예: import 정렬)이 가려진다.** 따라서 "CRLF 함정 패턴과 일치한다"는 이유로 flag를 통째로 기각하면 진짜 위반이 조용히 통과한다. 올바른 절차: 변경한 파일을 `tr -d '\r'`로 LF 정규화한 **임시 사본에 `gofmt -l`**(권장), 또는 `gofmt -d` 출력에서 줄바꿈 외의 실제 추가·삭제 라인만 보고 판정. 어느 경우에도 `gofmt -l` 노이즈에 반응해 **저장소 전반을 재포맷(대량 diff)하지 말 것**(줄바꿈 정책 정리는 별도 사이클).

## 하네스: 개발 워크플로우

**목표:** 기능 추가·버그 수정·개선을 brainstorming→plan→work→review→compound 5단계 파이프라인으로 진행하고, 매 사이클의 교훈을 시스템에 축적한다.

**트리거:** 새 개발 작업을 시작하거나("이 기능 개발해줘", "이 버그 잡자", "하네스 돌려줘") 기존 사이클의 후속/부분 재실행("다시 실행", "계획만 다시", "특정 단계만")을 요청받으면 `dev-workflow` 스킬을 사용하라. 단순 질문·일회성 편집은 직접 응답 가능.

**변경 이력:**
| 날짜 | 변경 내용 | 대상 | 사유 |
|------|----------|------|------|
| 2026-07-14 | 초기 구성 (5단계 파이프라인 골격) | 전체 | - |
| 2026-07-14 | 프로젝트 규칙 추가: 보조 도구 `pkg/gen` 단일 참조 + `go.work` explicit require 생략(`go mod tidy` 금지), PS `.ps1` UTF-8 BOM 저장 | CLAUDE.md | `webrtc-adapter-testing` 사이클(하네스 첫 실전)에서 드러난 재발성 함정 축적 |
| 2026-07-16 | (1) 프로젝트 규칙 추가: signaling 다중 sender 채널 수명주기 규칙(owner-close 금지 + `Enqueue*`/`Close()` 강제). (2) Windows 함정 추가: `-race`/cgo는 PowerShell 툴로 실행(Bash sandbox `cgo.exe` 차단), `gofmt -l` 전체 결과로 포맷 판정 금지(CRLF). (3) `wf-work`·`wf-review` 원칙 추가: IDE(gopls) 진단이 실제 빌드와 어긋나면 `go build`/`go test` 결과 신뢰 | CLAUDE.md, wf-work, wf-review | `signaling-concurrency-fix` 사이클에서 드러난 재발성 함정(방금 고친 crash의 회귀 방지 + 검증 신호 신뢰성) 축적 |
| 2026-07-19 | 프로젝트 규칙 추가: 보조 도구의 서버 신원·인증 경계 3종 — (1) 식별 축 정렬(userId 실존·유니크, deviceId 유니크; 신원검증/presence/track 라우팅 세 축 동시 점검), (2) `token.Generate`의 `os.Getenv("JWT_SECRET")` 호출시점 의존 → 플래그값 `os.Setenv` 정규화, (3) run 스크립트 env 주입 루프 drift 방지(새 env 의존 시 루프/export 동반 갱신, 서버 `.env` 권위 소스) | CLAUDE.md | `adapter-per-connection-token` 사이클에서 드러난 재발성 경계 지식(연결별 신원 발급 도구가 서버 식별 모델과 정렬돼야 함) 축적 |
| 2026-08-17 | (1) 프로젝트 규칙 추가: signaling 재협상 규칙 3종 — wrapper 단위 직렬화(`Negotiate`/`ApplyAnswer` 동일 뮤텍스, lost-wakeup 창 폐쇄), 언버퍼드 `Enqueue*`는 락 밖, 팬아웃 페이로드는 델타가 아닌 전송 시점 스냅샷(빈 상태도 non-nil로 전송). (2) `wf-plan` 원칙 추가: 부작용을 겸하는 루프에 skip을 넣기 전 그 루프의 다른 부작용 확인, skip 적용 층위까지 지정. (3) `wf-work` 원칙 추가: 테스트 하네스는 프로덕션 진입점 경유(재구현 시 미검증 명시). (4) `wf-review` 원칙 추가: "지워도 녹색인가"로 커버리지 공백 판정. (5) `wf-work`·`wf-review` 교정: `-race`의 PowerShell 우회는 **Windows 한정**임을 명시(무조건문이라 Linux에서 오해 소지), stale gopls 진단 예시에 파일 개명·이동 직후 케이스 추가 | CLAUDE.md, wf-plan, wf-work, wf-review | `signaling-fanout-snapshot` 사이클(이슈 #5)에서 드러난 재발성 동시성·계약 지식과 하네스 마찰 축적 |
| 2026-09-08 | (1) Windows 함정 **정정**: "`gofmt -l` 전체 결과로 판정 금지"가 "gofmt 결과 전체 무시"로 확대 적용돼 실제 위반(import 정렬)을 통과시켰다 → 금지문을 절차문으로 교체(변경 파일을 LF 정규화한 사본에 `gofmt -l`; CRLF 파일의 `gofmt -d`는 파일 전체 재작성으로 보여 실제 위반을 가림). (2) 프로젝트 규칙 추가: 생성 부작용이 있는 shard 컨테이너 3종 — 조회는 `getOrCreateShard` 미사용 읽기 전용 접근자, 세션 경계 값은 컨테이너 생성이 아닌 0→1 전이 시점 기록(`s.mu`→`shard.mu` 락 순서), "컨테이너 존재 ≠ 상태 있음"(teardown이 부활시킨 빈 shard 대비 참가자 수 가드는 필수). (3) `wf-brainstorm` 원칙 추가: 새 계약 표면의 첫 사례(스트림만 있던 서버의 첫 unary 등) 추가 시 그 표면 종류의 인터셉터/미들웨어 체인 활성·대칭 여부를 점검해 선행 작업화. (4) `wf-plan` 원칙 추가: 커버리지 자기검증 목록은 "그 상태를 만드는 부수 경로"를 열거해 승격(skip 원칙의 읽기 경로 버전). (5) `wf-review` 원칙 강화: "지워도 녹색인가"를 자문 → **실제 mutation 실행 + 도달 경로 판정 + 바이트 단위 원상복구 증명**, 포맷 판정 절차 명시. (6) `wf-work` 원칙 추가 + `reviewer` 정의 보강: gofmt 판정 절차, 검증용 임시 mutation은 원상복구 증명 조건으로 허용 | CLAUDE.md, wf-brainstorm, wf-plan, wf-work, wf-review, agents/reviewer | `channel-voice-start-time` 사이클에서 드러난 마찰 축적 — 예외 규칙의 과잉 일반화가 실제 위반을 가린 사례, 읽기 경로의 생성 부작용에서 오는 상태 오판, 계획 단계 커버리지 목록의 구조적 공백(리뷰어 mutation이 실증) |
