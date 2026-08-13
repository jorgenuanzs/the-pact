[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$InstallDir = (Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs\Pact"),
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repository = "jorgenuanzs/the-pact"
$architectureName = if ($env:PROCESSOR_ARCHITEW6432) {
    $env:PROCESSOR_ARCHITEW6432
} else {
    $env:PROCESSOR_ARCHITECTURE
}

$architecture = switch ($architectureName.ToUpperInvariant()) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Pact does not publish a Windows binary for architecture '$architectureName'." }
}

if ($Version -eq "latest") {
    $releaseParameters = @{
        Headers = @{ Accept = "application/vnd.github+json" }
        Uri = "https://api.github.com/repos/$repository/releases/latest"
    }
    $release = Invoke-RestMethod @releaseParameters
    $tag = [string]$release.tag_name
} else {
    $tag = $Version.Trim()
    if (-not $tag.StartsWith("v")) {
        $tag = "v$tag"
    }
}

if ($tag -notmatch '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') {
    throw "Invalid Pact version '$tag'."
}

$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$assetName = "pact_windows_$architecture.zip"
$releaseBase = "https://github.com/$repository/releases/download/$tag"
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("pact-install-" + [Guid]::NewGuid().ToString("N"))
$archivePath = Join-Path $temporaryDirectory $assetName
$checksumsPath = Join-Path $temporaryDirectory "checksums.txt"
$expandedPath = Join-Path $temporaryDirectory "expanded"

try {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    Invoke-WebRequest -UseBasicParsing -Uri "$releaseBase/$assetName" -OutFile $archivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$releaseBase/checksums.txt" -OutFile $checksumsPath

    $escapedAsset = [Regex]::Escape($assetName)
    $checksumLine = Get-Content $checksumsPath |
        Where-Object { $_ -match "\s+\*?$escapedAsset$" } |
        Select-Object -First 1
    if (-not $checksumLine -or $checksumLine -notmatch '^([a-fA-F0-9]{64})\s+') {
        throw "The release does not contain a valid SHA-256 checksum for $assetName."
    }
    $expectedHash = $Matches[1].ToLowerInvariant()
    $actualHash = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum mismatch for $assetName. The downloaded file was not installed."
    }

    Expand-Archive -LiteralPath $archivePath -DestinationPath $expandedPath
    $sourceExecutable = Join-Path $expandedPath "pact.exe"
    if (-not (Test-Path -LiteralPath $sourceExecutable -PathType Leaf)) {
        throw "The Pact release archive does not contain pact.exe."
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $destinationExecutable = Join-Path $InstallDir "pact.exe"
    Copy-Item -LiteralPath $sourceExecutable -Destination $destinationExecutable -Force

    if (-not $NoPathUpdate) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if (-not ($pathEntries | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') })) {
            $updatedPath = (@($pathEntries) + $InstallDir) -join ';'
            [Environment]::SetEnvironmentVariable("Path", $updatedPath, "User")
        }
        if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') })) {
            $env:Path = "$InstallDir;$env:Path"
        }
    }

    & $destinationExecutable version
    Write-Host "Pact $tag was installed at $destinationExecutable"
    if (-not $NoPathUpdate) {
        Write-Host "Open a new terminal before running 'pact'."
    }
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        Write-Warning "Git is required for pact init, pact connect, worktrees, and repository observation."
    }
} finally {
    if (Test-Path -LiteralPath $temporaryDirectory) {
        Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
    }
}
