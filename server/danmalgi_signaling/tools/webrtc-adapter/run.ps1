# webrtc-adapter 로컬 테스트 원클릭 실행 스크립트 (Windows/PowerShell 주 스크립트).
#
# 기본값은 standalone 모드 — Redis와 내부 API 서버가 시그널링 서버 안의 fake로 대체되므로
# 이 저장소만 띄우면 로컬 E2E 수동 검증이 된다. 외부 의존성을 하나도 요구하지 않는다.
# 실제 Redis/내부 API에 붙여 검증하려면 -WithBackend를 준다 (둘 다 미리 떠 있어야 한다).
#
# 흐름: 준비물 점검 -> 시그널링 서버 기동(go run ./cmd/webrtc) -> 어댑터 기동(go run .) -> 접속 URL 안내.
param([switch]$WithBackend)

$ErrorActionPreference = "Stop"
$Standalone = -not $WithBackend

$AdapterDir = $PSScriptRoot
Set-Location $AdapterDir
$RepoRoot = (Resolve-Path (Join-Path $AdapterDir "..\..")).Path

function Read-DotEnv {
    param([string]$Path)
    $result = @{}
    if (-not (Test-Path $Path)) {
        return $result
    }
    foreach ($rawLine in Get-Content -Path $Path) {
        $line = $rawLine.Trim()
        if ($line -eq "" -or $line.StartsWith("#")) { continue }
        $idx = $line.IndexOf("=")
        if ($idx -lt 0) { continue }
        $key = $line.Substring(0, $idx).Trim()
        $value = $line.Substring($idx + 1).Trim()
        if ($value.Length -ge 2) {
            $isDoubleQuoted = $value.StartsWith('"') -and $value.EndsWith('"')
            $isSingleQuoted = $value.StartsWith("'") -and $value.EndsWith("'")
            if ($isDoubleQuoted -or $isSingleQuoted) {
                $value = $value.Substring(1, $value.Length - 2)
            }
        }
        $result[$key] = $value
    }
    return $result
}

function Test-TcpReachable {
    param([string]$HostName, [int]$Port, [int]$TimeoutMs = 2000)
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $async = $client.BeginConnect($HostName, $Port, $null, $null)
        $ok = $async.AsyncWaitHandle.WaitOne($TimeoutMs)
        if (-not $ok) {
            $client.Close()
            return $false
        }
        $client.EndConnect($async)
        $client.Close()
        return $true
    } catch {
        return $false
    }
}

function Fail {
    param([string]$Message)
    Write-Error $Message
    exit 1
}

Write-Host "== 1. 준비물 점검 =="
if ($Standalone) {
    Write-Host "  - 모드: standalone (Redis/내부 API를 인프로세스 fake로 대체)"
} else {
    Write-Host "  - 모드: with-backend (실제 Redis/내부 API에 연결)"
}

# 1-1. 서버 .env — standalone에서는 있으면 쓰고 없으면 기본값으로 진행한다.
$ServerEnvPath = Join-Path $RepoRoot ".env"
$ServerEnv = @{}
if (Test-Path $ServerEnvPath) {
    $ServerEnv = Read-DotEnv $ServerEnvPath
    Write-Host "  - 서버 .env 확인됨 ($ServerEnvPath)"
} elseif ($Standalone) {
    Write-Host "  - 서버 .env 없음 — standalone 기본값으로 진행합니다"
} else {
    Fail "서버 .env가 없습니다: $ServerEnvPath (루트 README의 '시작하기' 절 참고)"
}

if (-not $Standalone) {
    foreach ($required in @("SERVER_PORT", "JWT_SECRET", "REDIS_HOST", "REDIS_PORT", "API_SERVER_HOST", "API_SERVER_PORT")) {
        if (-not $ServerEnv.ContainsKey($required) -or [string]::IsNullOrWhiteSpace($ServerEnv[$required])) {
            Fail "서버 .env에 $required 값이 없습니다: $ServerEnvPath"
        }
    }

    # 1-2. 내부 API / Redis 도달 확인 (standalone에서는 fake로 대체되므로 건너뛴다)
    if (-not (Test-TcpReachable -HostName $ServerEnv["REDIS_HOST"] -Port ([int]$ServerEnv["REDIS_PORT"]))) {
        Fail "Redis에 연결할 수 없습니다 ($($ServerEnv['REDIS_HOST']):$($ServerEnv['REDIS_PORT'])). 로컬 Redis가 떠 있는지 확인하거나, -WithBackend 없이 실행해 standalone으로 검증하세요."
    }
    Write-Host "  - Redis 도달 확인됨 ($($ServerEnv['REDIS_HOST']):$($ServerEnv['REDIS_PORT']))"

    if (-not (Test-TcpReachable -HostName $ServerEnv["API_SERVER_HOST"] -Port ([int]$ServerEnv["API_SERVER_PORT"]))) {
        Fail "내부 API 서버에 연결할 수 없습니다 ($($ServerEnv['API_SERVER_HOST']):$($ServerEnv['API_SERVER_PORT'])). 로컬 API 서버가 떠 있는지 확인하거나, -WithBackend 없이 실행해 standalone으로 검증하세요."
    }
    Write-Host "  - 내부 API 서버 도달 확인됨 ($($ServerEnv['API_SERVER_HOST']):$($ServerEnv['API_SERVER_PORT']))"
}

