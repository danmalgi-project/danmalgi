# danmalgi

## 프로젝트 구조

```
danmalgi/
├── docs/                       # 프로젝트 문서
│   ├── issue/                  # 이슈 기록 (문제 상황, 원인, 해결 과정)
│   └── issue-template/         # 이슈 문서 작성용 템플릿
├── proto/                      # 클라이언트 공개 API proto 정의
├── server/
│   ├── proto/                  # 서버 내부(private) proto 정의
│   ├── danmalgi_signaling/     # WebRTC 시그널링 서버
│   ├── (chat)                  # 채팅 서버 (예정)
│   └── (api)                   # API 서버 (예정)
├── (mobile)                    # 모바일 앱 (예정)
└── (desktop)                   # 데스크톱 앱 (예정)
```
