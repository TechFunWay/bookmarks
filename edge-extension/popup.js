// DOM元素
const elements = {
  syncStatus: document.getElementById('syncStatus'),
  statusMessage: document.getElementById('statusMessage'),
  serverUrl: document.getElementById('serverUrl'),
  syncDirection: document.getElementById('syncDirection'),
  syncInterval: document.getElementById('syncInterval'),
  syncButton: document.getElementById('syncButton'),
  lastSync: document.getElementById('lastSync')
};

// 初始化页面
async function initialize() {
  console.log('弹出页面初始化...');
  
  // 加载配置
  await loadConfig();
  
  // 加载同步状态
  await loadSyncStatus();
  
  // 绑定事件
  bindEvents();
  
  console.log('弹出页面初始化完成');
}

// 加载配置
async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      const config = response.config;
      
      // 更新UI
      elements.serverUrl.textContent = config.serverUrl;
      elements.syncDirection.textContent = getSyncDirectionText(config.syncDirection);
      elements.syncInterval.textContent = `${config.syncInterval}分钟`;
      
      // 更新最后同步时间
      if (config.lastSyncTime) {
        elements.lastSync.textContent = `最后同步: ${formatTime(config.lastSyncTime)}`;
      }
      
      console.log('配置加载成功:', config);
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

// 加载同步状态
async function loadSyncStatus() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getSyncStatus' });
    if (response) {
      if (response.syncInProgress) {
        showStatus('syncing', '正在同步中...');
        elements.syncButton.disabled = true;
        elements.syncButton.textContent = '同步中...';
      } else {
        showStatus('success', '就绪');
        elements.syncButton.disabled = false;
        elements.syncButton.textContent = '立即同步';
      }
    }
  } catch (error) {
    console.error('加载同步状态失败:', error);
  }
}

// 绑定事件
function bindEvents() {
  // 同步按钮点击事件
  elements.syncButton.addEventListener('click', async () => {
    console.log('同步按钮点击');
    
    // 禁用按钮
    elements.syncButton.disabled = true;
    elements.syncButton.textContent = '同步中...';
    showStatus('syncing', '正在同步中...');
    
    try {
      // 发送同步请求
      const response = await chrome.runtime.sendMessage({ action: 'sync' });
      console.log('同步请求响应:', response);
      
      // 延迟更新状态，等待同步完成
      setTimeout(async () => {
        await loadSyncStatus();
        await loadConfig(); // 重新加载配置，更新最后同步时间
      }, 3000);
    } catch (error) {
      console.error('同步请求失败:', error);
      showStatus('error', '同步失败');
      elements.syncButton.disabled = false;
      elements.syncButton.textContent = '立即同步';
    }
  });
  
  // 定期更新状态（每5秒）
  setInterval(async () => {
    await loadSyncStatus();
  }, 5000);
}

// 显示状态
function showStatus(type, message) {
  // 移除旧的类
  elements.syncStatus.className = 'sync-status';
  
  // 添加新的类
  elements.syncStatus.classList.add(type);
  
  // 更新消息
  elements.statusMessage.textContent = message;
  
  // 更新图标
  const iconElement = elements.syncStatus.querySelector('.status-icon');
  switch (type) {
    case 'success':
      iconElement.textContent = '✅';
      break;
    case 'error':
      iconElement.textContent = '❌';
      break;
    case 'syncing':
      iconElement.textContent = '🔄';
      break;
    default:
      iconElement.textContent = '📋';
  }
}

// 获取同步方向文本
function getSyncDirectionText(direction) {
  switch (direction) {
    case 'unidirectional':
      return '单向同步';
    case 'bidirectional':
      return '双向同步';
    default:
      return '未知';
  }
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

// 初始化
initialize();
