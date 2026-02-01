package logger

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// 日志轮转函数
func RotateLogFiles(logDir string) error {
	// 获取三天前的日期
	threeDaysAgo := time.Now().AddDate(0, 0, -3)

	// 遍历日志目录中的文件
	files, err := os.ReadDir(logDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		// 如果文件修改时间早于三天前，则删除
		if info.ModTime().Before(threeDaysAgo) {
			logFilePath := filepath.Join(logDir, file.Name())
			if err := os.Remove(logFilePath); err != nil {
				log.Printf("删除旧日志文件失败: %v", err)
			} else {
				log.Printf("已删除旧日志文件: %s", logFilePath)
			}
		}
	}

	return nil
}

// 自定义日志中间件
func LoggingMiddleware(logFile *os.File) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 包装ResponseWriter以捕获状态码和响应大小
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// 写入日志到文件
			logEntry := fmt.Sprintf("%s \"%s %s %s\" from %s - %d %dB in %s\n",
				start.Format("2006/01/02 15:04:05"),
				r.Method,
				r.URL.Path,
				r.Proto,
				r.RemoteAddr,
				wrapped.statusCode,
				wrapped.size,
				duration.String(),
			)

			// 同时写入日志文件
			logFile.WriteString(logEntry)
		})
	}
}

// responseWriter 包装 http.ResponseWriter 以捕获状态码和响应大小
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// CreateLogFile 创建并返回日志文件句柄
func CreateLogFile() (*os.File, error) {
	// 创建日志目录
	logDir := "./logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	// 清理旧日志文件
	if err := RotateLogFiles(logDir); err != nil {
		log.Printf("清理旧日志文件时出现错误: %v", err)
	}

	// 创建日志文件，按日期命名
	logFileName := filepath.Join(logDir, fmt.Sprintf("access_%s.log", time.Now().Format("20060102")))
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	return logFile, nil
}
