const elements = {
  syncStatus: document.getElementById('syncStatus'),
  statusMessage: document.getElementById('statusMessage'),
  serverUrl: document.getElementById('serverUrl'),
  e2aStatus: document.getElementById('e2aStatus'),
  a2eStatus: document.getElementById('a2eStatus'),
  syncButton: document.getElementById('syncButton'),
  syncFromAppButton: document.getElementById('syncFromAppButton'),
  syncAllButton: document.getElementById('syncAllButton'),
  lastSync: document.getElementById('lastSync'),
  syncProgressContainer: document.getElementById('syncProgressContainer'),
  progressBar: document.getElementById('progressBar'),
  progressText: document.getElementById('progressText'),
  progressDetail: document.getElementById('progressDetail'),
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
  if (device) {
    elements.serverUrl.textContent = device.serverUrl || '未设置';
    const e2a = device.e2a || {};
    const a2e = device.a2e || {};
    elements.e2aStatus.textContent = e2a.enableAutoSync
      ? `自动 · ${formatSyncInterval(e2a.syncInterval || 5)} · ${getSyncModeText(e2a.syncMode)}`
      : `手动 · ${getSyncModeText(e2a.syncMode)}`;
    elements.a2eStatus.textContent = a2e.enableAutoSync
      ? `自动 · ${formatSyncInterval(a2e.syncInterval || 5)} · ${getSyncModeText(a2e.syncMode)}`
      : `手动 · ${getSyncModeText(a2e.syncMode)}`;
  } else {
    elements.serverUrl.textContent = '未设置';
    elements.e2aStatus.textContent = '未设置';
    elements.a2eStatus.textContent = '未设置';
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
          hideProgress();
          resetSyncUI();
          stopStatusCheck();
          return;
        }
        if (response.syncProgress) showProgress(response.syncProgress);
        showStatus('syncing', '正在同步中...');
        disableAllButtons();
      } else {
        hideProgress();
        if (statusCheckCount > 0) {
          showStatus('success', '同步完成');
          await loadConfig();
        } else {
          showStatus('success', '就绪');
        }
        resetSyncUI();
        stopStatusCheck();
      }
    }
  } catch (error) {
    console.error('加载同步状态失败:', error);
    showStatus('error', '获取状态失败');
    hideProgress();
    resetSyncUI();
    stopStatusCheck();
  }
}

function showProgress(progress) {
  if (!elements.syncProgressContainer) return;
  elements.syncProgressContainer.style.display = 'block';
  if (progress.total > 0) {
    const percent = Math.round((progress.processed / progress.total) * 100);
    elements.progressBar.style.width = `${percent}%`;
    elements.progressDetail.textContent = `${progress.processed} / ${progress.total}`;
  } else {
    elements.progressBar.style.width = '0%';
    elements.progressDetail.textContent = '计算中...';
  }
  let stepText = progress.currentStep || '准备同步...';
  if (progress.deviceName) stepText = `[${progress.deviceName}] ${stepText}`;
  elements.progressText.textContent = stepText;
}

function hideProgress() {
  if (elements.syncProgressContainer) {
    elements.syncProgressContainer.style.display = 'none';
  }
}

function disableAllButtons() {
  elements.syncButton.disabled = true;
  elements.syncFromAppButton.disabled = true;
  elements.syncAllButton.disabled = true;
  elements.syncButton.textContent = '同步中...';
}

function resetSyncUI() {
  elements.syncButton.disabled = false;
  elements.syncButton.textContent = '同步（浏览器→应用）';
  elements.syncFromAppButton.disabled = false;
  elements.syncFromAppButton.textContent = '同步（应用→浏览器）';
  elements.syncAllButton.disabled = false;
  statusCheckCount = 0;
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

function bindEvents() {
  elements.syncButton.addEventListener('click', async () => {
    if (elements.syncButton.disabled) return;
    const deviceId = elements.deviceSelect.value;
    if (!deviceId) { showStatus('error', '请选择设备'); return; }
    disableAllButtons();
    showStatus('syncing', '正在同步（浏览器→应用）...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'sync', deviceId });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        hideProgress();
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      hideProgress();
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.syncFromAppButton.addEventListener('click', async () => {
    if (elements.syncFromAppButton.disabled) return;
    const deviceId = elements.deviceSelect.value;
    if (!deviceId) { showStatus('error', '请选择设备'); return; }
    disableAllButtons();
    showStatus('syncing', '正在同步（应用→浏览器）...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'syncFromApp', deviceId });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        hideProgress();
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      hideProgress();
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.syncAllButton.addEventListener('click', async () => {
    if (elements.syncAllButton.disabled) return;
    disableAllButtons();
    showStatus('syncing', '正在同步所有设备...');
    startStatusCheck();
    try {
      const response = await chrome.runtime.sendMessage({ action: 'syncAllDevices' });
      if (response && response.status === 'error') {
        showStatus('error', `同步失败: ${response.error || '未知错误'}`);
        hideProgress();
        resetSyncUI();
        stopStatusCheck();
      }
    } catch (error) {
      showStatus('error', '同步失败');
      hideProgress();
      resetSyncUI();
      stopStatusCheck();
    }
  });

  elements.deviceSelect.addEventListener('change', () => {
    if (currentConfig) updateDeviceDisplay();
  });

  document.getElementById('openWindowLink').addEventListener('click', (e) => {
    e.preventDefault();
    chrome.runtime.sendMessage({ action: 'openSyncWindow' }, () => {
      if (chrome.runtime.lastError) console.error('打开窗口失败:', chrome.runtime.lastError);
    });
  });
}

function showStatus(type, message) {
  elements.syncStatus.className = 'sync-status';
  elements.syncStatus.classList.add(type);
  elements.statusMessage.textContent = message;
  elements.statusMessage.style.display = 'block';
  setTimeout(() => { elements.statusMessage.style.display = 'none'; }, 3000);
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
    case 'merge': return '合并';
    case 'replace': return '替换';
    default: return '未知';
  }
}
