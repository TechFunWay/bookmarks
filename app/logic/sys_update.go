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
}

// NewUpgrade 创建升级管理器实例
func NewUpgrade(db *sql.DB, currentVersion string, logDir string) *Upgrade {
	upgrade := &Upgrade{
		db:             db,
		currentVersion: currentVersion,
		logger:         log.New(os.Stdout, "[UPGRADE] ", log.LstdFlags),
	}

	upgradeLogger, err := createUpgradeLogger(logDir)
	if err != nil {
		log.Printf("[WARNING] 创建升级日志记录器失败: %v", err)
		upgrade.upgradeLogger = upgrade.logger
	} else {
		upgrade.upgradeLogger = upgradeLogger
	}

	return upgrade
}

// createUpgradeLogger 创建升级专用日志记录器
func createUpgradeLogger(logDir string) (*log.Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	logFileName := filepath.Join(logDir, fmt.Sprintf("upgrade_%s.log", time.Now().Format("20060102")))
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open upgrade log file: %v", err)
	}

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
	// 硬编码所有可用版本
	allVersions := []string{"v1.7.0", "v1.8.0", "v1.9.0"}

	versions := []string{}
	for _, version := range allVersions {
		// 检查版本是否大于fromVersion
		cmp, err := CompareVersions(version, fromVersion)
		if err != nil {
			u.LogUpgrade("比较版本失败 %s vs %s: %v", version, fromVersion, err)
			continue
		}

		if cmp > 0 {
			versions = append(versions, version)
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

	// 执行指定版本的SQL语句
	u.LogUpgrade("执行版本 %s 的SQL语句", version)
	err = u.executeSQLForVersion(version)
	if err != nil {
		u.LogUpgrade("SQL语句执行失败: %v", err)
		u.recordUpgradeFailure(version, fmt.Sprintf("SQL语句执行失败: %v", err))
		return fmt.Errorf("执行SQL语句失败: %w", err)
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

// executeSQLForVersion 执行指定版本的SQL语句
func (u *Upgrade) executeSQLForVersion(version string) error {
	switch version {
	case "v1.7.0":
		return u.executeSQLForV1_7_0()
	case "v1.8.0":
		return u.executeSQLForV1_8_0()
	case "v1.9.0":
		return u.executeSQLForV1_9_0()
	default:
		return fmt.Errorf("未找到版本 %s 的SQL语句", version)
	}
}

// executeSQLForV1_7_0 执行v1.7.0版本的SQL语句
func (u *Upgrade) executeSQLForV1_7_0() error {
	sqlStatements := []string{
		"CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL, token TEXT, nickname TEXT, avatar TEXT, email TEXT, is_active INTEGER NOT NULL DEFAULT 1, is_admin INTEGER NOT NULL DEFAULT 0, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);",
		"CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);",
		"CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);",
		"CREATE INDEX IF NOT EXISTS idx_users_token ON users(token);",
		"CREATE TRIGGER IF NOT EXISTS trg_users_updated_at AFTER UPDATE ON users BEGIN UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id; END;",
		"ALTER TABLE nodes ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;",
		"CREATE INDEX IF NOT EXISTS idx_nodes_user_id ON nodes(user_id);",
		"CREATE INDEX IF NOT EXISTS idx_nodes_user_parent ON nodes(user_id, parent_id);",
		"CREATE TABLE IF NOT EXISTS sys_config (user_id INTEGER NOT NULL DEFAULT 0, key TEXT NOT NULL, value TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (user_id, key));",
		"CREATE TRIGGER IF NOT EXISTS trg_sys_config_updated_at AFTER UPDATE ON sys_config BEGIN UPDATE sys_config SET updated_at = CURRENT_TIMESTAMP WHERE user_id = NEW.user_id AND key = NEW.key; END;",
		"CREATE INDEX IF NOT EXISTS idx_sys_config_user_key ON sys_config(user_id, key);",
		"DROP TABLE IF EXISTS config;",
		"DROP TABLE IF EXISTS version;",
	}

	for i, stmt := range sqlStatements {
		u.LogUpgrade("执行SQL语句 #%d/%d: %s", i+1, len(sqlStatements), truncateString(stmt, 100))
		_, err := u.db.Exec(stmt)
		if err != nil {
			u.LogUpgrade("执行SQL语句失败: %v, 语句: %s", err, truncateString(stmt, 100))
		}
	}

	return nil
}

// executeSQLForV1_8_0 执行v1.8.0版本的SQL语句
func (u *Upgrade) executeSQLForV1_8_0() error {
	sqlStatements := []string{}

	for i, stmt := range sqlStatements {
		u.LogUpgrade("执行SQL语句 #%d/%d: %s", i+1, len(sqlStatements), truncateString(stmt, 100))
		_, err := u.db.Exec(stmt)
		if err != nil {
			u.LogUpgrade("执行SQL语句失败: %v, 语句: %s", err, truncateString(stmt, 100))
		}
	}

	return nil
}

// executeSQLForV1_9_0 执行v1.9.0版本的SQL语句
func (u *Upgrade) executeSQLForV1_9_0() error {
	sqlStatements := []string{
		"ALTER TABLE users ADD COLUMN api_key TEXT;",
		"CREATE INDEX IF NOT EXISTS idx_users_api_key ON users(api_key);",
	}

	for i, stmt := range sqlStatements {
		u.LogUpgrade("执行SQL语句 #%d/%d: %s", i+1, len(sqlStatements), truncateString(stmt, 100))
		_, err := u.db.Exec(stmt)
		if err != nil {
			u.LogUpgrade("执行SQL语句失败: %v, 语句: %s", err, truncateString(stmt, 100))
		}
	}

	return nil
}

// executeDataProcessingLogic 执行特定版本的数据处理业务逻辑
func (u *Upgrade) executeDataProcessingLogic(version string) error {
	// 这里可以添加特定版本的数据处理业务逻辑
	switch version {
	case "v1.7.0":
		return u.processDataForV1_7_0()
	case "v1.8.0":
		return u.processDataForV1_8_0()
	case "v1.9.0":
		return u.processDataForV1_9_0()
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
	// _, err := u.db.Exec("INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)", "feature_flag_v170", "enabled")
	// if err != nil {
	// 	return fmt.Errorf("更新配置项失败: %w", err)
	// }

	return nil
}

// processDataForV1_8_0 版本1.8.0的数据处理业务逻辑
func (u *Upgrade) processDataForV1_8_0() error {
	u.LogUpgrade("执行版本 v1.8.0 的数据处理业务逻辑")

	_, err := u.db.Exec(`
		INSERT OR IGNORE INTO sys_config (user_id, key, value)
		VALUES (0, 'allow_register', 'true')
	`)
	if err != nil {
		return fmt.Errorf("初始化默认配置失败: %w", err)
	}

	u.LogUpgrade("默认配置初始化完成：allow_register=true")
	return nil
}

// processDataForV1_9_0 版本1.9.0的数据处理业务逻辑
func (u *Upgrade) processDataForV1_9_0() error {
	u.LogUpgrade("执行版本 v1.9.0 的数据处理业务逻辑")

	// 为所有现有用户生成api_key
	_, err := u.db.Exec(`
		UPDATE users 
		SET api_key = lower(hex(randomblob(16)))
		WHERE api_key IS NULL OR api_key = ''
	`)
	if err != nil {
		return fmt.Errorf("为现有用户生成api_key失败: %w", err)
	}

	u.LogUpgrade("为现有用户生成api_key完成")
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
