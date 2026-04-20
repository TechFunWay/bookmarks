const elements = {
  syncStatus: document.getElementById('syncStatus'),
  statusMessage: document.getElementById('statusMessage'),
  serverUrl: document.getElementById('serverUrl'),
  currentDeviceName: document.getElementById('currentDeviceName'),
  e2aSyncStatusText: document.getElementById('e2aSyncStatusText'),
  a2eSyncStatusText: document.getElementById('a2eSyncStatusText'),
  syncButton: document.getElementById('syncButton'),
  syncFromAppButton: document.getElementById('syncFromAppButton'),
  syncAllButton: document.getElementById('syncAllButton'),
  configButton: document.getElementById('configButton'),
  lastSync: document.getElementById('lastSync'),
  e2aDirectionTitle: document.getElementById('e2aDirectionTitle'),
  dirEdge: document.getElementById('dirEdge'),
  dirApp: document.getElementById('dirApp'),
  dirSyncMode: document.getElementById('dirSyncMode'),
  a2eDirectionSection: document.getElementById('a2eDirectionSection'),
  a2eDirectionTitle: document.getElementById('a2eDirectionTitle'),
  dirA2ESource: document.getElementById('dirA2ESource'),
  dirA2ETarget: document.getElementById('dirA2ETarget'),
  dirA2ESyncMode: document.getElementById('dirA2ESyncMode'),
  progressSection: document.getElementById('progressSection'),
  progressBar: document.getElementById('progressBar'),
  progressStep: document.getElementById('progressStep'),
  progressCount: document.getElementById('progressCount'),
  deviceSelect: document.getElementById('deviceSelect')
};

let currentConfig = null;

async function initialize() {
  await loadConfig();
  await loadSyncStatus();
  bindEvents();
}

document.addEventListener('DOMContentLoaded', initialize);

function populateDeviceSelect(config) {
  const devices = config.devices || [];
  const savedId = elements.deviceSelect.value;
  elements.deviceSelect.innerHTML = '';
  devices.forEach(d => {
    const opt = document.createElement('option');
    opt.value = d.id;
    opt.textContent = d.name + (d.enabled ? '' : ' (已禁用)');
    elements.deviceSelect.appendChild(opt);
  });
  if (savedId && devices.find(d => d.id === savedId)) {
    elements.deviceSelect.value = savedId;
  } else if (devices.length > 0) {
    elements.deviceSelect.value = devices[0].id;
  }
}

function getSelectedDevice(config) {
  const deviceId = elements.deviceSelect.value;
  return (config.devices || []).find(d => d.id === deviceId);
}

async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      currentConfig = response.config;
      populateDeviceSelect(currentConfig);
      updateDeviceDisplay();
      if (currentConfig.lastSyncTime) {
        elements.lastSync.textContent = `最后同步: ${formatTime(currentConfig.lastSyncTime)}`;
      }
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

function updateDeviceDisplay() {
  const device = getSelectedDevice(currentConfig);
  if (!device) return;

  elements.serverUrl.textContent = device.serverUrl || '未设置';
  elements.currentDeviceName.textContent = device.name || '未设置';

  const e2a = device.e2a || {};
  const a2e = device.a2e || {};

  const edgePart = e2a.edgeFolderMode === 'select' ? (e2a.edgeFolderName || '未选择') : '全部书签';
  const appPart = e2a.appFolderMode === 'select' ? (e2a.appFolderName || '未选择') : '根目录';
  elements.dirEdge.textContent = edgePart;
  elements.dirApp.textContent = appPart;
  elements.dirSyncMode.textContent = getSyncModeText(e2a.syncMode);

  const e2aAutoText = e2a.enableAutoSync ? `自动 · ${formatSyncInterval(e2a.syncInterval || 5)}` : '手动';
  elements.e2aDirectionTitle.textContent = `浏览器 → 应用（${e2aAutoText}）`;
  elements.e2aSyncStatusText.textContent = e2a.enableAutoSync
    ? `自动 · ${formatSyncInterval(e2a.syncInterval || 5)} · ${getSyncModeText(e2a.syncMode)}`
    : `手动 · ${getSyncModeText(e2a.syncMode)}`;

  if (a2e.targetFolderId) {
    const srcPart = a2e.sourceFolderMode === 'select' ? (a2e.sourceFolderName || '未选择') : '全部书签';
    const tgtPart = a2e.targetFolderName || '未选择';
    elements.dirA2ESource.textContent = srcPart;
    elements.dirA2ETarget.textContent = tgtPart;
    elements.dirA2ESyncMode.textContent = getSyncModeText(a2e.syncMode);

    const a2eAutoText = a2e.enableAutoSync ? `自动 · ${formatSyncInterval(a2e.syncInterval || 5)}` : '手动';
    elements.a2eDirectionTitle.textContent = `应用 → 浏览器（${a2eAutoText}）`;
    elements.a2eSyncStatusText.textContent = a2e.enableAutoSync
      ? `自动 · ${formatSyncInterval(a2e.syncInterval || 5)} · ${getSyncModeText(a2e.syncMode)}`
      : `手动 · ${getSyncModeText(a2e.syncMode)}`;
  } else {
    elements.dirA2ESource.textContent = '全部书签';
    elements.dirA2ETarget.textContent = '未配置';
    elements.dirA2ESyncMode.textContent = '手动';
    elements.a2eSyncStatusText.textContent = '未配置';
  }
}

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
          showStatus('error', '同步超时，请重试');
          elements.progressStep.textContent = '同步超时';
          resetSyncUI();
          stopStatusCheck();
          return;
        }
        showStatus('syncing', '正在同步中...');
        if (response.syncProgress) updateProgress(response.syncProgress);
        disableAllButtons();
      } else {
        if (statusCheckCount > 0) {
          showStatus('success', '同步完成');
          await loadConfig();
        }
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

function resetSyncUI() {
  elements.syncButton.disabled = false;
  elements.syncButton.textContent = '同步（E→A）';
  elements.syncFromAppButton.disabled = false;
  elements.syncFromAppButton.textContent = '同步（A→E）';
  elements.syncAllButton.disabled = false;
  statusCheckCount = 0;
}

function showProgress() {
  elements.progressSection.classList.add('visible');
  elements.progressBar.style.width = '0%';
  elements.progressStep.textContent = '准备同步...';
  elements.progressCount.textContent = '0 / 0';
}

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
  let stepText = progress.currentStep || '准备同步...';
  if (progress.deviceName) stepText = `[${progress.deviceName}] ${stepText}`;
  elements.progressStep.textContent = stepText;
}

