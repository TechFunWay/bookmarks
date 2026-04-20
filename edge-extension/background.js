const DEFAULT_E2A = {
  enableAutoSync: false,
  syncInterval: 5,
  syncMode: 'merge',
  edgeFolderMode: 'all',
  edgeFolderId: null,
  edgeFolderName: '',
  appFolderMode: 'all',
  appFolderId: null,
  appFolderName: ''
};

const DEFAULT_A2E = {
  enableAutoSync: false,
  syncInterval: 5,
  syncMode: 'merge',
  sourceFolderMode: 'all',
  sourceFolderId: null,
  sourceFolderName: '',
  targetFolderMode: 'select',
  targetFolderId: null,
  targetFolderName: ''
};

const DEFAULT_DEVICE = {
  id: 'device_default',
  name: '默认设备',
  serverUrl: 'http://localhost:8901',
  apiKey: '',
  enabled: true,
  e2a: { ...DEFAULT_E2A },
  a2e: { ...DEFAULT_A2E }
};

const DEFAULT_CONFIG = {
  devices: [{ ...DEFAULT_DEVICE }],
  lastSyncTime: null,
  folderIdMap: {}
};

let config = { ...DEFAULT_CONFIG };
let deviceSyncStatus = {};
let syncProgress = {
  total: 0,
  processed: 0,
  currentStep: '',
  steps: ['获取Edge书签', '获取服务器数据', '对比差异', '执行同步', '完成']
};
let progressWindowId = null;

function normalizeUrl(url) {
  return url.replace(/\/+$/, '');
}

function getEnabledDevices() {
  if (!config.devices) return [];
  return config.devices.filter(d => d.enabled && d.apiKey);
}

function getDevicesWithE2aAutoSync() {
  return getEnabledDevices().filter(d => d.e2a && d.e2a.enableAutoSync);
}

function getDevicesWithA2eAutoSync() {
  return getEnabledDevices().filter(d => d.a2e && d.a2e.enableAutoSync);
}

function getDeviceSyncStatus(deviceId) {
  if (!deviceSyncStatus[deviceId]) {
    deviceSyncStatus[deviceId] = { inProgress: false, result: null, lastSyncTime: null, lastError: null };
  }
  return deviceSyncStatus[deviceId];
}

function isAnySyncInProgress() {
  return Object.values(deviceSyncStatus).some(s => s.inProgress);
}

function ensureDeviceDefaults(device) {
  if (!device.e2a) device.e2a = { ...DEFAULT_E2A };
  if (!device.a2e) device.a2e = { ...DEFAULT_A2E };
  device.e2a = { ...DEFAULT_E2A, ...device.e2a };
  device.a2e = { ...DEFAULT_A2E, ...device.a2e };
  return device;
}

