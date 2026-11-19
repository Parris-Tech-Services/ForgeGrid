Set shell = CreateObject("WScript.Shell")
shell.Run "powershell.exe -NoProfile -ExecutionPolicy Bypass -STA -WindowStyle Hidden -File ""C:\ProgramData\ForgeGrid\ForgeGridComputeMonitor.ps1""", 0, False
