const elements = {
  deviceList: document.getElementById('deviceList'),
  addDeviceBtn: document.getElementById('addDeviceBtn'),
  openSyncWindowBtn: document.getElementById('openSyncWindowBtn'),
  statusMessage: document.getElementById('statusMessage'),
  deviceModal: document.getElementById('deviceModal'),
  modalTitle: document.getElementById('modalTitle'),
  modalSaveBtn: document.getElementById('modalSaveBtn'),
  modalCancelBtn: document.getElementById('modalCancelBtn'),
  deviceName: document.getElementById('deviceName'),
  deviceServerUrl: document.getElementById('deviceServerUrl'),
  deviceApiKey: document.getElementById('deviceApiKey'),
  deviceEnabled: document.getElementById('deviceEnabled'),
  e2aEnableAutoSync: document.getElementById('e2aEnableAutoSync'),
  e2aSyncInterval: document.getElementById('e2aSyncInterval'),
  e2aSyncIntervalValue: document.getElementById('e2aSyncIntervalValue'),
  e2aEdgeFolder: document.getElementById('e2aEdgeFolder'),
  e2aEdgeFolderWrap: document.getElementById('e2aEdgeFolderWrap'),
  e2aAppFolder: document.getElementById('e2aAppFolder'),
  e2aAppFolderWrap: document.getElementById('e2aAppFolderWrap'),
  e2aSyncMode: document.getElementById('e2aSyncMode'),
  a2eEnableAutoSync: document.getElementById('a2eEnableAutoSync'),
  a2eSyncInterval: document.getElementById('a2eSyncInterval'),
  a2eSyncIntervalValue: document.getElementById('a2eSyncIntervalValue'),
  a2eSourceFolder: document.getElementById('a2eSourceFolder'),
  a2eSourceFolderWrap: document.getElementById('a2eSourceFolderWrap'),
  a2eTargetFolder: document.getElementById('a2eTargetFolder'),
  a2eSyncMode: document.getElementById('a2eSyncMode')
};

let currentConfig = null;
let editingDeviceId = null;

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

