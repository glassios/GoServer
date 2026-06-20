$configFile = Join-Path $PSScriptRoot "../configs/server.yaml"
$dsn = "postgres://postgres:postgres@localhost:5432/galaxy?sslmode=disable"

if (Test-Path $configFile) {
    $content = Get-Content $configFile -Raw
    if ($content -match 'dsn:\s*"([^"]+)"') {
        $dsn = $Matches[1]
    }
}

Write-Host "Restoring database using DSN: $dsn"
$sqlFile = Join-Path $PSScriptRoot "restore_db.sql"

if (Get-Command psql -ErrorAction SilentlyContinue) {
    psql $dsn -f $sqlFile
} else {
    Write-Warning "psql executable was not found in PATH. Please run the restore_db.sql manually using your PostgreSQL client."
}
