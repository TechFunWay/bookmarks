// DOM元素
const elements = {
  serverUrl: document.getElementById('serverUrl'),
  syncInterval: document.getElementById('syncInterval'),
  syncIntervalValue: document.getElementById('syncIntervalValue'),
  syncMode: document.getElementById('syncMode'),
  enableAutoSync: document.getElementById('enableAutoSync'),
  syncScope: document.getElementById('syncScope'),
  edgeFolder: document.getElementById('edgeFolder'),
  appFolder: document.getElementById('appFolder'),
  refreshFoldersBtn: document.getElementById('refreshFoldersBtn'),
  saveButton: document.getElementById('saveButton'),
  resetButton: document.getElementById('resetButton'),
  statusMessage: document.getElementById('statusMessage')
};

// 默认配置
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  syncInterval: 5,
  enableAutoSync: false,
  syncScope: 'all',
  syncMode: 'merge',
  edgeFolderId: null,
  edgeFolderName: '',
  appFolderId: null,
  appFolderName: '',
  lastSyncTime: null
};

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
    console.log('开始加载配置...');
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    console.log('getConfig响应:', response);
    
    if (response && response.config) {
      const config = response.config;
      console.log('配置对象:', config);
      
      elements.serverUrl.value = config.serverUrl || DEFAULT_CONFIG.serverUrl;
      elements.syncInterval.value = config.syncInterval || DEFAULT_CONFIG.syncInterval;
      elements.syncIntervalValue.textContent = formatSyncInterval(config.syncInterval || DEFAULT_CONFIG.syncInterval);
      elements.syncMode.value = config.syncMode || DEFAULT_CONFIG.syncMode;
      elements.enableAutoSync.checked = config.enableAutoSync !== false;
      elements.syncScope.value = config.syncScope || DEFAULT_CONFIG.syncScope;
      
      if (config.syncScope === 'folder') {
        document.getElementById('folderSyncSectionTitle').style.display = 'block';
        document.getElementById('folderSyncConfig').style.display = 'block';
        document.getElementById('edgeFolderConfig').style.display = 'none';
        document.getElementById('refreshFolders').style.display = 'block';
        await loadFolderLists();
        
        if (config.appFolderId) {
          elements.appFolder.value = config.appFolderId;
        }
      } else if (config.syncScope === 'edge-to-app') {
        document.getElementById('folderSyncSectionTitle').style.display = 'block';
        document.getElementById('folderSyncConfig').style.display = 'block';
        document.getElementById('edgeFolderConfig').style.display = 'block';
        document.getElementById('refreshFolders').style.display = 'block';
        await loadFolderLists();
        
        if (config.edgeFolderId) {
          elements.edgeFolder.value = config.edgeFolderId;
        }
        if (config.appFolderId) {
          elements.appFolder.value = config.appFolderId;
        }
      } else {
        document.getElementById('folderSyncSectionTitle').style.display = 'none';
        document.getElementById('folderSyncConfig').style.display = 'none';
        document.getElementById('edgeFolderConfig').style.display = 'none';
        document.getElementById('refreshFolders').style.display = 'none';
      }
      
      console.log('配置加载成功:', config);
    } else {
      loadDefaultConfig();
      console.log('使用默认配置');
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
  elements.syncScope.value = DEFAULT_CONFIG.syncScope;
}

// 绑定事件
function bindEvents() {
  elements.syncInterval.addEventListener('input', function() {
    const value = parseInt(this.value);
    elements.syncIntervalValue.textContent = formatSyncInterval(value);
  });
  
  elements.enableAutoSync.addEventListener('change', function() {
    console.log('启用自动同步:', this.checked);
  });
  
  elements.syncScope.addEventListener('change', async function() {
    console.log('同步到哪里:', this.value);
    
    if (this.value === 'folder') {
      document.getElementById('folderSyncSectionTitle').style.display = 'block';
      document.getElementById('folderSyncConfig').style.display = 'block';
      document.getElementById('edgeFolderConfig').style.display = 'none';
      document.getElementById('refreshFolders').style.display = 'block';
      await loadFolderLists();
      
      elements.appFolder.value = '';
      elements.edgeFolder.value = '';
    } else if (this.value === 'edge-to-app') {
      document.getElementById('folderSyncSectionTitle').style.display = 'block';
      document.getElementById('folderSyncConfig').style.display = 'block';
      document.getElementById('edgeFolderConfig').style.display = 'block';
      document.getElementById('refreshFolders').style.display = 'block';
      await loadFolderLists();
      
      elements.appFolder.value = '';
      elements.edgeFolder.value = '';
    } else {
      document.getElementById('folderSyncSectionTitle').style.display = 'none';
      document.getElementById('folderSyncConfig').style.display = 'none';
      document.getElementById('edgeFolderConfig').style.display = 'none';
      document.getElementById('refreshFolders').style.display = 'none';
      elements.appFolder.value = '';
      elements.edgeFolder.value = '';
    }
  });
  
  elements.refreshFoldersBtn.addEventListener('click', loadFolderLists);
  
  elements.saveButton.addEventListener('click', async () => {
    console.log('保存配置');
    await saveConfig();
  });
  
  elements.resetButton.addEventListener('click', async () => {
    console.log('恢复默认配置');
    loadDefaultConfig();
    await saveConfig();
  });
}

