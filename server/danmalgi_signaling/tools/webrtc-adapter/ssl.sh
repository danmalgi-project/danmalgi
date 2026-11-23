#!/usr/bin/env bash
# webrtc-adapter의 로컬 HTTPS(:8888)용 self-signed 인증서를 재발급한다.
# 결과물(server.key/server.crt)은 .gitignore에 포함되어 커밋되지 않는다.
set -euo pipefail
cd "$(dirname "$0")"

openssl req -x509 -nodes -days 365 \
  -newkey rsa:4096 \
  -keyout server.key \
  -out server.crt \
  -config san.cnf

echo "generated server.key / server.crt (CN=localhost, SAN=localhost,127.0.0.1)"
