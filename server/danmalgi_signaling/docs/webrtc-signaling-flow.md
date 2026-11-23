# WebRTC 시그널링 흐름

`SignalingService.Signaling` (gRPC 양방향 스트림) 위에서 일어나는 모든 일을 그림으로 정리한다.
코드 기준: `proto/public/signaling/v1/signaling.proto`, `internal/{handler,usecase/service,signaling,manager,domain}`

---

## 1. 토폴로지 — SFU

```mermaid
flowchart LR
    A["Client A<br/>user 1 / web-1"]
    B["Client B<br/>user 2 / web-2"]
    C["Client C<br/>user 3 / web-3"]

    S{{"danmalgi_webrtc_server<br/><b>SFU</b><br/>channel 42"}}

    A <==>|"gRPC stream<br/>+ SRTP"| S
    B <==>|"gRPC stream<br/>+ SRTP"| S
    C <==>|"gRPC stream<br/>+ SRTP"| S

    S -.->|"A의 트랙"| B
    S -.->|"A의 트랙"| C
    S -.->|"B의 트랙"| A
    S -.->|"C의 트랙"| A
```

> 클라이언트끼리 P2P를 맺지 않는다. 각자 **서버와 1:1 PeerConnection** 하나만 맺고, 서버가 되쏜다.

---

## 2. 누가 offer를 만드는가

```mermaid
flowchart LR
    subgraph C["Client"]
        CA["answer 생성"]
        CC["candidate 수집"]
    end

    subgraph S["Server"]
        SO["<b>offer 생성</b><br/>CreateOffer<br/>SetLocalDescription"]
        SC["candidate 수집"]
    end

    SO ==>|"① offer"| CA
    CA ==>|"② answer"| SO
    SC <-->|"③ candidate (양방향, trickle)"| CC

    X["❌ Client는 offer를 만들지 않는다"]
    style X fill:#4a1f1f,stroke:#a33,color:#fff
```

| | `SignalingRequest` (C→S) | `SignalingResponse` (S→C) |
|---|---|---|
| oneof 필드 | `answer`, `candidate` | `offer`, `answer`*, `candidate` |

<sub>* proto에 정의만 있고 서버는 보내지 않음</sub>

---

## 3. 컴포넌트 맵

```mermaid
flowchart TB
    CL(["Client"])

    subgraph L1["전송 · 인증"]
        GRPC["gRPC Server<br/><code>server/grpc.go</code>"]
        AUTH["AuthMiddleware<br/><code>middleware/auth_interceptor.go</code>"]
    end

    subgraph L2["진입"]
        H["SignalingHandler<br/><code>handler/signaling_handler.go</code>"]
        MAP["mapper<br/>domain ↔ proto"]
    end

    subgraph L3["유스케이스"]
        SVC["SignalingService<br/>PreSignaling · PostSignaling<br/>SendOffer · SignalAnswer · SignalCandidate"]
    end

    subgraph L4["시그널링 코어"]
        EV["SignalingEvent<br/>OnTrack · OnIceCandidate"]
        MGR[("SignalingManager<br/>채널별 wrapper·track")]
        WR["PeerConnectionWrapper<br/>Offer · Candidate · Err · done<br/>Negotiate · ApplyAnswer<br/><i>negotiationMu</i>"]
    end

    subgraph L5["외부"]
        PION["pion PeerConnection"]
        RDS[("Redis<br/>channel:N:voice")]
        API["user_store<br/>(내부 API)"]
    end

    CL --> GRPC --> AUTH --> H
    H <--> MAP
    H --> SVC
    SVC --> MGR
    SVC --> RDS
    SVC --> PION
    SVC -->|"콜백 등록"| EV
    PION -->|"콜백 발화"| EV
    EV --> MGR
    EV --> API
    EV -->|"Enqueue*"| WR
    WR -->|"채널 수신"| H
```

---

