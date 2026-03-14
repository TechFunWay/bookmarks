// DOM元素
const elements = {
  syncStatus: document.getElementById('syncStatus'),
  statusMessage: document.getElementById('statusMessage'),
  serverUrl: document.getElementById('serverUrl'),
  syncMode: document.getElementById('syncMode'),
  syncInterval: document.getElementById('syncInterval'),
  syncButton: document.getElementById('syncButton'),
  lastSync: document.getElementById('lastSync'),
  syncStatusText: document.getElementById('syncStatusText'),
  folderSyncStatus: document.getElementById('folderSyncStatus'),
  syncFolders: document.getElementById('syncFolders'),
  configButton: document.getElementById('configButton'),
  // 同步方向展示
  dirEdge: document.getElementById('dirEdge'),
  dirApp: document.getElementById('dirApp'),
  dirSyncMode: document.getElementById('dirSyncMode'),
  // 内嵌进度面板
  progressSection: document.getElementById('progressSection'),
  progressBar: document.getElementById('progressBar'),
  progressStep: document.getElementById('progressStep'),
  progressCount: document.getElementById('progressCount')
};

// 初始化页面
async function initialize() {
  console.log('【网址收藏夹】同步助手 - 独立窗口初始化...');

  await loadConfig();

  await loadSyncStatus();

  bindEvents();

  console.log('【网址收藏夹】同步助手 - 独立窗口初始化完成');
}

document.addEventListener('DOMContentLoaded', initialize);

document.addEventListener('unload', () => {
  stopStatusCheck();
});

// 加载配置
async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      const config = response.config;

      elements.serverUrl.textContent = config.serverUrl;
      elements.syncMode.textContent = getSyncModeText(config.syncMode);
      elements.syncInterval.textContent = config.enableAutoSync
        ? formatSyncInterval(config.syncInterval)
        : '未启用';

      if (config.lastSyncTime) {
        elements.lastSync.textContent = `最后同步: ${formatTime(config.lastSyncTime)}`;
      }

      // 同步方向展示块
      const edgeMode = config.edgeFolderMode || 'all';
      const appMode = config.appFolderMode || 'all';
      const edgePart = edgeMode === 'select' ? (config.edgeFolderName || '未选择') : '全部书签';
      const appPart = appMode === 'select' ? (config.appFolderName || '未选择') : '根目录';
      elements.dirEdge.textContent = edgePart;
      elements.dirApp.textContent = appPart;
      elements.dirSyncMode.textContent = getSyncModeText(config.syncMode);

      let syncInfo = '';
      if (config.enableAutoSync) {
        syncInfo += '自动同步';
        syncInfo += ` (${edgePart} → ${appPart})`;

        if (edgeMode === 'select' || appMode === 'select') {
          elements.folderSyncStatus.style.display = 'flex';
          elements.syncFolders.textContent = `${edgePart} → ${appPart}`;
        } else {
          elements.folderSyncStatus.style.display = 'none';
        }
      } else {
        syncInfo = '手动同步';
        elements.folderSyncStatus.style.display = 'none';
      }

      elements.syncStatusText.textContent = syncInfo;

      if (config.syncResult) {
        const stats = config.syncResult;
        let resultText = '';

        if (stats.folders > 0) {
          resultText += `创建文件夹: ${stats.folders} `;
        }
        if (stats.bookmarks > 0) {
          resultText += `创建书签: ${stats.bookmarks} `;
        }
        if (stats.skipped > 0) {
          resultText += `跳过节点: ${stats.skipped}`;
        }

        if (resultText) {
          console.log('显示同步结果:', resultText);
          showStatus('success', resultText.trim());
        }
      }

      console.log('配置加载成功:', config);
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

// 加载同步状态
let statusCheckInterval = null;
let statusCheckCount = 0;
const MAX_STATUS_CHECKS = 120;

async function loadSyncStatus() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getSyncStatus' });
    if (response) {
      if (response.syncInProgress) {
        statusCheckCount++;

        if (statusCheckCount >= MAX_STATUS_CHECKS) {
          console.warn('同步状态检查超时，停止检查');
          showStatus('error', '同步超时，请重试');
          elements.progressStep.textContent = '同步超时';
          resetSyncUI();
          stopStatusCheck();
          return;
        }

        showStatus('syncing', '正在同步中...');
        if (response.syncProgress) {
          updateProgress(response.syncProgress);
        }
        elements.syncButton.disabled = true;
        elements.syncButton.textContent = '同步中...';
      } else {
        hideProgress();

        if (statusCheckCount > 0) {
          // 状态轮询期间同步完成：显示完成提示
          showStatus('success', '同步完成');
          await loadConfig();
        }
        // 初始化时无需覆盖已有状态

        resetSyncUI();
        stopStatusCheck();
      }
    }
  } catch (error) {
    console.error('加载同步状态失败:', error);
    showStatus('error', '获取状态失败');
    resetSyncUI();
    stopStatusCheck();
  }
}

