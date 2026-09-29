; LiteRSS NSIS Installer Script
; This script creates a Windows installer for LiteRSS
;
; IMPORTANT: This script creates a Windows installer for LiteRSS
;
; To build:
;   makensis build/windows/installer.nsi
;
; All paths in this script are relative to the script directory.

!define APP_NAME "LiteRSS"
!define APP_VERSION "0.1.1"
!define APP_VERSION_NUMERIC "0.1.1.0"  ; NSIS requires X.X.X.X format
!define APP_PUBLISHER "kelch"
!define APP_URL "https://github.com/Mistakey/LiteRSS"
!define APP_DESCRIPTION "A desktop FreshRSS reader for unread articles"
!define APP_EXE "LiteRSS.exe"

; Include Modern UI
!include "MUI2.nsh"
!include "LogicLib.nsh"

; General Settings
Name "${APP_NAME} ${APP_VERSION}"
; Output path relative to script directory
OutFile "..\bin\LiteRSS-${APP_VERSION}-windows-amd64-installer.exe"
; Per-user install: the in-app update runs this installer silently over the
; running directory without UAC (spec D20, ADR 0019). A directory the user
; cannot write is still allowed; the update then elevates once.
InstallDir "$LOCALAPPDATA\Programs\${APP_NAME}"
InstallDirRegKey HKCU "Software\${APP_NAME}" "Install_Dir"
RequestExecutionLevel user

; MUI Settings
!define MUI_ABORTWARNING
; Use custom icons from build/windows/
!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"

; Welcome page
!insertmacro MUI_PAGE_WELCOME

; License page
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"

; Directory page
!insertmacro MUI_PAGE_DIRECTORY

; Instfiles page
!insertmacro MUI_PAGE_INSTFILES

; Finish page
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${APP_NAME}"
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchApp
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; Language
!insertmacro MUI_LANGUAGE "English"

; Version Information
VIProductVersion "${APP_VERSION_NUMERIC}"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "FileDescription" "${APP_DESCRIPTION}"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "CompanyName" "${APP_PUBLISHER}"
VIAddVersionKey "LegalCopyright" "Copyright (C) ${APP_PUBLISHER}"

; LaunchApp starts the app through explorer.exe, so it runs with the user's
; normal rights even when the update elevated this installer.
Function LaunchApp
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${APP_EXE}"'
FunctionEnd

; WaitForApp gives a LiteRSS that started this installer for an update up to
; 30 seconds to quit and release its executable.
Function WaitForApp
    StrCpy $1 0
    wait:
        IfFileExists "$INSTDIR\${APP_EXE}" 0 done
        ClearErrors
        FileOpen $0 "$INSTDIR\${APP_EXE}" a
        IfErrors 0 released
        IntOp $1 $1 + 1
        IntCmp $1 60 done
        Sleep 500
        Goto wait
    released:
        FileClose $0
    done:
FunctionEnd

; Installer Sections
Section "MainSection" SEC01
    SetOutPath "$INSTDIR"

    Call WaitForApp

    ; Copy the executable
    File "..\bin\${APP_EXE}"

    ; Create shortcuts
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
    CreateShortcut "$SMPROGRAMS\${APP_NAME}\Uninstall.lnk" "$INSTDIR\Uninstall.exe"
    CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"

    ; Write registry keys
    WriteRegStr HKCU "Software\${APP_NAME}" "Install_Dir" "$INSTDIR"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayName" "${APP_NAME}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayVersion" "${APP_VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "Publisher" "${APP_PUBLISHER}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "URLInfoAbout" "${APP_URL}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "UninstallString" "$INSTDIR\Uninstall.exe"
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "NoRepair" 1

    ; Create uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"

    ; A silent install is the in-app update: start the new version.
    ${If} ${Silent}
        Call LaunchApp
    ${EndIf}
SectionEnd

; Uninstaller Section
Section "Uninstall"
    ; Remove files
    Delete "$INSTDIR\${APP_EXE}"
    Delete "$INSTDIR\Uninstall.exe"

    ; Remove shortcuts
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\Uninstall.lnk"
    Delete "$DESKTOP\${APP_NAME}.lnk"
    RMDir "$SMPROGRAMS\${APP_NAME}"

    ; Remove installation directory
    RMDir "$INSTDIR"

    ; Remove registry keys
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
    DeleteRegKey HKCU "Software\${APP_NAME}"

    ; Note: User data directory is NOT removed to preserve user data
    ; Data location: %APPDATA%\LiteRSS
SectionEnd
