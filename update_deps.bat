@echo off
setlocal

echo 正在更新go.mod文件...
go mod edit -replace github.com/mattn/go-sqlite3=modernc.org/sqlite@latest

echo 下载新的依赖...
go mod tidy

echo 依赖更新完成！
endlocal