// 重置同步UI
function resetSyncUI() {
  elements.syncButton.disabled = false;
  elements.syncButton.textContent = '立即同步';
  statusCheckCount = 0;
}

// 显示内嵌进度面板
function showProgress() {
  elements.progressSection.classList.add('visible');
  elements.progressBar.style.width = '0%';
  elements.progressStep.textContent = '准备同步...';
  elements.progressCount.textContent = '0 / 0';
}

// 更新内嵌进度面板
function updateProgress(progress) {
  if (!progress) return;
  if (progress.total > 0) {
    const percent = Math.round((progress.processed / progress.total) * 100);
    elements.progressBar.style.width = `${percent}%`;
    elements.progressCount.textContent = `${progress.processed} / ${progress.total}`;
  } else {
    elements.progressBar.style.width = '0%';
    elements.progressCount.textContent = '0 / 0';
  }
  if (progress.currentStep) {
    elements.progressStep.textContent = progress.currentStep;
  }
}

// 隐藏进度面板（同步完成后保留，不隐藏）
function hideProgress() {
  // 进度面板在同步完成/失败后保留显示，用户可看到最终结果
}

function startStatusCheck() {
  if (statusCheckInterval) {
    clearInterval(statusCheckInterval);
  }

  statusCheckCount = 0;
  statusCheckInterval = setInterval(async () => {
    await loadSyncStatus();
  }, 500);
}

function stopStatusCheck() {
  if (statusCheckInterval) {
    clearInterval(statusCheckInterval);
    statusCheckInterval = null;
  }
}

// 显示状态（所有类型均永久显示，直到下次点击同步才清除）
function showStatus(type, message) {
  elements.syncStatus.className = 'sync-status';
  elements.syncStatus.classList.add(type);
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
}

// 格式化时间
function formatTime(timeString) {
  try {
    const date = new Date(timeString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  } catch (error) {
    return timeString;
  }
}

// 格式化同步间隔
function formatSyncInterval(minutes) {
  if (minutes < 60) {
    return `${minutes}分钟`;
  } else if (minutes < 1440) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (mins === 0) {
      return `${hours}小时`;
    }
    return `${hours}小时${mins}分钟`;
  } else {
    const days = Math.floor(minutes / 1440);
    const hours = (minutes % 1440) / 60;
    if (hours === 0) {
      return `${days}天`;
    }
    return `${days}天${Math.floor(hours)}小时`;
  }
}

// 获取同步方式文本
function getSyncModeText(mode) {
  switch (mode) {
    case 'merge':
      return '合并同步';
    case 'replace':
      return '替换同步';
    default:
      return '未知';
  }
}

// 绑定事件
function bindEvents() {
  elements.syncButton.addEventListener('click', async () => {
    console.log('同步按钮点击');

    if (elements.syncButton.disabled) {
      console.log('同步按钮已禁用，忽略点击');
      return;
    }

    // 清除上次状态提示，展开进度面板
    elements.statusMessage.style.display = 'none';
    showProgress();

    elements.syncButton.disabled = true;
    elements.syncButton.textContent = '同步中...';
    showStatus('syncing', '正在同步中...');

    startStatusCheck();

    try {
      const response = await chrome.runtime.sendMessage({ action: 'sync' });
      console.log('同步请求响应:', response);

      if (response && response.status === 'success') {
        console.log('同步成功，等待状态检查检测到完成');
      } else if (response && response.status === 'error') {
        console.error('同步失败:', response.error);
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        elements.progressStep.textContent = '同步失败';
        resetSyncUI();
        stopStatusCheck();
      } else {
        console.warn('同步响应状态未知:', response);
      }
    } catch (error) {
      console.error('同步请求失败:', error);
      showStatus('error', '同步失败');
      elements.progressStep.textContent = '同步失败';
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.configButton.addEventListener('click', () => {
    console.log('配置按钮点击');
    chrome.tabs.create({
      url: chrome.runtime.getURL('options.html'),
      active: true
    });
  });
}
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  console.log('sync-window 收到消息:', message.action);

  switch (message.action) {
    case 'configUpdated':
      console.log('收到配置更新通知，重新加载配置');
      loadConfig();
      break;
    case 'syncProgress':
      if (message.progress) {
        updateProgress(message.progress);
      }
      break;
    case 'syncComplete':
      console.log('收到同步完成通知');
      if (message.progress) {
        updateProgress(message.progress);
      }
      elements.progressBar.style.width = '100%';
      elements.progressStep.textContent = '同步完成';
      showStatus('success', '同步完成');
      loadConfig().then(() => {
        resetSyncUI();
        stopStatusCheck();
      });
      break;
    case 'syncError':
      console.error('收到同步错误通知:', message.error);
      showStatus('error', `同步失败: ${message.error}`);
      elements.progressStep.textContent = '同步失败';
      resetSyncUI();
      stopStatusCheck();
      break;
    default:
      console.log('未知消息:', message.action);
      break;
  }
});