function maskApiKey(key) {
  if (!key) return '';
  if (key.length <= 8) return '****';
  return key.substring(0, 4) + '****' + key.substring(key.length - 4);
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function renderDeviceList(devices) {
  elements.deviceList.innerHTML = '';
  devices.forEach(device => {
    const e2a = device.e2a || {};
    const a2e = device.a2e || {};
    const e2aText = e2a.enableAutoSync ? `E→A:自动(${formatSyncInterval(e2a.syncInterval || 5)})` : 'E→A:手动';
    const a2eText = a2e.enableAutoSync ? `A→E:自动(${formatSyncInterval(a2e.syncInterval || 5)})` : 'A→E:手动';

    const card = document.createElement('div');
    card.className = 'device-card' + (!device.enabled ? ' disabled-device' : '');
    card.innerHTML = `
      <div class="device-info">
        <div class="device-name-row">
          <span class="device-name">${escapeHtml(device.name)}</span>
          ${!device.enabled ? '<span class="device-badge disabled-badge">已禁用</span>' : ''}
          ${e2a.enableAutoSync ? '<span class="device-badge e2a-badge">E→A自动</span>' : ''}
          ${a2e.enableAutoSync ? '<span class="device-badge a2e-badge">A→E自动</span>' : ''}
        </div>
        <div class="device-detail">${escapeHtml(device.serverUrl)} · API Key: ${maskApiKey(device.apiKey)}</div>
        <div class="device-sync-summary">${e2aText} · ${a2eText}</div>
      </div>
      <div class="device-actions">
        <button class="btn-edit" data-id="${device.id}">编辑</button>
        <button class="btn-delete" data-id="${device.id}">删除</button>
      </div>
    `;
    elements.deviceList.appendChild(card);
  });

  elements.deviceList.querySelectorAll('.btn-edit').forEach(btn => {
    btn.addEventListener('click', () => openEditDeviceModal(btn.dataset.id));
  });

  elements.deviceList.querySelectorAll('.btn-delete').forEach(btn => {
    btn.addEventListener('click', async () => {
      if (currentConfig.devices.length <= 1) {
        showStatus('error', '至少保留一个设备');
        return;
      }
      if (confirm('确定删除此设备？')) {
        await chrome.runtime.sendMessage({ action: 'removeDevice', deviceId: btn.dataset.id });
        await loadConfig();
      }
    });
  });
}

function openAddDeviceModal() {
  editingDeviceId = null;
  elements.modalTitle.textContent = '添加设备';
  elements.deviceName.value = '';
  elements.deviceServerUrl.value = '';
  elements.deviceApiKey.value = '';
  elements.deviceEnabled.checked = true;
  resetModalSyncFields();
  elements.deviceModal.classList.add('visible');
}

function resetModalSyncFields() {
  elements.e2aEnableAutoSync.checked = false;
  elements.e2aSyncInterval.value = 5;
  elements.e2aSyncIntervalValue.textContent = '5分钟';
  setRadioValue('e2aEdgeFolderMode', 'all');
  elements.e2aEdgeFolderWrap.style.display = 'none';
  elements.e2aEdgeFolder.innerHTML = '<option value="">选择浏览器目录...</option>';
  setRadioValue('e2aAppFolderMode', 'all');
  elements.e2aAppFolderWrap.style.display = 'none';
  elements.e2aAppFolder.innerHTML = '<option value="">选择应用目录...</option>';
  elements.e2aSyncMode.value = 'merge';

  elements.a2eEnableAutoSync.checked = false;
  elements.a2eSyncInterval.value = 5;
  elements.a2eSyncIntervalValue.textContent = '5分钟';
  setRadioValue('a2eSourceFolderMode', 'all');
  elements.a2eSourceFolderWrap.style.display = 'none';
  elements.a2eSourceFolder.innerHTML = '<option value="">选择应用目录...</option>';
  elements.a2eTargetFolder.innerHTML = '<option value="">选择浏览器目录...</option>';
  elements.a2eSyncMode.value = 'merge';
}

function openEditDeviceModal(deviceId) {
  const device = currentConfig.devices.find(d => d.id === deviceId);
  if (!device) return;
  editingDeviceId = deviceId;
  elements.modalTitle.textContent = '编辑设备';
  elements.deviceName.value = device.name;
  elements.deviceServerUrl.value = device.serverUrl;
  elements.deviceApiKey.value = device.apiKey;
  elements.deviceEnabled.checked = device.enabled;

  const e2a = device.e2a || {};
  elements.e2aEnableAutoSync.checked = !!e2a.enableAutoSync;
  elements.e2aSyncInterval.value = e2a.syncInterval || 5;
  elements.e2aSyncIntervalValue.textContent = formatSyncInterval(e2a.syncInterval || 5);
  setRadioValue('e2aEdgeFolderMode', e2a.edgeFolderMode || 'all');
  elements.e2aEdgeFolderWrap.style.display = (e2a.edgeFolderMode || 'all') === 'select' ? 'flex' : 'none';
  setRadioValue('e2aAppFolderMode', e2a.appFolderMode || 'all');
  elements.e2aAppFolderWrap.style.display = (e2a.appFolderMode || 'all') === 'select' ? 'flex' : 'none';
  elements.e2aSyncMode.value = e2a.syncMode || 'merge';

  const a2e = device.a2e || {};
  elements.a2eEnableAutoSync.checked = !!a2e.enableAutoSync;
  elements.a2eSyncInterval.value = a2e.syncInterval || 5;
  elements.a2eSyncIntervalValue.textContent = formatSyncInterval(a2e.syncInterval || 5);
  setRadioValue('a2eSourceFolderMode', a2e.sourceFolderMode || 'all');
  elements.a2eSourceFolderWrap.style.display = (a2e.sourceFolderMode || 'all') === 'select' ? 'flex' : 'none';
  elements.a2eSyncMode.value = a2e.syncMode || 'merge';

  elements.deviceModal.classList.add('visible');

  if (e2a.edgeFolderMode === 'select') loadEdgeFolders('e2aEdgeFolder', e2a.edgeFolderId);
  if (e2a.appFolderMode === 'select') loadAppFolders('e2aAppFolder', deviceId, e2a.appFolderId);
  if (a2e.sourceFolderMode === 'select') loadAppFolders('a2eSourceFolder', deviceId, a2e.sourceFolderId);
  loadEdgeFolders('a2eTargetFolder', a2e.targetFolderId);
}

function closeDeviceModal() {
  elements.deviceModal.classList.remove('visible');
  editingDeviceId = null;
}

async function saveDeviceForm() {
  const name = elements.deviceName.value.trim();
  let serverUrl = elements.deviceServerUrl.value.trim();
  const apiKey = elements.deviceApiKey.value.trim();
  const enabled = elements.deviceEnabled.checked;

  serverUrl = serverUrl.replace(/\/+$/, '');

  if (!name) { showStatus('error', '请输入设备名称'); return; }
  if (!serverUrl) { showStatus('error', '请输入服务器地址'); return; }
  try { new URL(serverUrl); } catch { showStatus('error', '服务器地址格式错误'); return; }
  if (!apiKey) { showStatus('error', '请输入 API Key'); return; }

  const e2aEdgeFolderMode = getRadioValue('e2aEdgeFolderMode');
  const e2aAppFolderMode = getRadioValue('e2aAppFolderMode');
  const a2eSourceFolderMode = getRadioValue('a2eSourceFolderMode');

  if (e2aEdgeFolderMode === 'select' && !elements.e2aEdgeFolder.value) {
    showStatus('error', '请选择浏览器来源目录'); return;
  }
  if (e2aAppFolderMode === 'select' && !elements.e2aAppFolder.value) {
    showStatus('error', '请选择应用目标目录'); return;
  }
  if (elements.a2eEnableAutoSync.checked && !elements.a2eTargetFolder.value) {
    showStatus('error', '启用 A→E 同步时，必须选择浏览器目标目录'); return;
  }
  if (a2eSourceFolderMode === 'select' && !elements.a2eSourceFolder.value) {
    showStatus('error', '请选择应用来源目录'); return;
  }

  const e2a = {
    enableAutoSync: elements.e2aEnableAutoSync.checked,
    syncInterval: parseInt(elements.e2aSyncInterval.value),
    syncMode: elements.e2aSyncMode.value,
    edgeFolderMode: e2aEdgeFolderMode,
    edgeFolderId: e2aEdgeFolderMode === 'select' ? (elements.e2aEdgeFolder.value || null) : null,
    edgeFolderName: e2aEdgeFolderMode === 'select' ? getSelectedFolderName('e2aEdgeFolder') : '',
    appFolderMode: e2aAppFolderMode,
    appFolderId: e2aAppFolderMode === 'select' ? (elements.e2aAppFolder.value || null) : null,
    appFolderName: e2aAppFolderMode === 'select' ? getSelectedFolderName('e2aAppFolder') : ''
  };

  const a2e = {
    enableAutoSync: elements.a2eEnableAutoSync.checked,
    syncInterval: parseInt(elements.a2eSyncInterval.value),
    syncMode: elements.a2eSyncMode.value,
    sourceFolderMode: a2eSourceFolderMode,
    sourceFolderId: a2eSourceFolderMode === 'select' ? (elements.a2eSourceFolder.value || null) : null,
    sourceFolderName: a2eSourceFolderMode === 'select' ? getSelectedFolderName('a2eSourceFolder') : '',
    targetFolderMode: 'select',
    targetFolderId: elements.a2eTargetFolder.value || null,
    targetFolderName: getSelectedFolderName('a2eTargetFolder')
  };

  const updates = { name, serverUrl, apiKey, enabled, e2a, a2e };

  if (editingDeviceId) {
    await chrome.runtime.sendMessage({ action: 'updateDevice', deviceId: editingDeviceId, updates });
  } else {
    await chrome.runtime.sendMessage({
      action: 'addDevice',
      device: { id: 'device_' + Date.now(), ...updates }
    });
  }

  closeDeviceModal();
  await loadConfig();
}

async function loadEdgeFolders(selectId, savedValue) {
  try {
    const resp = await chrome.runtime.sendMessage({ action: 'getEdgeFolders' });
    if (resp && resp.folders) {
      const select = document.getElementById(selectId);
      const prev = savedValue || select.value;
      select.innerHTML = '<option value="">选择浏览器目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.path || f.title;
        select.appendChild(opt);
      });
      if (prev) select.value = prev;
    }
  } catch (e) { console.error('加载浏览器目录失败:', e); }
}

