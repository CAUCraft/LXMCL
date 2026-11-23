param([Parameter(Mandatory)][string]$Launcher)

# The smoke test owns the clipboard of a disposable hosted Windows runner.
$ErrorActionPreference = 'Stop'
if ($env:RUNNER_ENVIRONMENT -ne 'github-hosted') { throw 'Run only on an isolated GitHub-hosted runner.' }
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.IO.Compression.FileSystem

function Add-Mod($instance, $file, $id, $version) {
    $zip = [IO.Compression.ZipFile]::Open((Join-Path $instance.game_directory "mods/$file"), 'Create')
    try {
        $entry = [IO.StreamWriter]::new($zip.CreateEntry('fabric.mod.json').Open())
        try { $entry.Write((@{id=$id; version=$version} | ConvertTo-Json -Compress)) } finally { $entry.Dispose() }
    } finally { $zip.Dispose() }
}

function Press([string]$name) {
    $limit = [DateTime]::UtcNow.AddSeconds(20)
    $condition = [Windows.Automation.PropertyCondition]::new([Windows.Automation.AutomationElement]::NameProperty, $name)
    do {
        $process.Refresh()
        if ($process.HasExited) { throw "Launcher exited before $name" }
        if ($process.MainWindowHandle -ne [IntPtr]::Zero) {
            $window = [Windows.Automation.AutomationElement]::FromHandle($process.MainWindowHandle)
            foreach ($node in $window.FindAll([Windows.Automation.TreeScope]::Descendants, $condition)) {
                if (!$node.Current.IsEnabled) { continue }
                $pattern = $null
                if ($node.TryGetCurrentPattern([Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) { $pattern.Invoke(); return }
                if ($node.TryGetCurrentPattern([Windows.Automation.SelectionItemPattern]::Pattern, [ref]$pattern)) { $pattern.Select(); return }
            }
        }
        Start-Sleep -Milliseconds 120
    } while ([DateTime]::UtcNow -lt $limit)
    $labels = $window.FindAll([Windows.Automation.TreeScope]::Descendants, [Windows.Automation.Condition]::TrueCondition) | ForEach-Object { $_.Current.Name } | Where-Object { $_ }
    throw "Control unavailable: $name. Visible labels: $($labels -join ', ')"
}

function Assert-Copy($choice, [string]$expected) {
    Press '选择对照实例'
    Press $choice
    Set-Clipboard -Value 'waiting for the selected comparison'
    Press '复制差异清单'
    $limit = [DateTime]::UtcNow.AddSeconds(5)
    do {
        $actual = ([string](Get-Clipboard -Raw)).Replace("`r`n", "`n")
        if ($actual -ceq $expected) { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $limit)
    throw "Clipboard differs for $choice. Expected [$expected], got [$actual]"
}

$root = Join-Path $env:RUNNER_TEMP ("lxmcl-report-" + [guid]::NewGuid())
$instances = @(1..3 | ForEach-Object {
    $dir = Join-Path $root "instance-$_"
    $null = New-Item -ItemType Directory -Path (Join-Path $dir 'mods') -Force
    @{ id=$_; name="Profile $_"; version='1.21.1'; loader='Fabric'; loader_version='0.16.14'; revision=1; game_directory=$dir }
})
$instances[1].version = '1.21.4'
$state = @{ selected=3; next_id=4; instances=$instances; settings=@{game_root=$root; player='PrivatePlayer'; memory_mib=4096} }
$state | ConvertTo-Json -Depth 5 | Set-Content -Encoding UTF8 (Join-Path $root 'launcher.json')
Add-Mod $instances[0] 'base.jar' 'forest' '1.0'
Add-Mod $instances[1] 'new.jar.disabled' 'forest' '2.0'
Add-Mod $instances[1] 'one.jar' 'maps' '3'
Add-Mod $instances[1] 'two.jar' 'maps' '4'
Add-Mod $instances[2] 'renamed.jar' 'forest' '1.0'
Add-Mod $instances[0] 'lantern.jar' 'lantern' '3'
Add-Mod $instances[2] 'lantern.jar' 'lantern' '3'
Add-Mod $instances[1] 'camp.jar' 'camp' '4'
$process = Start-Process -PassThru -FilePath $Launcher -ArgumentList "--data-dir `"$root`""
try {
    foreach ($name in '实例', 'Profile 1', '资源', '模组', '比对实例模组') { Press $name }
    $base = '基准：Minecraft 1.21.1 · Fabric 0.16.14'
    $different = @($base, '对照：Minecraft 1.21.4 · Fabric 0.16.14', '新增 · camp：无 → 4 · 启用', '版本及启用状态 · forest：1.0 · 启用 → 2.0 · 停用', '缺少 · lantern：3 · 启用 → 无', '重复未配对 · maps') -join "`n"
    $equal = @($base, '对照：Minecraft 1.21.1 · Fabric 0.16.14', '未发现可确认的差异。') -join "`n"
    Assert-Copy 'Profile 2 · #2' $different
    Assert-Copy 'Profile 3 · #3' $equal
    Assert-Copy 'Profile 2 · #2' $different
    Write-Output 'Verified three native clipboard reads from the native launcher.'
} finally {
    if (!$process.HasExited) { $process.Kill(); $process.WaitForExit() }
    Remove-Item -LiteralPath $root -Recurse -Force
}
