# =============================================================================
# BoxCheckr Agent Script (Windows PowerShell)
# Generated for: {{.Email}}
# =============================================================================
#
# This script collects ONLY the following information:
#   - Your computer's hostname
#   - Operating system and version
#   - Whether disk encryption is enabled (BitLocker)
#   - Whether antivirus protection is active (Windows Defender)
#   - Whether Windows Firewall is enabled
#   - Whether screen lock is configured and its timeout
#
# NO personal files, passwords, browsing history, or sensitive data is collected.
# You can inspect this entire script before running it.
#
# Data is sent to: {{.ServerURL}}
# =============================================================================

$ErrorActionPreference = "Stop"

$TOKEN = "{{.Token}}"
$SERVER = "{{.ServerURL}}"

# Get system info
$Hostname = $env:COMPUTERNAME
$OS = "windows"
$OSVersion = (Get-CimInstance Win32_OperatingSystem).Version
$OSBuild = (Get-CimInstance Win32_OperatingSystem).BuildNumber

# Check BitLocker disk encryption
$DiskEncrypted = $false
$DiskDetails = ""
try {
    $BitLocker = Get-BitLockerVolume -MountPoint "C:" -ErrorAction SilentlyContinue
    if ($BitLocker.ProtectionStatus -eq "On") {
        $DiskEncrypted = $true
        $DiskDetails = "BitLocker enabled ($($BitLocker.EncryptionMethod))"
    } else {
        $DiskDetails = "BitLocker not enabled"
    }
} catch {
    $DiskDetails = "BitLocker status unknown (may require admin)"
}

# Check Windows Defender / antivirus
$AVEnabled = $false
$AVDetails = ""
try {
    $Defender = Get-MpComputerStatus -ErrorAction SilentlyContinue
    if ($Defender.RealTimeProtectionEnabled) {
        $AVEnabled = $true
        $AVDetails = "Windows Defender active"
        if ($Defender.AntivirusSignatureLastUpdated) {
            $LastUpdate = $Defender.AntivirusSignatureLastUpdated.ToString("yyyy-MM-dd")
            $AVDetails = "$AVDetails (signatures: $LastUpdate)"
        }
    } else {
        $AVDetails = "Windows Defender real-time protection disabled"
    }
} catch {
    # Try Windows Security Center as fallback
    try {
        $AV = Get-CimInstance -Namespace "root/SecurityCenter2" -ClassName AntiVirusProduct -ErrorAction SilentlyContinue
        if ($AV) {
            $AVEnabled = $true
            $AVDetails = $AV.displayName -join ", "
        }
    } catch {
        $AVDetails = "Antivirus status unknown"
    }
}

# Check Windows Firewall
$FWEnabled = $false
$FWDetails = ""
try {
    $FWProfiles = Get-NetFirewallProfile -ErrorAction SilentlyContinue
    $EnabledProfiles = $FWProfiles | Where-Object { $_.Enabled -eq $true }
    if ($EnabledProfiles) {
        $FWEnabled = $true
        $ProfileNames = ($EnabledProfiles | ForEach-Object { $_.Name }) -join ", "
        $FWDetails = "Windows Firewall enabled ($ProfileNames)"
    } else {
        $FWDetails = "Windows Firewall disabled"
    }
} catch {
    $FWDetails = "Firewall status unknown"
}