## 4. 연결 수립

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant I as AuthMiddleware
    participant H as Handler
    participant S as SignalingService
    participant P as pion PC
    participant R as Redis

    rect rgb(30,45,65)
    Note over C,I: 인증
    C->>I: Signaling() 개시<br/>md: authorization, channel_id
    I->>I: token.Validate → userId, deviceId
    I->>I: GetUserByID(userId)
    I-->>H: ctx { UID, DID, USER }
    end

    rect rgb(30,55,45)
    Note over H,R: PreSignaling
    H->>H: channel_id 파싱
    H->>S: PreSignaling(userId, channelId, deviceId)
    S->>P: NewPeerConnection<br/>STUN · NAT1To1 · UDP 49000-49010
    S->>P: AddTransceiver(audio, <b>recvonly</b>)
    S->>P: OnTrack · OnICECandidate · OnConnectionStateChange
    S->>S: Manager.Save(channelId, wrapper)
    S->>R: HSET channel:N:voice [deviceId]=userId<br/>EXPIRE 2m
    S-->>H: wrapper
    end

    rect rgb(55,45,30)
    Note over C,P: 협상
    H->>H: recv · send goroutine 기동
    H->>S: SendOffer(wrapper)
    S->>P: Negotiate: CreateOffer · SetLocalDescription
    S->>S: GetTracks → 명단 스냅샷 (자기 제외)
    H-->>C: Response{ offer + joined_user_tracks }
    C->>C: setRemoteDescription → createAnswer
    C->>H: Request{ answer }
    H->>S: SignalAnswer → ApplyAnswer<br/>(SetRemoteDescription + pending 확인)
    end

    par trickle ICE
        P-->>H: OnICECandidate
        H-->>C: Response{ candidate }
    and
        C->>H: Request{ candidate }
        H->>S: SignalCandidate → AddICECandidate
    end

    Note over C,P: ICE ✓ → DTLS ✓ → SRTP 흐름 시작
```

**첫 offer의 모양**

```mermaid
flowchart LR
    O["초기 offer"] --> M1["m=audio<br/><b>recvonly</b><br/>(서버가 당신 오디오를 받겠다)"]
    O --> JT["joined_user_tracks<br/>= <b>전송 시점 채널 명단</b><br/>(자기 제외 · 아무도 없으면 <b>빈 목록</b>)"]
```

> 첫 offer라고 해서 명단이 비는 게 아니다. 이미 발행 중인 참가자가 있으면 그 목록이 그대로 실린다.
> 반대로 아무도 없으면 **필드 부재가 아니라 빈 목록**으로 온다 — 그래야 "마지막 한 명이 나갔다"를 표현할 수 있다.

---

## 5. 핸들러 내부 — goroutine 3개

```mermaid
flowchart TB
    subgraph ST["Signaling 스트림 1개"]
        direction TB

        subgraph RG["recv goroutine"]
            R1["stream.Recv() 루프"]
        end

        subgraph SG["send goroutine"]
            S1["select on wrapper 채널"]
        end

        subgraph MG["handler 본체"]
            M1["select ctx.Done / done"]
        end
    end

    R1 -->|"answer"| A1["SignalAnswer<br/>ApplyAnswer<br/>pending이면 go SendOffer"]
    R1 -->|"candidate"| A2["SignalCandidate<br/>AddICECandidate"]
    R1 -->|"err"| A3["EnqueueErr"]

    WO(["wrapper.Offer"]) --> S1
    WC(["wrapper.Candidate"]) --> S1
    WE(["wrapper.Err"]) --> S1

    S1 -->|"offer / candidate"| OUT(["stream.Send → Client"])
    S1 -->|"err"| DN(["done ← err"]) --> M1

    CTX(["ctx.Done()"]) --> M1
    M1 --> RET["return → <b>defer PostSignaling</b>"]

    style RET fill:#4a1f1f,stroke:#a33,color:#fff
