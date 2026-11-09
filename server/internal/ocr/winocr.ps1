param([Parameter(Mandatory=$true)][string]$InB64, [Parameter(Mandatory=$true)][string]$OutPath)
# Windows 内置 OCR（Windows.Media.Ocr）桥接脚本。
# 通过 base64 传入图片路径、把结果 JSON 写到 OutPath，彻底规避中文编码问题。
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime | Out-Null
[void][Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage, ContentType=WindowsRuntime]
[void][Windows.Storage.Streams.DataWriter, Windows.Storage, ContentType=WindowsRuntime]
[void][Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics, ContentType=WindowsRuntime]
[void][Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics, ContentType=WindowsRuntime]
[void][Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType=WindowsRuntime]
[void][Windows.Globalization.Language, Windows.Foundation, ContentType=WindowsRuntime]

$asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' })[0]

function Await($op, $type) {
    $t = $asTask.MakeGenericMethod($type).Invoke($null, @($op))
    $t.Wait(-1) | Out-Null
    $t.Result
}

function Write-Json($obj) {
    $json = $obj | ConvertTo-Json -Depth 8 -Compress
    [IO.File]::WriteAllText($OutPath, $json, (New-Object Text.UTF8Encoding($false)))
}

try {
    $path = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($InB64))
    $bytes = [System.IO.File]::ReadAllBytes($path)

    $stream = New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
    $writer = New-Object Windows.Storage.Streams.DataWriter($stream)
    $writer.WriteBytes($bytes)
    $null = Await ($writer.StoreAsync()) ([uint32])
    $stream.Seek(0)

    $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $sb = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])

    $engine = $null
    foreach ($tag in @('zh-Hans-CN', 'zh-Hans', 'zh-CN')) {
        try {
            $lang = New-Object Windows.Globalization.Language $tag
            $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang)
        } catch { $engine = $null }
        if ($null -ne $engine) { break }
    }
    if ($null -eq $engine) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages() }
    if ($null -eq $engine) { Write-Json @{ error = 'no_ocr_engine' }; Write-Output 'DONE'; exit 0 }

    $res = Await ($engine.RecognizeAsync($sb)) ([Windows.Media.Ocr.OcrResult])

    $out = New-Object System.Collections.ArrayList
    foreach ($line in $res.Lines) {
        $words = New-Object System.Collections.ArrayList
        foreach ($w in $line.Words) {
            $r = $w.BoundingRect
            [void]$words.Add(@{ t = $w.Text; x = [math]::Round($r.X, 1); y = [math]::Round($r.Y, 1);
                                w = [math]::Round($r.Width, 1); h = [math]::Round($r.Height, 1) })
        }
        [void]$out.Add(@{ text = $line.Text; words = @($words) })
    }
    Write-Json @{
        width = $sb.PixelWidth; height = $sb.PixelHeight;
        angle = $(if ($null -ne $res.TextAngle) { $res.TextAngle } else { 0 });
        lang = $engine.RecognizerLanguage.LanguageTag;
        lines = @($out)
    }
    Write-Output 'DONE'
}
catch {
    Write-Json @{ error = $_.Exception.Message }
    Write-Output 'DONE'
    exit 0
}
