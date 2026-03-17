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
	username := flag.String("username", "", "要重置密码的用户名（留空则查找管理员账号）")
	newPassword := flag.String("password", "", "新密码")
	flag.Parse()

	// 验证密码参数
	if *newPassword == "" {
		fmt.Println("错误: 必须指定新密码")
		fmt.Println("\n使用方法:")
		fmt.Println("  1. 重置指定用户密码:")
		fmt.Println("     ./reset-password -username <用户名> -password <新密码>")
		fmt.Println("     ./reset-password -username admin -password 123456")
		fmt.Println("\n  2. 重置管理员密码（自动查找管理员账号）:")
		fmt.Println("     ./reset-password -password 123456")
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

	var userID int64
	var existingUsername string
	var isAdmin int

	// 如果没有指定用户名，查找管理员账号
	if *username == "" {
		fmt.Println("未指定用户名，正在查找管理员账号...")

		// 查询管理员账号（is_admin = 1 的用户）
		err = db.QueryRow("SELECT id, username, is_admin FROM users WHERE is_admin = 1 LIMIT 1").Scan(&userID, &existingUsername, &isAdmin)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Println("错误: 未找到管理员账号")
				os.Exit(1)
			}
			log.Fatalf("查询管理员失败: %v", err)
		}

		fmt.Printf("✓ 找到管理员账号: %s\n", existingUsername)
	} else {
		// 检查用户是否存在
		err = db.QueryRow("SELECT id, username, is_admin FROM users WHERE username = ?", strings.TrimSpace(*username)).Scan(&userID, &existingUsername, &isAdmin)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Printf("错误: 用户 '%s' 不存在\n", *username)
				os.Exit(1)
			}
			log.Fatalf("查询用户失败: %v", err)
		}

		if isAdmin == 1 {
			fmt.Printf("✓ 找到管理员账号: %s\n", existingUsername)
		} else {
			fmt.Printf("✓ 找到用户账号: %s\n", existingUsername)
		}
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
		fmt.Println("\n=========================================")
		fmt.Println("  密码重置成功！")
		fmt.Println("=========================================")
		fmt.Printf("用户名: %s\n", existingUsername)
		fmt.Printf("用户ID: %d\n", userID)
		if isAdmin == 1 {
			fmt.Printf("账号类型: 管理员\n")
		} else {
			fmt.Printf("账号类型: 普通用户\n")
		}
		fmt.Printf("新密码: %s\n", *newPassword)
		fmt.Println("\n现在可以使用新密码登录了。")
		fmt.Println("=========================================")
	} else {
		fmt.Println("错误: 密码重置失败")
		os.Exit(1)
	}
}
