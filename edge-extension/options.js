// DOM 元素
const elements = {
  // 基本配置
  serverUrl: document.getElementById('serverUrl'),
  apiKey: document.getElementById('apiKey'),
  saveButton: document.getElementById('saveButton'),
  resetButton: document.getElementById('resetButton'),
  statusMessage: document.getElementById('statusMessage'),

  // 浏览器→应用（E→A）
  enableAutoSync: document.getElementById('enableAutoSync'),
  e2aSyncInterval: document.getElementById('e2aSyncInterval'),
  e2aSyncIntervalValue: document.getElementById('e2aSyncIntervalValue'),
  edgeFolder: document.getElementById('edgeFolder'),
  edgeFolderWrap: document.getElementById('edgeFolderWrap'),
  appFolder: document.getElementById('appFolder'),
  appFolderWrap: document.getElementById('appFolderWrap'),
  e2aSyncMode: document.getElementById('e2aSyncMode'),

  // 应用→浏览器（A→E）
  enableAppToEdgeSync: document.getElementById('enableAppToEdgeSync'),
  a2eSyncInterval: document.getElementById('a2eSyncInterval'),
  a2eSyncIntervalValue: document.getElementById('a2eSyncIntervalValue'),
  appToEdgeSourceFolder: document.getElementById('appToEdgeSourceFolder'),
  a2eSourceFolderWrap: document.getElementById('a2eSourceFolderWrap'),
  appToEdgeTargetFolder: document.getElementById('appToEdgeTargetFolder'),
  a2eSyncMode: document.getElementById('a2eSyncMode')
};

// 配置默认值
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  apiKey: '',
  // E→A
  enableAutoSync: false,
  syncInterval: 5,
  edgeFolderMode: 'all',
  edgeFolderId: null,
  edgeFolderName: '',
  appFolderMode: 'all',
  appFolderId: null,
  appFolderName: '',
  syncMode: 'merge',
  // A→E
  enableAppToEdgeSync: false,
  appToEdgeSyncInterval: 5,
  appToEdgeSourceFolderMode: 'all',
  appToEdgeSourceFolderId: null,
  appToEdgeSourceFolderName: '',
  appToEdgeTargetFolderMode: 'select',
  appToEdgeTargetFolderId: null,
  appToEdgeTargetFolderName: '',
  appToEdgeSyncMode: 'merge',
  lastSyncTime: null
};

// ---- 工具函数 ----

function getRadioValue(name) {
  const checked = document.querySelector(`input[name="${name}"]:checked`);
  return checked ? checked.value : 'all';
}

function setRadioValue(name, value) {
  const radio = document.querySelector(`input[name="${name}"][value="${value}"]`);
  if (radio) radio.checked = true;
}

function formatSyncInterval(minutes) {
  minutes = parseInt(minutes) || 5;
  if (minutes < 60) return `${minutes}分钟`;
  if (minutes < 1440) {
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return m === 0 ? `${h}小时` : `${h}小时${m}分钟`;
  }
  const d = Math.floor(minutes / 1440);
  const h = Math.floor((minutes % 1440) / 60);
  return h === 0 ? `${d}天` : `${d}天${h}小时`;
}

function getSelectedFolderName(selectId) {
  const select = document.getElementById(selectId);
  if (!select) return '';
  const opt = select.options[select.selectedIndex];
  if (opt && opt.value) {
    const match = opt.textContent.match(/^([^\(]+)\s*\(/);
    return match ? match[1].trim() : opt.textContent.trim();
  }
  return '';
}

function showStatus(type, message) {
  elements.statusMessage.className = `status-message ${type}`;
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
  setTimeout(() => { elements.statusMessage.style.display = 'none'; }, 3000);
}

// ---- E→A 文件夹显示控制 ----

function updateEdgeFolderVisibility() {
  const mode = getRadioValue('edgeFolderMode');
  const wrap = elements.edgeFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) loadEdgeFolders();
}

function updateAppFolderVisibility() {
  const mode = getRadioValue('appFolderMode');
  const wrap = elements.appFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) loadAppFolders();
}

// ---- A→E 来源目录显示控制 ----