```

---

## 6. 트랙 발행 → 팬아웃 (핵심)

```mermaid
sequenceDiagram
    autonumber
    participant A as Client A
    participant PA as PC(A)
    participant EV as SignalingEvent
    participant MGR as Manager
    participant API as user_store
    participant PB as PC(B)
    participant B as Client B

    A->>PA: RTP 송신 시작
    PA->>EV: <b>OnTrack</b>(TrackRemote)
    EV->>EV: NewTrackLocalStaticRTP<br/>id=원본 트랙 ID · streamID=userId
    EV->>API: GetUserByID(A.userId)
    API-->>EV: User

    par ① 팬아웃 등록 (addTrack)
        EV->>MGR: AddTrack(channelId, track)
        EV->>MGR: GetTracks · Gets(channelId)
        loop 채널의 모든 wrapper
            EV->>EV: attachedTrackIDs<br/>= receivers ∪ senders
            EV->>EV: 자기 트랙 skip (echo 방지)
            EV->>PB: AddTransceiverFromTrack(track, <b>sendonly</b>)
            EV->>PB: go SendOffer(wrapper)<br/>(m-line이 는 wrapper만)
            EV->>MGR: GetTracks → 전송 직전 명단 재계산
            PB-->>B: Response{ offer + joined_user_tracks }
            B->>PB: Request{ answer }
        end
    and ② RTP 중계 (writeStream)
        loop 트랙이 살아있는 동안
            PA->>EV: TrackRemote.Read(rtpBuf)
            EV->>PB: TrackLocal.Write(rtpBuf)
            PB-->>B: SRTP
        end
    end
```

### 6.1 자기 목소리가 되돌아오지 않는 이유

필터가 **두 겹**이다. 집합 연산(`filterUnattachedTracks`)이 중복 부착을 막고, 그 안쪽의 명시적 소유자 검사가 echo를 막는다.

```mermaid
flowchart TB
    T["채널 트랙 목록<br/>{ trackA, trackB, trackC }"]

    subgraph WA["wrapper A 순회"]
        AA["attached(A)<br/>= <b>receivers</b>{trackA} ∪ senders{trackB, trackC}"]
        FA["filterUnattached<br/>→ <b>{ } 비어있음</b>"]
        RA["len(added)==0<br/>→ SendOffer 안 함 ✅"]
        AA --> FA --> RA
    end

    subgraph WB["wrapper B 순회"]
        AB["attached(B)<br/>= receivers{trackB} ∪ senders{trackC}"]
        FB["filterUnattached<br/>→ <b>{ trackA }</b>"]
        EB["track.User.ID == wrapper.UserId ?<br/>→ 아니오"]
        RB["AddTransceiver + go SendOffer ✅"]
        AB --> FB --> EB --> RB
    end

    T --> AA
    T --> AB

    style RA fill:#1f3a2a,stroke:#3a6,color:#fff
    style RB fill:#1f3a2a,stroke:#3a6,color:#fff
```

> A가 올린 트랙은 A의 PeerConnection에 **receiver**로 이미 잡혀 있다 → 집합 연산에서 제외.
> 같은 함수가 **중복 부착 방지**도 겸한다 (늦게 들어온 참가자에게 기존 트랙을 몰아 붙일 때).

**⚠️ echo 필터는 wrapper가 아니라 트랙 단위로 건다.** receiver에 트랙이 붙기 **전** 타이밍에는 자기 트랙이 "새 트랙"으로 보여 집합 연산만으로는 되돌아간다. 그래서 `newTracks` 루프 안에 `newTrack.User.ID == wrapper.UserId` 검사가 따로 있다(`snapshotForWrapper`의 자기 제외와 **같은 기준**).

바깥 wrapper 순회 상단에서 걸러선 안 된다 — `AddTransceiverFromTrack`이 실행되는 곳이 이 순회 **안**이라, wrapper를 통째로 skip하면 신규 입장자가 기존 트랙을 붙을 유일한 기회가 사라져 **아무 소리도 안 들리는** 회귀가 된다.

### 6.2 offer에 실리는 트랙 정보

```mermaid
flowchart LR
    OF["Offer"] --> SDP["sdp<br/>{ sdp_type, sdp_data }"]
    OF --> JUT["joined_user_tracks<br/><b>전송 시점 채널 전체 명단</b><br/>(자기 제외 · replace 계약)"]
    JUT --> UT["UserTrack"]
    UT --> U["user<br/>id · name · email · tag<br/>status · profile_image_url"]
    UT --> TID["track_id<br/>= TrackLocal.ID()<br/>(SDP 트랙 ID와 일치)"]
    UT --> TT["track_type<br/>AUDIO · CAM_* · SCREEN_*"]
