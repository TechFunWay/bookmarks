package logic

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// UpgradeRecord 升级记录模型
type UpgradeRecord struct {
	ID          int64     `json:"id"`
	Version     string    `json:"version"`
	Content     string    `json:"content"`
	UpgradeTime time.Time `json:"upgrade_time"`
	Result      string    `json:"result"`
	Status      int       `json:"status"` // 0: pending, 1: success, -1: failed
}

// Upgrade 系统升级管理器
type Upgrade struct {
	db             *sql.DB
	currentVersion string
	logger         *log.Logger
	upgradeLogger  *log.Logger // 升级专用日志记录器
	sqlDir         string
}

// NewUpgrade 创建升级管理器实例
func NewUpgrade(db *sql.DB, currentVersion string) *Upgrade {
	upgrade := &Upgrade{
		db:             db,
		currentVersion: currentVersion,
		logger:         log.New(os.Stdout, "[UPGRADE] ", log.LstdFlags),
		sqlDir:         "sql",
	}

	// 创建升级日志记录器
	upgradeLogger, err := createUpgradeLogger()
	if err != nil {
		log.Printf("[WARNING] 创建升级日志记录器失败: %v", err)
		upgrade.upgradeLogger = upgrade.logger // 回退到标准输出
	} else {
		upgrade.upgradeLogger = upgradeLogger
	}

	// 确保SQL目录存在
	if _, err := os.Stat(upgrade.sqlDir); os.IsNotExist(err) {
		os.MkdirAll(upgrade.sqlDir, 0755)
	}

	return upgrade
}

// createUpgradeLogger 创建升级专用日志记录器
func createUpgradeLogger() (*log.Logger, error) {
	// 创建日志目录
	logDir := "./logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	// 创建升级日志文件，按日期命名
	logFileName := filepath.Join(logDir, fmt.Sprintf("upgrade_%s.log", time.Now().Format("20060102")))
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open upgrade log file: %v", err)
	}

	// 创建日志记录器
	logger := log.New(logFile, "[UPGRADE] ", log.LstdFlags)

	return logger, nil
}

// LogUpgrade 记录升级日志
func (u *Upgrade) LogUpgrade(format string, v ...interface{}) {
	// 同时记录到标准输出和升级日志文件
	message := fmt.Sprintf(format, v...)
	u.logger.Print(message)
	if u.upgradeLogger != nil {
		u.upgradeLogger.Print(message)
	}
}

