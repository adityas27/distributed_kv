@echo off
REM Build and run benchmark tool

echo Building benchmark tool...
go build -o benchmark.exe benchmark.go
if %ERRORLEVEL% NEQ 0 (
    echo Build failed!
    exit /b 1
)

echo.
echo Starting benchmark...
echo.

REM Run with default settings
benchmark.exe %*

echo.
echo Benchmark complete!
