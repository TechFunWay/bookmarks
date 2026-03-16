package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"bookmark/app/utils"

	_ "modernc.org/sqlite"
)

func main() {
	// 定义命令行参数
	dbPath := flag.String("db", "./data/db/database.db", "数据库文件路径")
	username := flag.String("username", "", "要重置密码的用户名")
	newPassword := flag.String("password", "", "新密码")
	flag.Parse()

	// 验证参数
	if *username == "" {
		fmt.Println("错误: 必须指定用户名")
		fmt.Println("使用方法: go run cmd/reset-password.go -username <用户名> -password <新密码>")
		fmt.Println("示例: go run cmd/reset-password.go -username admin -password 123456")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *newPassword == "" {
		fmt.Println("错误: 必须指定新密码")
		fmt.Println("使用方法: go run cmd/reset-password.go -username <用户名> -password <新密码>")
		fmt.Println("示例: go run cmd/reset-password.go -username admin -password 123456")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if len(*newPassword) < 6 {
		fmt.Println("错误: 密码长度至少6位")
		os.Exit(1)
	}

	// 连接数据库
	db, err := sql.Open("sqlite", *dbPath+"?_foreign_keys=on")
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	defer db.Close()

	// 检查用户是否存在
	var userID int64
	var existingUsername string
	err = db.QueryRow("SELECT id, username FROM users WHERE username = ?", strings.TrimSpace(*username)).Scan(&userID, &existingUsername)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Printf("错误: 用户 '%s' 不存在\n", *username)
			os.Exit(1)
		}
		log.Fatalf("查询用户失败: %v", err)
	}

	// 前端已经 MD5 过一次，后端再进行一次 MD5（双重 MD5）
	firstHash := utils.MD5Hash(strings.TrimSpace(*newPassword), "")
	doubleHashedPassword := utils.MD5Hash(firstHash, "bookmarks")

	// 更新密码
	result, err := db.Exec(
		"UPDATE users SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		doubleHashedPassword, userID,
	)
	if err != nil {
		log.Fatalf("更新密码失败: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("✓ 密码重置成功！\n")
		fmt.Printf("用户名: %s\n", existingUsername)
		fmt.Printf("用户ID: %d\n", userID)
		fmt.Printf("新密码: %s\n", *newPassword)
		fmt.Printf("\n现在可以使用新密码登录了。\n")
	} else {
		fmt.Println("错误: 密码重置失败")
		os.Exit(1)
	}
}
