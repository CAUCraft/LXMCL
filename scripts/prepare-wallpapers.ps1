param([string]$Source = 'E:\下载\壁纸')
$ErrorActionPreference = 'Stop'
$output = Join-Path $PSScriptRoot '../ui/assets/wallpapers'
New-Item -ItemType Directory -Force -Path $output | Out-Null
$clips = @(
    @('cabin', 'Minecraft Overgrown Cabin_2_prob4.mp4'),
    @('castle', 'Minecraft castle Fixed_2_prob4.mp4'),
    @('aquarium', 'Minecraft Soothing Scenes – Relaxing Aquarium.mp4'),
    @('falling-snow', 'Minecraft Soothing Scenes – Relaxing Falling Snow.mp4'),
    @('fireplace', 'Minecraft Soothing Scenes – Relaxing Fireplace.mp4'),
    @('swamp', 'Minecraft Soothing Scenes – Relaxing Rainy Swamp.mp4'),
    @('serene-snow', 'Soothing Minecraft – Serene Snow.mp4')
)
foreach ($clip in $clips) {
    $inputPath = Join-Path $Source $clip[1]
    $videoPath = Join-Path $output ($clip[0] + '.mp4')
    $posterPath = Join-Path $output ($clip[0] + '.jpg')
    if (-not (Test-Path -LiteralPath $videoPath)) {
        & ffmpeg -hide_banner -loglevel error -threads 2 -i $inputPath -map 0:v:0 -an -sn -dn -vf 'fps=24,scale=1280:720:flags=lanczos,setsar=1' -c:v libx264 -preset fast -crf 23 -threads 2 -filter_threads 1 -pix_fmt yuv420p -movflags +faststart -y $videoPath
        if ($LASTEXITCODE -ne 0) { throw "Failed to prepare $($clip[0])" }
    }
    & ffmpeg -hide_banner -loglevel error -ss 5 -i $videoPath -frames:v 1 -vf 'scale=480:270' -update 1 -y $posterPath
    if ($LASTEXITCODE -ne 0) { throw "Failed to prepare poster $($clip[0])" }
    Write-Output "Prepared $($clip[0])"
}
