; installer.nsi — Flore 自定义 NSIS 安装器
;
; 在 wails build -nsis 之后（已生成 build\windows\installer\wails_tools.nsh）运行：
;   makensis -DARG_WAILS_AMD64_BINARY="build\bin\Flore.exe" installer.nsi
;
; 与 Wails 生成的 project.nsi 的不同：
;   - 后端已编入主程序（Flore.exe --backend 自衍生），无需独立 florebackend 二进制
;   - 使用 User 级安装（无需管理员），装到 %LOCALAPPDATA%\Programs\Flore
;   - 快捷方式可选（安装时用户勾选）

Unicode true

; === 覆盖 Wails 默认值（必须在 include wails_tools.nsh 之前，其 !ifndef 会跳过已定义项）===
!define REQUEST_EXECUTION_LEVEL "user"   ; 无需管理员权限
!define MUI_ICON "build\favicon.ico"
!define MUI_UNICON "build\favicon.ico"

; 引入 Wails 自动生成的 NSIS 宏与项目信息
!include "build\windows\installer\wails_tools.nsh"

; === 页面定义 ===
!include "MUI.nsh"

!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "立即启动 Flore"
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "build\bin\Flore-installer.exe"

InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

; === 安装组件 ===

Section "Flore 应用程序" SEC_APP
    SectionIn RO
    !insertmacro wails.setShellContext
    SetOutPath $INSTDIR
    !insertmacro wails.files
    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
SectionEnd

Section "开始菜单快捷方式" SEC_STARTMENU
    CreateShortCut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
SectionEnd

Section "桌面快捷方式" SEC_DESKTOP
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
SectionEnd

; === 组件描述 ===
LangString DESC_SEC_APP ${LANG_ENGLISH} "Flore RSS 阅读器核心程序（必需）。"
LangString DESC_SEC_STARTMENU ${LANG_ENGLISH} "在开始菜单中添加 Flore 快捷方式。"
LangString DESC_SEC_DESKTOP ${LANG_ENGLISH} "在桌面创建 Flore 快捷方式。"

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_APP} $(DESC_SEC_APP)
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_STARTMENU} $(DESC_SEC_STARTMENU)
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP} $(DESC_SEC_DESKTOP)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; === 卸载 ===
Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller
SectionEnd