# 서버 포트와 JWT 시크릿을 여기서 확정해 서버·어댑터 두 프로세스에 같은 값으로 주입한다.
# .env가 있으면 그 값이 권위 소스이고, 없을 때만(standalone) 로컬 전용 기본값을 쓴다.
# 두 프로세스에 같은 값을 넣으므로, 이 기본값이 cmd/webrtc/standalone.go의 기본값과
# 달라져도 검증은 깨지지 않는다(godotenv/서버 기본값은 이미 설정된 env를 덮어쓰지 않는다).
$ServerPort = if ($ServerEnv.ContainsKey("SERVER_PORT") -and $ServerEnv["SERVER_PORT"]) { $ServerEnv["SERVER_PORT"] } else { "8083" }
$JwtSecret = if ($ServerEnv.ContainsKey("JWT_SECRET") -and $ServerEnv["JWT_SECRET"]) { $ServerEnv["JWT_SECRET"] } else { "danmalgi-standalone-dev-secret" }
$env:SERVER_PORT = $ServerPort
$env:JWT_SECRET = $JwtSecret

# 1-3. 어댑터 설정(.env) 로드 및 인증서 존재 확인
$AdapterEnvPath = Join-Path $AdapterDir ".env"
$AdapterEnv = Read-DotEnv $AdapterEnvPath

$TlsCert = if ($AdapterEnv.ContainsKey("TLS_CERT") -and $AdapterEnv["TLS_CERT"]) { $AdapterEnv["TLS_CERT"] } else { "server.crt" }
$TlsKey = if ($AdapterEnv.ContainsKey("TLS_KEY") -and $AdapterEnv["TLS_KEY"]) { $AdapterEnv["TLS_KEY"] } else { "server.key" }

if (-not (Test-Path (Join-Path $AdapterDir $TlsCert)) -or -not (Test-Path (Join-Path $AdapterDir $TlsKey))) {
    Fail "어댑터 인증서가 없습니다 ($TlsCert / $TlsKey). 먼저 '.\ssl.ps1'을 실행해 로컬 인증서를 발급하세요."
}
Write-Host "  - 어댑터 인증서 확인됨 ($TlsCert, $TlsKey)"

# 어댑터 .env 값을 프로세스 환경변수로 반영 (main.go의 flag > env > 기본값 규칙을 그대로 재사용)
foreach ($key in @("GRPC_ADDR", "LISTEN_ADDR", "CHANNEL_ID", "JWT_TOKEN", "TLS_CERT", "TLS_KEY")) {
    if ($AdapterEnv.ContainsKey($key) -and $AdapterEnv[$key]) {
        Set-Item -Path "env:$key" -Value $AdapterEnv[$key]
    }
}

# GRPC_ADDR 미지정 시 위에서 확정한 SERVER_PORT로부터 유도
if (-not $env:GRPC_ADDR) {
    $env:GRPC_ADDR = "localhost:$ServerPort"
}

# 1-4. JWT 발급 (어댑터 .env에 JWT_TOKEN이 없으면 서버 JWT_SECRET으로 새로 발급)
if (-not $env:JWT_TOKEN) {
    Write-Host "  - JWT_TOKEN 미지정: gen-token으로 새로 발급합니다"
    $token = & go run ./cmd/gen-token --jwt-secret $JwtSecret
    if ($LASTEXITCODE -ne 0) {
        Fail "JWT 발급 실패 (gen-token exit code $LASTEXITCODE)"
    }
    $env:JWT_TOKEN = ($token | Select-Object -Last 1).Trim()
}
Write-Host "  - JWT 준비됨"

$ServerArgs = @("run", "./cmd/webrtc")
if ($Standalone) {
    $ServerArgs += "--standalone"
}

Write-Host "`n== 2. 시그널링 서버 기동 (go $($ServerArgs -join ' ')) =="
$ServerProc = Start-Process -FilePath "go" -ArgumentList $ServerArgs -WorkingDirectory $RepoRoot -PassThru -NoNewWindow

$serverReady = $false
for ($i = 0; $i -lt 15; $i++) {
    Start-Sleep -Seconds 1
    if (Test-TcpReachable -HostName "localhost" -Port ([int]$ServerPort) -TimeoutMs 500) {
        $serverReady = $true
        break
    }
}
if (-not $serverReady) {
    Stop-Process -Id $ServerProc.Id -ErrorAction SilentlyContinue
    Fail "시그널링 서버가 SERVER_PORT($ServerPort)에서 기동되지 않았습니다."
}
Write-Host "  - 서버 기동 확인됨 (:$ServerPort)"

# JWT_SECRET은 1단계에서 이미 $env에 확정해두었다 — 서버와 어댑터가 같은 값을 쓴다.
# 어댑터 .env(JWT_TOKEN 등) 키 루프에는 넣지 않는다: drift 방지를 위해 항상 이 값을 사용한다.

Write-Host "`n== 3. 어댑터 기동 (go run .) =="
$AdapterProc = Start-Process -FilePath "go" -ArgumentList @("run", ".") -WorkingDirectory $AdapterDir -PassThru -NoNewWindow

$listenAddr = if ($env:LISTEN_ADDR) { $env:LISTEN_ADDR } else { ":8888" }
$port = ($listenAddr -split ":")[-1]

Write-Host "`n== 4. 접속 안내 =="
Write-Host "브라우저에서 다음 주소로 접속하세요 (자체 서명 인증서 경고는 수락):"
Write-Host "  https://localhost:$port/"
Write-Host "`n종료하려면 Ctrl+C를 누르세요."

try {
    Wait-Process -Id @($ServerProc.Id, $AdapterProc.Id) -ErrorAction SilentlyContinue
} finally {
    Write-Host "`n프로세스를 종료합니다..."
    Stop-Process -Id $ServerProc.Id -ErrorAction SilentlyContinue
    Stop-Process -Id $AdapterProc.Id -ErrorAction SilentlyContinue
}
