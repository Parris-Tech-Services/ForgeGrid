param(
    [string]$Root = $PSScriptRoot
)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$form = New-Object System.Windows.Forms.Form
$form.Text = "ForgeGrid Runner Monitor"
$form.Size = New-Object System.Drawing.Size(800, 600)
$form.StartPosition = "CenterScreen"
$form.BackColor = [System.Drawing.Color]::FromArgb(255, 30, 30, 30)
$form.ForeColor = [System.Drawing.Color]::White

$fontTitle = New-Object System.Drawing.Font("Segoe UI", 16, [System.Drawing.FontStyle]::Bold)
$fontLabel = New-Object System.Drawing.Font("Segoe UI", 10, [System.Drawing.FontStyle]::Regular)
$fontMono = New-Object System.Drawing.Font("Consolas", 10)

$lblTitle = New-Object System.Windows.Forms.Label
$lblTitle.Text = "ForgeGrid Runner Monitor"
$lblTitle.Font = $fontTitle
$lblTitle.Location = New-Object System.Drawing.Point(20, 15)
$lblTitle.Size = New-Object System.Drawing.Size(740, 30)
$form.Controls.Add($lblTitle)

$lblStatus = New-Object System.Windows.Forms.Label
$lblStatus.Location = New-Object System.Drawing.Point(20, 55)
$lblStatus.Size = New-Object System.Drawing.Size(740, 25)
$lblStatus.Font = $fontLabel
$form.Controls.Add($lblStatus)

$lblConfig = New-Object System.Windows.Forms.Label
$lblConfig.Location = New-Object System.Drawing.Point(20, 85)
$lblConfig.Size = New-Object System.Drawing.Size(740, 25)
$lblConfig.Font = $fontLabel
$form.Controls.Add($lblConfig)

$lblNetwork = New-Object System.Windows.Forms.Label
$lblNetwork.Location = New-Object System.Drawing.Point(20, 115)
$lblNetwork.Size = New-Object System.Drawing.Size(740, 25)
$lblNetwork.Font = $fontLabel
$form.Controls.Add($lblNetwork)

$lblLogHeader = New-Object System.Windows.Forms.Label
$lblLogHeader.Text = "Log Output:"
$lblLogHeader.Location = New-Object System.Drawing.Point(20, 155)
$lblLogHeader.Size = New-Object System.Drawing.Size(740, 20)
$lblLogHeader.Font = $fontLabel
$form.Controls.Add($lblLogHeader)

$logTextBox = New-Object System.Windows.Forms.TextBox
$logTextBox.Location = New-Object System.Drawing.Point(20, 180)
$logTextBox.Size = New-Object System.Drawing.Size(740, 310)
$logTextBox.Multiline = $true
$logTextBox.ScrollBars = "Vertical"
$logTextBox.ReadOnly = $true
$logTextBox.BackColor = [System.Drawing.Color]::FromArgb(255, 18, 18, 18)
$logTextBox.ForeColor = [System.Drawing.Color]::LightGray
$logTextBox.Font = $fontMono
$form.Controls.Add($logTextBox)

# Helper for button style
function Style-Button($btn, $color) {
    $btn.BackColor = $color
    $btn.ForeColor = [System.Drawing.Color]::White
    $btn.FlatStyle = "Flat"
    $btn.Font = New-Object System.Drawing.Font("Segoe UI", 9, [System.Drawing.FontStyle]::Bold)
}

$btnStart = New-Object System.Windows.Forms.Button
$btnStart.Location = New-Object System.Drawing.Point(20, 510)
$btnStart.Size = New-Object System.Drawing.Size(120, 35)
$btnStart.Text = "Start Runner"
Style-Button $btnStart [System.Drawing.Color]::FromArgb(255, 46, 204, 113)
$form.Controls.Add($btnStart)

$btnStop = New-Object System.Windows.Forms.Button
$btnStop.Location = New-Object System.Drawing.Point(150, 510)
$btnStop.Size = New-Object System.Drawing.Size(120, 35)
$btnStop.Text = "Stop Runner"
Style-Button $btnStop [System.Drawing.Color]::FromArgb(255, 231, 76, 60)
$form.Controls.Add($btnStop)

$btnLogs = New-Object System.Windows.Forms.Button
$btnLogs.Location = New-Object System.Drawing.Point(280, 510)
$btnLogs.Size = New-Object System.Drawing.Size(120, 35)
$btnLogs.Text = "Open Logs Folder"
Style-Button $btnLogs [System.Drawing.Color]::FromArgb(255, 52, 152, 219)
$form.Controls.Add($btnLogs)

$btnRefresh = New-Object System.Windows.Forms.Button
$btnRefresh.Location = New-Object System.Drawing.Point(640, 510)
$btnRefresh.Size = New-Object System.Drawing.Size(120, 35)
$btnRefresh.Text = "Refresh"
Style-Button $btnRefresh [System.Drawing.Color]::FromArgb(255, 149, 165, 166)
$form.Controls.Add($btnRefresh)

