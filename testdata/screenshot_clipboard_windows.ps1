param([string]$Base, [string]$Peer)

Add-Type -AssemblyName System.Drawing, System.Windows.Forms
function New-Screenshot($dir, $name, [int]$width, [int]$height, $color) {
    $folder = [IO.Directory]::CreateDirectory((Join-Path $dir 'screenshots'))
    $path = Join-Path $folder.FullName $name
    $image = [Drawing.Bitmap]::new($width, $height)
    try {
        $image.SetPixel(0, 0, $color)
        $image.SetPixel($width - 1, $height - 1, [Drawing.Color]::FromArgb(101, 27, 92, 180))
        $image.Save($path, [Drawing.Imaging.ImageFormat]::Png)
    } finally { $image.Dispose() }
    return (Get-Item -LiteralPath $path)
}

function Assert-ImageCopy($file, $dimensions, [switch]$InViewer) {
    if (!$InViewer) { Find-Control $file.Name -Press }
    Find-Control $dimensions
    Set-Clipboard -Value 'waiting for the selected image'
    Find-Control '复制图片' -Press
    $expected = [IO.File]::ReadAllBytes($file.FullName)
    $limit = [DateTime]::UtcNow.AddSeconds(8)
    do {
        $object = [Windows.Forms.Clipboard]::GetDataObject()
        if ($object -and $object.GetDataPresent('PNG', $false)) {
            $stream = $object.GetData('PNG', $false)
            if ($stream -is [IO.MemoryStream]) {
                $actual = $stream.ToArray()
                # A Windows global memory block may include alignment padding after the PNG.
                if ($actual.Length -ge $expected.Length -and
                    [Convert]::ToBase64String($actual, 0, $expected.Length) -ceq [Convert]::ToBase64String($expected) -and
                    [Windows.Forms.Clipboard]::ContainsImage()) {
                    Write-Host "Verified screenshot '$($file.Name)' ($dimensions) on the Windows clipboard."
                    return
                }
            }
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $limit)
    throw "Did not receive the selected original PNG ($dimensions); clipboard formats: $($object.GetFormats() -join ', ')"
}

$wide = New-Screenshot $Base 'shared.png' 1703 3 ([Drawing.Color]::FromArgb(177, 42, 153, 228))
$tall = New-Screenshot $Base 'portrait.png' 7 19 ([Drawing.Color]::FromArgb(83, 213, 31, 142))
$other = New-Screenshot $Peer 'shared.png' 11 5 ([Drawing.Color]::FromArgb(223, 93, 21, 70))
$wide.LastWriteTimeUtc = $tall.LastWriteTimeUtc.AddMinutes(-1)
$files = @($wide, $tall, $other)
$before = $files | ForEach-Object { (Get-FileHash -LiteralPath $_.FullName).Hash + $_.LastWriteTimeUtc.Ticks }
Find-Control 'Profile 1' -Press
Find-Control '截图' -Press
Assert-ImageCopy $wide '1703 × 3'
Find-Control '查看大图' -Press
Find-Control '上一张' -Press
Assert-ImageCopy $tall '7 × 19' -InViewer
Find-Control '返回图库' -Press
Find-Control '查看大图'
Find-Control '1703 × 3'
Find-Control 'Profile 2' -Press
Find-Control '截图' -Press
Find-Control $other.Name -Press
Find-Control '11 × 5'
Find-Control '查看大图' -Press
Assert-ImageCopy $other '11 × 5' -InViewer
Find-Control '返回图库' -Press
Find-Control '查看大图'
$after = $files | ForEach-Object { $_.Refresh(); (Get-FileHash -LiteralPath $_.FullName).Hash + $_.LastWriteTimeUtc.Ticks }
if (($before -join ',') -cne ($after -join ',')) { throw 'Source screenshots changed.' }

# Simulate removal after preview, then check feedback and the existing clipboard.
Move-Item -LiteralPath $other.FullName -Destination ($other.FullName + '.moved')
Set-Clipboard -Value 'keep on read failure'
Find-Control '复制图片' -Press
Find-Control '复制通知详情'
if ((Get-Clipboard -Raw) -cne 'keep on read failure') { throw 'A failed copy replaced the clipboard.' }
Write-Output 'Verified three original PNGs, native bitmap availability, read failure and source preservation.'
