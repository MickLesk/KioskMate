<#
.SYNOPSIS
    Installs KioskMate for the current Windows user and starts it at logon.

.DESCRIPTION
    Copies kioskmate.exe to %LOCALAPPDATA%\Programs\KioskMate and registers a
    Task Scheduler task that starts KioskMate hidden when this user logs on.
    KioskMate then starts Microsoft Edge (or Google Chrome) full screen.

    Configuration:   %APPDATA%\KioskMate\config.json
    Browser profile: %LOCALAPPDATA%\KioskMate\Browser

    Run it again to update the installed binary. Use -Uninstall to remove the
    task and the installed binary; configuration and browser profiles stay.

.PARAMETER Source
    kioskmate.exe to install. Defaults to the file next to this script.

.PARAMETER InstallDir
    Target folder. Defaults to %LOCALAPPDATA%\Programs\KioskMate.

.PARAMETER TaskName
    Name of the scheduled task. Defaults to KioskMate.

.PARAMETER Start
    Start the task right after registering it.

.PARAMETER FirewallRule
    Allow inbound connections to the Admin UI on private and domain networks.
    Requires an elevated PowerShell.

.PARAMETER Uninstall
    Stop KioskMate, remove the scheduled task, the firewall rule and the
    installed binary.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\install.ps1 -Start

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\install.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [string]$Source = (Join-Path $PSScriptRoot 'kioskmate.exe'),
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\KioskMate'),
    [string]$TaskName = 'KioskMate',
    [switch]$Start,
    [switch]$FirewallRule,
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
$exe = Join-Path $InstallDir 'kioskmate.exe'
$firewallName = 'KioskMate Admin UI'
$user = "$env:USERDOMAIN\$env:USERNAME"

function Test-Elevated {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Stop-KioskMate {
    $task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($task -and $task.State -eq 'Running') {
        Stop-ScheduledTask -TaskName $TaskName
    }
    Get-Process -Name 'kioskmate' -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -eq $exe } |
        Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
}

if ($Uninstall) {
    Stop-KioskMate
    if (Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
        Write-Host "Removed scheduled task '$TaskName'."
    }
    if (Get-NetFirewallRule -DisplayName $firewallName -ErrorAction SilentlyContinue) {
        if (Test-Elevated) {
            Remove-NetFirewallRule -DisplayName $firewallName
            Write-Host "Removed firewall rule '$firewallName'."
        } else {
            Write-Warning "Firewall rule '$firewallName' left in place; run -Uninstall from an elevated PowerShell to remove it."
        }
    }
    if (Test-Path $exe) {
        Remove-Item $exe -Force
        Write-Host "Removed $exe."
    }
    if ((Test-Path $InstallDir) -and -not (Get-ChildItem $InstallDir -Force)) {
        Remove-Item $InstallDir -Force
    }
    Write-Host "Configuration and browser profiles were kept in $env:APPDATA\KioskMate and $env:LOCALAPPDATA\KioskMate."
    return
}

if (-not (Test-Path $Source -PathType Leaf)) {
    throw "kioskmate.exe not found at '$Source'. Pass -Source <path>."
}

Stop-KioskMate
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
$sourcePath = (Resolve-Path $Source).Path
if ($sourcePath -ne $exe) {
    Copy-Item -Path $sourcePath -Destination $exe -Force
}
# Files downloaded from the internet carry a Zone.Identifier stream that
# makes Windows ask before every start.
Unblock-File -Path $exe -ErrorAction SilentlyContinue

# KioskMate is a console program. PowerShell starts it with a hidden window,
# waits for it and starts it again after 10 seconds when it ended with an
# error. Task Scheduler alone only retries tasks that fail to launch.
$quotedExe = $exe -replace "'", "''"
$command = "while (`$true) { `$p = Start-Process -FilePath '$quotedExe' -WindowStyle Hidden -PassThru -Wait; if (`$p.ExitCode -eq 0) { exit 0 }; Start-Sleep -Seconds 10 }"
$action = New-ScheduledTaskAction -Execute 'powershell.exe' `
    -Argument "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -Command `"$command`"" `
    -WorkingDirectory $InstallDir
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
# Give the desktop and network a moment after logon.
$trigger.Delay = 'PT15S'
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -MultipleInstances IgnoreNew `
    -RestartCount 5 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit (New-TimeSpan -Seconds 0)

try {
    Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings `
        -Description 'Starts KioskMate and its full-screen browser at logon.' -Force | Out-Null
} catch {
    throw "Registering the scheduled task failed: $($_.Exception.Message). If access was denied, run this script from an elevated PowerShell."
}
Write-Host "Installed $exe and registered scheduled task '$TaskName' (at logon of $user)."

if ($FirewallRule) {
    if (-not (Test-Elevated)) {
        Write-Warning 'Skipping the firewall rule: run with -FirewallRule from an elevated PowerShell.'
    } else {
        Get-NetFirewallRule -DisplayName $firewallName -ErrorAction SilentlyContinue | Remove-NetFirewallRule
        New-NetFirewallRule -DisplayName $firewallName -Direction Inbound -Program $exe -Action Allow -Profile Private, Domain | Out-Null
        Write-Host "Added firewall rule '$firewallName' for private and domain networks."
    }
}

if ($Start) {
    Start-ScheduledTask -TaskName $TaskName
    Write-Host "Started '$TaskName'. On first start, read the setup token with:"
} else {
    Write-Host "KioskMate starts at the next logon, or now with: Start-ScheduledTask -TaskName '$TaskName'"
    Write-Host 'Read the setup token with:'
}
Write-Host "  & '$exe' --admin-info"
Write-Host 'Admin UI: http://localhost:33333'