$updateAction = {
    # 1. Process Status
    $proc = Get-Process ForgeGrid -ErrorAction SilentlyContinue
    if ($proc) {
        $uptime = (Get-Date) - $proc.StartTime
        $uptimeStr = "{0:00}:{1:00}:{2:00}" -f $uptime.Hours, $uptime.Minutes, $uptime.Seconds
        if ($uptime.Days -gt 0) { $uptimeStr = "$($uptime.Days)d $uptimeStr" }
        $mem = [math]::Round($proc.WorkingSet / 1MB, 2)
        $lblStatus.Text = "Status: RUNNING | PID: $($proc.Id) | Memory: $mem MB | Uptime: $uptimeStr"
        $lblStatus.ForeColor = [System.Drawing.Color]::LightGreen
        $btnStart.Enabled = $false
        $btnStart.BackColor = [System.Drawing.Color]::Gray
        $btnStop.Enabled = $true
        Style-Button $btnStop [System.Drawing.Color]::FromArgb(255, 231, 76, 60)
    } else {
        $lblStatus.Text = "Status: STOPPED"
        $lblStatus.ForeColor = [System.Drawing.Color]::Tomato
        $btnStart.Enabled = $true
        Style-Button $btnStart [System.Drawing.Color]::FromArgb(255, 46, 204, 113)
        $btnStop.Enabled = $false
        $btnStop.BackColor = [System.Drawing.Color]::Gray
    }

    # 2. Config Details
    $credsPath = Join-Path $env:LOCALAPPDATA "ForgeGrid\worker_creds.json"
    $coordUrl = "Unknown"
    $workerId = "Unknown"
    if (Test-Path $credsPath) {
        try {
            $creds = Get-Content $credsPath -Raw | ConvertFrom-Json
            $coordUrl = $creds.coordinator_url
            $workerId = $creds.worker_id
        } catch {}
    }
    $lblConfig.Text = "Worker ID: $workerId | Coordinator: $coordUrl"

    # 3. Network Check (Fast TCP)
    $netStatus = "Checking..."
    $netColor = [System.Drawing.Color]::LightGray
    if ($coordUrl -match "https?://([^:/]+):(\d+)") {
        $hostName = $matches[1]
        $port = $matches[2]
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $async = $tcp.BeginConnect($hostName, $port, $null, $null)
            $success = $async.AsyncWaitHandle.WaitOne(500, $true)
            if ($success -and $tcp.Connected) {
                $netStatus = "Coordinator Reachable (TCP Port $port OK)"
                $netColor = [System.Drawing.Color]::LightGreen
            } else {
                $netStatus = "Coordinator Unreachable (Timeout/Refused)"
                $netColor = [System.Drawing.Color]::Tomato
            }
            $tcp.Close()
        } catch {
            $netStatus = "Coordinator Unreachable (Error)"
            $netColor = [System.Drawing.Color]::Tomato
        }
    } else {
        $netStatus = "Invalid Coordinator URL in config."
        $netColor = [System.Drawing.Color]::Yellow
    }
    $lblNetwork.Text = "Network: $netStatus"
    $lblNetwork.ForeColor = $netColor

    # 4. Logs
    $logDir = Join-Path $env:LOCALAPPDATA "ForgeGrid\logs"
    $latestLog = Get-ChildItem $logDir -Filter "*.log" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($latestLog) {
        $lblLogHeader.Text = "Log Output ($($latestLog.Name)):"
        try {
            $lines = Get-Content $latestLog.FullName -Tail 22 -ErrorAction SilentlyContinue
            if ($lines) {
                $logTextBox.Text = ($lines -join "`r`n")
                $logTextBox.SelectionStart = $logTextBox.Text.Length
                $logTextBox.ScrollToCaret()
            }
        } catch { }
    } else {
        $lblLogHeader.Text = "Log Output (No log files found):"
        $logTextBox.Text = ""
    }
}

$btnStart.Add_Click({
    $exe = Join-Path $Root "ForgeGrid.exe"
    if (-not (Test-Path $exe)) {
        $distExe = Join-Path $Root "dist\ForgeGrid-USB\Windows\ForgeGrid.exe"
        if (Test-Path $distExe) { $exe = $distExe }
    }
    if (Test-Path $exe) {
        $logDir = Join-Path $env:LOCALAPPDATA "ForgeGrid\logs"
        New-Item -ItemType Directory -Force -Path $logDir | Out-Null
        $log = Join-Path $logDir ("runner_{0:yyyyMMdd_HHmmss}.log" -f (Get-Date))
        Start-Process -FilePath $exe -ArgumentList "-mode worker" -WorkingDirectory (Split-Path -Parent $exe) -RedirectStandardOutput $log -RedirectStandardError ($log + ".err") -WindowStyle Minimized
        Start-Sleep -Milliseconds 500
        &$updateAction
    }
})

$btnStop.Add_Click({
    Get-Process ForgeGrid -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Milliseconds 500
    &$updateAction
})

$btnLogs.Add_Click({
    $logDir = Join-Path $env:LOCALAPPDATA "ForgeGrid\logs"
    Start-Process "explorer.exe" -ArgumentList $logDir
})

$btnRefresh.Add_Click($updateAction)

$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 2000
$timer.Add_Tick($updateAction)
$timer.Start()

&$updateAction
$form.ShowDialog() | Out-Null
$timer.Stop()
