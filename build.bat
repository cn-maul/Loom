@echo off
echo Building frontend...
cd web
call npm run build
cd ..

echo Copying frontend files...
if not exist "internal\frontend\dist" mkdir "internal\frontend\dist"
xcopy /E /I /Y "web\dist\*" "internal\frontend\dist\"

echo Building Go binary...
go build -o relationship.exe ./cmd/server

echo Done!
echo Run: relationship.exe