async function loadAppFolders(selectId, deviceId, savedValue) {
  try {
    // 使用当前表单中的 serverUrl 和 apiKey，实现编辑时实时加载目录
    const serverUrl = elements.deviceServerUrl.value.trim();
    const apiKey = elements.deviceApiKey.value.trim();
    const resp = await chrome.runtime.sendMessage({
      action: 'getAppFolders',
      deviceId,
      serverUrl,
      apiKey
    });
    if (resp && resp.folders) {
      const select = document.getElementById(selectId);
      const prev = savedValue || select.value;
      select.innerHTML = '<option value="">选择应用目录...</option>';
      resp.folders.forEach(f => {
        const opt = document.createElement('option');
        opt.value = f.id;
        opt.textContent = f.title;
        select.appendChild(opt);
      });
      if (prev) select.value = prev;
    }
  } catch (e) { console.error('加载应用目录失败:', e); }
}

function updateE2aEdgeFolderVisibility() {
  const mode = getRadioValue('e2aEdgeFolderMode');
  const wrap = elements.e2aEdgeFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) loadEdgeFolders('e2aEdgeFolder');
}

function updateE2aAppFolderVisibility() {
  const mode = getRadioValue('e2aAppFolderMode');
  const wrap = elements.e2aAppFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) {
    loadAppFolders('e2aAppFolder', editingDeviceId);
  }
}