function migrateConfig(storedConfig) {
  if (storedConfig.serverUrl && !storedConfig.devices) {
    const device = {
      id: 'device_migrated_' + Date.now(),
      name: '默认设备',
      serverUrl: storedConfig.serverUrl,
      apiKey: storedConfig.apiKey || '',
      enabled: true,
      e2a: {
        enableAutoSync: !!storedConfig.enableAutoSync,
        syncInterval: storedConfig.syncInterval || 5,
        syncMode: storedConfig.syncMode || 'merge',
        edgeFolderMode: storedConfig.edgeFolderMode || 'all',
        edgeFolderId: storedConfig.edgeFolderId || null,
        edgeFolderName: storedConfig.edgeFolderName || '',
        appFolderMode: storedConfig.appFolderMode || 'all',
        appFolderId: storedConfig.appFolderId || null,
        appFolderName: storedConfig.appFolderName || ''
      },
      a2e: {
        enableAutoSync: !!storedConfig.enableAppToEdgeSync,
        syncInterval: storedConfig.appToEdgeSyncInterval || 5,
        syncMode: storedConfig.appToEdgeSyncMode || 'merge',
        sourceFolderMode: storedConfig.appToEdgeSourceFolderMode || 'all',
        sourceFolderId: storedConfig.appToEdgeSourceFolderId || null,
        sourceFolderName: storedConfig.appToEdgeSourceFolderName || '',
        targetFolderMode: storedConfig.appToEdgeTargetFolderMode || 'select',
        targetFolderId: storedConfig.appToEdgeTargetFolderId || null,
        targetFolderName: storedConfig.appToEdgeTargetFolderName || ''
      }
    };
    storedConfig.devices = [device];
    delete storedConfig.serverUrl;
    delete storedConfig.apiKey;
    delete storedConfig.activeDeviceId;
    delete storedConfig.enableAutoSync;
    delete storedConfig.syncInterval;
    delete storedConfig.syncMode;
    delete storedConfig.edgeFolderMode;
    delete storedConfig.edgeFolderId;
    delete storedConfig.edgeFolderName;
    delete storedConfig.appFolderMode;
    delete storedConfig.appFolderId;
    delete storedConfig.appFolderName;
    delete storedConfig.enableAppToEdgeSync;
    delete storedConfig.appToEdgeSyncInterval;
    delete storedConfig.appToEdgeSyncMode;
    delete storedConfig.appToEdgeSourceFolderMode;
    delete storedConfig.appToEdgeSourceFolderId;
    delete storedConfig.appToEdgeSourceFolderName;
    delete storedConfig.appToEdgeTargetFolderMode;
    delete storedConfig.appToEdgeTargetFolderId;
    delete storedConfig.appToEdgeTargetFolderName;
  }

  if (storedConfig.devices && storedConfig.devices.length > 0) {
    const hasGlobalSyncFields = storedConfig.enableAutoSync !== undefined || storedConfig.syncMode !== undefined;
    storedConfig.devices.forEach(device => {
      if (!device.e2a || hasGlobalSyncFields) {
        device.e2a = {
          enableAutoSync: storedConfig.enableAutoSync !== undefined ? !!storedConfig.enableAutoSync : (device.e2a?.enableAutoSync || false),
          syncInterval: storedConfig.syncInterval || device.e2a?.syncInterval || 5,
          syncMode: storedConfig.syncMode || device.e2a?.syncMode || 'merge',
          edgeFolderMode: storedConfig.edgeFolderMode || device.e2a?.edgeFolderMode || 'all',
          edgeFolderId: storedConfig.edgeFolderId ?? device.e2a?.edgeFolderId ?? null,
          edgeFolderName: storedConfig.edgeFolderName || device.e2a?.edgeFolderName || '',
          appFolderMode: storedConfig.appFolderMode || device.e2a?.appFolderMode || 'all',
          appFolderId: storedConfig.appFolderId ?? device.e2a?.appFolderId ?? null,
          appFolderName: storedConfig.appFolderName || device.e2a?.appFolderName || ''
        };
      }
      if (!device.a2e || hasGlobalSyncFields) {
        device.a2e = {
          enableAutoSync: storedConfig.enableAppToEdgeSync !== undefined ? !!storedConfig.enableAppToEdgeSync : (device.a2e?.enableAutoSync || false),
          syncInterval: storedConfig.appToEdgeSyncInterval || device.a2e?.syncInterval || 5,
          syncMode: storedConfig.appToEdgeSyncMode || device.a2e?.syncMode || 'merge',
          sourceFolderMode: storedConfig.appToEdgeSourceFolderMode || device.a2e?.sourceFolderMode || 'all',
          sourceFolderId: storedConfig.appToEdgeSourceFolderId ?? device.a2e?.sourceFolderId ?? null,
          sourceFolderName: storedConfig.appToEdgeSourceFolderName || device.a2e?.sourceFolderName || '',
          targetFolderMode: storedConfig.appToEdgeTargetFolderMode || device.a2e?.targetFolderMode || 'select',
          targetFolderId: storedConfig.appToEdgeTargetFolderId ?? device.a2e?.targetFolderId ?? null,
          targetFolderName: storedConfig.appToEdgeTargetFolderName || device.a2e?.targetFolderName || ''
        };
      }
      ensureDeviceDefaults(device);
    });
    if (hasGlobalSyncFields) {
      delete storedConfig.activeDeviceId;
      delete storedConfig.enableAutoSync;
      delete storedConfig.syncInterval;
      delete storedConfig.syncMode;
      delete storedConfig.edgeFolderMode;
      delete storedConfig.edgeFolderId;
      delete storedConfig.edgeFolderName;
      delete storedConfig.appFolderMode;
      delete storedConfig.appFolderId;
      delete storedConfig.appFolderName;
      delete storedConfig.enableAppToEdgeSync;
      delete storedConfig.appToEdgeSyncInterval;
      delete storedConfig.appToEdgeSyncMode;
      delete storedConfig.appToEdgeSourceFolderMode;
      delete storedConfig.appToEdgeSourceFolderId;
      delete storedConfig.appToEdgeSourceFolderName;
      delete storedConfig.appToEdgeTargetFolderMode;
      delete storedConfig.appToEdgeTargetFolderId;
      delete storedConfig.appToEdgeTargetFolderName;
    }
  }

  if (!storedConfig.devices || storedConfig.devices.length === 0) {
    storedConfig.devices = [{ ...DEFAULT_DEVICE }];
  }

  delete storedConfig.activeDeviceId;

  return storedConfig;
}

async function initialize() {
  console.log('【网址收藏夹】同步助手初始化...');

  await loadConfig();

  registerSyncAlarms();

  registerBookmarkListeners();

  const e2aDevices = getDevicesWithE2aAutoSync();
  if (e2aDevices.length > 0) {
    console.log(`${e2aDevices.length} 个设备启用了 E→A 自动同步，执行首次同步`);
    syncToAllDevices();
  } else {
    console.log('没有设备启用 E→A 自动同步，跳过首次同步');
  }

  console.log('【网址收藏夹】同步助手初始化完成');
}