```

> ⚠️ `track_type`은 pion `t.Kind()`(audio=1, video=2)를 그대로 넣어 proto enum(AUDIO=0, CAM_AUDIO=1, CAM_VIDEO=2)과 한 칸씩 어긋난다. 캠/화면공유 구분이 필요해지면 이 매핑부터 정리해야 한다.

### 6.3 스냅샷 계약 — 델타가 아니다

`joined_user_tracks`는 **매 offer마다 명단 전체**다. 클라이언트는 받을 때마다 **통째로 교체(replace)** 해야 하며, 누적(merge)하면 퇴장자가 영영 남는다.

```mermaid
flowchart TB
    subgraph OLD["❌ 이전: 델타"]
        D1["addTrack이 <b>이번에 붙인 것만</b> 실음"]
        D2["offer 하나 유실<br/>= 그 참가자 <b>영구 유실</b>"]
        D3["removeTrack은 nil<br/>= 누가 나갔는지 알 수 없음"]
        D1 --> D2 --> D3
    end

    subgraph NEW["✅ 현재: 스냅샷"]
        S1["SendOffer가 전송 직전<br/><b>GetTracks로 다시 계산</b>"]
        S2["offer 유실·지연돼도<br/><b>다음 성공 offer가 복구</b>"]
        S3["퇴장 = 명단에서 사라짐<br/>(빈 목록도 그대로 전송)"]
        S1 --> S2 --> S3
    end

    style D2 fill:#4a1f1f,stroke:#a33,color:#fff
    style S2 fill:#1f3a2a,stroke:#3a6,color:#fff
```

핵심은 **누가 명단을 만드는가**다. 호출자가 인자로 넘기지 않고 `SendOffer`가 전송 직전 매니저에서 직접 뜬다 — 그래서 실패한 전송이 손실로 남지 않는다.

> 이것이 이슈 #5("3명 접속 시 기존 참가자에게 2명만 보임")의 직접 원인이었다.

> ⚠️ **명단에 있다고 해당 m-line이 있는 건 아니다.** 스냅샷은 매니저의 전체 트랙에서 뜨는데, 마이크를 켜지 않은 참가자는 `addTrack`을 유발하지 않아 아직 SDP에 m-line이 없다. `track_id` ↔ m-line 매핑을 전제로 렌더링하면 "이름은 뜨는데 소리가 없는" 상태가 보일 수 있다 (후속: 입장 트리거 팬아웃).

---

## 7. 재협상 트리거

```mermaid
flowchart LR
    E1["연결 직후"] --> SO["<b>SendOffer</b>"]
    E2["누가 트랙을 켬<br/>(OnTrack → addTrack)"] --> SO
    E3["누구 트랙이 끊김<br/>(Read 에러 → removeTrack)"] --> SO
    SO --> OF(["Response{ offer }"]) --> CA(["Request{ answer }"])

    SO -.->|"항상"| J1["joined_user_tracks<br/><b>전송 시점 명단 스냅샷</b>"]
