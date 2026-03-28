package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const (
	minPasswordLength = 6
)

type App struct {
	dbPath   string
	username string
	password string
}

func main() {
	app := &App{}

	// 解析命令行参数
	flag.StringVar(&app.dbPath, "db", "./data/db/database.db", "数据库文件路径")
	flag.StringVar(&app.username, "username", "", "要重置密码的用户名（必需）")
	flag.StringVar(&app.password, "password", "", "新密码（必需，至少6位）")
	
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "使用方法:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  reset-password -db <数据库路径> -username <用户名> -password <新密码>\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "参数说明:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\n示例:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  重置 admin 用户的密码为 123456:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "    reset-password -username admin -password 123456\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  指定数据库路径:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "    reset-password -db /path/to/database.db -username admin -password 123456\n")
	}
	
	flag.Parse()

	// 验证必需参数
	if app.username == "" || app.password == "" {
		fmt.Println("错误: 必须提供用户名和密码")
		fmt.Println("\n使用 -h 查看帮助信息")
		os.Exit(1)
	}

	// 验证密码长度
	if len(app.password) < minPasswordLength {
		fmt.Printf("错误: 密码长度不能少于 %d 位\n", minPasswordLength)
		os.Exit(1)
	}

	// 执行密码重置
	if err := app.resetPassword(); err != nil {
		log.Fatalf("重置密码失败: %v", err)
	}

	fmt.Printf("✓ 用户 %s 的密码已成功重置\n", app.username)
	fmt.Println("请使用新密码登录系统")
}

func (a *App) resetPassword() error {
	// 打开数据库连接
	db, err := sql.Open("sqlite3", a.dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	// 验证数据库连接
	if err := db.Ping(); err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	// 检查用户是否存在
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", a.username).Scan(&count)
	if err != nil {
		return fmt.Errorf("查询用户失败: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("用户 %s 不存在", a.username)
	}

	// 更新用户密码（使用 MD5 加密）
	hashedPassword := md5Hash(a.password)
	result, err := db.Exec("UPDATE users SET password = ? WHERE username = ?", hashedPassword, a.username)
	if err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}

	// 验证是否成功更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取更新结果失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("未更新任何记录")
	}

	return nil
}

// MD5 哈希函数（与前端加密逻辑保持一致）
func md5Hash(text string) string {
	// 简单的 MD5 实现
	// 注意：在实际生产环境中，建议使用更安全的哈希算法如 bcrypt、scrypt 或 Argon2
	// 这里为了与现有系统保持兼容，使用 MD5
	
	// 导入 crypto/md5 包
	h := crypto.MD5.New()
	h.Write([]byte(text))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// 确保导入了 crypto 包
import (
	"crypto"
)