# Check screen lock settings
$SLEnabled = $false
$SLTimeout = 0
$SLDetails = ""
try {
    # Check screen saver timeout from registry
    $SSTimeout = (Get-ItemProperty -Path "HKCU:\Control Panel\Desktop" -Name ScreenSaveTimeOut -ErrorAction SilentlyContinue).ScreenSaveTimeOut
    $SSSecure = (Get-ItemProperty -Path "HKCU:\Control Panel\Desktop" -Name ScreenSaverIsSecure -ErrorAction SilentlyContinue).ScreenSaverIsSecure

    if ($SSSecure -eq "1" -and $SSTimeout) {
        $SLEnabled = $true
        $SLTimeout = [math]::Round([int]$SSTimeout / 60)
        $SLDetails = "Screen saver lock after $SLTimeout minutes"
    } else {
        # Check power settings for screen timeout
        $PowerTimeout = powercfg /query SCHEME_CURRENT SUB_VIDEO VIDEOIDLE 2>$null | Select-String "Current AC Power Setting Index" | ForEach-Object { $_.ToString().Split(":")[1].Trim() }
        if ($PowerTimeout) {
            $TimeoutSeconds = [Convert]::ToInt32($PowerTimeout, 16)
            if ($TimeoutSeconds -gt 0) {
                $SLTimeout = [math]::Round($TimeoutSeconds / 60)
                $SLDetails = "Display timeout $SLTimeout minutes (lock status depends on sign-in options)"
            }
        }
        if (-not $SLDetails) {
            $SLDetails = "Screen lock not configured via screen saver"
        }
    }
} catch {
    $SLDetails = "Screen lock settings unknown"
}

# Build payload
$Payload = @{
    hostname = $Hostname
    os = $OS
    os_version = "$OSVersion (Build $OSBuild)"
    disk_encrypted = $DiskEncrypted
    disk_encryption_details = $DiskDetails
    antivirus_enabled = $AVEnabled
    antivirus_details = $AVDetails
    firewall_enabled = $FWEnabled
    firewall_details = $FWDetails
    screen_lock_enabled = $SLEnabled
    screen_lock_timeout = $SLTimeout
    screen_lock_details = $SLDetails
} | ConvertTo-Json

Write-Host "BoxCheckr Agent"
Write-Host "==============="
Write-Host "Hostname: $Hostname"
Write-Host "OS: Windows $OSVersion (Build $OSBuild)"
Write-Host "Disk Encrypted: $DiskEncrypted ($DiskDetails)"
Write-Host "Antivirus: $AVEnabled ($AVDetails)"
Write-Host "Firewall: $FWEnabled ($FWDetails)"
Write-Host "Screen Lock: $SLEnabled ($SLDetails)"
Write-Host ""

# Send to server
Write-Host "Sending inventory to $SERVER..."
try {
    $Headers = @{
        "Authorization" = "Bearer $TOKEN"
        "Content-Type" = "application/json"
    }
    $Response = Invoke-RestMethod -Uri "$SERVER/api/v1/inventory" -Method POST -Headers $Headers -Body $Payload
    Write-Host "Success! Inventory submitted."
} catch {
    Write-Host "Error submitting inventory: $_"
    exit 1
}

# {{if eq .Mode "monitor"}}
# =============================================================================
# Install scheduled monitoring (runs weekly on Mondays at 9am)
# =============================================================================
Write-Host ""
Write-Host "Installing weekly monitoring..."

$ScriptDir = "$env:LOCALAPPDATA\BoxCheckr"
if (-not (Test-Path $ScriptDir)) {
    New-Item -ItemType Directory -Path $ScriptDir -Force | Out-Null
}

# Create scheduled task
$ScriptURL = "{{.ServerURL}}/machines/{{.MachineID}}/script?mode=onetime"
$TaskAction = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -Command `"irm '$ScriptURL' | iex`""
$TaskTrigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday -At 9am
$TaskSettings = New-ScheduledTaskSettingsSet -StartWhenAvailable -DontStopOnIdleEnd

Register-ScheduledTask -TaskName "BoxCheckr Agent" -Action $TaskAction -Trigger $TaskTrigger -Settings $TaskSettings -Force | Out-Null

Write-Host "Installed scheduled task (runs Mondays at 9am)"
Write-Host "To uninstall: Unregister-ScheduledTask -TaskName 'BoxCheckr Agent' -Confirm:`$false"
Write-Host "             Remove-Item -Recurse '$ScriptDir'"
# {{end}}
