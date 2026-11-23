# webrtc-adapter의 로컬 HTTPS(:8888)용 self-signed 인증서를 재발급한다.
# 결과물(server.key/server.crt)은 .gitignore에 포함되어 커밋되지 않는다.
# 사전 요구사항: openssl이 PATH에 있어야 한다 (Git for Windows에 포함된 openssl로 충분).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$openssl = Get-Command openssl -ErrorAction SilentlyContinue
if (-not $openssl) {
    Write-Error "openssl을 찾을 수 없습니다. Git for Windows(Git Bash)를 설치하거나 PATH에 openssl을 추가하세요."
    exit 1
}

& openssl req -x509 -nodes -days 365 `
    -newkey rsa:4096 `
    -keyout server.key `
    -out server.crt `
    -config san.cnf

if ($LASTEXITCODE -ne 0) {
    Write-Error "인증서 발급 실패 (openssl exit code $LASTEXITCODE)"
    exit $LASTEXITCODE
}

Write-Host "generated server.key / server.crt (CN=localhost, SAN=localhost,127.0.0.1)"
