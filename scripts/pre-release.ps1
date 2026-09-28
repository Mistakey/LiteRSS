# Shared verification; configure Python with $env:PYTHON if needed.
$ErrorActionPreference = "Stop"
$pythonCommand = if ($env:PYTHON) { $env:PYTHON } else { "python" }
& $pythonCommand "$PSScriptRoot/verify.py" release
exit $LASTEXITCODE