// 保存配置
async function saveConfig() {
  try {
    const config = {
      serverUrl: elements.serverUrl.value.trim(),
      syncInterval: parseInt(elements.syncInterval.value),
      enableAutoSync: elements.enableAutoSync.checked,
      syncScope: elements.syncScope.value,
      syncMode: elements.syncMode.value,
      edgeFolderId: elements.edgeFolder.value || null,
      edgeFolderName: getSelectedFolderName('edgeFolder'),
      appFolderId: elements.appFolder.value || null,
      appFolderName: getSelectedFolderName('appFolder')
    };
    
    if (!validateConfig(config)) {
      return;
    }
    
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
  } catch (error) {
    showStatus('error', '服务器地址格式错误');
    return false;
  }
  
  if (isNaN(config.syncInterval) || config.syncInterval < 1 || config.syncInterval > 1440) {
    showStatus('error', '同步间隔必须在1-1440分钟之间');
    return false;
  }
  
  if (!['all', 'folder', 'edge-to-app'].includes(config.syncScope)) {
    showStatus('error', '同步范围无效');
    return false;
  }
  
  if (config.syncScope === 'folder' && !config.appFolderId) {
    showStatus('error', '请选择应用文件夹');
    return false;
  }
  
  if (config.syncScope === 'edge-to-app' && !config.edgeFolderId) {
    showStatus('error', '请选择Edge文件夹');
    return false;
  }
  
  if (config.syncScope === 'edge-to-app' && !config.appFolderId) {
    showStatus('error', '请选择应用文件夹');
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
async function loadFolderLists() {
  try {
    console.log('========== 开始加载文件夹列表 ==========');
    console.log('当前 syncScope 值:', elements.syncScope.value);
    
    const syncScope = elements.syncScope.value;
    
    if (syncScope === 'edge-to-app') {
      console.log('加载Edge文件夹列表...');
      const edgeResponse = await chrome.runtime.sendMessage({ action: 'getEdgeFolders' });
      console.log('Edge文件夹响应:', edgeResponse);
      
      if (edgeResponse && edgeResponse.folders) {
        const edgeSelect = elements.edgeFolder;
        edgeSelect.innerHTML = '<option value="">选择Edge文件夹...</option>';
        
        edgeResponse.folders.forEach(folder => {
          const option = document.createElement('option');
          option.value = folder.id;
          option.textContent = `${folder.path} (${folder.children?.length || 0})`;
          edgeSelect.appendChild(option);
        });
        
        console.log('Edge文件夹列表加载成功，共', edgeResponse.folders.length, '个文件夹');
      } else {
        console.error('Edge文件夹列表加载失败，响应:', edgeResponse);
      }
      
      console.log('加载应用文件夹列表...');
      const appResponse = await chrome.runtime.sendMessage({ action: 'getAppFolders' });
      console.log('应用文件夹响应:', appResponse);
      
      if (appResponse && appResponse.folders) {
        const appSelect = elements.appFolder;
        appSelect.innerHTML = '<option value="">选择应用文件夹...</option>';
        
        appResponse.folders.forEach(folder => {
          const option = document.createElement('option');
          option.value = folder.id;
          option.textContent = `${folder.path} (${folder.children?.length || 0})`;
          appSelect.appendChild(option);
        });
        
        console.log('应用文件夹列表加载成功，共', appResponse.folders.length, '个文件夹');
      } else {
        console.error('应用文件夹列表加载失败，响应:', appResponse);
      }
    } else if (syncScope === 'folder') {
      console.log('加载应用文件夹列表...');
      const response = await chrome.runtime.sendMessage({ action: 'getAppFolders' });
      console.log('应用文件夹响应:', response);
      
      if (response && response.folders) {
        const select = elements.appFolder;
        select.innerHTML = '<option value="">选择应用文件夹...</option>';
        
        response.folders.forEach(folder => {
          const option = document.createElement('option');
          option.value = folder.id;
          option.textContent = `${folder.path} (${folder.children?.length || 0})`;
          select.appendChild(option);
        });
        
        console.log('应用文件夹列表加载成功，共', response.folders.length, '个文件夹');
      } else {
        console.error('应用文件夹列表加载失败，响应:', response);
      }
    }
    console.log('========== 文件夹列表加载完成 ==========');
  } catch (error) {
    console.error('加载文件夹列表失败:', error);
    console.error('错误堆栈:', error.stack);
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

// 初始化
document.addEventListener('DOMContentLoaded', initialize);
