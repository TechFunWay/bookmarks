package logic

import (
	"fmt"
	"testing"
)

func TestUpgradeManager(t *testing.T) {
	// 测试升级管理器基本功能
	manager := NewUpgrade("v1.0.0")

	if manager.currentVersion != "v1.0.0" {
		t.Errorf("期望版本 v1.0.0，实际得到 %s", manager.currentVersion)
	}

	if manager.upgradeScripts == nil {
		t.Error("升级脚本列表不应为nil")
	}

	if manager.logger == nil {
		t.Error("日志记录器不应为nil")
	}

	fmt.Println("UpgradeManager 基本功能测试通过")
}

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.1.0", -1},
		{"v1.1.0", "v1.0.0", 1},
		{"v1.0.0", "v2.0.0", -1},
	}

	for _, test := range tests {
		result := CompareVersions(test.v1, test.v2)
		if result != test.expected {
			t.Errorf("CompareVersions(%s, %s) = %d; 期望 %d", test.v1, test.v2, result, test.expected)
		}
	}

	fmt.Println("版本比较功能测试通过")
}

func TestSystemInfo(t *testing.T) {
	upgrade := NewUpgrade("v1.0.0")
	info := upgrade.GetSystemInfo()

	if len(info) == 0 {
		t.Error("系统信息不应为空")
	}

	requiredKeys := []string{"os", "arch", "goVersion", "currentTime"}
	for _, key := range requiredKeys {
		if _, exists := info[key]; !exists {
			t.Errorf("缺少必需的键: %s", key)
		}
	}

	fmt.Println("系统信息获取功能测试通过")
}

func TestRunSystemUpgrade(t *testing.T) {
	upgrade := NewUpgrade("v1.0.0")

	err := upgrade.ExecuteUpgrades()
	if err != nil {
		t.Errorf("系统升级执行失败: %v", err)
	}

	fmt.Println("系统升级执行测试通过")
}

func TestGetUpgradeStatus(t *testing.T) {
	upgrade := NewUpgrade("v1.0.0")
	status := upgrade.GetStatus()

	if status == nil {
		t.Error("升级状态不应为nil")
	}

	systemInfo, exists := status["systemInfo"]
	if !exists {
		t.Error("应包含系统信息键")
	} else if systemInfo == nil {
		t.Error("系统信息不应为nil")
	} else {
		// 确保它是map类型
		if _, ok := systemInfo.(map[string]interface{}); !ok {
			t.Errorf("系统信息应为map[string]interface{}类型，实际类型: %T", systemInfo)
		}
	}

	fmt.Println("升级状态获取测试通过")
}
