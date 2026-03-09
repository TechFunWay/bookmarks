package utils

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"
)

// RandomInt 生成指定范围内的随机整数 [min, max]
func RandomInt(min, max int) int {
	if min >= max {
		return min
	}
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
}

// RandomString 生成指定长度的随机字符串
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// RetryWithBackoff 带退避策略的重试函数
func RetryWithBackoff(maxRetries int, initialDelay time.Duration, fn func() error) error {
	var err error
	delay := initialDelay

	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		time.Sleep(delay)
		delay *= 2 // 指数退避
	}

	return err
}

// ChunkSlice 将切片分割成指定大小的块
func ChunkSlice(slice []interface{}, chunkSize int) [][]interface{} {
	var chunks [][]interface{}
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// MapKeys 获取映射的所有键
func MapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// StringToInt 安全地将字符串转换为整数
func StringToInt(s string) (int, error) {
	var result int
	negative := false
	i := 0

	if len(s) > 0 && s[0] == '-' {
		negative = true
		i = 1
	}

	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid integer format: %s", s)
		}
		result = result*10 + int(s[i]-'0')
	}

	if negative {
		result = -result
	}

	return result, nil
}

// InRange 检查值是否在指定范围内
func InRange(value, min, max int) bool {
	return value >= min && value <= max
}

// AbsInt 返回整数的绝对值
func AbsInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// UniqueInts 返回整数切片中的唯一值
func UniqueInts(slice []int) []int {
	seen := make(map[int]bool)
	result := []int{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// SumInts 计算整数切片的总和
func SumInts(slice []int) int {
	sum := 0
	for _, v := range slice {
		sum += v
	}
	return sum
}

// AverageInts 计算整数切片的平均值
func AverageInts(slice []int) float64 {
	if len(slice) == 0 {
		return 0
	}
	return float64(SumInts(slice)) / float64(len(slice))
}

// FilterInts 过滤整数切片
func FilterInts(slice []int, predicate func(int) bool) []int {
	result := []int{}
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// MapInts 映射整数切片
func MapInts(slice []int, mapper func(int) int) []int {
	result := make([]int, len(slice))
	for i, item := range slice {
		result[i] = mapper(item)
	}
	return result
}

// MD5Hash 计算字符串的 MD5 哈希值（32位小写十六进制）
// salt 为盐值，为空字符串时不添加盐
func MD5Hash(text string, salt string) string {
	hasher := md5.New()
	if salt != "" {
		hasher.Write([]byte(text + salt))
	} else {
		hasher.Write([]byte(text))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