function updateA2ESourceVisibility() {
  const mode = getRadioValue('appToEdgeSourceFolderMode');
  const wrap = elements.a2eSourceFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) loadA2ESourceFolders();
}

// ---- 目录列表加载 ----

async function loadEdgeFolders() {
  try {
    const resp = await chrome.runtime.sendMessage({ action: 'getEdgeFolders' });
    if (resp && resp.folders) {
      const saved = elements.edgeFolder.value;
      elements.edgeFolder.innerHTML = '<option value="">选择浏览器目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.path || f.title;
        elements.edgeFolder.appendChild(opt);
      });
      if (saved) elements.edgeFolder.value = saved;
    }
  } catch (e) { console.error('加载浏览器目录失败:', e); }
}

async function loadAppFolders() {
  try {
    const resp = await chrome.runtime.sendMessage({ action: 'getAppFolders' });
    if (resp && resp.folders) {
      const saved = elements.appFolder.value;
      elements.appFolder.innerHTML = '<option value="">选择应用目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.title;
        elements.appFolder.appendChild(opt);
      });
      if (saved) elements.appFolder.value = saved;
    }
  } catch (e) { console.error('加载应用目录失败:', e); }
}

async function loadA2ESourceFolders() {
  try {
    const resp = await chrome.runtime.sendMessage({ action: 'getAppFolders' });
    if (resp && resp.folders) {
      const saved = elements.appToEdgeSourceFolder.value;
      elements.appToEdgeSourceFolder.innerHTML = '<option value="">选择应用目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.title;
        elements.appToEdgeSourceFolder.appendChild(opt);
      });
      if (saved) elements.appToEdgeSourceFolder.value = saved;
    }
  } catch (e) { console.error('加载A→E来源目录失败:', e); }
}

async function loadA2ETargetFolders() {
  try {
    const resp = await chrome.runtime.sendMessage({ action: 'getEdgeFolders' });
    if (resp && resp.folders) {
      const saved = elements.appToEdgeTargetFolder.value;
      elements.appToEdgeTargetFolder.innerHTML = '<option value="">选择浏览器目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.path || f.title;
        elements.appToEdgeTargetFolder.appendChild(opt);
      });
      if (saved) elements.appToEdgeTargetFolder.value = saved;
    }
  } catch (e) { console.error('加载A→E目标目录失败:', e); }
}

// ---- 初始化 ----

async function initialize() {
  console.log('【网址收藏夹】同步助手 - 选项初始化...');
  await loadConfig();
  bindEvents();
  console.log('【网址收藏夹】同步助手 - 选项初始化完成');
}

// ---- 加载配置 ----

