#!/bin/bash
# =============================================================================
# BoxCheckr Agent Script (macOS)
# Generated for: {{.Email}}
# =============================================================================
#
# This script collects ONLY the following information:
#   - Your computer's hostname
#   - Operating system and version
#   - Whether disk encryption is enabled (FileVault)
#   - Whether antivirus protection is active (XProtect)
#   - Whether the firewall is enabled
#   - Whether screen lock is configured and its timeout
#
# NO personal files, passwords, browsing history, or sensitive data is collected.
# You can inspect this entire script before running it.
#
# Data is sent to: {{.ServerURL}}
# =============================================================================

set -e

TOKEN="{{.Token}}"
SERVER="{{.ServerURL}}"

# Get system info
OS="darwin"
OS_VERSION=$(sw_vers -productVersion)
HOSTNAME=$(hostname)

# Check FileVault disk encryption
DISK_ENCRYPTED=false
DISK_DETAILS=""
if fdesetup status 2>/dev/null | grep -q "FileVault is On"; then
    DISK_ENCRYPTED=true
    DISK_DETAILS="FileVault enabled"
else
    DISK_DETAILS="FileVault disabled"
fi

# Check antivirus (XProtect is built into macOS)
AV_ENABLED=false
AV_DETAILS=""
if [[ -d "/Library/Apple/System/Library/CoreServices/XProtect.bundle" ]]; then
    AV_ENABLED=true
    AV_DETAILS="XProtect active"
fi
# Check for common third-party AV
if pgrep -x "falcon" &>/dev/null || pgrep -x "CrowdStrike" &>/dev/null; then
    AV_DETAILS="$AV_DETAILS, CrowdStrike Falcon"
fi
if pgrep -x "SentinelAgent" &>/dev/null; then
    AV_DETAILS="$AV_DETAILS, SentinelOne"
fi

# Check firewall status
FW_ENABLED=false
FW_DETAILS=""
if command -v /usr/libexec/ApplicationFirewall/socketfilterfw &>/dev/null; then
    FW_STATUS=$(/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate 2>/dev/null)
    if echo "$FW_STATUS" | grep -q "enabled"; then
        FW_ENABLED=true
        FW_DETAILS="macOS Application Firewall enabled"
    else
        FW_DETAILS="macOS Application Firewall disabled"
    fi
else
    FW_DETAILS="Firewall status unknown"
fi

# Check screen lock settings
SL_ENABLED=false
SL_TIMEOUT=0
SL_DETAILS=""

# On modern macOS, check multiple places for screen lock settings
# 1. Check if password is required after sleep/screen saver (askForPassword)
ASK_FOR_PWD=$(defaults read com.apple.screensaver askForPassword 2>/dev/null || echo "")
# 2. Check the delay before password is required (0 = immediately)
ASK_DELAY=$(defaults read com.apple.screensaver askForPasswordDelay 2>/dev/null || echo "")

# If askForPassword returns nothing, it might be using system defaults (enabled by default on newer macOS)
# or controlled by MDM/security policy
if [[ "$ASK_FOR_PWD" == "1" ]] || [[ -z "$ASK_FOR_PWD" ]]; then
    # Check if we can verify via security authorizationdb (indicates password required)
    if security authorizationdb read system.login.screensaver 2>/dev/null | grep -q "authenticate-session-owner"; then
        SL_ENABLED=true
    fi
    # Also check if Touch ID or password is required (via bioutil or system_profiler would need admin)
    # For non-admin detection, assume enabled if delay is 0 or not set (immediate lock)
    if [[ "$ASK_DELAY" == "0" ]] || [[ -z "$ASK_DELAY" ]]; then
        SL_ENABLED=true
    elif [[ "$ASK_FOR_PWD" == "1" ]]; then
        SL_ENABLED=true
    fi
fi

# Get screen saver / display sleep timeout
IDLE_TIME=$(defaults -currentHost read com.apple.screensaver idleTime 2>/dev/null || echo "0")
DISPLAY_SLEEP=$(pmset -g 2>/dev/null | grep displaysleep | awk '{print $2}' || echo "0")