async function loadConfig() {
  try {
    const storedConfig = await chrome.storage.local.get('config');
    if (storedConfig.config) {
      const migrated = migrateConfig(storedConfig.config);
      config = { ...DEFAULT_CONFIG, ...migrated };
      config.devices = config.devices.map(d => ensureDeviceDefaults({ ...d }));
      await chrome.storage.local.set({ config });
      console.log('配置加载成功');
    } else {
      await chrome.storage.local.set({ config });
      console.log('使用默认配置');
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }
}

function registerSyncAlarms() {
  chrome.alarms.clearAll();

  config.devices.forEach(device => {
    if (!device.enabled || !device.apiKey) return;

    if (device.e2a && device.e2a.enableAutoSync) {
      const alarmName = `e2a_${device.id}`;
      chrome.alarms.create(alarmName, {
        periodInMinutes: device.e2a.syncInterval || 5
      });
      console.log(`设备 ${device.name} E→A 定时器已注册，间隔: ${device.e2a.syncInterval}分钟`);
    }

    if (device.a2e && device.a2e.enableAutoSync) {
      const alarmName = `a2e_${device.id}`;
      chrome.alarms.create(alarmName, {
        periodInMinutes: device.a2e.syncInterval || 5
      });
      console.log(`设备 ${device.name} A→E 定时器已注册，间隔: ${device.a2e.syncInterval}分钟`);
    }
  });

  chrome.alarms.onAlarm.removeListener(handleAlarm);
  chrome.alarms.onAlarm.addListener(handleAlarm);
}

function handleAlarm(alarm) {
  if (alarm.name.startsWith('e2a_')) {
    const deviceId = alarm.name.substring(4);
    const device = config.devices.find(d => d.id === deviceId);
    if (device && device.enabled && device.e2a && device.e2a.enableAutoSync) {
      console.log(`执行定时 E→A 同步: ${device.name}`);
      syncBookmarks(deviceId).catch(e => console.error(`定时 E→A 同步失败 [${device.name}]:`, e));
    }
  }
  if (alarm.name.startsWith('a2e_')) {
    const deviceId = alarm.name.substring(4);
    const device = config.devices.find(d => d.id === deviceId);
    if (device && device.enabled && device.a2e && device.a2e.enableAutoSync) {
      console.log(`执行定时 A→E 同步: ${device.name}`);
      syncFromApp(deviceId).catch(e => console.error(`定时 A→E 同步失败 [${device.name}]:`, e));
    }
  }
}

function registerBookmarkListeners() {
  chrome.bookmarks.onCreated.addListener((id, bookmark) => {
    console.log('书签创建:', id, bookmark);
    const devices = getDevicesWithE2aAutoSync();
    if (devices.length > 0) {
      setTimeout(() => { syncToAllDevices(); }, 1000);
    }
  });

  chrome.bookmarks.onRemoved.addListener((id, removeInfo) => {
    console.log('书签删除:', id, removeInfo);
    if (getDevicesWithE2aAutoSync().length > 0) {
      syncToAllDevices();
    }
  });

  chrome.bookmarks.onChanged.addListener((id, changeInfo) => {
    console.log('书签修改:', id, changeInfo);
    if (getDevicesWithE2aAutoSync().length > 0) {
      syncToAllDevices();
    }
  });

  chrome.bookmarks.onMoved.addListener((id, moveInfo) => {
    console.log('书签移动:', id, moveInfo);
    if (getDevicesWithE2aAutoSync().length > 0) {
      syncToAllDevices();
    }
  });

  chrome.bookmarks.onChildrenReordered.addListener((id, reorderInfo) => {
    console.log('书签重排序:', id, reorderInfo);
    if (getDevicesWithE2aAutoSync().length > 0) {
      syncToAllDevices();
    }
  });

  console.log('书签变更监听器已注册');
}

async function syncBookmarks(deviceId) {
  if (!deviceId) {
    console.error('syncBookmarks 需要指定 deviceId');
    return;
  }
  const device = config.devices.find(d => d.id === deviceId);
  if (!device) {
    console.error('设备不存在:', deviceId);
    return;
  }

  const status = getDeviceSyncStatus(deviceId);
  if (status.inProgress) {
    console.log('设备', device.name, '同步已在进行中，跳过');
    return;
  }

  status.inProgress = true;
  syncProgress = {
    total: 0,
    processed: 0,
    currentStep: '准备同步...',
    steps: ['获取Edge书签', '获取服务器数据', '对比差异', '执行同步', '完成'],
    deviceId: deviceId,
    deviceName: device.name
  };

  try {
    console.log('开始同步书签到设备:', device.name);

    const e2a = device.e2a || DEFAULT_E2A;

    syncProgress.currentStep = '获取Edge书签...';
    let edgeBookmarks;
    if (e2a.edgeFolderMode === 'select' && e2a.edgeFolderId) {
      edgeBookmarks = await getEdgeBookmarks(e2a.edgeFolderId);
    } else {
      edgeBookmarks = await getEdgeBookmarks();
    }
    const bookmarkCount = countBookmarks(edgeBookmarks);
    syncProgress.total = bookmarkCount;

    sendProgressToWindow();

    if (bookmarkCount === 0) {
      syncProgress.currentStep = '同步完成';
      syncProgress.processed = 0;
      status.lastSyncTime = new Date().toISOString();
      status.result = { folders: 0, bookmarks: 0, skipped: 0 };
      config.lastSyncTime = status.lastSyncTime;
      await chrome.storage.local.set({ config });
      sendSyncCompleteToWindow(deviceId);
      return { stats: { folders: 0, bookmarks: 0, skipped: 0 } };
    }

    syncProgress.currentStep = '获取服务器数据...';
    syncProgress.processed = Math.floor(syncProgress.total * 0.3);
    sendProgressToWindow();

    syncProgress.currentStep = '执行同步...';
    syncProgress.processed = Math.floor(syncProgress.total * 0.5);
    sendProgressToWindow();

    const syncResult = await syncToBackend(edgeBookmarks, device);
    syncProgress.processed = syncProgress.total;
    syncProgress.currentStep = '同步完成';

    if (syncResult && syncResult.stats) {
      status.result = syncResult.stats;
    }

    status.lastSyncTime = new Date().toISOString();
    config.lastSyncTime = status.lastSyncTime;
    await chrome.storage.local.set({ config });

    sendSyncCompleteToWindow(deviceId);
    return syncResult;
  } catch (error) {
    console.error('同步书签到设备', device.name, '失败:', error);
    syncProgress.currentStep = '同步失败: ' + error.message;
    status.lastError = error.message;
    sendSyncErrorToWindow(error.message, deviceId);
    throw error;
  } finally {
    status.inProgress = false;
  }
}

async function syncToAllDevices() {
  const devices = getDevicesWithE2aAutoSync();
  if (devices.length === 0) return {};
  const promises = devices.map(device =>
    syncBookmarks(device.id).catch(error => ({ error: error.message }))
  );
  const results = await Promise.allSettled(promises);
  const output = {};
  devices.forEach((device, i) => {
    const r = results[i];
    output[device.id] = r.status === 'fulfilled' ? r.value : { error: r.reason?.message || '未知错误' };
  });
  return output;
}

async function getEdgeBookmarks(folderId = null) {
  return new Promise((resolve, reject) => {
    if (folderId) {
      console.log('获取指定 Edge 文件夹，ID:', folderId);
      chrome.bookmarks.getSubTree(folderId, (tree) => {
        if (chrome.runtime.lastError) {
          console.error('获取 Edge 文件夹失败:', chrome.runtime.lastError);
          reject(chrome.runtime.lastError);
        } else {
          resolve(tree);
        }
      });
    } else {
      console.log('获取全部 Edge 书签');
      chrome.bookmarks.getTree((tree) => {
        if (chrome.runtime.lastError) {
          console.error('获取全部 Edge 书签失败:', chrome.runtime.lastError);
          reject(chrome.runtime.lastError);
        } else {
          resolve(tree);
        }
      });
    }
  });
}

function countBookmarks(bookmarks) {
  let count = 0;
  function traverse(node) {
    if (node.url) count++;
    if (node.children) node.children.forEach(child => traverse(child));
  }
  bookmarks.forEach(root => traverse(root));
  return count;
}

function convertToApiFormat(edgeBookmarks) {
  const result = [];
  function traverse(edgeNode, parentId = null) {
    const node = {
      type: edgeNode.url ? 'bookmark' : 'folder',
      title: edgeNode.title || '未命名',
      position: edgeNode.index || 0
    };
    if (edgeNode.url) {
      node.url = edgeNode.url;
    }
    if (edgeNode.children && edgeNode.children.length > 0) {
      node.children = edgeNode.children.map(child => traverse(child));
    }
    return node;
  }
  edgeBookmarks.forEach(root => {
    if (root.children) {
      root.children.forEach(child => {
        result.push(traverse(child));
      });
    }
  });
  return result;
}

async function fetchServerBookmarks(device) {
  try {
    const response = await fetch(`${normalizeUrl(device.serverUrl)}/api/sync/bookmarks`, {
      headers: { 'X-API-Key': device.apiKey }
    });
    if (!response.ok) {
      throw new Error(`获取服务器书签失败: ${response.status}`);
    }
    const data = await response.json();
    return data.bookmarks || [];
  } catch (error) {
    console.error('获取服务器书签失败:', error);
    return [];
  }
}

async function fetchServerFolders(device) {
  try {
    const response = await fetch(`${normalizeUrl(device.serverUrl)}/api/sync/folders`, {
      headers: { 'X-API-Key': device.apiKey }
    });
    if (!response.ok) {
      throw new Error(`获取服务器文件夹失败: ${response.status}`);
    }
    const data = await response.json();
    return data.folders || [];
  } catch (error) {
    console.error('获取服务器文件夹失败:', error);
    return [];
  }
}

function convertEdgeToSyncFormat(edgeBookmarks, targetParentId = null) {
  const bookmarks = [];
  const folders = [];
  const edgeIdToTempId = new Map();
  let tempIdCounter = 1;

  function traverse(node, parentTempId = null) {
    if (node.url) {
      bookmarks.push({
        temp_id: null,
        edge_parent_id: parentTempId,
        title: node.title || '未命名',
        url: node.url,
        favicon_url: null,
        position: node.index || 0
      });
    } else {
      const tempId = tempIdCounter++;
      edgeIdToTempId.set(node.id, tempId);
      const folder = {
        temp_id: tempId,
        edge_id: node.id,
        edge_parent_id: parentTempId,
        title: node.title || '未命名文件夹',
        position: node.index || 0
      };
      folders.push(folder);
      if (node.children && node.children.length > 0) {
        node.children.forEach(child => traverse(child, tempId));
      }
    }
  }

  edgeBookmarks.forEach(node => traverse(node, null));
  return { bookmarks, folders, edgeIdToTempId, targetParentId };
}

function generateBookmarkBatchRequest(edgeData, serverBookmarks, serverFolders, tempIdToServerId, targetParentId, syncMode) {
  const request = {
    create: { bookmarks: [], folders: [] },
    update: { bookmarks: [], folders: [] },
    delete: { bookmark_ids: [], folder_ids: [] }
  };

  edgeData.bookmarks.forEach(edgeBookmark => {
    let parentServerId = null;
    if (edgeBookmark.edge_parent_id) {
      parentServerId = tempIdToServerId.get(edgeBookmark.edge_parent_id) || null;
    } else if (targetParentId) {
      parentServerId = targetParentId;
    }

    const existingBookmark = serverBookmarks.find(b =>
      b.parent_id === parentServerId &&
      b.title === edgeBookmark.title &&
      b.url === edgeBookmark.url
    );

    if (existingBookmark) {
      request.update.bookmarks.push({
        id: existingBookmark.id,
        parent_id: parentServerId,
        title: edgeBookmark.title,
        url: edgeBookmark.url,
        favicon_url: edgeBookmark.favicon_url,
        position: edgeBookmark.position
      });
    } else {
      request.create.bookmarks.push({
        parent_id: parentServerId,
        title: edgeBookmark.title,
        url: edgeBookmark.url,
        favicon_url: edgeBookmark.favicon_url,
        position: edgeBookmark.position
      });
    }
  });

  if (syncMode === 'replace') {
    const edgeBookmarkKeys = new Set(edgeData.bookmarks.map(b => {
      const parentSrvId = b.edge_parent_id
        ? (tempIdToServerId.get(b.edge_parent_id) ?? null)
        : (targetParentId ?? null);
      return `${parentSrvId}_${b.title}_${b.url}`;
    }));

    const scopeFolderIds = targetParentId !== null
      ? collectScopeFolderIds(targetParentId, serverFolders)
      : null;

    serverBookmarks.forEach(serverBookmark => {
      if (scopeFolderIds !== null && !scopeFolderIds.has(serverBookmark.parent_id)) {
        return;
      }
      const key = `${serverBookmark.parent_id ?? null}_${serverBookmark.title}_${serverBookmark.url}`;
      if (!edgeBookmarkKeys.has(key)) {
        request.delete.bookmark_ids.push(serverBookmark.id);
      }
    });
  }

  return request;
}

function getFolderDepth(folder, allFolders) {
  let depth = 0;
  let current = folder;
  while (current.edge_parent_id) {
    depth++;
    current = allFolders.find(f => f.temp_id === current.edge_parent_id);
    if (!current) break;
  }
  return depth;
}

async function callBatchAPI(batchRequest, device) {
  const response = await fetch(`${normalizeUrl(device.serverUrl)}/api/sync/batch`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': device.apiKey
    },
    body: JSON.stringify(batchRequest)
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`后端API错误: ${response.status}, ${errorText}`);
  }

  return await response.json();
}

