---
name: dev-workflow
description: danmalgi-webrtc-server의 개발 워크플로우 하네스 오케스트레이터. 기능 추가/버그 수정/개선을 brainstorming→plan→work→review→compound 5단계 파이프라인으로 진행한다. "이 기능 개발해줘", "이 버그 워크플로우로 잡자", "하네스 돌려줘", "개발 파이프라인 시작", "기능 만들자"처럼 새 개발 작업을 시작할 때 사용하라. 또한 "다시 실행", "이어서", "계획만 다시", "리뷰부터 다시", "특정 단계만", "이전 결과 기반으로 보완"처럼 기존 사이클의 후속/부분 재실행 요청에도 사용하라. 단순 질문·일회성 편집에는 쓰지 않는다.
---

# dev-workflow — 개발 워크플로우 오케스트레이터

danmalgi-webrtc-server의 개발을 **brainstorming → plan → work → review → compound** 파이프라인으로 조율한다. "누가 언제 어떤 순서로 협업하는가"를 정의한다. 각 단계의 "어떻게"는 해당 `wf-*` 스킬과 `.claude/agents/*.md`에 있다.

## 실행 모드 (하이브리드)

파이프라인은 순차 의존이다. 단계별 특성에 따라 실행 모드를 나눈다:

| 단계 | 에이전트 | 실행 모드 | 이유 |
|------|----------|----------|------|
| 1 brainstorm | brainstormer | **인라인** (메인이 `wf-brainstorm` 로드) | 사용자 대화가 핵심 (질문/확정) |
| 2 plan | planner | **인라인** (메인이 `wf-plan` 로드) | 계획 승인에 사용자 개입 |
| 3 work | worker | **서브에이전트** (`Agent`, sonnet) | 독립 구현, 결과만 반환 |
| 4 review | reviewer | **서브에이전트** (`Agent`, opus) | 독립 검수 |
| 5 compound | compounder | **서브에이전트** (`Agent`, opus) | 독립 시스템 반영 |

서브에이전트 호출 시 `subagent_type`으로 해당 에이전트를 지정하고 `model`을 위 표대로 명시한다. work는 기본 sonnet이되, 계획이 아키텍처 수준 변경이면 opus로 오버라이드한다.

## 데이터 전달 프로토콜 (파일 기반)

- 사이클마다 `.claude/workflow/<slug>/` 폴더를 만든다 (`<slug>`는 작업을 대표하는 kebab-case).
- 단계 산출물: `01_brainstorm.md` · `02_plan.md` · `03_work.md` · `04_review.md` · `05_compound.md`.
- 각 단계는 **이전 단계의 파일을 입력으로 읽는다.** 서브에이전트에는 슬러그 경로를 프롬프트에 넘겨, 필요한 파일을 스스로 읽게 한다.
- 코드 변경은 실제 작업 트리에 반영되고, `03_work.md`는 그 요약/근거를 담는다.
- 중간 파일은 보존한다 (사후 검증·감사 추적, compound의 입력).

## Phase 0: 컨텍스트 확인

시작 시 실행 모드를 판별한다:

1. `.claude/workflow/`에 관련 `<slug>` 폴더가 **없음** → **초기 실행** (Phase 1부터).
2. 폴더가 **있고** 사용자가 특정 단계/부분 수정을 요청 → **부분 재실행** (해당 단계 스킬/에이전트만 재호출, 이전 산출물을 읽고 개선).
3. 폴더가 **있고** 사용자가 새 작업(다른 주제)을 요청 → **새 사이클** (새 `<slug>` 폴더 생성).

어느 경우인지 애매하면 사용자에게 한 번 확인한다.

## Phase 1~5: 파이프라인 실행

### Phase 1 — brainstorm (인라인)
`wf-brainstorm` 스킬을 로드해 수행. 미해결 질문은 `AskUserQuestion`으로 사용자에게 확정. → `01_brainstorm.md`.
**게이트:** 문제 정의와 접근이 사용자 확인을 받았는가? 아니면 여기서 멈추고 확인.

