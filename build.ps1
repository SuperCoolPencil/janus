# Build script for janus on Windows.
# Sets the CGo flags needed to find WinFsp's FUSE-compatible headers and import lib,
# then builds janus.exe.
#
# Why CGo flags are needed:
#   cgofuse uses CGo to call into WinFsp's native C layer. The C compiler (gcc,
#   provided by MSYS2/MinGW) needs to find:
#     - fuse_common.h  (in WinFsp's inc/fuse/ directory)
#     - winfsp-x64.lib (the import library that links against winfsp-x64.dll)
#
# WinFsp installs to "C:\Program Files (x86)\WinFsp" by default.
# We use the 8.3 short path (PROGRA~2) to avoid spaces in the path, which can
# confuse the gcc command-line argument parser on Windows.
#
# Usage:
#   .\build.ps1            - build janus.exe
#   .\build.ps1 -Run       - build and print usage

param(
    [switch]$Run
)

$env:CGO_CFLAGS  = "-IC:/PROGRA~2/WinFsp/inc/fuse"
$env:CGO_LDFLAGS = "-LC:/PROGRA~2/WinFsp/lib -lwinfsp-x64"

Write-Host "Building janus.exe..." -ForegroundColor Cyan
go build -o janus.exe .

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed." -ForegroundColor Red
    exit 1
}

Write-Host "Build succeeded: janus.exe" -ForegroundColor Green

if ($Run) {
    Write-Host ""
    .\janus.exe
}