```

> 트리거가 무엇이든 실리는 건 **언제나 전체 명단**이다. "추가일 때만 있음 / 제거일 때는 없음" 같은 분기는 없다.

### 7.1 glare — 이전 offer의 answer를 기다리는 중이라면

여러 참가자가 동시에 들어오면 `addTrack`이 wrapper마다 `go SendOffer`를 띄우는데, 그 PeerConnection이 아직 `have-local-offer`면 pion이 두 번째 offer를 거부한다. 예전에는 이 에러를 조용히 삼켜 **그 offer에 실린 명단이 통째로 사라졌다.**

```mermaid
sequenceDiagram
    autonumber
    participant F as 팬아웃 goroutine
    participant W as wrapper<br/>(negotiationMu)
    participant P as pion PC
    participant C as Client

    F->>W: Negotiate(prepare)
    W->>P: SignalingState() = <b>stable</b> ✅
    W->>P: CreateOffer · SetLocalDescription
    W-->>F: sent = true
    F->>C: EnqueueOffer (<b>락 밖</b>)
    Note over P: 상태 = have-local-offer

    rect rgb(55,45,30)
    Note over F,P: 이때 새 참가자 입장 → 두 번째 SendOffer
    F->>W: Negotiate(prepare)
    W->>P: SignalingState() = <b>have-local-offer</b> ❌
    W->>W: pendingNegotiation = true
    W-->>F: sent = false (에러 아님, 버려도 안전)
    end

    C->>W: answer 도착 → ApplyAnswer
    W->>P: SetRemoteDescription → stable
    W->>W: pending 소비 (true → false)
    W-->>F: pending = true
    Note over F: SignalAnswer가 go SendOffer로 <b>재개</b><br/>→ 이번엔 완전한 명단을 싣는다
```

| 규칙 | 이유 |
|---|---|
| `Negotiate`와 `ApplyAnswer`는 **같은** `negotiationMu` | 한쪽이 락 밖이면 lost-wakeup — pending을 세운 직후 소비자가 이미 지나가 **깨울 answer가 영영 안 온다** (에러도 로그도 없이 그 참가자만 정지) |
| `Enqueue*`는 락 **밖**에서 | Offer 채널이 언버퍼드라 락을 쥔 채 send하면 클라이언트 읽기 속도에 재협상 전체가 묶인다 |
| 지연된 offer는 그냥 버린다 | 스냅샷 계약 덕에 손실이 없다 → pending이 카운터가 아니라 `bool`이어도 충분 |
| 재개는 `go`로 | 호출자가 handler의 recv 루프라 인라인 실행하면 인바운드 시그널링이 멈춘다 |

---

## 8. 종료

```mermaid
sequenceDiagram
    autonumber
    participant A as Client A
    participant EV as SignalingEvent
    participant PB as PC(B)
    participant B as Client B
    participant H as Handler(A)
    participant S as Service
    participant R as Redis

    rect rgb(55,45,30)
    Note over A,B: ① 트랙만 끊긴 경우
    A--xEV: RTP 중단 → TrackRemote.Read 에러
    EV->>EV: writeStream 종료 → <b>defer removeTrack</b>
    EV->>EV: ① Manager.RemoveTrack (먼저!)
    loop 소유자(track.User.ID == wrapper.UserId) 제외
        EV->>PB: ② RemoveTrack(sender)
        EV->>PB: ③ go SendOffer(wrapper)
        PB-->>B: Response{ offer + 퇴장자 빠진 명단 }
        B->>PB: Request{ answer }
    end
    end

    rect rgb(60,30,30)
    Note over A,R: ② 스트림이 끊긴 경우
    A--xH: EOF / ctx.Done / Err
    H->>S: <b>defer PostSignaling</b>
    S->>R: HDEL channel:N:voice [deviceId]
    S->>S: Manager.Delete(channelId, wrapper)
    S->>S: PeerConnection.Close()
    S->>S: wrapper.Close()  → close(done)
    Note over S: 이후 모든 Enqueue*는 false 반환 (drop, panic 없음)
    end
