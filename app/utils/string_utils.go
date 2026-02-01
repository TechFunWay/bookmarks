package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// HashString 计算字符串的SHA256哈希值
func HashString(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// ValidateEmail 验证邮箱格式
func ValidateEmail(email string) bool {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, err := regexp.MatchString(pattern, email)
	if err != nil {
		return false
	}
	return matched
}

// FormatDate 格式化日期
func FormatDate(t time.Time, layout string) string {
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return t.Format(layout)
}

// TruncateString 截断字符串
func TruncateString(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}

// ContainsString 检查字符串切片是否包含指定字符串
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveDuplicatesFromSlice 从字符串切片中移除重复元素
func RemoveDuplicatesFromSlice(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// JoinStrings 用指定分隔符合并字符串切片
func JoinStrings(slice []string, separator string) string {
	return strings.Join(slice, separator)
}

// ReverseString 反转字符串
func ReverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// IsEmpty 检查字符串是否为空（空字符串或只包含空白字符）
func IsEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}

// PadLeft 左填充字符串
func PadLeft(s, pad string, length int) string {
	for len(s) < length {
		s = pad + s
	}
	return s
}

// PadRight 右填充字符串
func PadRight(s, pad string, length int) string {
	for len(s) < length {
		s += pad
	}
	return s
}

// SnakeToCamel 将蛇形命名转换为驼峰命名
func SnakeToCamel(snake string) string {
	words := strings.Split(snake, "_")
	for i, word := range words {
		if i == 0 {
			words[i] = strings.ToLower(word)
		} else {
			words[i] = strings.Title(strings.ToLower(word))
		}
	}
	return strings.Join(words, "")
}

// CamelToSnake 将驼峰命名转换为蛇形命名
func CamelToSnake(camel string) string {
	var result []rune
	for i, r := range camel {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// FormatFileSize 格式化文件大小
func FormatFileSize(sizeInBytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case sizeInBytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(sizeInBytes)/float64(TB))
	case sizeInBytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(sizeInBytes)/float64(GB))
	case sizeInBytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(sizeInBytes)/float64(MB))
	case sizeInBytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(sizeInBytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", sizeInBytes)
	}
}

// MinInt 返回两个整数中的较小值
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt 返回两个整数中的较大值
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ClampInt 将整数值限制在指定范围内
func ClampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
