Generate Go code from all proto files in the `proto/` directory into `pkg/gen/`.

Run the following steps using the Bash tool:

1. Create the output directory:
```bash
mkdir -p pkg/gen
```

2. Compile `proto/public/` protos. `-I proto/public` 사용 (canonical name이 올바른 출력 경로를 결정). `M` 플래그로 go_package 지정 (공유 proto에 go_package를 고정하지 않기 위함):
```bash
MODULE="github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public"

protoc \
  -I proto/public \
  --go_out=pkg/gen/public \
  --go_opt=paths=source_relative \
  --go_opt=Muser/v1/user.proto="${MODULE}/user/v1;userv1" \
  --go_opt=Mchat/v1/chat.proto="${MODULE}/chat/v1;chatv1" \
  --go_opt=Mauth/v1/auth.proto="${MODULE}/auth/v1;authv1" \
  --go_opt=Mdm/v1/dm.proto="${MODULE}/dm/v1;dmv1" \
  --go_opt=Mfriend/v1/friend.proto="${MODULE}/friend/v1;friendv1" \
  --go_opt=Mrelationship/v1/relationship.proto="${MODULE}/relationship/v1;relationshipv1" \
  --go_opt=Msignaling/v1/signaling.proto="${MODULE}/signaling/v1;signalingv1" \
  --go-grpc_out=pkg/gen/public \
  --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Muser/v1/user.proto="${MODULE}/user/v1;userv1" \
  --go-grpc_opt=Mchat/v1/chat.proto="${MODULE}/chat/v1;chatv1" \
  --go-grpc_opt=Mauth/v1/auth.proto="${MODULE}/auth/v1;authv1" \
  --go-grpc_opt=Mdm/v1/dm.proto="${MODULE}/dm/v1;dmv1" \
  --go-grpc_opt=Mfriend/v1/friend.proto="${MODULE}/friend/v1;friendv1" \
  --go-grpc_opt=Mrelationship/v1/relationship.proto="${MODULE}/relationship/v1;relationshipv1" \
  --go-grpc_opt=Msignaling/v1/signaling.proto="${MODULE}/signaling/v1;signalingv1" \
  proto/public/user/v1/user.proto \
  proto/public/chat/v1/chat.proto \
  proto/public/auth/v1/auth.proto \
  proto/public/dm/v1/dm.proto \
  proto/public/friend/v1/friend.proto \
  proto/public/relationship/v1/relationship.proto \
  proto/public/signaling/v1/signaling.proto
```

3. Compile `proto/private/` protos. `-I proto -I proto/public` 사용 (`user/v1/user.proto` import 해소). private proto는 이 프로젝트 전용이므로 proto 파일 자체에 go_package가 있음:
```bash
MODULE="github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public"

protoc \
  -I proto \
  -I proto/public \
  --go_out=pkg/gen \
  --go_opt=paths=source_relative \
  --go_opt=Muser/v1/user.proto="${MODULE}/user/v1;userv1" \
  --go-grpc_out=pkg/gen \
  --go-grpc_opt=paths=source_relative \
  --go-grpc_opt=Muser/v1/user.proto="${MODULE}/user/v1;userv1" \
  proto/private/device_store/v1/device_store.proto \
  proto/private/dm_store/v1/dm_store.proto \
  proto/private/user_store/v1/user_store.proto
```

4. Verify the generated files exist under `pkg/gen/`.

If protoc or the plugins (`protoc-gen-go`, `protoc-gen-go-grpc`) are not installed, report the missing tools and stop.

## 참고: include path 설계

현재 proto 구조:
- `proto/public/` — `import "user/v1/user.proto"` 사용
- `proto/private/` — `import "user/v1/user.proto"` 사용 (user/v1/user.proto가 실제로 public에 위치)

두 명령으로 나누는 이유:
- `public/` 컴파일 시 `-I proto/public`만 사용 → canonical name이 `user/v1/...`, `chat/v1/...`이 되어 출력 경로가 `pkg/gen/public/` 하위에 올바르게 생성됨
- `private/` 컴파일 시 `-I proto -I proto/public` 사용 → `user/v1/user.proto` import는 `-I proto/public`으로 해소, `private/` 파일 자체는 `-I proto`로 canonical name 결정
- 같은 invocation에서 `proto/public/user/v1/user.proto`를 직접 넣으면 동일 파일이 두 canonical name(`user/v1/...`, `public/user/v1/...`)으로 등록되어 충돌 발생

## go_package를 M 플래그로 지정하는 이유

공유 proto 파일(서브모듈)에 `go_package`를 고정하면 다른 프로젝트에서 사용할 때 경로가 맞지 않음.
각 프로젝트가 빌드 시 `M` 플래그로 자기 Go import path를 지정하는 것이 올바른 패턴.

## 새 proto 파일 추가 시

- `proto/public/` 하위 파일 → 2번 명령에 파일 경로 + `M` 플래그 추가
- `proto/private/` 하위 파일 → 3번 명령에 파일 경로 추가 (go_package가 proto에 있으면 M 플래그 불필요)
