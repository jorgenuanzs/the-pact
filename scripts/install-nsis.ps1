[CmdletBinding()]
param(
    [ValidateRange(1, 10)]
    [int]$Attempts = 3
)

$ErrorActionPreference = "Stop"

function Find-MakeNSIS {
    $candidates = [System.Collections.Generic.List[string]]::new()

    $command = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if ($command) {
        $candidates.Add($command.Source)
    }

    if (${env:ProgramFiles(x86)}) {
        $candidates.Add((Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe"))
    }
    if ($env:ProgramFiles) {
        $candidates.Add((Join-Path $env:ProgramFiles "NSIS\makensis.exe"))
    }
    if ($env:ChocolateyInstall) {
        $candidates.Add((Join-Path $env:ChocolateyInstall "bin\makensis.exe"))
    }
    if ($env:ChocolateyToolsLocation) {
        $candidates.Add((Join-Path $env:ChocolateyToolsLocation "nsis\makensis.exe"))
    }

    foreach ($candidate in ($candidates | Select-Object -Unique)) {
        if ($candidate -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }

    return $null
}

$makeNSIS = Find-MakeNSIS

if (-not $makeNSIS) {
    if (-not (Get-Command choco.exe -ErrorAction SilentlyContinue)) {
        throw "NSIS is not installed and Chocolatey is unavailable."
    }

    for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
        Write-Host "Installing NSIS with Chocolatey (attempt $attempt of $Attempts)..."
        & choco.exe install nsis --yes --no-progress
        $chocoExitCode = $LASTEXITCODE

        $makeNSIS = Find-MakeNSIS
        if ($makeNSIS) {
            break
        }

        Write-Warning "Chocolatey exited with code $chocoExitCode, but makensis.exe was not installed."
        if ($attempt -lt $Attempts) {
            Start-Sleep -Seconds ([Math]::Min(30, 5 * $attempt))
        }
    }
}

if (-not $makeNSIS) {
    throw "NSIS installation failed after $Attempts attempts: makensis.exe was not found."
}

$nsisDirectory = Split-Path -Parent $makeNSIS
$env:PATH = "$nsisDirectory;$env:PATH"

if ($env:GITHUB_PATH) {
    $nsisDirectory | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
}

& $makeNSIS /VERSION
if ($LASTEXITCODE -ne 0) {
    throw "makensis.exe was found at '$makeNSIS' but could not run successfully."
}

Write-Host "NSIS is ready at $makeNSIS"
