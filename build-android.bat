REM filepath: build-android.bat
@echo off
setlocal enabledelayedexpansion

REM ���������в���
set "SKIP_AAR=0"
set "SKIP_APK=0"
set "ONLY_AAR=0"
set "ONLY_APK=0"
set "AUTO_INSTALL=0"

:parse_args
if "%~1"=="" goto :end_parse_args
if /i "%~1"=="-SkipAAR" set "SKIP_AAR=1"
if /i "%~1"=="-SkipAPK" set "SKIP_APK=1"
if /i "%~1"=="-OnlyAAR" set "ONLY_AAR=1"
if /i "%~1"=="-OnlyAPK" set "ONLY_APK=1"
if /i "%~1"=="-Install" set "AUTO_INSTALL=1"
if /i "%~1"=="-Help" goto :show_help
if /i "%~1"=="-h" goto :show_help
shift
goto :parse_args

:show_help
echo �÷�: build-android.bat [ѡ��]
echo.
echo ѡ��:
echo   -OnlyAAR      ֻ���� AAR ��,������ APK
echo   -OnlyAPK      ֻ���� APK,���� AAR ����
echo   -SkipAAR      ���� AAR ����,ֱ�ӱ��� APK
echo   -SkipAPK      ֻ���� AAR,������ APK
echo   -Install      ������ɺ��Զ���װ���豸
echo   -Help         ��ʾ�˰�����Ϣ
echo.
echo ʾ��:
echo   build-android.bat                    # ������������
echo   build-android.bat -OnlyAAR           # ֻ���� AAR
echo   build-android.bat -OnlyAPK           # ֻ���� APK
echo   build-android.bat -SkipAAR           # ���� AAR,ֻ���� APK
echo   build-android.bat -Install           # �����������Զ���װ
echo.
exit /b 0

:end_parse_args

echo ========================================
echo   ���� Cloud Clipboard Android Ӧ��
echo ========================================
echo.

REM ������֤
if "%ONLY_AAR%"=="1" if "%ONLY_APK%"=="1" (
    echo [����] -OnlyAAR �� -OnlyAPK ����ͬʱʹ��
    pause
    exit /b 1
)

if "%SKIP_AAR%"=="1" if "%ONLY_AAR%"=="1" (
    echo [����] -SkipAAR �� -OnlyAAR ����ͬʱʹ��
    pause
    exit /b 1
)

REM ȷ��ִ����Щ����
set "BUILD_AAR=1"
set "BUILD_APK=1"

if "%SKIP_AAR%"=="1" set "BUILD_AAR=0"
if "%ONLY_APK%"=="1" set "BUILD_AAR=0"
if "%SKIP_APK%"=="1" set "BUILD_APK=0"
if "%ONLY_AAR%"=="1" set "BUILD_APK=0"

REM ����Ҫ����
echo [���] ��֤��Ҫ����...

java -version >nul 2>&1
if errorlevel 1 (
    echo [����] δ��⵽ Java,���Ȱ�װ JDK 17
    echo ���ص�ַ: https://adoptium.net/
    pause
    exit /b 1
)
echo [?] Java �Ѱ�װ

if "%BUILD_AAR%"=="1" (
    go version >nul 2>&1
    if errorlevel 1 (
        echo [����] δ��⵽ Go,���Ȱ�װ Go 1.22+
        echo ���ص�ַ: https://go.dev/dl/
        pause
        exit /b 1
    )
    echo [?] Go �Ѱ�װ

    gomobile version >nul 2>&1
    if errorlevel 1 (
        echo [����] gomobile δ��װ,���ڰ�װ...
        go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20260211191516-dcd2a3258864
        if errorlevel 1 (
            echo [����] gomobile ��װʧ��
            pause
            exit /b 1
        )
    )
    echo [?] gomobile �Ѱ�װ
)

set "ROOT_DIR=%CD%"

REM ==================== ���� 1: ���� AAR ====================
if "%BUILD_AAR%"=="1" (
    echo.
    echo ========================================
    echo   ���� 1: ���� Go AAR ��
    echo ========================================

    cd cloud-clip\mobile
    if errorlevel 1 (
        echo [����] �Ҳ��� cloud-clip\mobile Ŀ¼
        pause
        exit /b 1
    )

    echo [��Ϣ] �����ɵ� AAR �ļ�...
    if exist cloudclipservice.aar del /f /q cloudclipservice.aar
    if exist cloudclipservice-sources.jar del /f /q cloudclipservice-sources.jar

    echo [��Ϣ] ��ʼ���� AAR...
    echo [����] gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w"

    set "JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8"
    echo [��Ϣ] ������ Java ����Ϊ UTF-8

    gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android -ldflags="-s -w" github.com/jonnyan404/cloud-clipboard-go/cloud-clip/mobile

    if errorlevel 1 (
        echo [����] AAR ����ʧ��!
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo [?] AAR ����ɹ�: cloudclipservice.aar

    echo [��Ϣ] ���� AAR �� Android ��Ŀ...
    if not exist "%ROOT_DIR%\android\app\libs" mkdir "%ROOT_DIR%\android\app\libs"
    copy /y cloudclipservice.aar "%ROOT_DIR%\android\app\libs\"
    if errorlevel 1 (
        echo [����] AAR ����ʧ��
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )
    echo [?] AAR �Ѹ��Ƶ� android\app\libs\

    cd "%ROOT_DIR%"
) else (
    echo.
    echo [����] ���� AAR ����
    
    if not exist "%ROOT_DIR%\android\app\libs\cloudclipservice.aar" (
        echo [����] �Ҳ��� AAR �ļ�: android\app\libs\cloudclipservice.aar
        echo ������������������ʹ�� -OnlyAAR �������� AAR
        pause
        exit /b 1
    )
    echo [?] �ҵ����� AAR �ļ�
)

REM ==================== ���� 2: ���� APK ====================
if "%BUILD_APK%"=="1" (
    echo.
    echo ========================================
    echo   ���� 2: ���� Android APK
    echo ========================================

    cd android
    if errorlevel 1 (
        echo [����] �Ҳ��� android Ŀ¼
        pause
        exit /b 1
    )

    echo [��Ϣ] ������Ŀ...
    call gradlew.bat clean
    if errorlevel 1 (
        echo [����] Gradle clean ʧ��
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo [��Ϣ] ���� Debug APK...
    call gradlew.bat assembleDebug
    if errorlevel 1 (
        echo [����] APK ����ʧ��!
        cd "%ROOT_DIR%"
        pause
        exit /b 1
    )

    echo.
    echo ========================================
    echo   �����ɹ�!
    echo ========================================
    echo.
    echo [?] APK λ��: android\app\build\outputs\apk\debug\app-debug.apk
    echo.

    if "%AUTO_INSTALL%"=="1" (
        echo [��Ϣ] ���ڰ�װ APK...
        call gradlew.bat installDebug
        if errorlevel 1 (
            echo [����] APK ��װʧ��,���ֶ���װ
        ) else (
            echo [?] APK �ѳɹ���װ���豸
        )
    ) else (
        set /p INSTALL_CHOICE="�Ƿ�װ���豸? (y/n): "
        if /i "!INSTALL_CHOICE!"=="y" (
            echo [��Ϣ] ���ڰ�װ APK...
            call gradlew.bat installDebug
            if errorlevel 1 (
                echo [����] APK ��װʧ��,���ֶ���װ
            ) else (
                echo [?] APK �ѳɹ���װ���豸
            )
        )
    )

    cd "%ROOT_DIR%"
) else (
    echo.
    echo [����] ���� APK ����
)

echo.
echo [���] �������̽���
pause