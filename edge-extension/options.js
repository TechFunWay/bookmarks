// DOM元素
const elements = {
  serverUrl: document.getElementById('serverUrl'),
  apiKey: document.getElementById('apiKey'),
  syncInterval: document.getElementById('syncInterval'),
  syncIntervalValue: document.getElementById('syncIntervalValue'),
  syncMode: document.getElementById('syncMode'),
  enableAutoSync: document.getElementById('enableAutoSync'),
  edgeFolder: document.getElementById('edgeFolder'),
  appFolder: document.getElementById('appFolder'),
  refreshFoldersBtn: document.getElementById('refreshFoldersBtn'),
  saveButton: document.getElementById('saveButton'),
  resetButton: document.getElementById('resetButton'),
  statusMessage: document.getElementById('statusMessage')
};

// 配置默认值
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  apiKey: '',
  syncInterval: 5,
  enableAutoSync: false,
  edgeFolderMode: 'all',   // 'all' | 'select'
  edgeFolderId: null,
  edgeFolderName: '',
  appFolderMode: 'all',    // 'all' | 'select'
  appFolderId: null,
  appFolderName: '',
  syncMode: 'merge',
  lastSyncTime: null
};

// 获取当前选中的 radio 值
function getRadioValue(name) {
  const checked = document.querySelector(`input[name="${name}"]:checked`);
  return checked ? checked.value : 'all';
}

// 设置 radio 值
function setRadioValue(name, value) {
  const radio = document.querySelector(`input[name="${name}"][value="${value}"]`);
  if (radio) radio.checked = true;
}

// 根据 mode 显示/隐藏对应的 select
function updateFolderSelectVisibility() {
  const edgeMode = getRadioValue('edgeFolderMode');
  const appMode = getRadioValue('appFolderMode');

  const edgeWasHidden = elements.edgeFolder.style.display === 'none';
  const appWasHidden = elements.appFolder.style.display === 'none';

  elements.edgeFolder.style.display = edgeMode === 'select' ? 'block' : 'none';
  elements.appFolder.style.display = appMode === 'select' ? 'block' : 'none';

  // 有任意一个"指定目录"时显示刷新按钮
  const showRefresh = edgeMode === 'select' || appMode === 'select';
  document.getElementById('refreshFolders').style.display = showRefresh ? 'block' : 'none';

  // 只在对应下拉从隐藏变为显示时才加载，避免切换另一侧 radio 触发不必要的刷新
  const needLoadEdge = edgeMode === 'select' && edgeWasHidden;
  const needLoadApp = appMode === 'select' && appWasHidden;
  if (needLoadEdge || needLoadApp) {
    loadFolderLists(needLoadEdge, needLoadApp);
  }
}

// 初始化页面
async function initialize() {
  console.log('【网址收藏夹】同步助手 - 选项初始化...');
  await loadConfig();
  bindEvents();
  console.log('【网址收藏夹】同步助手 - 选项初始化完成');
}