async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      const c = response.config;

      // 基本配置
      elements.serverUrl.value = c.serverUrl || DEFAULT_CONFIG.serverUrl;
      elements.apiKey.value = c.apiKey || DEFAULT_CONFIG.apiKey;

      // E→A：开关 & 频率
      elements.enableAutoSync.checked = c.enableAutoSync !== false;
      const e2aInterval = c.syncInterval || DEFAULT_CONFIG.syncInterval;
      elements.e2aSyncInterval.value = e2aInterval;
      elements.e2aSyncIntervalValue.textContent = formatSyncInterval(e2aInterval);

      // E→A：浏览器来源目录
      const edgeMode = c.edgeFolderMode || DEFAULT_CONFIG.edgeFolderMode;
      setRadioValue('edgeFolderMode', edgeMode);
      elements.edgeFolderWrap.style.display = edgeMode === 'select' ? 'flex' : 'none';
      if (edgeMode === 'select') {
        await loadEdgeFolders();
        if (c.edgeFolderId) elements.edgeFolder.value = c.edgeFolderId;
      }

      // E→A：应用目标目录
      const appMode = c.appFolderMode || DEFAULT_CONFIG.appFolderMode;
      setRadioValue('appFolderMode', appMode);
      elements.appFolderWrap.style.display = appMode === 'select' ? 'flex' : 'none';
      if (appMode === 'select') {
        await loadAppFolders();
        if (c.appFolderId) elements.appFolder.value = c.appFolderId;
      }

      // E→A：同步方式
      elements.e2aSyncMode.value = c.syncMode || DEFAULT_CONFIG.syncMode;

      // A→E：开关 & 频率
      elements.enableAppToEdgeSync.checked = !!c.enableAppToEdgeSync;
      const a2eInterval = c.appToEdgeSyncInterval || DEFAULT_CONFIG.appToEdgeSyncInterval;
      elements.a2eSyncInterval.value = a2eInterval;
      elements.a2eSyncIntervalValue.textContent = formatSyncInterval(a2eInterval);

      // A→E：来源目录
      const srcMode = c.appToEdgeSourceFolderMode || DEFAULT_CONFIG.appToEdgeSourceFolderMode;
      setRadioValue('appToEdgeSourceFolderMode', srcMode);
      elements.a2eSourceFolderWrap.style.display = srcMode === 'select' ? 'flex' : 'none';
      if (srcMode === 'select') {
        await loadA2ESourceFolders();
        if (c.appToEdgeSourceFolderId) elements.appToEdgeSourceFolder.value = c.appToEdgeSourceFolderId;
      }

      // A→E：目标浏览器目录（始终加载）
      await loadA2ETargetFolders();
      if (c.appToEdgeTargetFolderId) elements.appToEdgeTargetFolder.value = c.appToEdgeTargetFolderId;

      // A→E：同步方式
      elements.a2eSyncMode.value = c.appToEdgeSyncMode || DEFAULT_CONFIG.appToEdgeSyncMode;

      console.log('配置加载成功:', c);
    } else {
      loadDefaultConfig();
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

// ---- 加载默认配置 ----

function loadDefaultConfig() {
  elements.serverUrl.value = DEFAULT_CONFIG.serverUrl;
  elements.apiKey.value = DEFAULT_CONFIG.apiKey;

  // E→A
  elements.enableAutoSync.checked = DEFAULT_CONFIG.enableAutoSync;
  elements.e2aSyncInterval.value = DEFAULT_CONFIG.syncInterval;
  elements.e2aSyncIntervalValue.textContent = formatSyncInterval(DEFAULT_CONFIG.syncInterval);
  setRadioValue('edgeFolderMode', DEFAULT_CONFIG.edgeFolderMode);
  setRadioValue('appFolderMode', DEFAULT_CONFIG.appFolderMode);
  elements.edgeFolderWrap.style.display = 'none';
  elements.appFolderWrap.style.display = 'none';
  elements.e2aSyncMode.value = DEFAULT_CONFIG.syncMode;

  // A→E
  elements.enableAppToEdgeSync.checked = DEFAULT_CONFIG.enableAppToEdgeSync;
  elements.a2eSyncInterval.value = DEFAULT_CONFIG.appToEdgeSyncInterval;
  elements.a2eSyncIntervalValue.textContent = formatSyncInterval(DEFAULT_CONFIG.appToEdgeSyncInterval);
  setRadioValue('appToEdgeSourceFolderMode', DEFAULT_CONFIG.appToEdgeSourceFolderMode);
  elements.a2eSourceFolderWrap.style.display = 'none';
  elements.a2eSyncMode.value = DEFAULT_CONFIG.appToEdgeSyncMode;
}

// ---- 绑定事件 ----

function bindEvents() {
  // E→A 同步频率滑块
  elements.e2aSyncInterval.addEventListener('input', function() {
    elements.e2aSyncIntervalValue.textContent = formatSyncInterval(this.value);
  });

  // A→E 同步频率滑块
  elements.a2eSyncInterval.addEventListener('input', function() {
    elements.a2eSyncIntervalValue.textContent = formatSyncInterval(this.value);
  });

  // E→A：浏览器来源目录 radio
  document.querySelectorAll('input[name="edgeFolderMode"]').forEach(r => {
    r.addEventListener('change', updateEdgeFolderVisibility);
  });

  // E→A：应用目标目录 radio
  document.querySelectorAll('input[name="appFolderMode"]').forEach(r => {
    r.addEventListener('change', updateAppFolderVisibility);
  });

  // E→A：文件夹刷新按钮
  document.getElementById('refreshEdgeFolderBtn').addEventListener('click', () => loadEdgeFolders());
  document.getElementById('refreshAppFolderBtn').addEventListener('click', () => loadAppFolders());

  // A→E：来源目录 radio
  document.querySelectorAll('input[name="appToEdgeSourceFolderMode"]').forEach(r => {
    r.addEventListener('change', updateA2ESourceVisibility);
  });

  // A→E：目录刷新按钮
  document.getElementById('refreshA2ESourceBtn').addEventListener('click', () => loadA2ESourceFolders());
  document.getElementById('refreshA2ETargetBtn').addEventListener('click', () => loadA2ETargetFolders());

  // 打开同步窗口
  document.getElementById('openSyncWindowBtn').addEventListener('click', () => {
    chrome.tabs.create({ url: chrome.runtime.getURL('sync-window.html') });
  });

  // 保存 / 重置
  elements.saveButton.addEventListener('click', async () => { await saveConfig(); });
  elements.resetButton.addEventListener('click', async () => {
    loadDefaultConfig();
    await saveConfig();
  });
}

// ---- 保存配置 ----

async function saveConfig() {
  try {
    const edgeFolderMode = getRadioValue('edgeFolderMode');
    const appFolderMode = getRadioValue('appFolderMode');
    const srcMode = getRadioValue('appToEdgeSourceFolderMode');

    const config = {
      serverUrl: elements.serverUrl.value.trim(),
      apiKey: elements.apiKey.value.trim(),

      // E→A
      enableAutoSync: elements.enableAutoSync.checked,
      syncInterval: parseInt(elements.e2aSyncInterval.value),
      edgeFolderMode,
      edgeFolderId: edgeFolderMode === 'select' ? (elements.edgeFolder.value || null) : null,
      edgeFolderName: edgeFolderMode === 'select' ? getSelectedFolderName('edgeFolder') : '',
      appFolderMode,
      appFolderId: appFolderMode === 'select' ? (elements.appFolder.value || null) : null,
      appFolderName: appFolderMode === 'select' ? getSelectedFolderName('appFolder') : '',
      syncMode: elements.e2aSyncMode.value,

      // A→E
      enableAppToEdgeSync: elements.enableAppToEdgeSync.checked,
      appToEdgeSyncInterval: parseInt(elements.a2eSyncInterval.value),
      appToEdgeSourceFolderMode: srcMode,
      appToEdgeSourceFolderId: srcMode === 'select' ? (elements.appToEdgeSourceFolder.value || null) : null,
      appToEdgeSourceFolderName: srcMode === 'select' ? getSelectedFolderName('appToEdgeSourceFolder') : '',
      appToEdgeTargetFolderMode: 'select',
      appToEdgeTargetFolderId: elements.appToEdgeTargetFolder.value || null,
      appToEdgeTargetFolderName: getSelectedFolderName('appToEdgeTargetFolder'),
      appToEdgeSyncMode: elements.a2eSyncMode.value
    };

    if (!validateConfig(config)) return;

    const response = await chrome.runtime.sendMessage({ action: 'updateConfig', config });
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

// ---- 验证配置 ----

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
    showStatus('error', '浏览器→应用同步间隔必须在 1-1440 分钟之间');
    return false;
  }
  if (isNaN(config.appToEdgeSyncInterval) || config.appToEdgeSyncInterval < 1 || config.appToEdgeSyncInterval > 1440) {
    showStatus('error', '应用→浏览器同步间隔必须在 1-1440 分钟之间');
    return false;
  }
  if (config.edgeFolderMode === 'select' && !config.edgeFolderId) {
    showStatus('error', '请选择浏览器来源目录');
    return false;
  }
  if (config.appFolderMode === 'select' && !config.appFolderId) {
    showStatus('error', '请选择应用目标目录（浏览器→应用）');
    return false;
  }
  if (config.enableAppToEdgeSync && !config.appToEdgeTargetFolderId) {
    showStatus('error', '开启应用→浏览器同步时，必须选择浏览器目标目录');
    return false;
  }
  if (config.enableAppToEdgeSync && config.appToEdgeSourceFolderMode === 'select' && !config.appToEdgeSourceFolderId) {
    showStatus('error', '请选择应用来源目录（应用→浏览器）');
    return false;
  }
  return true;
}

// ---- 启动 ----
document.addEventListener('DOMContentLoaded', initialize);
