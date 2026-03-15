// DOM元素
const elements = {
  syncStatus: document.getElementById('syncStatus'),
  statusMessage: document.getElementById('statusMessage'),
  serverUrl: document.getElementById('serverUrl'),
  syncButton: document.getElementById('syncButton'),
  syncFromAppButton: document.getElementById('syncFromAppButton'),
  lastSync: document.getElementById('lastSync'),
  // 配置信息区
  e2aSyncStatusText: document.getElementById('e2aSyncStatusText'),
  a2eSyncStatusItem: document.getElementById('a2eSyncStatusItem'),
  a2eSyncStatusText: document.getElementById('a2eSyncStatusText'),
  configButton: document.getElementById('configButton'),
  // 浏览器→应用方向展示
  e2aDirectionTitle: document.getElementById('e2aDirectionTitle'),
  dirEdge: document.getElementById('dirEdge'),
  dirApp: document.getElementById('dirApp'),
  dirSyncMode: document.getElementById('dirSyncMode'),
  // 应用→浏览器方向展示
  a2eDirectionSection: document.getElementById('a2eDirectionSection'),
  a2eDirectionTitle: document.getElementById('a2eDirectionTitle'),
  dirA2ESource: document.getElementById('dirA2ESource'),
  dirA2ETarget: document.getElementById('dirA2ETarget'),
  dirA2ESyncMode: document.getElementById('dirA2ESyncMode'),
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

      // 服务器地址
      elements.serverUrl.textContent = config.serverUrl || '未设置';

      // 最后同步时间
      if (config.lastSyncTime) {
        elements.lastSync.textContent = `最后同步: ${formatTime(config.lastSyncTime)}`;
      }

      // ---- 浏览器→应用（E→A）----
      const edgeMode = config.edgeFolderMode || 'all';
      const appMode  = config.appFolderMode  || 'all';
      const edgePart = edgeMode === 'select' ? (config.edgeFolderName || '未选择') : '全部书签';
      const appPart  = appMode  === 'select' ? (config.appFolderName  || '未选择') : '根目录';

      // E→A 方向展示块
      elements.dirEdge.textContent     = edgePart;
      elements.dirApp.textContent      = appPart;
      elements.dirSyncMode.textContent = getSyncModeText(config.syncMode);

      // E→A 标题：「浏览器 → 应用（自动同步 · 每 5 分钟）」或「（手动同步）」
      const e2aAutoText = config.enableAutoSync
        ? `自动同步 · 每 ${formatSyncInterval(config.syncInterval)}`
        : '手动同步';
      elements.e2aDirectionTitle.textContent = `浏览器 → 应用（${e2aAutoText}）`;

      // E→A 配置信息行
      elements.e2aSyncStatusText.textContent =
        config.enableAutoSync
          ? `自动同步，每 ${formatSyncInterval(config.syncInterval)}，${getSyncModeText(config.syncMode)}`
          : `手动同步，${getSyncModeText(config.syncMode)}`;

      // ---- 应用→浏览器（A→E）----
      if (config.appToEdgeTargetFolderId) {
        const srcMode = config.appToEdgeSourceFolderMode || 'all';
        const srcPart = srcMode === 'select' ? (config.appToEdgeSourceFolderName || '未选择') : '全部书签';
        const tgtPart = config.appToEdgeTargetFolderName || '未选择目标目录';

        elements.dirA2ESource.textContent   = srcPart;
        elements.dirA2ETarget.textContent   = tgtPart;
        elements.dirA2ESyncMode.textContent = getSyncModeText(config.appToEdgeSyncMode);

        const a2eAutoText = config.enableAppToEdgeSync
          ? `自动同步 · 每 ${formatSyncInterval(config.appToEdgeSyncInterval || 5)}`
          : '手动同步';
        elements.a2eDirectionTitle.textContent = `应用 → 浏览器（${a2eAutoText}）`;

        elements.a2eSyncStatusText.textContent = config.enableAppToEdgeSync
          ? `自动同步，每 ${formatSyncInterval(config.appToEdgeSyncInterval || 5)}，${getSyncModeText(config.appToEdgeSyncMode)}`
          : `手动同步，${getSyncModeText(config.appToEdgeSyncMode)}`;
      } else {
        // 未配置时显示提示文案
        elements.dirA2ESource.textContent   = '全部书签';
        elements.dirA2ETarget.textContent   = '未配置（请先去设置页选择目标目录）';
        elements.dirA2ESyncMode.textContent = '手动同步';
        elements.a2eSyncStatusText.textContent = '未配置';
      }

      if (config.syncResult) {
        const stats = config.syncResult;
        let resultText = '';
        if (stats.folders   > 0) resultText += `创建文件夹: ${stats.folders} `;
        if (stats.bookmarks > 0) resultText += `创建书签: ${stats.bookmarks} `;
        if (stats.skipped   > 0) resultText += `跳过节点: ${stats.skipped}`;
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
  elements.syncButton.textContent = '立即同步（浏览器→应用）';
  if (elements.syncFromAppButton) {
    elements.syncFromAppButton.disabled = false;
    elements.syncFromAppButton.textContent = '立即同步（应用→浏览器）';
  }
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
    if (elements.syncFromAppButton) elements.syncFromAppButton.disabled = true;
    showStatus('syncing', '正在同步中（浏览器→应用）...');

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

  // 应用→浏览器同步按钮
  if (elements.syncFromAppButton) {
    elements.syncFromAppButton.addEventListener('click', async () => {
      if (elements.syncFromAppButton.disabled) return;

      elements.statusMessage.style.display = 'none';
      showProgress();

      elements.syncFromAppButton.disabled = true;
      elements.syncFromAppButton.textContent = '同步中...';
      elements.syncButton.disabled = true;
      showStatus('syncing', '正在同步中（应用→浏览器）...');

      startStatusCheck();

      try {
        const response = await chrome.runtime.sendMessage({ action: 'syncFromApp' });
        console.log('应用→浏览器同步响应:', response);

        if (response && response.status === 'success') {
          console.log('应用→浏览器同步成功');
        } else if (response && response.status === 'error') {
          showStatus('error', `同步失败: ${response.error || '未知错误'}`);
          elements.progressStep.textContent = '同步失败';
          resetSyncUI();
          stopStatusCheck();
        }
      } catch (error) {
        console.error('应用→浏览器同步失败:', error);
        showStatus('error', '同步失败');
        elements.progressStep.textContent = '同步失败';
        resetSyncUI();
        stopStatusCheck();
      }
    });
  }

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