// 加载配置
async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });

    if (response && response.config) {
      const config = response.config;

      elements.serverUrl.value = config.serverUrl || DEFAULT_CONFIG.serverUrl;
      elements.apiKey.value = config.apiKey || DEFAULT_CONFIG.apiKey;
      elements.syncInterval.value = config.syncInterval || DEFAULT_CONFIG.syncInterval;
      elements.syncIntervalValue.textContent = formatSyncInterval(config.syncInterval || DEFAULT_CONFIG.syncInterval);
      elements.syncMode.value = config.syncMode || DEFAULT_CONFIG.syncMode;
      elements.enableAutoSync.checked = config.enableAutoSync !== false;

      // 兼容旧配置：将 syncScope 映射为新字段
      if (config.syncScope && !config.edgeFolderMode) {
        if (config.syncScope === 'edge-to-app') {
          config.edgeFolderMode = 'select';
          config.appFolderMode = 'select';
        } else if (config.syncScope === 'folder') {
          config.edgeFolderMode = 'all';
          config.appFolderMode = 'select';
        } else {
          config.edgeFolderMode = 'all';
          config.appFolderMode = 'all';
        }
      }

      setRadioValue('edgeFolderMode', config.edgeFolderMode || DEFAULT_CONFIG.edgeFolderMode);
      setRadioValue('appFolderMode', config.appFolderMode || DEFAULT_CONFIG.appFolderMode);

      // 先显示/隐藏 select，再赋值（loadFolderLists 是异步的，等它完成后再设选中值）
      const edgeMode = config.edgeFolderMode || DEFAULT_CONFIG.edgeFolderMode;
      const appMode = config.appFolderMode || DEFAULT_CONFIG.appFolderMode;

      elements.edgeFolder.style.display = edgeMode === 'select' ? 'block' : 'none';
      elements.appFolder.style.display = appMode === 'select' ? 'block' : 'none';

      const showRefresh = edgeMode === 'select' || appMode === 'select';
      document.getElementById('refreshFolders').style.display = showRefresh ? 'block' : 'none';

      if (showRefresh) {
        await loadFolderLists(edgeMode === 'select', appMode === 'select');
      }

      if (config.edgeFolderId) elements.edgeFolder.value = config.edgeFolderId;
      if (config.appFolderId) elements.appFolder.value = config.appFolderId;

      console.log('配置加载成功:', config);
    } else {
      loadDefaultConfig();
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

// 加载默认配置
function loadDefaultConfig() {
  elements.serverUrl.value = DEFAULT_CONFIG.serverUrl;
  elements.syncInterval.value = DEFAULT_CONFIG.syncInterval;
  elements.syncIntervalValue.textContent = formatSyncInterval(DEFAULT_CONFIG.syncInterval);
  elements.syncMode.value = DEFAULT_CONFIG.syncMode;
  elements.enableAutoSync.checked = DEFAULT_CONFIG.enableAutoSync;
  setRadioValue('edgeFolderMode', DEFAULT_CONFIG.edgeFolderMode);
  setRadioValue('appFolderMode', DEFAULT_CONFIG.appFolderMode);
  elements.edgeFolder.style.display = 'none';
  elements.appFolder.style.display = 'none';
  document.getElementById('refreshFolders').style.display = 'none';
}

// 绑定事件
function bindEvents() {
  elements.syncInterval.addEventListener('input', function() {
    elements.syncIntervalValue.textContent = formatSyncInterval(parseInt(this.value));
  });

  // 浏览器目录 radio 切换
  document.querySelectorAll('input[name="edgeFolderMode"]').forEach(radio => {
    radio.addEventListener('change', () => updateFolderSelectVisibility());
  });

  // 应用目录 radio 切换
  document.querySelectorAll('input[name="appFolderMode"]').forEach(radio => {
    radio.addEventListener('change', () => updateFolderSelectVisibility());
  });

  elements.refreshFoldersBtn.addEventListener('click', () => {
    const edgeMode = getRadioValue('edgeFolderMode');
    const appMode = getRadioValue('appFolderMode');
    loadFolderLists(edgeMode === 'select', appMode === 'select');
  });

  document.getElementById('openSyncWindowBtn').addEventListener('click', () => {
    chrome.tabs.create({ url: chrome.runtime.getURL('sync-window.html') });
  });

  elements.saveButton.addEventListener('click', async () => {
    await saveConfig();
  });

  elements.resetButton.addEventListener('click', async () => {
    loadDefaultConfig();
    await saveConfig();
  });
}

// 保存配置
async function saveConfig() {
  try {
    const edgeFolderMode = getRadioValue('edgeFolderMode');
    const appFolderMode = getRadioValue('appFolderMode');

    const config = {
      serverUrl: elements.serverUrl.value.trim(),
      apiKey: elements.apiKey.value.trim(),
      syncInterval: parseInt(elements.syncInterval.value),
      enableAutoSync: elements.enableAutoSync.checked,
      edgeFolderMode,
      edgeFolderId: edgeFolderMode === 'select' ? (elements.edgeFolder.value || null) : null,
      edgeFolderName: edgeFolderMode === 'select' ? getSelectedFolderName('edgeFolder') : '',
      appFolderMode,
      appFolderId: appFolderMode === 'select' ? (elements.appFolder.value || null) : null,
      appFolderName: appFolderMode === 'select' ? getSelectedFolderName('appFolder') : '',
      syncMode: elements.syncMode.value
    };

    if (!validateConfig(config)) return;

    const response = await chrome.runtime.sendMessage({
      action: 'updateConfig',
      config
    });

    if (response && response.status === 'success') {
      showStatus('success', '配置保存成功');
    } else {
      showStatus('error', '配置保存失败');
    }
  } catch (error) {
    console.error('保存配置失败:', error);
    showStatus('error', '保存配置失败');
  }
}

// 验证配置
function validateConfig(config) {
  if (!config.serverUrl) {
    showStatus('error', '请输入服务器地址');
    return false;
  }

  try {
    new URL(config.serverUrl);
  } catch {
    showStatus('error', '服务器地址格式错误');
    return false;
  }

  if (!config.apiKey) {
    showStatus('error', '请输入 API Key');
    return false;
  }

  if (isNaN(config.syncInterval) || config.syncInterval < 1 || config.syncInterval > 1440) {
    showStatus('error', '同步间隔必须在1-1440分钟之间');
    return false;
  }

  if (config.edgeFolderMode === 'select' && !config.edgeFolderId) {
    showStatus('error', '请选择浏览器同步目录');
    return false;
  }

  if (config.appFolderMode === 'select' && !config.appFolderId) {
    showStatus('error', '请选择应用同步目录');
    return false;
  }

  return true;
}

// 获取选中文件夹名称
function getSelectedFolderName(selectId) {
  const select = document.getElementById(selectId);
  const selectedOption = select.options[select.selectedIndex];
  if (selectedOption && selectedOption.value) {
    const match = selectedOption.textContent.match(/^([^\(]+)\s*\(/);
    return match ? match[1].trim() : selectedOption.textContent.trim();
  }
  return '';
}

// 加载文件夹列表
async function loadFolderLists(loadEdge = false, loadApp = false) {
  try {
    if (loadEdge) {
      const edgeResponse = await chrome.runtime.sendMessage({ action: 'getEdgeFolders' });
      if (edgeResponse && edgeResponse.folders) {
        elements.edgeFolder.innerHTML = '<option value="">选择浏览器目录...</option>';
        edgeResponse.folders.forEach(folder => {
          const option = document.createElement('option');
          option.value = folder.id;
          option.textContent = folder.path || folder.title;
          elements.edgeFolder.appendChild(option);
        });
      }
    }

    if (loadApp) {
      const appResponse = await chrome.runtime.sendMessage({ action: 'getAppFolders' });
      if (appResponse && appResponse.folders) {
        elements.appFolder.innerHTML = '<option value="">选择应用目录...</option>';
        appResponse.folders.forEach(folder => {
          const option = document.createElement('option');
          option.value = folder.id;
          option.textContent = folder.title;
          elements.appFolder.appendChild(option);
        });
      }
    }
  } catch (error) {
    console.error('加载文件夹列表失败:', error);
  }
}

// 显示状态
function showStatus(type, message) {
  elements.statusMessage.className = `status-message ${type}`;
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
  setTimeout(() => {
    elements.statusMessage.style.display = 'none';
  }, 3000);
}

// 格式化同步间隔
function formatSyncInterval(minutes) {
  if (minutes < 60) {
    return `${minutes}分钟`;
  } else if (minutes < 1440) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    return mins === 0 ? `${hours}小时` : `${hours}小时${mins}分钟`;
  } else {
    const days = Math.floor(minutes / 1440);
    const hours = Math.floor((minutes % 1440) / 60);
    return hours === 0 ? `${days}天` : `${days}天${hours}小时`;
  }
}

// 初始化
document.addEventListener('DOMContentLoaded', initialize);