### Phase 2 — plan (인라인)
`wf-plan` 스킬을 로드해 수행. `01`을 입력으로 계획 작성. → `02_plan.md`.
**게이트:** 사용자에게 계획을 제시하고 승인받는다. 승인 전에는 work로 넘어가지 않는다.

### Phase 3 — work (서브에이전트)
`worker`(sonnet, 복잡하면 opus)를 `Agent`로 호출. 프롬프트에 `<slug>` 경로를 주고 `02_plan.md`를 실행하게 한다. → 코드 변경 + `03_work.md`.

### Phase 4 — review (서브에이전트)
`reviewer`(opus)를 `Agent`로 호출. `01`·`02`·`03`과 `git diff`를 검수. → `04_review.md`.

**재작업 루프 (핵심):**
- `04_review.md` 판정이 **재작업 필요**면 → blocking 항목을 담아 `worker`를 재호출(Phase 3로) → 다시 `reviewer` 재호출(Phase 4).
- 판정이 **PASS**면 Phase 5로.
- **루프 상한 3회.** 3회 후에도 PASS 못 하면 중단하고 남은 blocking을 사용자에게 보고한다 (무한 루프 방지).

### Phase 5 — compound (서브에이전트)
`compounder`(opus)를 `Agent`로 호출. `01`~`04`에서 교훈을 추출해 `CLAUDE.md`·`wf-*`·에이전트 정의에 반영하고 변경 이력을 갱신. → `05_compound.md`.

## 에러 핸들링

- **단계 산출물 누락:** 다음 단계 시작 전 이전 파일 존재를 확인한다. 없으면 해당 단계를 먼저 실행하거나 사용자에게 알린다.
- **서브에이전트 실패:** 1회 재시도. 재실패 시 그 단계 결과 없이 진행하지 말고, 실패를 사용자에게 보고하고 중단한다 (코드 변경 단계이므로 무단 진행은 위험).
- **재작업 루프 미수렴:** 위 3회 상한. 상한 도달 시 blocking 목록과 함께 사용자 판단을 요청한다.
- **계획 결함 판명(리뷰가 planner로 되돌림):** Phase 3이 아니라 Phase 2로 돌아가 계획을 수정한 뒤 재실행한다.
- **상충 데이터:** 삭제하지 않고 출처를 병기해 산출물에 남긴다.

## 팀 크기 / 규모

파이프라인은 단계당 에이전트 1명(순차). 재작업 루프에서만 worker↔reviewer가 반복된다. 조율 오버헤드가 낮으므로 서브에이전트 직접 호출로 충분하다 (팀 생성 불필요).

## 테스트 시나리오

**정상 흐름:** "DM에 읽음 표시 기능 추가" 요청
→ Phase 0: `<slug>=dm-read-receipt` 신규 → Phase 1: proto(dm)·usecase 영향 탐색, 옵션 제시, 사용자 확정 → Phase 2: 작업 분해·proto 재생성 절차·테스트 계획, 사용자 승인 → Phase 3: worker가 TDD 구현 + 테스트, 빌드·테스트 녹색 → Phase 4: reviewer가 handler↔proto↔mapper shape 교차 비교, PASS → Phase 5: "proto 변경 시 mapper 갱신 체크" 교훈을 CLAUDE.md에 반영. 5개 산출물 + 변경 이력 갱신.

**에러 흐름 (재작업 루프):** Phase 4에서 reviewer가 "새 fx provider가 `module.go`에 미배선 → 런타임 조립 실패" blocking 발견 → PASS 아님 → worker 재호출로 배선 추가 → reviewer 재검증 PASS → Phase 5 진행. (3회 내 미수렴 시 사용자 보고 후 중단.)

## 진화

매 사이클 종료(Phase 5) 후, 결과에 개선점이 있는지 사용자에게 기회를 준다. 피드백 유형별 반영 대상은 `wf-compound` 스킬 표 참조. 모든 변경은 `CLAUDE.md` 변경 이력에 기록한다.
