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
  syncFolders: document.getElementById('syncFolders')
};

// 初始化页面
async function initialize() {
  console.log('【网址收藏夹】同步助手 - 弹出页面初始化...');
  
  await loadConfig();
  
  await loadSyncStatus();
  
  bindEvents();
  
  console.log('【网址收藏夹】同步助手 - 弹出页面初始化完成');
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
      elements.syncInterval.textContent = formatSyncInterval(config.syncInterval);
      
      if (config.lastSyncTime) {
        elements.lastSync.textContent = `最后同步: ${formatTime(config.lastSyncTime)}`;
      }
      
      let syncInfo = '';
      if (config.enableAutoSync) {
        syncInfo += '自动同步';
        if (config.syncScope === 'folder') {
          const appFolderName = config.appFolderName || '未选择';
          syncInfo += ` (同步到 ${appFolderName})`;
          elements.folderSyncStatus.style.display = 'flex';
          elements.syncFolders.textContent = `同步到 ${appFolderName}`;
        } else if (config.syncScope === 'edge-to-app') {
          const edgeFolderName = config.edgeFolderName || '未选择';
          const appFolderName = config.appFolderName || '未选择';
          syncInfo += ` (${edgeFolderName} → ${appFolderName})`;
          elements.folderSyncStatus.style.display = 'flex';
          elements.syncFolders.textContent = `${edgeFolderName} → ${appFolderName}`;
        } else {
          syncInfo += ' (同步到根目录)';
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
const MAX_STATUS_CHECKS = 120; // 最多检查120次（60秒）

async function loadSyncStatus() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getSyncStatus' });
    if (response) {
      if (response.syncInProgress) {
        statusCheckCount++;
        
        if (statusCheckCount >= MAX_STATUS_CHECKS) {
          console.warn('同步状态检查超时，停止检查');
          showStatus('error', '同步超时，请重试');
          elements.syncButton.disabled = false;
          elements.syncButton.textContent = '立即同步';
          stopStatusCheck();
          return;
        }
        
        showStatus('syncing', '正在同步中...');
        elements.syncButton.disabled = true;
        elements.syncButton.textContent = '同步中...';
      } else {
        showStatus('success', '就绪');
        elements.syncButton.disabled = false;
        elements.syncButton.textContent = '立即同步';
        stopStatusCheck();
        await loadConfig();
      }
    }
  } catch (error) {
    console.error('加载同步状态失败:', error);
    showStatus('error', '获取状态失败');
    elements.syncButton.disabled = false;
    elements.syncButton.textContent = '立即同步';
    stopStatusCheck();
  }
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

// 绑定事件
function bindEvents() {
  elements.syncButton.addEventListener('click', async () => {
    console.log('同步按钮点击');
    
    elements.syncButton.disabled = true;
    elements.syncButton.textContent = '同步中...';
    showStatus('syncing', '正在同步中...');
    
    try {
      const response = await chrome.runtime.sendMessage({ action: 'sync' });
      console.log('同步请求响应:', response);
      
      if (response && response.status === 'success') {
        console.log('同步成功');
        showStatus('success', '同步完成');
        await loadConfig();
      } else if (response && response.status === 'error') {
        console.error('同步失败:', response.error);
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
      } else {
        console.warn('同步响应状态未知:', response);
        startStatusCheck();
      }
    } catch (error) {
      console.error('同步请求失败:', error);
      showStatus('error', '同步失败');
      elements.syncButton.disabled = false;
      elements.syncButton.textContent = '立即同步';
    }
  });
}

// 显示状态
function showStatus(type, message) {
  elements.syncStatus.className = 'sync-status';
  
  elements.syncStatus.classList.add(type);
  
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
  
  setTimeout(() => {
    elements.statusMessage.style.display = 'none';
  }, 3000);
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

// 初始化
initialize();