function collectDescendantFolderIds(parentId, allFolders) {
  const ids = new Set();
  const queue = [parentId];
  while (queue.length > 0) {
    const cur = queue.shift();
    allFolders.forEach(f => {
      if (f.parent_id === cur) {
        ids.add(f.id);
        queue.push(f.id);
      }
    });
  }
  return ids;
}

function collectScopeFolderIds(parentId, allFolders) {
  const ids = collectDescendantFolderIds(parentId, allFolders);
  if (parentId !== null) ids.add(parentId);
  return ids;
}

async function syncToBackend(edgeBookmarks, device) {
  try {
    if (!device || !device.apiKey) {
      throw new Error('请先配置设备的 API Key');
    }

    const e2a = device.e2a || DEFAULT_E2A;

    const serverBookmarks = await fetchServerBookmarks(device);
    const serverFolders = await fetchServerFolders(device);

    let targetParentId = null;
    if (e2a.appFolderMode === 'select' && e2a.appFolderId) {
      targetParentId = parseInt(e2a.appFolderId, 10);
      console.log('同步到应用指定文件夹 ID:', targetParentId);
    }

    let nodesToSync;
    if (e2a.edgeFolderMode === 'select' && e2a.edgeFolderId && edgeBookmarks.length === 1 && !edgeBookmarks[0].url) {
      nodesToSync = edgeBookmarks[0].children || [];
      console.log('指定浏览器目录，只同步其子节点，数量:', nodesToSync.length);
    } else {
      nodesToSync = edgeBookmarks;
    }

    const edgeData = convertEdgeToSyncFormat(nodesToSync, targetParentId);
    console.log('Edge 数据转换完成:', {
      folders: edgeData.folders.length,
      bookmarks: edgeData.bookmarks.length
    });

    console.log('=== 第一步：按层级同步文件夹 ===');

    const tempIdToServerId = new Map();
    const maxDepth = Math.max(0, ...edgeData.folders.map(f => getFolderDepth(f, edgeData.folders)));

    for (let depth = 0; depth <= maxDepth; depth++) {
      const layerFolders = edgeData.folders.filter(
        f => getFolderDepth(f, edgeData.folders) === depth
      );
      if (layerFolders.length === 0) continue;

      const layerRequest = {
        create: { bookmarks: [], folders: [] },
        update: { bookmarks: [], folders: [] },
        delete: { bookmark_ids: [], folder_ids: [] }
      };

      layerFolders.forEach(edgeFolder => {
        let parentServerId = null;
        if (edgeFolder.edge_parent_id) {
          parentServerId = tempIdToServerId.get(edgeFolder.edge_parent_id) || null;
        } else if (targetParentId) {
          parentServerId = targetParentId;
        }

        const existingFolder = serverFolders.find(f =>
          f.parent_id === parentServerId &&
          f.title === edgeFolder.title
        );

        if (existingFolder) {
          layerRequest.update.folders.push({
            id: existingFolder.id,
            parent_id: parentServerId,
            title: edgeFolder.title,
            position: edgeFolder.position
          });
          tempIdToServerId.set(edgeFolder.temp_id, existingFolder.id);
        } else {
          layerRequest.create.folders.push({
            parent_id: parentServerId,
            title: edgeFolder.title,
            position: edgeFolder.position,
            _temp_id: edgeFolder.temp_id
          });
        }
      });

      if (depth === maxDepth && e2a.syncMode === 'replace') {
        const edgeFolderTitles = new Set(edgeData.folders.map(f => f.title));
        const scopeParentId = targetParentId;
        serverFolders.forEach(serverFolder => {
          if (serverFolder.parent_id === scopeParentId && !edgeFolderTitles.has(serverFolder.title)) {
            layerRequest.delete.folder_ids.push(serverFolder.id);
          }
        });
      }

      const cleanFolders = layerRequest.create.folders.map(({ _temp_id, ...rest }) => rest);
      const sendRequest = {
        ...layerRequest,
        create: { ...layerRequest.create, folders: cleanFolders }
      };

      if (
        sendRequest.create.folders.length > 0 ||
        sendRequest.update.folders.length > 0 ||
        sendRequest.delete.folder_ids.length > 0
      ) {
        const layerResult = await callBatchAPI(sendRequest, device);
        const createList = layerRequest.create.folders;
        layerResult.created?.folders?.forEach((folder, index) => {
          if (index < createList.length) {
            const tempId = createList[index]._temp_id;
            tempIdToServerId.set(tempId, folder.id);
          }
        });
      }
    }

    console.log('=== 第二步：同步书签 ===');
    const bookmarkBatchRequest = generateBookmarkBatchRequest(
      edgeData,
      serverBookmarks,
      serverFolders,
      tempIdToServerId,
      targetParentId,
      e2a.syncMode
    );

    const bookmarkResult = await callBatchAPI(bookmarkBatchRequest, device);

    const result = {
      created: {
        folders: [],
        bookmarks: bookmarkResult.created?.bookmarks || []
      },
      updated: {
        folders: [],
        bookmarks: bookmarkResult.updated?.bookmarks || []
      },
      deleted: {
        folder_ids: [],
        bookmark_ids: bookmarkResult.deleted?.bookmark_ids || []
      },
      errors: [...(bookmarkResult.errors || [])]
    };

    const stats = {
      folders: tempIdToServerId.size,
      bookmarks: result.created.bookmarks.length,
      updated_folders: result.updated.folders.length,
      updated_bookmarks: result.updated.bookmarks.length,
      deleted_folders: result.deleted.folder_ids.length,
      deleted_bookmarks: result.deleted.bookmark_ids.length,
      errors: result.errors.length
    };

    console.log('同步完成，统计:', stats);
    return { stats, result };
  } catch (error) {
    console.error('同步到后端失败:', error);
    throw error;
  }
}

