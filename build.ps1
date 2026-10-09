$ErrorActionPreference = "Stop"

$app = "gtr"
$src = "./cmd/gtr"
$out = "./build"
$version = "0.0.1"

if(Test-Path $out){
    Remove-Item $out -Recurse -Force
}

New-Item -ItemType Directory -Path $out | Out-Null

$platforms = @(
    @{ GOOS="windows"; GOARCH="amd64"; EXT=".exe" }
    @{ GOOS="windows"; GOARCH="arm64"; EXT=".exe" }
    @{ GOOS="linux";   GOARCH="amd64"; EXT="" }
    @{ GOOS="linux";   GOARCH="arm64"; EXT="" }
    @{ GOOS="darwin";  GOARCH="amd64"; EXT="" }
    @{ GOOS="darwin";  GOARCH="arm64"; EXT="" }
)

# GOOS/GOARCH/CGO_ENABLED are restored after the build; otherwise the next
# go command in the same console would silently build for another platform.
$saved = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }

try {
foreach($p in $platforms){

    $env:CGO_ENABLED = "0"
    $env:GOOS = $p.GOOS
    $env:GOARCH = $p.GOARCH

    $dir = Join-Path $out "$version-$($p.GOOS)-$($p.GOARCH)"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    
    $file = Join-Path $dir "$app$($p.EXT)"
    Write-Host "Building $file"

    & go build `
        -trimpath `
        "-ldflags=-s -w -X main.Version=$version" `
        -o $file `
        $src

    if ($LASTEXITCODE -ne 0) { throw "go build failed for $($p.GOOS)/$($p.GOARCH)" }
}
} finally {
    foreach ($k in $saved.Keys) {
        if ($null -eq $saved[$k]) { Remove-Item -Path "env:$k" -ErrorAction SilentlyContinue }
        else { Set-Item -Path "env:$k" -Value $saved[$k] }
    }
}

Write-Host ""
Write-Host "Done."