// CreateUpgradeTable 创建升级记录表
func (u *Upgrade) CreateUpgradeTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS sys_update (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version TEXT NOT NULL,
		content TEXT NOT NULL,
		upgrade_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		result TEXT NOT NULL,
		status INTEGER NOT NULL DEFAULT 0  -- 0: pending, 1: success, -1: failed
	);
	`

	_, err := u.db.Exec(query)
	if err != nil {
		return fmt.Errorf("创建升级记录表失败: %w", err)
	}

	u.LogUpgrade("升级记录表创建成功")
	return nil
}

// GetLastSuccessfulUpgrade 获取最后一次成功的升级记录
func (u *Upgrade) GetLastSuccessfulUpgrade() (*UpgradeRecord, error) {
	query := `
	SELECT id, version, content, upgrade_time, result, status
	FROM sys_update
	WHERE status = 1
	ORDER BY id DESC
	LIMIT 1
	`

	row := u.db.QueryRow(query)
	record := &UpgradeRecord{}
	var upgradeTime string

	err := row.Scan(&record.ID, &record.Version, &record.Content, &upgradeTime, &record.Result, &record.Status)
	if err != nil {
		if err == sql.ErrNoRows {
			// 没有找到记录，返回空记录
			return &UpgradeRecord{}, nil
		}
		return nil, fmt.Errorf("查询升级记录失败: %w", err)
	}

	// 解析时间
	parsedTime, err := time.Parse("2006-01-02 15:04:05", upgradeTime)
	if err != nil {
		// 尝试另一种格式
		parsedTime, err = time.Parse("2006-01-02T15:04:05Z", upgradeTime)
		if err != nil {
			return nil, fmt.Errorf("解析升级时间失败: %w", err)
		}
	}
	record.UpgradeTime = parsedTime

	return record, nil
}

// ParseVersion 解析版本号
func ParseVersion(version string) (major, minor, patch int, err error) {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	// 移除开头的 "v"
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("版本号格式错误: %s", version)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("主版本号不是数字: %s", parts[0])
	}

	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("次版本号不是数字: %s", parts[1])
	}

	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("修订版本号不是数字: %s", parts[2])
	}

	return major, minor, patch, nil
}

// CompareVersions 比较两个版本号
// 返回值: -1 如果 v1 < v2, 0 如果 v1 == v2, 1 如果 v1 > v2
func CompareVersions(v1, v2 string) (int, error) {
	major1, minor1, patch1, err := ParseVersion(v1)
	if err != nil {
		return 0, fmt.Errorf("解析版本号 %s 失败: %w", v1, err)
	}

	major2, minor2, patch2, err := ParseVersion(v2)
	if err != nil {
		return 0, fmt.Errorf("解析版本号 %s 失败: %w", v2, err)
	}

	if major1 != major2 {
		if major1 < major2 {
			return -1, nil
		}
		return 1, nil
	}

	if minor1 != minor2 {
		if minor1 < minor2 {
			return -1, nil
		}
		return 1, nil
	}

	if patch1 != patch2 {
		if patch1 < patch2 {
			return -1, nil
		}
		return 1, nil
	}

	return 0, nil
}

// GetAvailableVersions 获取可用的升级版本列表
func (u *Upgrade) GetAvailableVersions(fromVersion string) ([]string, error) {
	// 读取SQL目录下的所有SQL文件
	files, err := os.ReadDir(u.sqlDir)
	if err != nil {
		return nil, fmt.Errorf("读取SQL目录失败: %w", err)
	}

	versions := []string{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		if strings.HasSuffix(filename, ".sql") {
			// 提取版本号，例如从 "v1.7.0.sql" 提取 "v1.7.0"
			version := strings.TrimSuffix(filename, ".sql")

			// 验证版本号格式
			_, _, _, err := ParseVersion(version)
			if err != nil {
				u.LogUpgrade("跳过无效版本文件: %s", filename)
				continue
			}

			// 检查版本是否大于fromVersion
			cmp, err := CompareVersions(version, fromVersion)
			if err != nil {
				u.LogUpgrade("比较版本失败 %s vs %s: %v", version, fromVersion, err)
				continue
			}

			if cmp > 0 { // 当前版本大于fromVersion
				versions = append(versions, version)
			}
		}
	}

	// 按版本号排序
	sort.Slice(versions, func(i, j int) bool {
		cmp, err := CompareVersions(versions[i], versions[j])
		if err != nil {
			return false
		}
		return cmp < 0
	})

	return versions, nil
}

// ExecuteUpgrade 执行单个版本的升级
func (u *Upgrade) ExecuteUpgrade(version string) error {
	u.LogUpgrade("开始执行版本 %s 的升级", version)

	// 记录升级开始
	err := u.recordUpgradeStart(version, fmt.Sprintf("开始升级到版本 %s", version))
	if err != nil {
		return fmt.Errorf("记录升级开始失败: %w", err)
	}

	// 尝试执行SQL升级脚本
	sqlFilePath := filepath.Join(u.sqlDir, version+".sql")

	// 检查SQL文件是否存在
	if _, err := os.Stat(sqlFilePath); err == nil {
		u.LogUpgrade("执行SQL升级脚本: %s", sqlFilePath)
		err = u.executeSQLFile(sqlFilePath)
		if err != nil {
			u.LogUpgrade("SQL升级脚本执行失败: %v", err)
			u.recordUpgradeFailure(version, fmt.Sprintf("SQL脚本执行失败: %v", err))
			return fmt.Errorf("执行SQL升级脚本失败: %w", err)
		}
	} else {
		u.LogUpgrade("SQL文件不存在，跳过: %s", sqlFilePath)
	}

	// 执行特定版本的数据处理业务逻辑
	err = u.executeDataProcessingLogic(version)
	if err != nil {
		u.LogUpgrade("数据处理业务逻辑执行失败: %v", err)
		u.recordUpgradeFailure(version, fmt.Sprintf("数据处理业务逻辑执行失败: %v", err))
		return fmt.Errorf("执行数据处理业务逻辑失败: %w", err)
	}

	// 记录升级成功
	err = u.recordUpgradeSuccess(version, fmt.Sprintf("版本 %s 升级成功", version))
	if err != nil {
		return fmt.Errorf("记录升级成功失败: %w", err)
	}

	u.LogUpgrade("版本 %s 升级完成", version)
	return nil
}

// executeSQLFile 执行SQL文件
func (u *Upgrade) executeSQLFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取SQL文件失败: %w", err)
	}

	sqlContent := string(content)

	// 分割SQL语句并执行
	statements := u.splitSQLStatements(sqlContent)

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		u.LogUpgrade("执行SQL语句 #%d/%d: %s", i+1, len(statements), truncateString(stmt, 100))

		_, err := u.db.Exec(stmt)
		if err != nil {
			// 检查是否是因为重复创建引起的错误，如果是则忽略
			errMsg := err.Error()
			if u.shouldIgnoreError(stmt, errMsg) {
				u.LogUpgrade("SQL语句因已存在而被忽略（正常情况）: %s", truncateString(stmt, 100))
				continue
			}
			return fmt.Errorf("执行SQL语句失败: %w, 语句: %s", err, stmt)
		}
	}

	return nil
}

// shouldIgnoreError 判断错误是否应该被忽略（即对象已存在）
func (u *Upgrade) shouldIgnoreError(stmt, errMsg string) bool {
	stmtUpper := strings.ToUpper(strings.TrimSpace(stmt))

	// 检查是否是创建表的语句
	if strings.Contains(stmtUpper, "CREATE TABLE") && strings.Contains(stmtUpper, "IF NOT EXISTS") {
		// 如果使用了 IF NOT EXISTS，则不应该忽略任何错误
		return false
	}

	// 检查是否是创建索引的语句
	if strings.Contains(stmtUpper, "CREATE INDEX") && strings.Contains(stmtUpper, "IF NOT EXISTS") {
		// 如果使用了 IF NOT EXISTS，则不应该忽略任何错误
		return false
	}

	// 检查错误消息是否包含"已存在"类型的错误
	if strings.Contains(errMsg, "duplicate column name") ||
		strings.Contains(errMsg, "column already exists") ||
		strings.Contains(errMsg, "already exists") ||
		strings.Contains(errMsg, "table.*already exists") ||
		strings.Contains(errMsg, "index.*already exists") {
		return true
	}

	// 对于 ALTER TABLE ADD COLUMN 语句，如果有重复列错误则忽略
	if strings.Contains(stmtUpper, "ALTER TABLE") && strings.Contains(stmtUpper, "ADD") {
		if strings.Contains(errMsg, "duplicate column name") {
			return true
		}
	}

	return false
}

// splitSQLStatements 分割SQL语句
func (u *Upgrade) splitSQLStatements(sqlContent string) []string {
	statements := []string{}

	// 确保内容以换行符结尾，便于处理
	content := strings.ReplaceAll(sqlContent, "\r\n", "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// 将内容按行分割
	lines := strings.Split(content, "\n")

	var currentStmt strings.Builder
	inBlockComment := false
	inStringLiteral := false
	stringDelimiter := byte(0)
	inBeginEndBlock := false

	for _, line := range lines {
		// 检查是否在块注释中
		if !inStringLiteral && strings.Contains(line, "/*") {
			// 检查是否在同一行开始和结束
			startIdx := strings.Index(line, "/*")
			endIdx := strings.Index(line[startIdx+2:], "*/")

			if endIdx != -1 {
				// 注释在同一行开始和结束
				endIdx += startIdx + 2
				// 移除注释部分
				line = line[:startIdx] + line[endIdx+2:]
			} else {
				// 注释开始，但不结束
				inBlockComment = true
				continue
			}
		}

		if inBlockComment {
			endIdx := strings.Index(line, "*/")
			if endIdx != -1 {
				// 注释结束
				inBlockComment = false
				line = line[endIdx+2:]
			} else {
				// 整行都是注释
				continue
			}
		}

		if inBlockComment {
			continue
		}

		// 处理行内的内容
		i := 0
		for i < len(line) {
			char := line[i]

			// 检查是否是字符串开始/结束
			if !inStringLiteral && (char == '\'' || char == '"') {
				inStringLiteral = true
				stringDelimiter = char
			} else if inStringLiteral && char == stringDelimiter {
				// 检查是否是转义的引号
				if i > 0 && line[i-1] != '\\' {
					inStringLiteral = false
					stringDelimiter = 0
				}
			} else if !inStringLiteral {
				// 检查 BEGIN 关键字（用于触发器）
				trimmedLine := strings.TrimSpace(line[:i+1])
				if strings.ToUpper(trimmedLine) == "BEGIN" {
					inBeginEndBlock = true
				}
				// 检查 END 关键字
				if strings.ToUpper(trimmedLine) == "END" {
					inBeginEndBlock = false
				}
				// 只有在非 BEGIN...END 块中，分号才表示语句结束
				if !inBeginEndBlock && char == ';' {
					// 找到语句结束符
					currentStmt.WriteString(line[:i+1]) // 包含分号

					statement := strings.TrimSpace(currentStmt.String())
					if statement != "" {
						// 移除行注释
						statement = u.removeCommentsFromStatement(statement)
						if strings.TrimSpace(statement) != "" {
							statements = append(statements, statement)
						}
					}

					// 重置并处理剩余部分
					currentStmt.Reset()
					line = strings.TrimSpace(line[i+1:])
					i = 0
					continue
				}
			}

			i++
		}

		// 如果行处理完后还有内容，添加到当前语句
		if len(line) > 0 {
			if currentStmt.Len() > 0 {
				currentStmt.WriteString("\n")
			}
			currentStmt.WriteString(line)
		}
	}

	// 处理最后一个语句（如果没有以分号结尾）
	if currentStmt.Len() > 0 {
		statement := strings.TrimSpace(currentStmt.String())
		if statement != "" {
			statement = u.removeCommentsFromStatement(statement)
			if strings.TrimSpace(statement) != "" {
				statements = append(statements, statement)
			}
		}
	}

	return statements
}

// removeCommentsFromStatement 移除语句中的注释
func (u *Upgrade) removeCommentsFromStatement(stmt string) string {
	lines := strings.Split(stmt, "\n")
	var resultLines []string

	for _, line := range lines {
		// 查找行注释 --
		commentIdx := strings.Index(line, "--")
		if commentIdx != -1 {
			line = line[:commentIdx]
		}

		line = strings.TrimSpace(line)
		if line != "" {
			resultLines = append(resultLines, line)
		}
	}

	return strings.Join(resultLines, "\n")
}

// executeDataProcessingLogic 执行特定版本的数据处理业务逻辑
func (u *Upgrade) executeDataProcessingLogic(version string) error {
	// 这里可以添加特定版本的数据处理业务逻辑
	switch version {
	case "v1.7.0":
		return u.processDataForV1_7_0()
	default:
		// 对于未特殊处理的版本，可以执行通用数据处理逻辑
		u.LogUpgrade("执行版本 %s 的通用数据处理逻辑", version)
		return nil
	}
}

// processDataForV1_7_0 版本1.7.0的数据处理业务逻辑
func (u *Upgrade) processDataForV1_7_0() error {
	u.LogUpgrade("执行版本 v1.7.0 的数据处理业务逻辑")

	// 示例：更新配置项
	_, err := u.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)", "feature_flag_v170", "enabled")
	if err != nil {
		return fmt.Errorf("更新配置项失败: %w", err)
	}

	return nil
}

// recordUpgradeStart 记录升级开始
func (u *Upgrade) recordUpgradeStart(version, content string) error {
	query := `
	INSERT INTO sys_update (version, content, result, status)
	VALUES (?, ?, ?, ?)
	`

	_, err := u.db.Exec(query, version, content, "升级开始", 0) // 0: pending
	if err != nil {
		return fmt.Errorf("记录升级开始失败: %w", err)
	}

	return nil
}

// recordUpgradeSuccess 记录升级成功
func (u *Upgrade) recordUpgradeSuccess(version, content string) error {
	query := `
	UPDATE sys_update
	SET content = ?, result = ?, status = ?
	WHERE version = ? AND status = ?
	`

	_, err := u.db.Exec(query, content, "升级成功", 1, version, 0) // 从 pending(0) 更新到 success(1)
	if err != nil {
		return fmt.Errorf("记录升级成功失败: %w", err)
	}

	return nil
}

// recordUpgradeFailure 记录升级失败
func (u *Upgrade) recordUpgradeFailure(version, content string) error {
	query := `
	UPDATE sys_update
	SET content = ?, result = ?, status = ?
	WHERE version = ? AND status = ?
	`

	_, err := u.db.Exec(query, content, "升级失败", -1, version, 0) // 从 pending(0) 更新到 failed(-1)
	if err != nil {
		return fmt.Errorf("记录升级失败失败: %w", err)
	}

	return nil
}

// PerformUpgrade 执行完整的升级流程
func (u *Upgrade) PerformUpgrade() error {
	u.LogUpgrade("开始系统升级流程")

	// 1. 确保升级表存在
	err := u.CreateUpgradeTable()
	if err != nil {
		return fmt.Errorf("创建升级表失败: %w", err)
	}

	// 2. 获取最后一次成功的升级记录
	lastUpgrade, err := u.GetLastSuccessfulUpgrade()
	if err != nil {
		return fmt.Errorf("获取最后一次升级记录失败: %w", err)
	}

	var fromVersion string
	if lastUpgrade.Version == "" {
		// 如果没有之前的升级记录，说明是第一次使用升级功能，从v1.7.0开始
		fromVersion = "v1.6.0"
		u.LogUpgrade("首次升级，从v1.7.0开始")
	} else {
		fromVersion = lastUpgrade.Version
		u.LogUpgrade("从版本 %s 继续升级", fromVersion)
	}

	u.LogUpgrade("当前版本: %s, 升级起始版本: %s", u.currentVersion, fromVersion)

	// 3. 获取需要执行的版本列表
	versions, err := u.GetAvailableVersions(fromVersion)
	if err != nil {
		return fmt.Errorf("获取可用版本失败: %w", err)
	}

	u.LogUpgrade("发现 %d 个待升级版本: %v", len(versions), versions)

	// 4. 按顺序执行每个版本的升级
	for _, version := range versions {
		cmpWithCurrent, err := CompareVersions(version, u.currentVersion)
		if err != nil {
			u.LogUpgrade("比较版本失败 %s vs %s: %v", version, u.currentVersion, err)
			continue
		}

		cmpWithFrom, err := CompareVersions(version, fromVersion)
		if err != nil {
			u.LogUpgrade("比较版本失败 %s vs %s: %v", version, fromVersion, err)
			continue
		}

		// 升级区间：大于数据库查到的版本，且小于等于当前程序版本
		if cmpWithFrom > 0 && cmpWithCurrent <= 0 { // version > fromVersion && version <= u.currentVersion
			u.LogUpgrade("版本 %s 在升级区间内（%s < %s <= %s），开始执行升级", version, fromVersion, version, u.currentVersion)
			err = u.ExecuteUpgrade(version)
			if err != nil {
				return fmt.Errorf("执行版本 %s 升级失败: %w", version, err)
			}
		} else {
			u.LogUpgrade("版本 %s 不在升级区间内，跳过（%s < %s <= %s 不成立）", version, fromVersion, version, u.currentVersion)
		}
	}

	u.LogUpgrade("系统升级流程完成")
	return nil
}

// GetSystemInfo 获取系统信息
func (u *Upgrade) GetSystemInfo() map[string]interface{} {
	info := make(map[string]interface{})

	info["os"] = runtime.GOOS
	info["arch"] = runtime.GOARCH
	info["numCPU"] = runtime.NumCPU()
	info["goVersion"] = runtime.Version()
	info["currentDir"], _ = os.Getwd()

	// 获取可执行文件路径
	ex, err := os.Executable()
	if err == nil {
		info["exePath"] = filepath.Dir(ex)
	}

	info["currentTime"] = time.Now().Format(time.RFC3339)
	info["currentVersion"] = u.currentVersion

	return info
}

// GetStatus 获取升级状态
func (u *Upgrade) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})

	// 获取系统信息
	sysInfo := u.GetSystemInfo()
	status["systemInfo"] = sysInfo

	// 获取最近的升级记录
	lastUpgrade, err := u.GetLastSuccessfulUpgrade()
	if err != nil {
		status["lastUpgradeError"] = err.Error()
	} else {
		status["lastSuccessfulUpgrade"] = lastUpgrade
	}

	// 升级状态
	status["upgradeCompleted"] = true
	status["lastCheckTime"] = sysInfo["currentTime"]

	return status
}

// truncateString 截断字符串以便日志显示
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