async function getEdgeFolders() {
  try {
    const tree = await getEdgeBookmarks();
    const folders = [];
    function traverse(node, path = '') {
      if (!node.url) {
        const currentPath = path ? `${path}/${node.title}` : node.title;
        folders.push({
          id: node.id,
          title: node.title,
          path: currentPath,
          children: node.children || []
        });
        if (node.children) {
          node.children.forEach(child => traverse(child, currentPath));
        }
      }
    }
    tree.forEach(root => traverse(root));
    return folders;
  } catch (error) {
    console.error('获取 Edge 文件夹失败:', error);
    return [];
  }
}

async function getAppFolders(deviceId, tempServerUrl, tempApiKey) {
  try {
    // 优先使用传入的临时 serverUrl 和 apiKey（用于编辑时实时加载）
    let serverUrl = tempServerUrl;
    let apiKey = tempApiKey;

    // 如果没有传入临时值，从已保存的设备配置中获取
    if (!serverUrl || !apiKey) {
      if (!deviceId) return [];
      const device = config.devices.find(d => d.id === deviceId);
      if (!device || !device.apiKey) {
        return [];
      }
      serverUrl = serverUrl || device.serverUrl;
      apiKey = apiKey || device.apiKey;
    }

    if (!serverUrl || !apiKey) return [];

    const response = await fetch(`${normalizeUrl(serverUrl)}/api/sync/folders`, {
      headers: { 'X-API-Key': apiKey }
    });

    if (!response.ok) {
      throw new Error(`获取应用文件夹失败: ${response.status}`);
    }

    const data = await response.json();
    const folders = data.folders || [];

    const folderMap = new Map();
    folders.forEach(folder => {
      folderMap.set(folder.id, folder);
    });

    function buildFullPath(folder) {
      if (!folder.parent_id) {
        return folder.title;
      }
      const parent = folderMap.get(folder.parent_id);
      if (!parent) {
        return folder.title;
      }
      return `${buildFullPath(parent)}/${folder.title}`;
    }

    return folders.map(folder => ({
      id: folder.id,
      title: buildFullPath(folder)
    }));
  } catch (error) {
    console.error('获取应用文件夹失败:', error);
    return [];
  }
}

