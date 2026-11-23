#!/usr/bin/env bash
# webrtc-adapter 로컬 테스트 원클릭 실행 스크립트 (run.ps1과 동일한 흐름의 병행 스크립트).
#
# 기본값은 standalone 모드 — Redis와 내부 API 서버가 시그널링 서버 안의 fake로 대체되므로
# 이 저장소만 띄우면 로컬 E2E 수동 검증이 된다. 외부 의존성을 하나도 요구하지 않는다.
# 실제 Redis/내부 API에 붙여 검증하려면 --with-backend를 준다 (둘 다 미리 떠 있어야 한다).
#
# 흐름: 준비물 점검 -> 시그널링 서버 기동(go run ./cmd/webrtc) -> 어댑터 기동(go run .) -> 접속 URL 안내.
set -euo pipefail

ADAPTER_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$ADAPTER_DIR/../.." && pwd)"
cd "$ADAPTER_DIR"

STANDALONE=true
for arg in "$@"; do
  case "$arg" in
    --with-backend) STANDALONE=false ;;
    --standalone) STANDALONE=true ;;
    *) echo "알 수 없는 인자: $arg (사용법: $0 [--with-backend])" >&2; exit 1 ;;
  esac
done

fail() {
  echo "오류: $1" >&2
  exit 1
}

# .env 스타일 파일에서 KEY 값을 읽는다 (주석/빈 줄 무시, 값의 앞뒤 따옴표 제거).
read_env_value() {
  local file="$1" key="$2"
  [ -f "$file" ] || return 0
  local line
  line="$(grep -E "^${key}=" "$file" | tail -n1 || true)"
  [ -n "$line" ] || return 0
  local value="${line#*=}"
  value="${value%\"}"
  value="${value#\"}"
  value="${value%\'}"
  value="${value#\'}"
  echo "$value"
}

test_tcp_reachable() {
  local host="$1" port="$2"
  (exec 3<>"/dev/tcp/${host}/${port}") 2>/dev/null
  local ok=$?
  exec 3>&- 2>/dev/null || true
  return $ok
}

echo "== 1. 준비물 점검 =="
if [ "$STANDALONE" = true ]; then
  echo "  - 모드: standalone (Redis/내부 API를 인프로세스 fake로 대체)"
else
  echo "  - 모드: with-backend (실제 Redis/내부 API에 연결)"
fi

# 서버 .env — standalone에서는 있으면 쓰고 없으면 기본값으로 진행한다.
SERVER_ENV_PATH="$REPO_ROOT/.env"
if [ -f "$SERVER_ENV_PATH" ]; then
  echo "  - 서버 .env 확인됨 ($SERVER_ENV_PATH)"
elif [ "$STANDALONE" = true ]; then
  echo "  - 서버 .env 없음 — standalone 기본값으로 진행합니다"
else
  fail "서버 .env가 없습니다: $SERVER_ENV_PATH (루트 README의 '시작하기' 절 참고)"
fi

SERVER_PORT="$(read_env_value "$SERVER_ENV_PATH" SERVER_PORT)"
JWT_SECRET_VALUE="$(read_env_value "$SERVER_ENV_PATH" JWT_SECRET)"

if [ "$STANDALONE" != true ]; then
  REDIS_HOST="$(read_env_value "$SERVER_ENV_PATH" REDIS_HOST)"
  REDIS_PORT="$(read_env_value "$SERVER_ENV_PATH" REDIS_PORT)"
  API_SERVER_HOST="$(read_env_value "$SERVER_ENV_PATH" API_SERVER_HOST)"
  API_SERVER_PORT="$(read_env_value "$SERVER_ENV_PATH" API_SERVER_PORT)"

  for pair in "SERVER_PORT:$SERVER_PORT" "JWT_SECRET:$JWT_SECRET_VALUE" "REDIS_HOST:$REDIS_HOST" "REDIS_PORT:$REDIS_PORT" "API_SERVER_HOST:$API_SERVER_HOST" "API_SERVER_PORT:$API_SERVER_PORT"; do
    name="${pair%%:*}"
    value="${pair#*:}"
    [ -n "$value" ] || fail "서버 .env에 ${name} 값이 없습니다: $SERVER_ENV_PATH"
  done

  # 내부 API / Redis 도달 확인 (standalone에서는 fake로 대체되므로 건너뛴다)
  test_tcp_reachable "$REDIS_HOST" "$REDIS_PORT" || fail "Redis에 연결할 수 없습니다 ($REDIS_HOST:$REDIS_PORT). 로컬 Redis가 떠 있는지 확인하거나, --with-backend 없이 실행해 standalone으로 검증하세요."
  echo "  - Redis 도달 확인됨 ($REDIS_HOST:$REDIS_PORT)"

  test_tcp_reachable "$API_SERVER_HOST" "$API_SERVER_PORT" || fail "내부 API 서버에 연결할 수 없습니다 ($API_SERVER_HOST:$API_SERVER_PORT). 로컬 API 서버가 떠 있는지 확인하거나, --with-backend 없이 실행해 standalone으로 검증하세요."
  echo "  - 내부 API 서버 도달 확인됨 ($API_SERVER_HOST:$API_SERVER_PORT)"
