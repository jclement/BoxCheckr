#!/bin/bash
# =============================================================================
# BoxCheckr Agent Script (Linux)
# Generated for: {{.Email}}
# =============================================================================
#
# This script collects ONLY the following information:
#   - Your computer's hostname
#   - Operating system and version
#   - Whether disk encryption is enabled (LUKS)
#   - Whether antivirus protection is active (ClamAV or other)
#   - Whether the firewall is enabled (ufw/firewalld/iptables)
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

# Check if running as root (needed for accurate firewall detection)
if [[ $EUID -ne 0 ]]; then
    echo "Warning: Not running as root. Some checks (firewall status) may be inaccurate."
    echo "For best results, run with: sudo bash"
    echo ""
fi

# Get system info
OS="linux"
if [[ -f /etc/os-release ]]; then
    OS_VERSION=$(grep VERSION_ID /etc/os-release | cut -d'"' -f2)
    OS_NAME=$(grep ^NAME /etc/os-release | cut -d'"' -f2)
else
    OS_VERSION="unknown"
    OS_NAME="Linux"
fi
HOSTNAME=$(hostname)

# Check LUKS disk encryption
DISK_ENCRYPTED=false
DISK_DETAILS=""
if command -v lsblk &>/dev/null; then
    if lsblk -o TYPE 2>/dev/null | grep -q "crypt"; then
        DISK_ENCRYPTED=true
        DISK_DETAILS="LUKS encryption detected"
    else
        DISK_DETAILS="No LUKS encryption detected"
    fi
fi
# Also check for dm-crypt
if [[ -d /sys/class/block ]] && ls /sys/class/block/dm-* &>/dev/null; then
    for dm in /sys/class/block/dm-*; do
        if [[ -f "$dm/dm/uuid" ]] && grep -q "CRYPT" "$dm/dm/uuid" 2>/dev/null; then
            DISK_ENCRYPTED=true
            DISK_DETAILS="dm-crypt encryption detected"
            break
        fi
    done
fi

# Check antivirus
AV_ENABLED=false
AV_DETAILS=""
# Check ClamAV
if systemctl is-active --quiet clamav-daemon 2>/dev/null; then
    AV_ENABLED=true
    AV_DETAILS="ClamAV active"
elif pgrep -x "clamd" &>/dev/null; then
    AV_ENABLED=true
    AV_DETAILS="ClamAV active"
fi
# Check CrowdStrike Falcon
if pgrep -x "falcon-sensor" &>/dev/null; then
    AV_ENABLED=true
    AV_DETAILS="${AV_DETAILS:+$AV_DETAILS, }CrowdStrike Falcon"
fi
# Check SentinelOne
if pgrep -x "sentinelone" &>/dev/null; then
    AV_ENABLED=true
    AV_DETAILS="${AV_DETAILS:+$AV_DETAILS, }SentinelOne"
fi

# Check firewall status
FW_ENABLED=false
FW_DETAILS=""
# Check ufw (Ubuntu/Debian)
if command -v ufw &>/dev/null; then
    UFW_STATUS=$(ufw status 2>/dev/null | head -1)
    if echo "$UFW_STATUS" | grep -q "active"; then
        FW_ENABLED=true
        FW_DETAILS="ufw active"
    else
        FW_DETAILS="ufw inactive"
    fi
# Check firewalld (RHEL/Fedora/CentOS)
elif command -v firewall-cmd &>/dev/null; then
    if systemctl is-active --quiet firewalld 2>/dev/null; then
        FW_ENABLED=true
        FW_DETAILS="firewalld active"
    else
        FW_DETAILS="firewalld inactive"
    fi
# Check iptables as fallback
elif command -v iptables &>/dev/null; then
    RULES=$(iptables -L 2>/dev/null | wc -l)
    if [[ "$RULES" -gt 8 ]]; then
        FW_ENABLED=true
        FW_DETAILS="iptables rules configured"
    else
        FW_DETAILS="iptables (minimal/no rules)"
    fi
else
    FW_DETAILS="No firewall detected"
fi

# Check screen lock settings
SL_ENABLED=false
SL_TIMEOUT=0
SL_DETAILS=""
# Check GNOME settings
if command -v gsettings &>/dev/null; then
    LOCK_ENABLED=$(gsettings get org.gnome.desktop.screensaver lock-enabled 2>/dev/null || echo "false")
    if [[ "$LOCK_ENABLED" == "true" ]]; then
        SL_ENABLED=true
        IDLE_DELAY=$(gsettings get org.gnome.desktop.session idle-delay 2>/dev/null | grep -oE '[0-9]+' || echo "0")
        if [[ "$IDLE_DELAY" -gt 0 ]]; then
            SL_TIMEOUT=$((IDLE_DELAY / 60))
            SL_DETAILS="GNOME screen lock after ${SL_TIMEOUT} minutes"
        else
            SL_DETAILS="GNOME screen lock enabled (no idle timeout)"
        fi
    else
        SL_DETAILS="GNOME screen lock disabled"
    fi
# Check KDE settings
elif [[ -f "$HOME/.config/kscreenlockerrc" ]]; then
    if grep -q "Autolock=true" "$HOME/.config/kscreenlockerrc" 2>/dev/null; then
        SL_ENABLED=true
        TIMEOUT=$(grep "Timeout=" "$HOME/.config/kscreenlockerrc" 2>/dev/null | cut -d= -f2 || echo "0")
        if [[ "$TIMEOUT" -gt 0 ]]; then
            SL_TIMEOUT=$TIMEOUT
            SL_DETAILS="KDE screen lock after ${SL_TIMEOUT} minutes"
        else
            SL_DETAILS="KDE screen lock enabled"
        fi
    else
        SL_DETAILS="KDE screen lock disabled"
    fi
else
    SL_DETAILS="Screen lock settings unknown"
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
echo "OS: $OS_NAME $OS_VERSION"
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

# Add cron job
CRON_CMD="curl -fsSL '{{.ServerURL}}/machines/{{.MachineID}}/script?mode=onetime' | bash"
CRON_ENTRY="0 9 * * 1 $CRON_CMD # boxcheckr"
(crontab -l 2>/dev/null | grep -v boxcheckr; echo "$CRON_ENTRY") | crontab -

# Create uninstall helper
cat > "$HOME/.boxcheckr/uninstall.sh" << 'UNINSTALL'
#!/bin/bash
crontab -l 2>/dev/null | grep -v boxcheckr | crontab -
rm -rf "$HOME/.boxcheckr"
echo "BoxCheckr agent uninstalled."
UNINSTALL
chmod +x "$HOME/.boxcheckr/uninstall.sh"

echo "Installed cron job (runs Mondays at 9am)"
echo "To uninstall: ~/.boxcheckr/uninstall.sh"
{{end}}
