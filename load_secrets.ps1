# Usage: . .\load_secrets.ps1
# Note: You must dot-source this script for variables to persist!

$SecretFile = ".spice_secrets"
if (Test-Path $SecretFile) {
    Write-Host "Loading secrets from $SecretFile..."
    Get-Content $SecretFile | ForEach-Object {
        if ($_ -match "^([^#=]+)=(.*)$") {
            $Key = $matches[1].Trim()
            $Value = $matches[2].Trim()
            [Environment]::SetEnvironmentVariable($Key, $Value, "Process")
            Write-Host "Set $Key"
        }
    }
    Write-Host "Secrets loaded. You can now run 'make win-amd64'"
} else {
    Write-Warning "$SecretFile not found in current directory."
}
