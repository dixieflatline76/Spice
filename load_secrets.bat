@echo off
echo Loading secrets from .spice_secrets...
if exist ".spice_secrets" (
    for /f "usebackq tokens=1,* delims==" %%A in (".spice_secrets") do (
        if "%%A" neq "" (
             set "%%A=%%B"
             echo Set %%A
        )
    )
    echo Secrets loaded.
) else (
    echo Warning: .spice_secrets not found in current directory.
)
