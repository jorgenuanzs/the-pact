[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$InstallDir = (Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs\Pact"),
    [string]$GitHubToken = "",
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repository = "jorgenuanzs/the-pact"

if ([string]::IsNullOrWhiteSpace($GitHubToken)) {
    if (-not [string]::IsNullOrWhiteSpace($env:GH_TOKEN)) {
        $GitHubToken = $env:GH_TOKEN
    } elseif (-not [string]::IsNullOrWhiteSpace($env:GITHUB_TOKEN)) {
        $GitHubToken = $env:GITHUB_TOKEN
    } elseif (Get-Command gh -ErrorAction SilentlyContinue) {
        $tokenFromGitHubCLI = & gh auth token 2>$null
        if ($LASTEXITCODE -eq 0) {
            $GitHubToken = ([string]$tokenFromGitHubCLI).Trim()
        }
    }
}

$apiHeaders = @{
    Accept = "application/vnd.github+json"
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent" = "Pact-Windows-Installer"
}
$assetHeaders = @{
    Accept = "application/octet-stream"
    "X-GitHub-Api-Version" = "2022-11-28"
    "User-Agent" = "Pact-Windows-Installer"
}
if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    $apiHeaders["Authorization"] = "Bearer $GitHubToken"
    $assetHeaders["Authorization"] = "Bearer $GitHubToken"
}

function Invoke-GitHubJson {
    param([Parameter(Mandatory)][string]$Uri)

    for ($attempt = 1; $attempt -le 4; $attempt++) {
        try {
            return Invoke-RestMethod -Headers $apiHeaders -Uri $Uri
        } catch {
            if ($attempt -eq 4) {
                throw
            }
            Start-Sleep -Seconds ([Math]::Min([Math]::Pow(2, $attempt), 8))
        }
    }
}

function Save-GitHubAsset {
    param(
        [Parameter(Mandatory)][string]$Uri,
        [Parameter(Mandatory)][string]$Destination
    )

    for ($attempt = 1; $attempt -le 4; $attempt++) {
        try {
            Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
            Invoke-WebRequest -UseBasicParsing -Headers $assetHeaders -Uri $Uri -OutFile $Destination | Out-Null
            return
        } catch {
            if ($attempt -eq 4) {
                throw
            }
            Start-Sleep -Seconds ([Math]::Min([Math]::Pow(2, $attempt), 8))
        }
    }
}

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
    $releaseUri = "https://api.github.com/repos/$repository/releases/latest"
} else {
    $tag = $Version.Trim()
    if (-not $tag.StartsWith("v")) {
        $tag = "v$tag"
    }
    $encodedTag = [Uri]::EscapeDataString($tag)
    $releaseUri = "https://api.github.com/repos/$repository/releases/tags/$encodedTag"
}

try {
    $release = Invoke-GitHubJson -Uri $releaseUri
} catch {
    throw "Unable to read the Pact release from GitHub. If the repository is private, authenticate with 'gh auth login' or set GH_TOKEN to a token with Contents read access. $($_.Exception.Message)"
}
$tag = [string]$release.tag_name
if ($tag -notmatch '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') {
    throw "Invalid Pact version '$tag'."
}

$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$assetName = "pact_windows_$architecture.zip"
$archiveAsset = @($release.assets) | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
$checksumsAsset = @($release.assets) | Where-Object { $_.name -eq "checksums.txt" } | Select-Object -First 1
if (-not $archiveAsset -or -not $checksumsAsset) {
    throw "Pact release $tag does not contain $assetName and checksums.txt."
}
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("pact-install-" + [Guid]::NewGuid().ToString("N"))
$archivePath = Join-Path $temporaryDirectory $assetName
$checksumsPath = Join-Path $temporaryDirectory "checksums.txt"
$expandedPath = Join-Path $temporaryDirectory "expanded"

try {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    Save-GitHubAsset -Uri ([string]$archiveAsset.url) -Destination $archivePath
    Save-GitHubAsset -Uri ([string]$checksumsAsset.url) -Destination $checksumsPath

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