function updateA2eSourceFolderVisibility() {
  const mode = getRadioValue('a2eSourceFolderMode');
  const wrap = elements.a2eSourceFolderWrap;
  const wasHidden = wrap.style.display === 'none';
  wrap.style.display = mode === 'select' ? 'flex' : 'none';
  if (mode === 'select' && wasHidden) {
    loadAppFolders('a2eSourceFolder', editingDeviceId);
  }
}

function reloadVisibleFolderLists() {
  // 重新加载当前可见的应用目录列表
  if (elements.e2aAppFolderWrap.style.display !== 'none') {
    loadAppFolders('e2aAppFolder', editingDeviceId);
  }
  if (elements.a2eSourceFolderWrap.style.display !== 'none') {
    loadAppFolders('a2eSourceFolder', editingDeviceId);
  }
}

async function initialize() {
  await loadConfig();
  bindEvents();
}

async function loadConfig() {
  try {
    const response = await chrome.runtime.sendMessage({ action: 'getConfig' });
    if (response && response.config) {
      currentConfig = response.config;
      renderDeviceList(currentConfig.devices || []);
    }
  } catch (error) {
    console.error('加载配置失败:', error);
    showStatus('error', '加载配置失败');
  }
}

function bindEvents() {
  elements.e2aSyncInterval.addEventListener('input', function() {
    elements.e2aSyncIntervalValue.textContent = formatSyncInterval(this.value);
  });
  elements.a2eSyncInterval.addEventListener('input', function() {
    elements.a2eSyncIntervalValue.textContent = formatSyncInterval(this.value);
  });

  // 当 serverUrl 或 apiKey 变化时，自动重新加载已显示的目录列表
  elements.deviceServerUrl.addEventListener('change', reloadVisibleFolderLists);
  elements.deviceApiKey.addEventListener('change', reloadVisibleFolderLists);

  document.querySelectorAll('input[name="e2aEdgeFolderMode"]').forEach(r => {
    r.addEventListener('change', updateE2aEdgeFolderVisibility);
  });
  document.querySelectorAll('input[name="e2aAppFolderMode"]').forEach(r => {
    r.addEventListener('change', updateE2aAppFolderVisibility);
  });
  document.querySelectorAll('input[name="a2eSourceFolderMode"]').forEach(r => {
    r.addEventListener('change', updateA2eSourceFolderVisibility);
  });

  document.getElementById('refreshE2aEdgeBtn').addEventListener('click', () => loadEdgeFolders('e2aEdgeFolder'));
  document.getElementById('refreshE2aAppBtn').addEventListener('click', () => {
    loadAppFolders('e2aAppFolder', editingDeviceId);
  });
  document.getElementById('refreshA2eSourceBtn').addEventListener('click', () => {
    loadAppFolders('a2eSourceFolder', editingDeviceId);
  });
  document.getElementById('refreshA2eTargetBtn').addEventListener('click', () => loadEdgeFolders('a2eTargetFolder'));

  elements.addDeviceBtn.addEventListener('click', openAddDeviceModal);
  elements.modalCancelBtn.addEventListener('click', closeDeviceModal);
  elements.modalSaveBtn.addEventListener('click', saveDeviceForm);

  let mouseDownTarget = null;
  elements.deviceModal.addEventListener('mousedown', (e) => {
    mouseDownTarget = e.target;
  });
  elements.deviceModal.addEventListener('mouseup', (e) => {
    if (mouseDownTarget === elements.deviceModal && e.target === elements.deviceModal) {
      closeDeviceModal();
    }
    mouseDownTarget = null;
  });

  elements.openSyncWindowBtn.addEventListener('click', () => {
    chrome.tabs.create({ url: chrome.runtime.getURL('sync-window.html') });
  });
}

document.addEventListener('DOMContentLoaded', initialize);
