# =============================================================================
# BoxCheckr Agent Script (Windows PowerShell)
# Generated for: {{.Email}}
# =============================================================================
#
# This script collects ONLY the following information:
#   - Your computer's hostname
#   - Operating system and version
#   - Whether disk encryption is enabled (BitLocker)
#   - Whether antivirus protection is active (Defender, McAfee, Norton, etc.)
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

# Check antivirus (Windows Defender + third-party like McAfee, Norton, etc.)
$AVEnabled = $false
$AVDetails = ""
$AVProducts = @()

# Check Windows Security Center for all registered AV products
try {
    $SecurityCenterAV = Get-CimInstance -Namespace "root/SecurityCenter2" -ClassName AntiVirusProduct -ErrorAction SilentlyContinue
    if ($SecurityCenterAV) {
        foreach ($av in $SecurityCenterAV) {
            # productState is a bitmask: bits 4-7 indicate if enabled, bits 8-11 indicate if up-to-date
            $state = $av.productState
            $enabled = (($state -shr 12) -band 0xF) -eq 1
            if ($enabled) {
                $AVProducts += $av.displayName
            }
        }
    }
} catch {
    # Security Center not available (older Windows or Server edition)
}

# Also check Windows Defender directly for more details
try {
    $Defender = Get-MpComputerStatus -ErrorAction SilentlyContinue
    if ($Defender.RealTimeProtectionEnabled) {
        if ("Windows Defender" -notin $AVProducts) {
            $AVProducts += "Windows Defender"
        }
        if ($Defender.AntivirusSignatureLastUpdated) {
            $LastUpdate = $Defender.AntivirusSignatureLastUpdated.ToString("yyyy-MM-dd")
            # Add signature date to Defender entry
            $AVProducts = $AVProducts | ForEach-Object {
                if ($_ -eq "Windows Defender") { "$_ (signatures: $LastUpdate)" } else { $_ }
            }
        }
    }
} catch {
    # Defender cmdlets not available
}

if ($AVProducts.Count -gt 0) {
    $AVEnabled = $true
    $AVDetails = $AVProducts -join ", "
} else {
    $AVDetails = "No active antivirus detected"
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

# Check screen lock and sign-in settings
$SLEnabled = $false
$SLTimeout = 0
$SLDetails = ""
$SLFeatures = @()

try {
    # Check "Require sign-in" setting (Settings > Accounts > Sign-in options)
    # 0 = Never, 1 = When PC wakes from sleep
    $SignInRequired = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" -Name "DisableLockWorkstation" -ErrorAction SilentlyContinue).DisableLockWorkstation
    $WakeSignIn = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Policies\Microsoft\Power\PowerSettings\0e796bdb-100d-47d6-a2d5-f7d2daa51f51" -Name "ACSettingIndex" -ErrorAction SilentlyContinue).ACSettingIndex

    # Also check user-level setting
    if ($null -eq $WakeSignIn) {
        $WakeSignIn = (Get-ItemProperty -Path "HKCU:\Control Panel\Desktop" -Name "DelayLockInterval" -ErrorAction SilentlyContinue).DelayLockInterval
        # 0 = immediately, other values = delay in seconds, missing = system default (usually immediate)
    }

    if ($SignInRequired -ne 1) {
        # Lock workstation is NOT disabled
        if ($null -eq $WakeSignIn -or $WakeSignIn -eq 0) {
            $SLFeatures += "Sign-in required on wake"
            $SLEnabled = $true
        }
    }

    # Check Dynamic Lock (locks when Bluetooth device leaves range)
    $DynamicLock = (Get-ItemProperty -Path "HKCU:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Name "EnableGoodbye" -ErrorAction SilentlyContinue).EnableGoodbye
    if ($DynamicLock -eq 1) {
        $SLFeatures += "Dynamic Lock enabled"
        $SLEnabled = $true
    }

    # Check screen saver timeout with password protection
    $SSTimeout = (Get-ItemProperty -Path "HKCU:\Control Panel\Desktop" -Name ScreenSaveTimeOut -ErrorAction SilentlyContinue).ScreenSaveTimeOut
    $SSSecure = (Get-ItemProperty -Path "HKCU:\Control Panel\Desktop" -Name ScreenSaverIsSecure -ErrorAction SilentlyContinue).ScreenSaverIsSecure

    if ($SSSecure -eq "1" -and $SSTimeout) {
        $SLEnabled = $true
        $SLTimeout = [math]::Round([int]$SSTimeout / 60)
        $SLFeatures += "Screen saver lock ($SLTimeout min)"
    }

    # Check display timeout from power settings
    $PowerTimeout = powercfg /query SCHEME_CURRENT SUB_VIDEO VIDEOIDLE 2>$null | Select-String "Current AC Power Setting Index" | ForEach-Object { $_.ToString().Split(":")[1].Trim() }
    if ($PowerTimeout) {
        $TimeoutSeconds = [Convert]::ToInt32($PowerTimeout, 16)
        if ($TimeoutSeconds -gt 0) {
            $DisplayTimeout = [math]::Round($TimeoutSeconds / 60)
            if ($SLTimeout -eq 0 -or $DisplayTimeout -lt $SLTimeout) {
                $SLTimeout = $DisplayTimeout
            }
            $SLFeatures += "Display off ($DisplayTimeout min)"
        }
    }

    # Check console lock timeout (inactivity lock, separate from screen saver)
    $InactivityTimeout = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" -Name "InactivityTimeoutSecs" -ErrorAction SilentlyContinue).InactivityTimeoutSecs
    if ($InactivityTimeout -and $InactivityTimeout -gt 0) {
        $InactivityMins = [math]::Round($InactivityTimeout / 60)
        $SLFeatures += "Inactivity lock ($InactivityMins min)"
        $SLEnabled = $true
        if ($SLTimeout -eq 0 -or $InactivityMins -lt $SLTimeout) {
            $SLTimeout = $InactivityMins
        }
    }

    if ($SLFeatures.Count -gt 0) {
        $SLDetails = $SLFeatures -join ", "
    } else {
        $SLDetails = "No automatic lock configured"
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
