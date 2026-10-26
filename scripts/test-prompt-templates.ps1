# Prompt-only regression gate. Missing original Harnesses are failures, never skips.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$ClaudeBin,
    [Parameter(Mandatory=$true)][string]$PiCli,
    [Parameter(Mandatory=$true)][string]$CodexBin,
    [Parameter(Mandatory=$true)][string]$DSHHome,
    [string]$NodeBin = 'node',
    [string]$LogDir = (Join-Path ([IO.Path]::GetTempPath()) ('magpie-prompt-gate-' + [guid]::NewGuid().ToString('N')))
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
foreach ($file in @($ClaudeBin, $PiCli, $CodexBin)) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Required original entry missing: $file" }
}
if (-not (Test-Path -LiteralPath (Join-Path $DSHHome 'profiles/web/package.json') -PathType Leaf)) { throw 'Required original DSH profile missing' }
$NodeBin = (Get-Command $NodeBin -ErrorAction Stop).Source
& $NodeBin -e "require.resolve('playwright', { paths: [process.argv[1]] })" (Join-Path $repo 'internal/gui/tests') 2>&1 | Out-Host
if ($LASTEXITCODE -ne 0) { throw 'Playwright is required; configure NODE_PATH for an existing installation before running the gate' }
New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
$LogDir = (Resolve-Path -LiteralPath $LogDir).Path
$buildDir = Join-Path $LogDir ('build-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $buildDir | Out-Null
$MagpieBin = Join-Path $buildDir 'magpie.exe'
$savedEnv = @{}
foreach ($key in @('BROWSER','MAGPIE_VERIFY_DSH_HOME','MAGPIE_VERIFY_NODE_BIN','MAGPIE_VERIFY_DSH_CONFIGURED_OPTIONAL','MAGPIE_VERIFY_CODEX_BIN')) {
    $savedEnv[$key] = [Environment]::GetEnvironmentVariable($key)
    [Environment]::SetEnvironmentVariable($key, $null)
}
$results = [Collections.Generic.List[object]]::new()
function Invoke-Stage([string]$Name, [string]$Command, [string[]]$Arguments) {
    $log = Join-Path $LogDir ($Name + '.log')
    $watch = [Diagnostics.Stopwatch]::StartNew()
    & $Command @Arguments 2>&1 | Tee-Object -FilePath $log | Out-Host
    $code = $LASTEXITCODE
    $watch.Stop()
    $results.Add([pscustomobject]@{stage=$Name; exit=$code; seconds=$watch.Elapsed.TotalSeconds; command=$Command; arguments=$Arguments; log=$log})
    $results | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $LogDir 'stages.json') -Encoding utf8
    if ($code -ne 0) { throw "Prompt gate failed: $Name (exit $code); evidence: $log" }
    return $log
}
Push-Location $repo
try {
    $sourceFiles = & git ls-files --cached --others --exclude-standard
    if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate the current source snapshot' }
    $sourceFiles = $sourceFiles | Where-Object { ($_ -match '\.go$' -and $_ -notmatch '_test\.go$') -or ($_ -match '^internal/gui/assets/' -and $_ -match '\.(js|css|html)$') -or $_ -eq 'internal/library/prompt_dsh_probe.mjs' } | Sort-Object -Unique
    $sourceBefore = @($sourceFiles | ForEach-Object { [pscustomobject]@{path=$_; sha256=(Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash} })
    $sourceBefore | ConvertTo-Json -Depth 3 | Set-Content -LiteralPath (Join-Path $LogDir 'source-snapshot.json') -Encoding utf8
    # Build only into a fresh gate-owned directory, never overwrite an input binary.
    $null = Invoke-Stage 'build-current-source' 'go' @('build','-tags','nogui','-o',$MagpieBin,'.')
    $publicLog = Invoke-Stage 'public-api' 'go' @('test','-tags','nogui','./internal/library','./internal/gui','-run','^TestPromptTemplate','-count=1','-timeout=10m','-json')
    $publicEvents = Get-Content -LiteralPath $publicLog | ForEach-Object { if ($_ -match '^\{') { $_ | ConvertFrom-Json } }
    foreach ($sentinel in @('TestPromptTemplateDeploysAndDoesNotBecomeInstructions','TestPromptTemplateNormalSyncAndMissingBody','TestPromptTemplatePiRuntimeChecksCommandsAndReusesResults','TestPromptTemplateCodexNativeIncompleteSourcesAndInvalidation','TestPromptTemplateDSHNativeNameAndExplicitOnlyPolicy','TestPromptTemplateAPISavesAndReads','TestPromptTemplateDeployedFactSurvivesExpiredNativeEvidence','TestPromptTemplateSaveDoesNotRescanAllSourcesForEveryTemplate')) {
        if (-not ($publicEvents | Where-Object { $_.Test -eq $sentinel -and $_.Action -eq 'pass' })) { throw ('Required public regression did not execute and pass: ' + $sentinel) }
    }
    $null = Invoke-Stage 'browser-two-engines-five-languages' $NodeBin @('--test','internal/gui/tests/library-prompts.test.cjs','internal/gui/tests/library-prompt-remove.test.cjs','internal/gui/tests/library-prompt-enabled.test.cjs','internal/gui/tests/library-prompt-harness.test.cjs')
    $null = Invoke-Stage 'prompt-i18n' $NodeBin @('--test','--test-name-pattern','native prompt capability diagnostics','internal/gui/tests/i18n-keys.test.cjs')
    $null = Invoke-Stage 'original-claude' $NodeBin @('internal/library/testdata/verify-claude-prompts.cjs',$ClaudeBin,$MagpieBin)
    $null = Invoke-Stage 'original-pi' $NodeBin @('internal/library/testdata/verify-pi-prompts.cjs',$PiCli,$MagpieBin)
    $null = Invoke-Stage 'original-codex' $NodeBin @('internal/library/testdata/verify-codex-prompts.cjs',$CodexBin,$MagpieBin)
    $env:MAGPIE_VERIFY_DSH_HOME = $DSHHome
    $env:MAGPIE_VERIFY_NODE_BIN = $NodeBin
    $env:MAGPIE_VERIFY_DSH_CONFIGURED_OPTIONAL = '1'
    $nativeTest = 'TestPromptTemplateOriginalDSHConfiguredNativeInvocation'
    $nativeLog = Invoke-Stage 'original-dsh' 'go' @('test','-tags','nogui','./internal/library','-run',('^'+$nativeTest+'$'),'-count=1','-timeout=10m','-json')
    $events = Get-Content -LiteralPath $nativeLog | ForEach-Object { if ($_ -match '^\{') { $_ | ConvertFrom-Json } }
    if (-not ($events | Where-Object { $_.Test -eq $nativeTest -and $_.Action -eq 'pass' }) -or ($events | Where-Object { $_.Test -eq $nativeTest -and $_.Action -eq 'skip' })) {
        throw 'Original DSH native invocation must execute and pass; opt-in skip is not acceptance'
    }
    $sourceFilesAfter = & git ls-files --cached --others --exclude-standard
    if ($LASTEXITCODE -ne 0) { throw 'Cannot verify the final source snapshot' }
    $sourceFilesAfter = $sourceFilesAfter | Where-Object { ($_ -match '\.go$' -and $_ -notmatch '_test\.go$') -or ($_ -match '^internal/gui/assets/' -and $_ -match '\.(js|css|html)$') -or $_ -eq 'internal/library/prompt_dsh_probe.mjs' } | Sort-Object -Unique
    if (($sourceFilesAfter -join "`n") -ne ($sourceFiles -join "`n")) { throw 'Source files changed during the gate; rebuild and repeat' }
    foreach ($source in $sourceBefore) {
        if ((Get-FileHash -LiteralPath $source.path -Algorithm SHA256).Hash -ne $source.sha256) { throw ('Source changed during the gate; rebuild and repeat: ' + $source.path) }
    }
    [pscustomobject]@{result='pass'; originalHarnesses=4; binary=$MagpieBin; binarySHA256=(Get-FileHash -LiteralPath $MagpieBin -Algorithm SHA256).Hash; evidence=$LogDir} | ConvertTo-Json -Compress | Write-Output
} finally {
    Pop-Location
    foreach ($key in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($key, $savedEnv[$key]) }
}
