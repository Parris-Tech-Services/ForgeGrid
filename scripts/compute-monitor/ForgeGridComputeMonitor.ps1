
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$form = New-Object System.Windows.Forms.Form
$form.Text = 'ForgeGrid Compute Monitor'
$form.Size = New-Object System.Drawing.Size(420,230)
$form.StartPosition = 'CenterScreen'
$form.TopMost = $true
$form.BackColor = [System.Drawing.Color]::FromArgb(28,31,36)
$form.ForeColor = [System.Drawing.Color]::White
$title = New-Object System.Windows.Forms.Label
$title.Text = 'ForgeGrid Compute Monitor'
$title.Font = New-Object System.Drawing.Font('Segoe UI',14,[System.Drawing.FontStyle]::Bold)
$title.Location = New-Object System.Drawing.Point(14,12)
$title.Size = New-Object System.Drawing.Size(380,28)
$form.Controls.Add($title)
$label = New-Object System.Windows.Forms.Label
$label.Font = New-Object System.Drawing.Font('Consolas',10)
$label.Location = New-Object System.Drawing.Point(16,52)
$label.Size = New-Object System.Drawing.Size(380,122)
$label.ForeColor = [System.Drawing.Color]::FromArgb(220,235,255)
$form.Controls.Add($label)
$lastCpu = @{}
$lastTime = Get-Date
function Update-ForgeGridStats {
  $now = Get-Date
  $elapsed = [Math]::Max(0.1, ($now - $script:lastTime).TotalSeconds)
  $procs = @(Get-Process ForgeGrid -ErrorAction SilentlyContinue)
  $cpuPct = 0.0
  $ramMb = 0.0
  foreach ($p in $procs) {
    $ramMb += $p.WorkingSet64 / 1MB
    $prev = $script:lastCpu[$p.Id]
    if ($null -ne $prev) { $cpuPct += (($p.CPU - $prev) / $elapsed) * 100.0 }
    $script:lastCpu[$p.Id] = $p.CPU
  }
  $ids = @($procs | ForEach-Object { $_.Id })
  foreach ($k in @($script:lastCpu.Keys)) { if ($ids -notcontains $k) { $script:lastCpu.Remove($k) } }
  $script:lastTime = $now
  $cores = [Environment]::ProcessorCount
  $hostName = $env:COMPUTERNAME
  if ($procs.Count -eq 0) {
    $label.Text = "Host: $hostName`r`nForgeGrid processes: 0`r`nCPU: 0.0%`r`nRAM: 0.0 MB`r`nStatus: worker not currently running"
  } else {
    $label.Text = ("Host: {0}`r`nForgeGrid processes: {1}`r`nCPU: {2:N1}% of one core ({3:N1}% total)`r`nRAM: {4:N1} MB`r`nPIDs: {5}" -f $hostName,$procs.Count,$cpuPct,($cpuPct/[Math]::Max(1,$cores)),$ramMb,($ids -join ', '))
  }
}
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 1000
$timer.Add_Tick({ Update-ForgeGridStats })
$form.Add_Shown({ Update-ForgeGridStats; $timer.Start() })
[void]$form.ShowDialog()
