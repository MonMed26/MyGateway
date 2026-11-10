$ErrorActionPreference = "Stop"

Write-Host "==> Building dashboard (Next.js static export)" -ForegroundColor Cyan
Push-Location "$PSScriptRoot\..\web"
try {
    if (-not (Test-Path "node_modules")) {
        npm install --no-audit --no-fund
    }
    npm run build
}
finally {
    Pop-Location
}

Write-Host "==> Embedding dashboard into the Go binary" -ForegroundColor Cyan
$dist = "$PSScriptRoot\..\internal\webui\dist"
if (Test-Path $dist) {
    Remove-Item -Recurse -Force "$dist\*"
}
New-Item -ItemType Directory -Force -Path $dist | Out-Null
Copy-Item -Recurse -Force "$PSScriptRoot\..\web\out\*" $dist

Write-Host "==> Building gateway binary" -ForegroundColor Cyan
Push-Location "$PSScriptRoot\.."
try {
    go build -o gateway.exe ./cmd/gateway
}
finally {
    Pop-Location
}

Write-Host "Done. Run .\gateway.exe" -ForegroundColor Green
