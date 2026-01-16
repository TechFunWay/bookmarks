// DOM元素
const elements = {
  serverUrl: document.getElementById('serverUrl'),
  syncInterval: document.getElementById('syncInterval'),
  syncIntervalValue: document.getElementById('syncIntervalValue'),
  syncDirection: document.getElementById('syncDirection'),
  firstSyncMode: document.getElementById('firstSyncMode'),
  saveButton: document.getElementById('saveButton'),
  resetButton: document.getElementById('resetButton'),
  statusMessage: document.getElementById('statusMessage')
};

// 默认配置
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  syncInterval: 5,
  syncDirection: 'bidirectional',
  firstSyncMode: 'merge'
};

// 初始化页面
async function initialize() {
  console.log('选项页面初始化...');
  
  // 加载配置
  await loadConfig();
  
  // 绑定事件
  bindEvents();
  
  console.log('选项页面初始化完成');
}

// 加载配置
async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      const config = response.config;
      
      // 更新表单
      elements.serverUrl.value = config.serverUrl || DEFAULT_CONFIG.serverUrl;
      elements.syncInterval.value = config.syncInterval || DEFAULT_CONFIG.syncInterval;
      elements.syncIntervalValue.textContent = `${config.syncInterval || DEFAULT_CONFIG.syncInterval}分钟`;
      elements.syncDirection.value = config.syncDirection || DEFAULT_CONFIG.syncDirection;
      elements.firstSyncMode.value = config.firstSyncMode || DEFAULT_CONFIG.firstSyncMode;
      
      console.log('配置加载成功:', config);
    } else {
      // 使用默认配置
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
  elements.syncIntervalValue.textContent = `${DEFAULT_CONFIG.syncInterval}分钟`;
  elements.syncDirection.value = DEFAULT_CONFIG.syncDirection;
  elements.firstSyncMode.value = DEFAULT_CONFIG.firstSyncMode;
}

// 绑定事件
function bindEvents() {
  // 同步间隔滑块
  elements.syncInterval.addEventListener('input', function() {
    const value = this.value;
    elements.syncIntervalValue.textContent = `${value}分钟`;
  });
  
  // 保存按钮
  elements.saveButton.addEventListener('click', async () => {
    console.log('保存配置按钮点击');
    
    // 禁用按钮
    elements.saveButton.disabled = true;
    elements.saveButton.textContent = '保存中...';
    
    try {
      // 获取表单值
      const config = {
        serverUrl: elements.serverUrl.value.trim(),
        syncInterval: parseInt(elements.syncInterval.value),
        syncDirection: elements.syncDirection.value,
        firstSyncMode: elements.firstSyncMode.value
      };
      
      // 验证配置
      if (!validateConfig(config)) {
        return;
      }
      
      // 保存配置
      const response = await chrome.runtime.sendMessage({ 
        action: 'updateConfig',
        config 
      });
      
      if (response && response.status === 'success') {
        showStatus('success', '配置保存成功');
        console.log('配置保存成功:', config);
      } else {
        showStatus('error', '配置保存失败');
        console.error('配置保存失败:', response);
      }
    } catch (error) {
      console.error('保存配置失败:', error);
      showStatus('error', '保存配置失败');
    } finally {
      // 启用按钮
      elements.saveButton.disabled = false;
      elements.saveButton.textContent = '保存配置';
    }
  });
  
  // 重置按钮
  elements.resetButton.addEventListener('click', () => {
    console.log('重置配置按钮点击');
    loadDefaultConfig();
    showStatus('success', '已恢复默认配置');
  });
}

// 验证配置
function validateConfig(config) {
  // 验证服务器地址
  if (!config.serverUrl) {
    showStatus('error', '请输入服务器地址');
    return false;
  }
  
  // 验证URL格式
  try {
    new URL(config.serverUrl);
  } catch (error) {
    showStatus('error', '服务器地址格式错误');
    return false;
  }
  
  // 验证同步间隔
  if (isNaN(config.syncInterval) || config.syncInterval < 1 || config.syncInterval > 60) {
    showStatus('error', '同步间隔必须在1-60分钟之间');
    return false;
  }
  
  // 验证同步方向
  if (!['unidirectional', 'bidirectional'].includes(config.syncDirection)) {
    showStatus('error', '同步方向无效');
    return false;
  }
  
  // 验证首次同步模式
  if (!['merge', 'replace'].includes(config.firstSyncMode)) {
    showStatus('error', '首次同步模式无效');
    return false;
  }
  
  return true;
}

// 显示状态消息
function showStatus(type, message) {
  // 移除旧的类
  elements.statusMessage.className = 'status-message';
  
  // 添加新的类
  elements.statusMessage.classList.add(type);
  
  // 更新消息
  elements.statusMessage.textContent = message;
  
  // 3秒后隐藏消息
  setTimeout(() => {
    elements.statusMessage.style.display = 'none';
  }, 3000);
}

// 初始化
initialize();
