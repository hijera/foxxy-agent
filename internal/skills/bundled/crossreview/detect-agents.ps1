# detect-agents.ps1 - probe which console code-agent CLIs are installed.
# Windows twin of detect-agents.sh; same tab-separated output contract:
#   agent <TAB> binary_path <TAB> models_cmd <TAB> run_template
#
# run_template uses {model}, {brief} and {out} placeholders; the caller bakes
# {model} in at roster-write time and the coordinator substitutes {brief} and
# {out} per run. run_command uses PowerShell on Windows, where '<' input
# redirection is rejected, so templates feed the brief through
# 'Get-Content -Raw {brief} |' instead.

$ErrorActionPreference = 'SilentlyContinue'

# Invoke-ProbeOk <file> <args> <seconds> - run the probe bounded by a timeout,
# like 'timeout(1)' in the .sh twin; a hung CLI must not stall detection.
function Invoke-ProbeOk([string]$file, [string[]]$argz, [int]$seconds) {
    try {
        $psi = [System.Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $file
        # ArgumentList is .NET Core only; our probe flags carry no spaces, so a
        # plain joined Arguments string works on Windows PowerShell 5.1 too.
        $psi.Arguments = ($argz -join ' ')
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $proc = [System.Diagnostics.Process]::Start($psi)
        # Drain both streams asynchronously so a chatty probe cannot fill a
        # pipe buffer and deadlock WaitForExit.
        $null = $proc.StandardOutput.ReadToEndAsync()
        $null = $proc.StandardError.ReadToEndAsync()
        if (-not $proc.WaitForExit($seconds * 1000)) {
            try { $proc.Kill() } catch {}
            return $false
        }
        return ($proc.ExitCode -eq 0)
    } catch {
        return $false
    }
}

function Test-Binary([string]$bin) {
    $cmd = Get-Command $bin -ErrorAction SilentlyContinue
    if (-not $cmd) { return $null }
    $path = $cmd.Source
    # The name resolving is not enough: the binary must run --version or
    # --help successfully.
    if (Invoke-ProbeOk $path @('--version') 10) { return $path }
    if (Invoke-ProbeOk $path @('--help') 10) { return $path }
    return $null
}

function Test-ModelsCmd([string[]]$argv) {
    $file = $argv[0]
    $rest = @()
    if ($argv.Length -gt 1) { $rest = $argv[1..($argv.Length - 1)] }
    return (Invoke-ProbeOk $file $rest 20)
}

function Emit([string]$agent, [string]$path, [string]$models, [string]$template) {
    Write-Output ($agent + "`t" + $path + "`t" + $models + "`t" + $template)
}

$p = Test-Binary 'claude'
if ($p) {
    Emit 'claude' $p '' 'Get-Content -Raw {brief} | claude -p --model {model} --output-format text --permission-mode plan > {out}'
}

$p = Test-Binary 'codex'
if ($p) {
    $m = ''
    if (Test-ModelsCmd @('codex', 'debug', 'models')) { $m = 'codex debug models' }
    Emit 'codex' $p $m 'Get-Content -Raw {brief} | codex exec -m {model} --sandbox read-only - > {out}'
}

$p = Test-Binary 'foxxycode'
if ($p) {
    Emit 'foxxycode' $p '' 'foxxycode -p -i {brief} --model {model} --mode ask > {out}'
}

$p = Test-Binary 'opencode'
if ($p) {
    $m = ''
    if (Test-ModelsCmd @('opencode', 'models')) { $m = 'opencode models' }
    Emit 'opencode' $p $m 'opencode run -m {model} "$(cat {brief})" > {out}'
}

foreach ($b in @('cursor-agent', 'agent', 'cursor')) {
    $p = Test-Binary $b
    if ($p) {
        $m = ''
        if (Test-ModelsCmd @($b, '--list-models')) { $m = "$b --list-models" }
        Emit 'cursor' $p $m "$b -p --mode ask --model {model} --output-format text `"`$(cat {brief})`" > {out}"
        break
    }
}

$p = Test-Binary 'devin'
if ($p) {
    $m = ''
    if (Test-ModelsCmd @('devin', 'models', 'list')) { $m = 'devin models list' }
    Emit 'devin' $p $m 'devin -p --model {model} --prompt-file {brief} > {out}'
}

$p = Test-Binary 'koda'
if ($p) {
    Emit 'koda' $p '' 'koda "$(cat {brief})" > {out}'
}
