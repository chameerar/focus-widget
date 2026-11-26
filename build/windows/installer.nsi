; Per-user installer for Focus Widget. No administrator rights needed.
; makensis -DVERSION=x.y.z -DSRC=<folder with focus-widget.exe> -DOUTFILE=<installer.exe> installer.nsi

Unicode true
!include "MUI2.nsh"

!define NAME "Focus Widget"
!define EXE "focus-widget.exe"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\FocusWidget"
; The WebView2 runtime registers under this client ID, per machine or per user.
!define WEBVIEW2_KEY "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Name "${NAME}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\${NAME}"
RequestExecutionLevel user
SetCompressor /SOLID lzma
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${NAME}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "${NAME} installer"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 Focus Widget contributors"

!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

; Closes a running copy so its files can be replaced or removed.
!macro CloseApp
  nsExec::Exec 'taskkill /IM "${EXE}" /T'
  Sleep 800
  nsExec::Exec 'taskkill /IM "${EXE}" /T /F'
!macroend

Section "Install"
  !insertmacro CloseApp
  SetOutPath "$INSTDIR"
  File "${SRC}\${EXE}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortcut "$SMPROGRAMS\${NAME}.lnk" "$INSTDIR\${EXE}"

  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1

  ; Windows 11 and current Windows 10 include WebView2. Point to it if it's missing.
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\${WEBVIEW2_KEY}" "pv"
  StrCmp $0 "" 0 webview2_found
  ReadRegStr $0 HKCU "Software\${WEBVIEW2_KEY}" "pv"
  StrCmp $0 "" 0 webview2_found
    MessageBox MB_OK|MB_ICONINFORMATION "${NAME} needs the Microsoft Edge WebView2 Runtime. \
      Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and then start ${NAME}."
  webview2_found:
SectionEnd

Section "Uninstall"
  !insertmacro CloseApp
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${NAME}.lnk"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"

  ; "Open at login", and the toast registration the app makes when it first notifies.
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "focus-widget"
  ReadRegStr $0 HKCU "Software\Classes\AppUserModelId\${NAME}" "CustomActivator"
  StrCmp $0 "" +2
    DeleteRegKey HKCU "Software\Classes\CLSID\$0"
  DeleteRegKey HKCU "Software\Classes\AppUserModelId\${NAME}"
  ; Settings in %AppData%\Focus Widget and WebView2 data in %LocalAppData%\Focus Widget are kept.
SectionEnd
