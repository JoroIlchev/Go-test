@echo off
rem Builds the signed release APK and puts it next to the hub as marquee-player.apk.
rem Finds a JDK 21 (or 17) by itself, so JAVA_HOME does not have to be set by hand.
rem Extra arguments go to Gradle, e.g. build-release.cmd --offline
setlocal
cd /d "%~dp0"
chcp 65001 >nul

set "JDK="
for /d %%D in ("C:\Program Files\Eclipse Adoptium\jdk-21*" "C:\Program Files\Zulu\zulu-21*" "C:\Program Files\Microsoft\jdk-21*" "C:\Program Files\Java\jdk-21*") do if exist "%%~D\bin\java.exe" set "JDK=%%~D"
if not defined JDK if exist "C:\Program Files\Android\Android Studio\jbr\bin\java.exe" set "JDK=C:\Program Files\Android\Android Studio\jbr"
if not defined JDK for /d %%D in ("C:\Program Files\Eclipse Adoptium\jdk-17*" "C:\Program Files\Zulu\zulu-17*" "C:\Program Files\Microsoft\jdk-17*" "C:\Program Files\Java\jdk-17*") do if exist "%%~D\bin\java.exe" set "JDK=%%~D"
if defined JDK set "JAVA_HOME=%JDK%"
if not defined JAVA_HOME (
  echo Няма намерен JDK 21 или 17. Инсталирай Eclipse Temurin 21 и пусни пак.
  exit /b 1
)
set "PATH=%JAVA_HOME%\bin;%PATH%"
echo JDK: %JAVA_HOME%

if not exist marquee-release.jks goto nokey
if not exist keystore.properties goto nokey

call "%~dp0gradlew.bat" assembleRelease %*
if errorlevel 1 exit /b 1

copy /y app\build\outputs\apk\release\app-release.apk ..\marquee-player.apk >nul || exit /b 1
echo.
echo Готово: %~dp0..\marquee-player.apk
exit /b 0

:nokey
echo Липсва ключът за подписване: android\marquee-release.jks и android\keystore.properties.
echo Копирай ги от резервното копие (не са в git).
exit /b 1
