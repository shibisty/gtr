# Installs gtr on Windows (PowerShell 5.1 or 7):
#
#   irm https://raw.githubusercontent.com/shibisty/gtr/main/install.ps1 | iex
#
# What it does:
#   1. downloads the gtr release archive for this system and SHA256SUMS from
#      GitHub releases and checks the archive's SHA-256;
#   2. puts gtr.exe into %USERPROFILE%\.gtr\bin (GTR_HOME\bin) and creates the
#      go and gofmt shims there (`gtr self shims`);
#   3. adds that directory to the user PATH.
#
# Settings (environment variables):
#   GTR_VERSION=0.2.0       a version instead of the latest release
#   GTR_HOME=D:\gtr         the gtr home instead of %USERPROFILE%\.gtr
#   GTR_NO_SHIMS=1          do not create the go/gofmt shims
#   GTR_NO_MODIFY_PATH=1    do not change the user PATH
#   GITHUB_TOKEN=...        for the GitHub API rate limit

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue" # Invoke-WebRequest is very slow with a progress bar
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Install-Gtr {
    $repo = "shibisty/gtr"
    $api = if ($env:GTR_GITHUB_API) { $env:GTR_GITHUB_API.TrimEnd("/") } else { "https://api.github.com" }
    $download = if ($env:GTR_DOWNLOAD_BASE) { $env:GTR_DOWNLOAD_BASE.TrimEnd("/") } else { "https://github.com/$repo/releases/download" }
    $gtrHome = if ($env:GTR_HOME) { $env:GTR_HOME } else { Join-Path $env:USERPROFILE ".gtr" }
    $bin = Join-Path $gtrHome "bin"

    # A 32-bit PowerShell on 64-bit Windows reports x86; the real one is in
    # PROCESSOR_ARCHITEW6432.
    $machine = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($machine) {
        "AMD64" { "amd64" }
        "ARM64" { "arm64" }
        default { throw "gtr-install: unsupported architecture $machine" }
    }

    $headers = @{}
    if ($env:GITHUB_TOKEN) { $headers["Authorization"] = "Bearer $env:GITHUB_TOKEN" }

    $version = "$env:GTR_VERSION".TrimStart("v")
    if (-not $version) {
        $latest = Invoke-RestMethod -Uri "$api/repos/$repo/releases/latest" -Headers $headers -UseBasicParsing
        $version = "$($latest.tag_name)".TrimStart("v")
        if (-not $version) { throw "gtr-install: no release found in $repo" }
    }

    $asset = "gtr_${version}_windows_${arch}.zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("gtr-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "gtr-install: downloading gtr $version for windows/$arch"
        $zip = Join-Path $tmp $asset
        $sums = Join-Path $tmp "SHA256SUMS"
        Invoke-WebRequest -Uri "$download/v$version/$asset" -OutFile $zip -UseBasicParsing
        Invoke-WebRequest -Uri "$download/v$version/SHA256SUMS" -OutFile $sums -UseBasicParsing

        $want = $null
        foreach ($line in Get-Content $sums) {
            $f = $line -split "\s+"
            if ($f.Count -ge 2 -and $f[1].TrimStart("*") -eq $asset) { $want = $f[0].ToLower() }
        }
        if (-not $want) { throw "gtr-install: SHA256SUMS does not list $asset" }
        $got = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLower()
        if ($got -ne $want) { throw "gtr-install: SHA-256 mismatch for $asset (expected $want, got $got)" }

        $x = Join-Path $tmp "x"
        Expand-Archive -Path $zip -DestinationPath $x
        $exe = Join-Path $x "gtr.exe"
        if (-not (Test-Path $exe)) { throw "gtr-install: $asset does not contain gtr.exe" }

        New-Item -ItemType Directory -Path $bin -Force | Out-Null
        $target = Join-Path $bin "gtr.exe"
        # A running gtr.exe cannot be overwritten, but it can be renamed.
        if (Test-Path $target) {
            Remove-Item "$target.old" -Force -ErrorAction SilentlyContinue
            Move-Item $target "$target.old" -Force
        }
        Move-Item $exe $target -Force
        Remove-Item "$target.old" -Force -ErrorAction SilentlyContinue
        Write-Host "gtr-install: installed $target"

        $env:GTR_HOME = $gtrHome
        if (-not $env:GTR_NO_SHIMS) {
            & $target self shims | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "gtr-install: gtr self shims failed" }
            Write-Host "gtr-install: created the go and gofmt shims in $bin"
        }
    } finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    if (-not $env:GTR_NO_MODIFY_PATH) {
        # The user PATH straight from the registry, unexpanded: entries such as
        # %USERPROFILE%\AppData\Local\Microsoft\WindowsApps must stay variables
        # (Get/SetEnvironmentVariable would freeze them to today's values), and
        # $env:Path also holds the machine PATH.
        $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey("Environment", $true)
        try {
            $userPath = [string]$key.GetValue("Path", "", [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            $entries = @($userPath -split ";" | Where-Object { $_ })
            $first = if ($entries.Count -gt 0) { $entries[0].TrimEnd("\") } else { "" }
            if ($first -ne $bin.TrimEnd("\")) {
                $rest = @($entries | Where-Object { $_.TrimEnd("\") -ne $bin.TrimEnd("\") })
                $key.SetValue("Path", ((@($bin) + $rest) -join ";"), [Microsoft.Win32.RegistryValueKind]::ExpandString)
                Write-Host "gtr-install: added $bin to the front of the user PATH"
            }
            if ($gtrHome -ne (Join-Path $env:USERPROFILE ".gtr")) {
                $key.SetValue("GTR_HOME", $gtrHome, [Microsoft.Win32.RegistryValueKind]::String)
            }
        } finally {
            $key.Close()
        }
        # Tell running programs (Explorer, new terminals) that the environment
        # changed: setting a variable through the API broadcasts WM_SETTINGCHANGE.
        [Environment]::SetEnvironmentVariable("GTR_INSTALL_REFRESH", "1", "User")
        [Environment]::SetEnvironmentVariable("GTR_INSTALL_REFRESH", $null, "User")
        if (-not (($env:Path -split ";") -contains $bin)) {
            $env:Path = "$bin;$env:Path"
        }
    }
    Write-Host "gtr-install: done; open a new terminal and run ``gtr help``"
}

Install-Gtr