fi

# 서버 포트와 JWT 시크릿을 여기서 확정해 서버·어댑터 두 프로세스에 같은 값으로 주입한다.
# .env가 있으면 그 값이 권위 소스이고, 없을 때만(standalone) 로컬 전용 기본값을 쓴다.
# 두 프로세스에 같은 값을 넣으므로, 이 기본값이 cmd/webrtc/standalone.go의 기본값과
# 달라져도 검증은 깨지지 않는다(godotenv/서버 기본값은 이미 설정된 env를 덮어쓰지 않는다).
SERVER_PORT="${SERVER_PORT:-8083}"
JWT_SECRET_VALUE="${JWT_SECRET_VALUE:-danmalgi-standalone-dev-secret}"
export SERVER_PORT
export JWT_SECRET="$JWT_SECRET_VALUE"

ADAPTER_ENV_PATH="$ADAPTER_DIR/.env"
TLS_CERT_VALUE="$(read_env_value "$ADAPTER_ENV_PATH" TLS_CERT)"
TLS_KEY_VALUE="$(read_env_value "$ADAPTER_ENV_PATH" TLS_KEY)"
TLS_CERT_VALUE="${TLS_CERT_VALUE:-server.crt}"
TLS_KEY_VALUE="${TLS_KEY_VALUE:-server.key}"

[ -f "$ADAPTER_DIR/$TLS_CERT_VALUE" ] && [ -f "$ADAPTER_DIR/$TLS_KEY_VALUE" ] \
  || fail "어댑터 인증서가 없습니다 ($TLS_CERT_VALUE / $TLS_KEY_VALUE). 먼저 './ssl.sh'를 실행해 로컬 인증서를 발급하세요."
echo "  - 어댑터 인증서 확인됨 ($TLS_CERT_VALUE, $TLS_KEY_VALUE)"

# 어댑터 .env 값을 프로세스 환경변수로 반영 (main.go의 flag > env > 기본값 규칙을 그대로 재사용)
for key in GRPC_ADDR LISTEN_ADDR CHANNEL_ID JWT_TOKEN TLS_CERT TLS_KEY; do
  value="$(read_env_value "$ADAPTER_ENV_PATH" "$key")"
  if [ -n "$value" ]; then
    export "$key=$value"
  fi
done

: "${GRPC_ADDR:=localhost:$SERVER_PORT}"
export GRPC_ADDR

if [ -z "${JWT_TOKEN:-}" ]; then
  echo "  - JWT_TOKEN 미지정: gen-token으로 새로 발급합니다"
  JWT_TOKEN="$(go run ./cmd/gen-token --jwt-secret "$JWT_SECRET_VALUE" | tail -n1)"
  [ -n "$JWT_TOKEN" ] || fail "JWT 발급 실패"
  export JWT_TOKEN
fi
echo "  - JWT 준비됨"

SERVER_ARGS=(run ./cmd/webrtc)
if [ "$STANDALONE" = true ]; then
  SERVER_ARGS+=(--standalone)
fi

echo
echo "== 2. 시그널링 서버 기동 (go ${SERVER_ARGS[*]}) =="
(cd "$REPO_ROOT" && go "${SERVER_ARGS[@]}") &
SERVER_PID=$!

server_ready=false
for _ in $(seq 1 15); do
  sleep 1
  if test_tcp_reachable "localhost" "$SERVER_PORT"; then
    server_ready=true
    break
  fi
done
if [ "$server_ready" != true ]; then
  kill "$SERVER_PID" 2>/dev/null || true
  fail "시그널링 서버가 SERVER_PORT(${SERVER_PORT})에서 기동되지 않았습니다."
fi
echo "  - 서버 기동 확인됨 (:$SERVER_PORT)"

# JWT_SECRET은 1단계에서 이미 export 해두었다 — 서버와 어댑터가 같은 값을 쓴다.
# 어댑터 .env(JWT_TOKEN 등) 키 루프에는 넣지 않는다: drift 방지를 위해 항상 이 값을 사용한다.

echo
echo "== 3. 어댑터 기동 (go run .) =="
go run . &
ADAPTER_PID=$!

LISTEN_ADDR_VALUE="${LISTEN_ADDR:-:8888}"
PORT="${LISTEN_ADDR_VALUE##*:}"

echo
echo "== 4. 접속 안내 =="
echo "브라우저에서 다음 주소로 접속하세요 (자체 서명 인증서 경고는 수락):"
echo "  https://localhost:${PORT}/"
echo
echo "종료하려면 Ctrl+C를 누르세요."

cleanup() {
  echo
  echo "프로세스를 종료합니다..."
  kill "$SERVER_PID" "$ADAPTER_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

wait "$SERVER_PID" "$ADAPTER_PID"