async function syncFromApp(deviceId) {
  if (!deviceId) {
    throw new Error('syncFromApp 需要指定 deviceId');
  }
  const device = config.devices.find(d => d.id === deviceId);
  if (!device) {
    throw new Error('设备不存在: ' + deviceId);
  }

  const status = getDeviceSyncStatus(deviceId);
  if (status.inProgress) {
    console.log('设备', device.name, 'A→E同步已在进行中，跳过');
    return;
  }

  const a2e = device.a2e || DEFAULT_A2E;

  status.inProgress = true;
  syncProgress = {
    total: 0,
    processed: 0,
    currentStep: '准备同步（应用→浏览器）...',
    steps: ['拉取应用数据', '准备浏览器目录', '写入书签', '完成'],
    deviceId: deviceId,
    deviceName: device.name
  };

  try {
    if (!device.apiKey) throw new Error('请先配置设备的 API Key');

    if (!a2e.targetFolderId) {
      throw new Error('请先在设备设置中指定「浏览器目标目录」');
    }
    const targetEdgeFolderId = String(a2e.targetFolderId);

    await new Promise((resolve, reject) => {
      chrome.bookmarks.get(targetEdgeFolderId, (result) => {
        if (chrome.runtime.lastError || !result || result.length === 0) {
          reject(new Error(`浏览器目标目录不存在，请重新选择`));
        } else {
          resolve(result[0]);
        }
      });
    });

    syncProgress.currentStep = '拉取应用数据...';
    sendProgressToWindow();

    let treeUrl = `${normalizeUrl(device.serverUrl)}/api/sync/tree`;
    if (a2e.sourceFolderMode === 'select' && a2e.sourceFolderId) {
      treeUrl += `?folder_id=${a2e.sourceFolderId}`;
    }

    const treeResp = await fetch(treeUrl, {
      headers: { 'X-API-Key': device.apiKey }
    });
    if (!treeResp.ok) {
      const errText = await treeResp.text();
      throw new Error(`拉取应用数据失败: ${treeResp.status} ${errText}`);
    }
    const treeData = await treeResp.json();
    const appNodes = treeData.nodes || [];
    console.log('拉取到应用节点数:', appNodes.length);

    function countNodes(nodes) {
      let n = 0;
      nodes.forEach(node => {
        n++;
        if (node.children) n += countNodes(node.children);
      });
      return n;
    }
    syncProgress.total = countNodes(appNodes);
    sendProgressToWindow();

    syncProgress.currentStep = '准备浏览器目录...';
    sendProgressToWindow();

    const existingEdgeChildren = await new Promise((resolve, reject) => {
      chrome.bookmarks.getChildren(targetEdgeFolderId, (children) => {
        if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
        else resolve(children || []);
      });
    });

    syncProgress.currentStep = '写入书签...';
    sendProgressToWindow();

    const stats = { created: 0, updated: 0, deleted: 0, folders: 0 };
    await writeNodesToEdge(appNodes, targetEdgeFolderId, existingEdgeChildren, a2e.syncMode, stats);

    syncProgress.processed = syncProgress.total;
    syncProgress.currentStep = '同步完成';
    status.lastSyncTime = new Date().toISOString();
    status.result = stats;
    config.lastSyncTime = status.lastSyncTime;
    config.syncResult = stats;
    await chrome.storage.local.set({ config });

    console.log('应用→浏览器同步完成:', stats);
    sendSyncCompleteToWindow(deviceId);
    return { stats };
  } catch (error) {
    console.error('应用→浏览器同步失败:', error);
    syncProgress.currentStep = '同步失败: ' + error.message;
    status.lastError = error.message;
    config.lastError = error.message;
    await chrome.storage.local.set({ config });
    sendSyncErrorToWindow(error.message, deviceId);
    throw error;
  } finally {
    status.inProgress = false;
  }
}