```

> ⚠️ `removeTrack`의 **순서가 계약의 일부**다. 매니저에서 먼저 지워야(①) 이후 `SendOffer`가 뜨는 스냅샷에서 퇴장자가 빠진다. 이 제거를 `defer`로 미루면 퇴장자가 명단에 그대로 남는다.

**세 갈래가 한 곳으로 수렴**

```mermaid
flowchart LR
    T1["클라이언트 스트림 종료<br/>(io.EOF)"] --> CV
    T2["gRPC 컨텍스트 취소<br/>(ctx.Done)"] --> CV
    T3["내부 에러<br/>(EnqueueErr → done)"] --> CV
    CV{{"handler return"}} --> PS["defer PostSignaling"]
    PS --> P1["Redis HDEL"]
    PS --> P2["Manager.Delete<br/>(마지막이면 shard 삭제)"]
    PS --> P3["PeerConnection.Close()"]
    PS --> P4["wrapper.Close()"]
```

---

## 9. 서버가 들고 있는 상태

```mermaid
flowchart TB
    SM["SignalingManager<br/><i>RWMutex</i>"]
    SM --> SH1["shards[42] ChannelShard<br/><i>자체 RWMutex</i>"]
    SM --> SH2["shards[43] ChannelShard"]

    SH1 --> W["wrappers []*PeerConnectionWrapper"]
    SH1 --> T["tracks []*UserTrack"]

    W --> W1["wrapper(user1/web-1)"]
    W --> W2["wrapper(user2/web-2)"]
    T --> T1["track(user1, audio)"]
    T --> T2["track(user2, audio)"]

    GET["Gets() · GetTracks()"] -.->|"항상 <b>복사본</b> 반환<br/>순회 중 변경돼도 race 없음"| SH1
```

> 상태가 프로세스 메모리에 있으므로 **단일 인스턴스 전제**다. 수평 확장하려면 채널 단위 라우팅(같은 채널 → 같은 인스턴스)이나 SFU 간 릴레이가 필요하다.

**Redis presence**

```mermaid
flowchart LR
    K["Hash<br/><code>channel:42:voice</code>"]
    K --> F1["field: web-1 → 1"]
    K --> F2["field: web-2 → 2"]

    P["PreSignaling"] -->|"HSET + EXPIRE 2m"| K
    PO["PostSignaling"] -->|"HDEL field"| K

    N["⚠️ TTL은 <b>신규 입장 시에만</b> 갱신<br/>2분 넘게 입장이 없으면 키 만료<br/>(미디어 연결 자체엔 영향 없음)"]
    K -.-> N
    style N fill:#4a3a1f,stroke:#a83,color:#fff
```

---

## 10. 식별 축 3개

```mermaid
flowchart TB
    T["JWT claims<br/><b>userId</b> + <b>deviceId</b>"]

    T --> I1["① 신원 검증<br/>AuthMiddleware → GetUserByID"]
    T --> I2["② presence<br/>Redis HSET, field = deviceId"]
    T --> I3["③ 트랙 라우팅<br/>track.User.ID == wrapper.UserId"]

    I1 --> R1["<b>userId는 실존 유저</b>"]
    I2 --> R2["<b>deviceId는 연결별 유니크</b>"]
    I3 --> R3["<b>참가자별 userId 유니크</b>"]

    R1 -.->|"위반"| V1["연결 거부<br/>(+ known-issues.md 패닉 이슈)"]
    R2 -.->|"위반"| V2["두 연결이 한 칸 공유<br/>→ 한쪽 종료가 상대 presence까지 삭제"]
    R3 -.->|"위반"| V3["서로의 트랙을<br/>자기 것으로 오인"]

    style V1 fill:#4a1f1f,stroke:#a33,color:#fff
    style V2 fill:#4a1f1f,stroke:#a33,color:#fff
    style V3 fill:#4a1f1f,stroke:#a33,color:#fff