if [[ "$SL_ENABLED" == "true" ]]; then
    if [[ "$IDLE_TIME" -gt 0 ]]; then
        SL_TIMEOUT=$((IDLE_TIME / 60))
        if [[ "$ASK_DELAY" == "0" ]] || [[ -z "$ASK_DELAY" ]]; then
            SL_DETAILS="Screen lock immediately after ${SL_TIMEOUT} min idle"
        else
            DELAY_MIN=$((ASK_DELAY / 60))
            SL_DETAILS="Screen lock ${DELAY_MIN} min after ${SL_TIMEOUT} min idle"
        fi
    elif [[ "$DISPLAY_SLEEP" -gt 0 ]]; then
        SL_TIMEOUT=$DISPLAY_SLEEP
        SL_DETAILS="Display sleep ${SL_TIMEOUT} min (password/Touch ID required)"
    else
        SL_DETAILS="Password/Touch ID required (no auto-idle configured)"
    fi
else
    # Not enabled - check why
    if [[ "$ASK_FOR_PWD" == "0" ]]; then
        SL_DETAILS="Password not required after sleep"
    else
        SL_DETAILS="Screen lock status unknown"
    fi
fi

# Build JSON payload
JSON=$(cat <<EOF
{
    "hostname": "$HOSTNAME",
    "os": "$OS",
    "os_version": "$OS_VERSION",
    "disk_encrypted": $DISK_ENCRYPTED,
    "disk_encryption_details": "$DISK_DETAILS",
    "antivirus_enabled": $AV_ENABLED,
    "antivirus_details": "$AV_DETAILS",
    "firewall_enabled": $FW_ENABLED,
    "firewall_details": "$FW_DETAILS",
    "screen_lock_enabled": $SL_ENABLED,
    "screen_lock_timeout": $SL_TIMEOUT,
    "screen_lock_details": "$SL_DETAILS"
}
EOF
)

echo "BoxCheckr Agent"
echo "==============="
echo "Hostname: $HOSTNAME"
echo "OS: macOS $OS_VERSION"
echo "Disk Encrypted: $DISK_ENCRYPTED ($DISK_DETAILS)"
echo "Antivirus: $AV_ENABLED ($AV_DETAILS)"
echo "Firewall: $FW_ENABLED ($FW_DETAILS)"
echo "Screen Lock: $SL_ENABLED ($SL_DETAILS)"
echo ""

# Send to server
echo "Sending inventory to $SERVER..."
RESPONSE=$(curl -s -X POST "$SERVER/api/v1/inventory" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$JSON")

if echo "$RESPONSE" | grep -q '"status":"ok"'; then
    echo "Success! Inventory submitted."
else
    echo "Error submitting inventory: $RESPONSE"
    exit 1
fi

{{if eq .Mode "monitor"}}
# =============================================================================
# Install scheduled monitoring (runs weekly on Mondays at 9am)
# =============================================================================
echo ""
echo "Installing weekly monitoring..."

mkdir -p "$HOME/.boxcheckr"

# Create LaunchAgent plist
PLIST_PATH="$HOME/Library/LaunchAgents/com.boxcheckr.agent.plist"
mkdir -p "$HOME/Library/LaunchAgents"

cat > "$PLIST_PATH" << 'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.boxcheckr.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>-c</string>
        <string>curl -fsSL "{{.ServerURL}}/machines/{{.MachineID}}/script?mode=onetime" | bash</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Weekday</key>
        <integer>1</integer>
        <key>Hour</key>
        <integer>9</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>/tmp/boxcheckr.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/boxcheckr.log</string>
</dict>
</plist>
PLIST

launchctl load "$PLIST_PATH" 2>/dev/null || true

# Create uninstall helper
cat > "$HOME/.boxcheckr/uninstall.sh" << 'UNINSTALL'
#!/bin/bash
launchctl unload "$HOME/Library/LaunchAgents/com.boxcheckr.agent.plist" 2>/dev/null
rm -f "$HOME/Library/LaunchAgents/com.boxcheckr.agent.plist"
rm -rf "$HOME/.boxcheckr"
echo "BoxCheckr agent uninstalled."
UNINSTALL
chmod +x "$HOME/.boxcheckr/uninstall.sh"

echo "Installed LaunchAgent (runs Mondays at 9am)"
echo "To uninstall: ~/.boxcheckr/uninstall.sh"
{{end}}