async function syncFromAppAllDevices() {
  const devices = getDevicesWithA2eAutoSync();
  if (devices.length === 0) return {};
  const promises = devices.map(device =>
    syncFromApp(device.id).catch(error => ({ error: error.message }))
  );
  const results = await Promise.allSettled(promises);
  const output = {};
  devices.forEach((device, i) => {
    const r = results[i];
    output[device.id] = r.status === 'fulfilled' ? r.value : { error: r.reason?.message || '未知错误' };
  });
  return output;
}

async function writeNodesToEdge(appNodes, parentEdgeId, existingChildren, syncMode, stats) {
  const existingByTitle = new Map();
  existingChildren.forEach(n => {
    const key = `${n.url ? 'bm' : 'dir'}_${n.title}`;
    if (!existingByTitle.has(key)) existingByTitle.set(key, []);
    existingByTitle.get(key).push(n);
  });

  const processedEdgeIds = new Set();

  for (const appNode of appNodes) {
    const isFolder = appNode.type === 'folder';
    const key = `${isFolder ? 'dir' : 'bm'}_${appNode.title}`;
    const candidates = existingByTitle.get(key) || [];

    if (isFolder) {
      let edgeFolder = candidates.find(c => !c.url);
      if (!edgeFolder) {
        edgeFolder = await new Promise((resolve, reject) => {
          chrome.bookmarks.create({ parentId: parentEdgeId, title: appNode.title }, (node) => {
            if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
            else resolve(node);
          });
        });
        stats.folders++;
        stats.created++;
      } else {
        processedEdgeIds.add(edgeFolder.id);
      }

      if (appNode.children && appNode.children.length > 0) {
        const folderChildren = await new Promise((resolve, reject) => {
          chrome.bookmarks.getChildren(edgeFolder.id, (children) => {
            if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
            else resolve(children || []);
          });
        });
        await writeNodesToEdge(appNode.children, edgeFolder.id, folderChildren, syncMode, stats);
      }
      processedEdgeIds.add(edgeFolder.id);
    } else {
      let existingBm = existingChildren.find(c => c.url === appNode.url);
      if (!existingBm) existingBm = candidates.find(c => c.url);

      if (existingBm) {
        if (existingBm.title !== appNode.title) {
          await new Promise((resolve, reject) => {
            chrome.bookmarks.update(existingBm.id, { title: appNode.title }, (node) => {
              if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
              else resolve(node);
            });
          });
          stats.updated++;
        }
        processedEdgeIds.add(existingBm.id);
      } else {
        const newNode = await new Promise((resolve, reject) => {
          chrome.bookmarks.create({
            parentId: parentEdgeId,
            title: appNode.title,
            url: appNode.url
          }, (node) => {
            if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
            else resolve(node);
          });
        });
        processedEdgeIds.add(newNode.id);
        stats.created++;
        syncProgress.processed++;
        sendProgressToWindow();
      }
    }
  }

  if (syncMode === 'replace') {
    for (const child of existingChildren) {
      if (!processedEdgeIds.has(child.id)) {
        try {
          await new Promise((resolve, reject) => {
            if (child.url) {
              chrome.bookmarks.remove(child.id, () => {
                if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
                else resolve();
              });
            } else {
              chrome.bookmarks.removeTree(child.id, () => {
                if (chrome.runtime.lastError) reject(chrome.runtime.lastError);
                else resolve();
              });
            }
          });
          stats.deleted++;
        } catch (e) {
          console.error(`删除浏览器节点失败: ${child.title}`, e);
        }
      }
    }
  }
}

async function updateConfig(newConfig) {
  if (newConfig.serverUrl && !newConfig.devices) {
    newConfig = migrateConfig(newConfig);
  }
  config = { ...DEFAULT_CONFIG, ...newConfig };
  if (config.devices) {
    config.devices = config.devices.map(d => ensureDeviceDefaults({ ...d }));
  }
  await chrome.storage.local.set({ config });

  registerSyncAlarms();

  console.log('配置已更新:', config);
  chrome.runtime.sendMessage({ action: 'configUpdated', config }).catch(() => {});
}

async function addDevice(device) {
  if (!device.id) {
    device.id = 'device_' + Date.now();
  }
  if (!device.name) {
    device.name = '新设备';
  }
  if (device.enabled === undefined) {
    device.enabled = true;
  }
  if (device.serverUrl) {
    device.serverUrl = normalizeUrl(device.serverUrl);
  }
  ensureDeviceDefaults(device);
  config.devices.push(device);
  await chrome.storage.local.set({ config });
  registerSyncAlarms();
  chrome.runtime.sendMessage({ action: 'configUpdated' }).catch(() => {});
  return { status: 'success', device };
}

async function removeDevice(deviceId) {
  config.devices = config.devices.filter(d => d.id !== deviceId);
  delete deviceSyncStatus[deviceId];
  await chrome.storage.local.set({ config });
  registerSyncAlarms();
  chrome.runtime.sendMessage({ action: 'configUpdated' }).catch(() => {});
  return { status: 'success' };
}