function startStatusCheck() {
  if (statusCheckInterval) clearInterval(statusCheckInterval);
  statusCheckCount = 0;
  statusCheckInterval = setInterval(async () => { await loadSyncStatus(); }, 500);
}

function stopStatusCheck() {
  if (statusCheckInterval) {
    clearInterval(statusCheckInterval);
    statusCheckInterval = null;
  }
}

function disableAllButtons() {
  elements.syncButton.disabled = true;
  elements.syncFromAppButton.disabled = true;
  elements.syncAllButton.disabled = true;
}

function showStatus(type, message) {
  elements.syncStatus.className = 'sync-status';
  elements.syncStatus.classList.add(type);
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
}

function formatTime(timeString) {
  try {
    const date = new Date(timeString);
    return date.toLocaleString('zh-CN', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit'
    });
  } catch { return timeString; }
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
  const h = (minutes % 1440) / 60;
  return h === 0 ? `${d}天` : `${d}天${Math.floor(h)}小时`;
}

function getSyncModeText(mode) {
  switch (mode) {
    case 'merge': return '合并同步';
    case 'replace': return '替换同步';
    default: return '未知';
  }
}

function bindEvents() {
  elements.syncButton.addEventListener('click', async () => {
    if (elements.syncButton.disabled) return;
    const deviceId = elements.deviceSelect.value;
    if (!deviceId) { showStatus('error', '请选择设备'); return; }
    showProgress();
    disableAllButtons();
    elements.syncButton.textContent = '同步中...';
    showStatus('syncing', '正在同步（浏览器→应用）...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'sync', deviceId });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        elements.progressStep.textContent = '同步失败';
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      elements.progressStep.textContent = '同步失败';
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.syncFromAppButton.addEventListener('click', async () => {
    if (elements.syncFromAppButton.disabled) return;
    const deviceId = elements.deviceSelect.value;
    if (!deviceId) { showStatus('error', '请选择设备'); return; }
    showProgress();
    disableAllButtons();
    elements.syncFromAppButton.textContent = '同步中...';
    showStatus('syncing', '正在同步（应用→浏览器）...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'syncFromApp', deviceId });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        elements.progressStep.textContent = '同步失败';
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      elements.progressStep.textContent = '同步失败';
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.syncAllButton.addEventListener('click', async () => {
    if (elements.syncAllButton.disabled) return;
    showProgress();
    disableAllButtons();
    showStatus('syncing', '正在同步所有设备...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'syncAllDevices' });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        elements.progressStep.textContent = '同步失败';
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      elements.progressStep.textContent = '同步失败';
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.deviceSelect.addEventListener('change', () => {
    if (currentConfig) updateDeviceDisplay();
  });

  elements.configButton.addEventListener('click', () => {
    chrome.tabs.create({ url: chrome.runtime.getURL('options.html'), active: true });
  });
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  switch (message.action) {
    case 'configUpdated':
      loadConfig();
      break;
    case 'syncProgress':
      if (message.progress) updateProgress(message.progress);
      break;
    case 'syncComplete':
      if (message.progress) updateProgress(message.progress);
      elements.progressBar.style.width = '100%';
      const completeDevice = message.deviceName ? `[${message.deviceName}] ` : '';
      elements.progressStep.textContent = `${completeDevice}同步完成`;
      showStatus('success', '同步完成');
      loadConfig().then(() => { resetSyncUI(); stopStatusCheck(); });
      break;
    case 'syncError':
      const errorDevice = message.deviceName ? `[${message.deviceName}] ` : '';
      showStatus('error', `${errorDevice}同步失败: ${message.error}`);
      elements.progressStep.textContent = `${errorDevice}同步失败`;
      resetSyncUI();
      stopStatusCheck();
      break;
    default:
      break;
  }
});