```

> 새 클라이언트/어댑터를 만들 때 세 축을 **한꺼번에** 점검한다.

---

## 11. 채널 수명주기 (동시성 규칙)

```mermaid
flowchart LR
    subgraph SND["다중 sender (동시 send)"]
        S1["OnICECandidate<br/>(pion 콜백)"]
        S2["SendOffer<br/>(addTrack / removeTrack)"]
        S3["recv goroutine 에러"]
    end

    S1 & S2 & S3 ==>|"<b>Enqueue*()</b>"| SEL{"select<br/>ch ← v<br/>/<br/>← done"}

    SEL -->|"열림"| CH(["wrapper 채널"]) --> RCV["send goroutine<br/>stream.Send"]
    SEL -->|"done 닫힘"| DR["false 반환<br/>drop (panic ❌ · 누수 ❌)"]

    CLS["wrapper.Close()<br/>close(done)<br/><i>sync.Once</i>"] -.-> SEL

    BAD["❌ close(Offer/Candidate/Err)<br/>= send on closed channel panic<br/>= 별도 goroutine → recovery도 못 잡음<br/>= <b>프로세스 전체 사망</b>"]
    style BAD fill:#4a1f1f,stroke:#a33,color:#fff
    style DR fill:#1f3a2a,stroke:#3a6,color:#fff
```

| 규칙 | |
|---|---|
| ① 소유자가 데이터 채널을 close하지 않는다 | 종료 신호는 오직 `wrapper.Close()` (= `close(done)`) |
| ② 직접 `ch <-` 금지 | 항상 `EnqueueOffer` / `EnqueueCandidate` / `EnqueueErr` |

> 새 send 지점이나 새 채널을 추가할 때도 동일 규칙을 적용한다.

---

## 12. 클라이언트 상태 머신

```mermaid
stateDiagram-v2
    state "스트림 개시" as OPEN
    state "offer 대기" as WAIT
    state "answer 전송" as ANS
    state "ICE 협상 중" as ICE
    state "연결됨" as CONN

    [*] --> OPEN: md(authorization, channel_id)
    OPEN --> WAIT
    WAIT --> ANS: offer 수신 → setRemote → createAnswer → setLocal
    ANS --> ICE
    ICE --> CONN: ICE ✓ DTLS ✓

    CONN --> ANS: 재협상 offer (트랙 추가/제거)
    CONN --> CONN: candidate 송수신

    CONN --> [*]: 스트림 종료
    ICE --> [*]: 실패

    note right of ANS
        offer는 연결 중 여러 번 온다.
        매번 처리해야 한다.
    end note
```

**체크리스트**

- [ ] `authorization: Bearer <JWT>` + `channel_id` 메타데이터로 스트림 개시
- [ ] `offer` 수신 → `setRemoteDescription` → `createAnswer` → `setLocalDescription` → `answer` 전송 **(여러 번 발생)**
- [ ] 로컬 candidate 생기는 즉시 전송 / 서버 candidate 받는 즉시 `addIceCandidate`
- [ ] `joined_user_tracks`를 받을 때마다 참가자 목록을 **통째로 교체** — 누적(merge)하면 퇴장자가 남는다
- [ ] 빈 목록은 "아무도 없음"이라는 **유효한 상태**다 — 무시하면 마지막 참가자가 사라지지 않는다
- [ ] offer를 직접 만들지 않는다 — 마이크를 나중에 켜도 서버가 재협상을 주도

**로컬 검증**

```mermaid
flowchart LR
    BR["브라우저<br/>index.html"] <-->|"WebSocket<br/>?userId=N"| AD["webrtc-adapter<br/>WS ↔ gRPC 브릿지"]
    AD -->|"userId로 JWT 서명"| AD
    AD <-->|"gRPC stream<br/>md: authorization, channel_id"| SV["danmalgi_webrtc_server"]
    BR <-.->|"SRTP (직접)"| SV
```

<sub>자세한 사용법: [`tools/webrtc-adapter/README.md`](../tools/webrtc-adapter/README.md)</sub>

---

## 관련 문서

- [`README.md`](../README.md) — 환경 변수, 실행 방법
- [`docs/known-issues.md`](known-issues.md) — 존재하지 않는 userId로 인한 패닉 등 후속 과제
- [`CLAUDE.md`](../CLAUDE.md) — 채널 수명주기·식별 축 규칙 원본
