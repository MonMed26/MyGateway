$ErrorActionPreference = "Stop"

Write-Host "Starting Go gateway on :8080 and Next.js dev server on :3000" -ForegroundColor Cyan

$gateway = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/gateway" -PassThru -WorkingDirectory "$PSScriptRoot\.."
try {
    Push-Location "$PSScriptRoot\..\web"
    npm run dev
}
finally {
    Pop-Location
    Stop-Process -Id $gateway.Id -Force -ErrorAction SilentlyContinue
}
