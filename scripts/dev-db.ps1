param([ValidateSet('start','stop')][string]$Action='start')
$ErrorActionPreference='Stop'
$projectRoot=Split-Path $PSScriptRoot -Parent
$localRoot=Join-Path $projectRoot '.local'
$version='17.11'
$archive=Join-Path $localRoot "postgresql-$version.zip"
$nativeRoot=Join-Path $localRoot "postgresql-$version"
$bin=Join-Path $nativeRoot 'pgsql\bin'
$data=Join-Path $localRoot 'pgdata-17'
$log=Join-Path $localRoot 'postgres.log'
$pgCtl=Join-Path $bin 'pg_ctl.exe'
$port=if($env:DEV_PG_PORT){$env:DEV_PG_PORT}else{'5433'}
$dbUser=if($env:DEV_PG_USER){$env:DEV_PG_USER}else{'chathm'}
$dbPassword=if($env:DEV_PG_PASSWORD){$env:DEV_PG_PASSWORD}else{'chathm'}
$dbName=if($env:DEV_PG_DATABASE){$env:DEV_PG_DATABASE}else{'chathm_dev'}
if($port -notmatch '^\d+$' -or $dbUser -notmatch '^[a-z_][a-z0-9_]*$' -or $dbName -notmatch '^[a-z_][a-z0-9_]*_(dev|test)$') { throw 'Configuracao invalida para o PostgreSQL local.' }

if($Action -eq 'stop') {
 if((Test-Path -LiteralPath $pgCtl) -and (Test-Path -LiteralPath (Join-Path $data 'PG_VERSION'))) {
  & $pgCtl status -D $data *> $null
  if($LASTEXITCODE -eq 0) { & $pgCtl stop -D $data -m fast -w; if($LASTEXITCODE -ne 0){throw 'Falha ao parar PostgreSQL local.'} }
 }
 exit 0
}

New-Item -ItemType Directory -Path $localRoot -Force | Out-Null
if(!(Test-Path -LiteralPath $pgCtl)) {
 if(!(Test-Path -LiteralPath $archive)) {
  Write-Host "[dev] Baixando PostgreSQL $version do distribuidor oficial EDB..."
  $partialArchive="$archive.part"
  & curl.exe --fail --location --silent --show-error --retry 3 --retry-delay 2 --connect-timeout 15 --speed-time 30 --speed-limit 1024 --continue-at - "https://get.enterprisedb.com/postgresql/postgresql-$version-1-windows-x64-binaries.zip" --output $partialArchive
  if($LASTEXITCODE -ne 0){throw 'Falha no download do PostgreSQL.'}
  Move-Item -LiteralPath $partialArchive -Destination $archive
 }
 Write-Host '[dev] Preparando binarios locais do PostgreSQL...'
 Add-Type -AssemblyName System.IO.Compression.FileSystem
 $zip=[IO.Compression.ZipFile]::OpenRead($archive)
 try {
  foreach($entry in $zip.Entries) {
   if($entry.FullName -notmatch '^pgsql/(bin|lib|share)/'){continue}
   $target=[IO.Path]::GetFullPath((Join-Path $nativeRoot $entry.FullName))
   $allowedRoot=[IO.Path]::GetFullPath($nativeRoot)+[IO.Path]::DirectorySeparatorChar
   if(!$target.StartsWith($allowedRoot,[StringComparison]::OrdinalIgnoreCase)){throw 'Caminho invalido no arquivo PostgreSQL.'}
   if($entry.FullName.EndsWith('/')){New-Item -ItemType Directory -Path $target -Force | Out-Null;continue}
   New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force | Out-Null
   [IO.Compression.ZipFileExtensions]::ExtractToFile($entry,$target,$true)
  }
 } finally { $zip.Dispose() }
}

if(!(Test-Path -LiteralPath (Join-Path $data 'PG_VERSION'))) {
 $passwordFile=Join-Path $localRoot 'postgres-init-password'
 [IO.File]::WriteAllText($passwordFile,$dbPassword,[Text.UTF8Encoding]::new($false))
 try {
  & (Join-Path $bin 'initdb.exe') -D $data -U $dbUser --encoding=UTF8 --locale=C --auth=scram-sha-256 --pwfile=$passwordFile
  if($LASTEXITCODE -ne 0){throw 'Falha ao inicializar PostgreSQL local.'}
 } finally { Remove-Item -LiteralPath $passwordFile -ErrorAction SilentlyContinue }
}

& $pgCtl status -D $data *> $null
if($LASTEXITCODE -ne 0) {
 $serverOptions="-h 127.0.0.1 -p $port -c timezone=UTC"
 $startArgs="start -D `"$data`" -l `"$log`" -o `"$serverOptions`" -w -t 30"
 $started=Start-Process -FilePath $pgCtl -ArgumentList $startArgs -WindowStyle Hidden -PassThru
 # Start-Process -Wait also waits for the PostgreSQL server descendants.
 # Wait only for pg_ctl, which exits after the server reports ready.
 $started.WaitForExit()
 if($started.ExitCode -ne 0){throw "Falha ao iniciar banco local. Consulte $log"}
}
$env:PGPASSWORD=$dbPassword
try {
 $exists=& (Join-Path $bin 'psql.exe') -h 127.0.0.1 -p $port -U $dbUser -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$dbName'"
 if($LASTEXITCODE -ne 0){throw 'Falha ao validar conexao PostgreSQL local.'}
 if($exists -ne '1') {
  & (Join-Path $bin 'createdb.exe') -h 127.0.0.1 -p $port -U $dbUser $dbName
  if($LASTEXITCODE -ne 0){throw 'Falha ao criar banco de desenvolvimento.'}
 }
} finally { Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue }
Write-Host "[dev] PostgreSQL pronto em 127.0.0.1:$port/$dbName"