async function updateDevice(deviceId, updates) {
  const index = config.devices.findIndex(d => d.id === deviceId);
  if (index === -1) {
    throw new Error('设备不存在');
  }
  if (updates.serverUrl) {
    updates.serverUrl = normalizeUrl(updates.serverUrl);
  }
  config.devices[index] = ensureDeviceDefaults({ ...config.devices[index], ...updates });
  await chrome.storage.local.set({ config });
  registerSyncAlarms();
  chrome.runtime.sendMessage({ action: 'configUpdated' }).catch(() => {});
  return { status: 'success', device: config.devices[index] };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  console.log('收到消息:', message.action, message);

  switch (message.action) {
    case 'sync':
      console.log('处理 sync 消息（浏览器→应用）');
      if (!message.deviceId) {
        sendResponse({ status: 'error', error: '请指定 deviceId' });
        return;
      }
      syncBookmarks(message.deviceId).then((result) => {
        sendResponse(result);
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'syncFromApp':
      console.log('处理 syncFromApp 消息');
      if (!message.deviceId) {
        sendResponse({ status: 'error', error: '请指定 deviceId' });
        return;
      }
      syncFromApp(message.deviceId).then((result) => {
        sendResponse({ status: 'success', result });
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'syncAllDevices':
      console.log('处理 syncAllDevices 消息');
      syncToAllDevices().then((result) => {
        sendResponse({ status: 'success', result });
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'syncFromAppAllDevices':
      console.log('处理 syncFromAppAllDevices 消息');
      syncFromAppAllDevices().then((result) => {
        sendResponse({ status: 'success', result });
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'getConfig':
      sendResponse({ config });
      break;
    case 'updateConfig':
      updateConfig(message.config).then(() => {
        sendResponse({ status: 'success' });
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'getSyncStatus':
      sendResponse({
        syncInProgress: isAnySyncInProgress(),
        deviceSyncStatus,
        syncProgress,
        lastSyncTime: config.lastSyncTime,
        lastError: config.lastError
      });
      break;
    case 'getAppFolders':
      getAppFolders(message.deviceId, message.serverUrl, message.apiKey).then(folders => {
        sendResponse({ folders });
      }).catch(error => {
        sendResponse({ folders: [], error: error.message });
      });
      return true;
    case 'getEdgeFolders':
      getEdgeFolders().then(folders => {
        sendResponse({ folders });
      }).catch(error => {
        sendResponse({ folders: [], error: error.message });
      });
      return true;
    case 'addDevice':
      addDevice(message.device).then(result => {
        sendResponse(result);
      }).catch(error => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'removeDevice':
      removeDevice(message.deviceId).then(result => {
        sendResponse(result);
      }).catch(error => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'updateDevice':
      updateDevice(message.deviceId, message.updates).then(result => {
        sendResponse(result);
      }).catch(error => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'getDevices':
      sendResponse({ devices: config.devices || [] });
      break;
    case 'openSyncWindow':
      try {
        openSyncWindow();
        sendResponse({ status: 'success' });
      } catch (error) {
        sendResponse({ status: 'error', error: error.message });
      }
      break;
    default:
      sendResponse({ status: 'unknown action' });
      break;
  }
});

function sendProgressToWindow() {
  chrome.runtime.sendMessage({
    action: 'syncProgress',
    progress: { ...syncProgress },
    deviceId: syncProgress.deviceId || null,
    deviceName: syncProgress.deviceName || null
  }, () => {
    if (chrome.runtime.lastError) {}
  });
}

function sendSyncCompleteToWindow(deviceId) {
  chrome.runtime.sendMessage({
    action: 'syncComplete',
    progress: { ...syncProgress },
    deviceId: deviceId || syncProgress.deviceId || null,
    deviceName: syncProgress.deviceName || null
  }, () => {
    if (chrome.runtime.lastError) {}
  });
}

function sendSyncErrorToWindow(errorMsg, deviceId) {
  chrome.runtime.sendMessage({
    action: 'syncError',
    error: errorMsg,
    deviceId: deviceId || syncProgress.deviceId || null,
    deviceName: syncProgress.deviceName || null
  }, () => {
    if (chrome.runtime.lastError) {}
  });
}

initialize();

function openSyncWindow() {
  try {
    chrome.windows.create({
      url: chrome.runtime.getURL('sync-window.html'),
      type: 'normal',
      width: 800,
      height: 600
    }, (window) => {
      if (chrome.runtime.lastError) {
        console.error('创建窗口失败:', chrome.runtime.lastError);
      } else if (window && window.id) {
        progressWindowId = window.id;
      }
    });
  } catch (error) {
    console.error('打开同步窗口异常:', error);
  }
}

chrome.runtime.onInstalled.addListener((details) => {
  console.log('插件安装/更新:', details);
  if (details.reason === 'install') {
    chrome.tabs.create({ url: chrome.runtime.getURL('sync-window.html') });
  } else if (details.reason === 'update') {
    console.log('插件已更新到版本:', chrome.runtime.getManifest().version);
  }
});

chrome.action.onClicked.addListener(() => {
  const syncWindowUrl = chrome.runtime.getURL('sync-window.html');
  chrome.tabs.query({}, (tabs) => {
    const existing = tabs.find(t => t.url === syncWindowUrl);
    if (existing) {
      chrome.tabs.update(existing.id, { active: true });
      chrome.windows.update(existing.windowId, { focused: true });
    } else {
      chrome.tabs.create({ url: syncWindowUrl });
    }
  });
});

chrome.runtime.onStartup.addListener(() => {
  console.log('浏览器启动，初始化【网址收藏夹】同步助手');
  initialize();
});
