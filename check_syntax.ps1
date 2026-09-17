$scriptContent = Get-Content ./scripts/install-windows-worker.ps1 -Raw
$errors = [System.Management.Automation.Language.Parser]::ParseInput($scriptContent, [ref]$null, [ref]$null)
if ($errors.Count -gt 0) {
    Write-Error "Syntax errors found:"
    $errors | ForEach-Object { 
        Write-Host "Error details:"
        $_.psobject.properties | ForEach-Object { Write-Host "$($_.Name): $($_.Value)" }
    }
    exit 1
} else {
    Write-Host "Syntax OK"
}
