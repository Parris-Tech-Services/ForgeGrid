# Enrol this machine as an AgentBridge agent (Windows).
#
# Prerequisite: someone with access to the relay host has already run
#   agent-bridge register -name <this-machine>
# and given you the one-time token out of band.
#
# The token is read as a SecureString, converted only to be piped to the client,
# and cleared immediately. It is not written to disk except in the client config
# file that the client itself creates.

param(
    [string]$ForgeGrid = ".\ForgeGrid.exe"
)

$ErrorActionPreference = "Stop"

$relayUrl = Read-Host "Relay URL [https://relay.lan:9090]"
if ([string]::IsNullOrWhiteSpace($relayUrl)) { $relayUrl = "https://relay.lan:9090" }

$relayFp = Read-Host "Relay TLS fingerprint"
$agentName = Read-Host "Agent name for this machine"

$secure = Read-Host "Agent token" -AsSecureString
$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
}

if ([string]::IsNullOrWhiteSpace($relayFp) -or
    [string]::IsNullOrWhiteSpace($agentName) -or
    [string]::IsNullOrWhiteSpace($plain)) {
    throw "fingerprint, agent name and token are all required"
}

try {
    $plain | & $ForgeGrid agent-bridge configure-client `
        -name $agentName -url $relayUrl -fingerprint $relayFp -token-stdin
} finally {
    $plain = $null
}

Write-Host ""
Write-Host "Enrolled as $agentName." -ForegroundColor Green
Write-Host "Verify with:"
Write-Host "  $ForgeGrid agent-bridge channel read -channel household -limit 5"
