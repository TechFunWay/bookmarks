// 配置默认值
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  apiKey: '',  // API Key 用于认证
  syncInterval: 5,
  enableAutoSync: false,
  syncMode: 'merge',
  edgeFolderMode: 'all',   // 'all' | 'select'
  edgeFolderId: null,
  edgeFolderName: '',
  appFolderMode: 'all',    // 'all' | 'select'
  appFolderId: null,
  appFolderName: '',
  lastSyncTime: null,
  folderIdMap: {}  // Edge 文件夹 ID 到服务器文件夹 ID 的映射
};

// 全局变量
let config = { ...DEFAULT_CONFIG };
let syncInProgress = false;
let syncProgress = {
  total: 0,
  processed: 0,
  currentStep: '',
  steps: ['获取Edge书签', '获取服务器数据', '对比差异', '执行同步', '完成']
};
let progressWindowId = null;

// 初始化插件
async function initialize() {
  console.log('【网址收藏夹】同步助手初始化...');
  
  await loadConfig();
  
  registerSyncAlarm();
  
  registerBookmarkListeners();
  
  if (config.enableAutoSync) {
    console.log('启用自动同步，执行首次同步');
    await syncBookmarks();
  } else {
    console.log('自动同步未启用，跳过首次同步');
  }
  
  console.log('【网址收藏夹】同步助手初始化完成');
}

// 加载配置
async function loadConfig() {
  try {
    const storedConfig = await chrome.storage.local.get('config');
    if (storedConfig.config) {
      config = { ...DEFAULT_CONFIG, ...storedConfig.config };
      console.log('配置加载成功:', config);
      console.log('服务器地址:', config.serverUrl);
    } else {
      await chrome.storage.local.set({ config });
      console.log('使用默认配置:', config);
      console.log('默认服务器地址:', config.serverUrl);
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }
}

// 注册同步定时器
function registerSyncAlarm() {
  chrome.alarms.clear('syncBookmarks');
  
  if (config.enableAutoSync) {
    chrome.alarms.create('syncBookmarks', {
      periodInMinutes: config.syncInterval
    });
    console.log(`同步定时器已注册，间隔: ${config.syncInterval}分钟`);
  } else {
    console.log('自动同步未启用，不注册定时器');
  }
  
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === 'syncBookmarks' && config.enableAutoSync) {
      console.log('执行定时同步');
      syncBookmarks();
    }
  });
}

// 注册书签变更监听器
function registerBookmarkListeners() {
  chrome.bookmarks.onCreated.addListener((id, bookmark) => {
    console.log('书签创建:', id, bookmark);
    if (config.enableAutoSync) {
      setTimeout(() => {
        syncBookmarks();
      }, 1000);
    }
  });
  
  chrome.bookmarks.onRemoved.addListener((id, removeInfo) => {
    console.log('书签删除:', id, removeInfo);
    if (config.enableAutoSync) {
      syncBookmarks();
    }
  });
  
  chrome.bookmarks.onChanged.addListener((id, changeInfo) => {
    console.log('书签修改:', id, changeInfo);
    if (config.enableAutoSync) {
      syncBookmarks();
    }
  });
  
  chrome.bookmarks.onMoved.addListener((id, moveInfo) => {
    console.log('书签移动:', id, moveInfo);
    if (config.enableAutoSync) {
      syncBookmarks();
    }
  });
  
  chrome.bookmarks.onChildrenReordered.addListener((id, reorderInfo) => {
    console.log('书签重排序:', id, reorderInfo);
    if (config.enableAutoSync) {
      syncBookmarks();
    }
  });
  
  console.log('书签变更监听器已注册');
}

// 同步书签
async function syncBookmarks() {
  if (syncInProgress) {
    console.log('同步已在进行中，跳过');
    return;
  }

  syncInProgress = true;
  syncProgress = {
    total: 0,
    processed: 0,
    currentStep: '准备同步...',
    steps: ['获取Edge书签', '获取服务器数据', '对比差异', '执行同步', '完成']
  };

  try {
    console.log('开始同步书签...');
    console.log('同步配置:', {
      enableAutoSync: config.enableAutoSync,
      syncScope: config.syncScope,
      syncMode: config.syncMode,
      edgeFolderId: config.edgeFolderId,
      appFolderId: config.appFolderId
    });

    // 步骤1: 获取Edge书签
    syncProgress.currentStep = '获取Edge书签...';
    let edgeBookmarks;
    if (config.edgeFolderMode === 'select' && config.edgeFolderId) {
      console.log('从Edge指定目录同步，Edge文件夹 ID:', config.edgeFolderId);
      edgeBookmarks = await getEdgeBookmarks(config.edgeFolderId);
    } else {
      console.log('同步全部书签');
      edgeBookmarks = await getEdgeBookmarks();
    }
    const bookmarkCount = countBookmarks(edgeBookmarks);
    syncProgress.total = bookmarkCount;
    console.log('获取到Edge书签数量:', bookmarkCount);

    // 发送进度更新
    sendProgressToWindow();

    if (bookmarkCount === 0) {
      console.warn('没有书签需要同步');
      syncProgress.currentStep = '同步完成';
      syncProgress.processed = 0;
      config.lastSyncTime = new Date().toISOString();
      await chrome.storage.local.set({ config });
      sendSyncCompleteToWindow();
      return { stats: { folders: 0, bookmarks: 0, skipped: 0 } };
    }

    // 步骤2: 获取服务器数据
    syncProgress.currentStep = '获取服务器数据...';
    syncProgress.processed = Math.floor(syncProgress.total * 0.3);
    sendProgressToWindow();

    // 步骤3: 执行同步
    syncProgress.currentStep = '执行同步...';
    syncProgress.processed = Math.floor(syncProgress.total * 0.5);
    sendProgressToWindow();

    const syncResult = await syncToBackend(edgeBookmarks);
    console.log('同步到后端完成，结果:', syncResult);
    syncProgress.processed = syncProgress.total;
    syncProgress.currentStep = '同步完成';

    if (syncResult && syncResult.stats) {
      config.syncResult = syncResult.stats;
      console.log('保存同步结果:', syncResult.stats);
    }

    config.lastSyncTime = new Date().toISOString();
    await chrome.storage.local.set({ config });

    console.log('书签同步完成');
    sendSyncCompleteToWindow();
    return syncResult;
  } catch (error) {
    console.error('同步书签失败:', error);
    syncProgress.currentStep = '同步失败: ' + error.message;
    config.lastError = error.message;
    await chrome.storage.local.set({ config });
    sendSyncErrorToWindow(error.message);
    throw error;
  } finally {
    syncInProgress = false;
    console.log('同步状态已重置');
  }
}

// 获取Edge浏览器书签
async function getEdgeBookmarks(folderId = null) {
  return new Promise((resolve, reject) => {
    if (folderId) {
      console.log('获取指定 Edge 文件夹，ID:', folderId);
      chrome.bookmarks.getSubTree(folderId, (tree) => {
        if (chrome.runtime.lastError) {
          console.error('获取 Edge 文件夹失败:', chrome.runtime.lastError);
          reject(chrome.runtime.lastError);
        } else {
          console.log('获取到 Edge 文件夹树:', tree);
          if (tree && tree.length > 0) {
            console.log('第一个节点示例:', JSON.stringify(tree[0], null, 2));
          }
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
          console.log('获取到全部 Edge 书签树');
          if (tree && tree.length > 0) {
            console.log('第一个节点示例:', JSON.stringify(tree[0], null, 2));
          }
          resolve(tree);
        }
      });
    }
  });
}

// 计算书签数量
function countBookmarks(bookmarks) {
  let count = 0;
  
  function traverse(node) {
    if (node.url) {
      count++;
    }
    if (node.children) {
      node.children.forEach(child => traverse(child));
    }
  }
  
  bookmarks.forEach(root => traverse(root));
  return count;
}

// 转换为后端API格式
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
      console.log('添加书签:', edgeNode.title, 'URL:', edgeNode.url);
    }
    
    if (edgeNode.children && edgeNode.children.length > 0) {
      node.children = edgeNode.children.map(child => traverse(child));
    }
    
    return node;
  }
  
  edgeBookmarks.forEach(root => {
    console.log('处理根节点:', root.title, 'children:', root.children?.length);
    if (root.children) {
      root.children.forEach(child => {
        const processed = traverse(child);
        result.push(processed);
        console.log('添加节点:', processed.title, 'type:', processed.type);
      });
    }
  });
  
  console.log('转换完成，共', result.length, '个节点');
  return result;
}

// 获取服务器书签列表
async function fetchServerBookmarks() {
  try {
    console.log('获取服务器书签列表...');
    const response = await fetch(`${config.serverUrl}/api/sync/bookmarks`, {
      headers: {
        'X-API-Key': config.apiKey
      }
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      console.error('获取服务器书签失败:', errorText);
      throw new Error(`获取服务器书签失败: ${response.status}`);
    }
    
    const data = await response.json();
    console.log('获取到服务器书签数量:', data.bookmarks?.length || 0);
    return data.bookmarks || [];
  } catch (error) {
    console.error('获取服务器书签失败:', error);
    return [];
  }
}

// 获取服务器文件夹列表
async function fetchServerFolders() {
  try {
    console.log('获取服务器文件夹列表...');
    const response = await fetch(`${config.serverUrl}/api/sync/folders`, {
      headers: {
        'X-API-Key': config.apiKey
      }
    });
    
    if (!response.ok) {
      const errorText = await response.text();
      console.error('获取服务器文件夹失败:', errorText);
      throw new Error(`获取服务器文件夹失败: ${response.status}`);
    }
    
    const data = await response.json();
    console.log('获取到服务器文件夹数量:', data.folders?.length || 0);
    return data.folders || [];
  } catch (error) {
    console.error('获取服务器文件夹失败:', error);
    return [];
  }
}

// 将 Edge 书签转换为同步格式
function convertEdgeToSyncFormat(edgeBookmarks, targetParentId = null) {
  const bookmarks = [];
  const folders = [];
  const edgeIdToTempId = new Map();  // Edge ID -> 临时 ID 映射
  let tempIdCounter = 1;

  function traverse(node, parentTempId = null) {
    if (node.url) {
      // 书签
      bookmarks.push({
        temp_id: null,
        edge_parent_id: parentTempId,  // Edge 层级关系中的父级临时 ID
        title: node.title || '未命名',
        url: node.url,
        favicon_url: null,
        position: node.index || 0
      });
    } else {
      // 文件夹
      const tempId = tempIdCounter++;
      edgeIdToTempId.set(node.id, tempId);  // 保存 Edge ID 到临时 ID 的映射

      const folder = {
        temp_id: tempId,
        edge_id: node.id,  // 原始 Edge ID
        edge_parent_id: parentTempId,  // Edge 层级关系中的父级临时 ID
        title: node.title || '未命名文件夹',
        position: node.index || 0
      };
      folders.push(folder);

      // 递归处理子节点
      if (node.children && node.children.length > 0) {
        node.children.forEach(child => traverse(child, tempId));
      }
    }
  }

  // 遍历所有根节点
  edgeBookmarks.forEach(node => traverse(node, null));

  return { bookmarks, folders, edgeIdToTempId, targetParentId };
}

// 生成文件夹批量操作请求
function generateFolderBatchRequest(edgeData, serverFolders, syncMode, targetParentId) {
  const request = {
    create: { bookmarks: [], folders: [] },
    update: { bookmarks: [], folders: [] },
    delete: { bookmark_ids: [], folder_ids: [] }
  };

  // 按层级排序文件夹（先父后子）
  const sortedFolders = [...edgeData.folders].sort((a, b) => {
    const depthA = getFolderDepth(a, edgeData.folders);
    const depthB = getFolderDepth(b, edgeData.folders);
    return depthA - depthB;
  });

  // 处理文件夹
  sortedFolders.forEach(edgeFolder => {
    // 计算父文件夹的服务器 ID
    let parentServerId = null;
    if (edgeFolder.edge_parent_id) {
      // 有父文件夹，需要查找对应的服务器 ID
      const parentFolder = edgeData.folders.find(f => f.temp_id === edgeFolder.edge_parent_id);
      if (parentFolder) {
        // 父文件夹会在之前处理，这里先标记为需要后续映射
        parentServerId = `temp_${edgeFolder.edge_parent_id}`;
      }
    } else if (targetParentId) {
      // 根文件夹使用目标父文件夹
      parentServerId = targetParentId;
    }

    // 检查服务器上是否已存在同名文件夹（在同一父文件夹下）
    const existingFolder = serverFolders.find(f =>
      f.parent_id === parentServerId &&
      f.title === edgeFolder.title
    );

    if (existingFolder) {
      // 更新现有文件夹
      request.update.folders.push({
        id: existingFolder.id,
        parent_id: parentServerId,
        title: edgeFolder.title,
        position: edgeFolder.position
      });
    } else {
      // 创建新文件夹
      request.create.folders.push({
        parent_id: parentServerId,
        title: edgeFolder.title,
        position: edgeFolder.position
      });
    }
  });

  // Replace 模式：删除服务器上有但 Edge 没有的内容
  if (syncMode === 'replace') {
    const edgeFolderKeys = new Set(edgeData.folders.map(f =>
      `${f.edge_parent_id || 'null'}_${f.title}`
    ));

    serverFolders.forEach(serverFolder => {
      const key = `${serverFolder.parent_id || 'null'}_${serverFolder.title}`;
      if (!edgeFolderKeys.has(key)) {
        request.delete.folder_ids.push(serverFolder.id);
      }
    });
  }

  return request;
}

// 生成书签批量操作请求
function generateBookmarkBatchRequest(edgeData, serverBookmarks, serverFolders, tempIdToServerId, targetParentId, syncMode) {
  const request = {
    create: { bookmarks: [], folders: [] },
    update: { bookmarks: [], folders: [] },
    delete: { bookmark_ids: [], folder_ids: [] }
  };

  // 处理书签
  edgeData.bookmarks.forEach(edgeBookmark => {
    // 计算父文件夹的服务器 ID
    let parentServerId = null;
    if (edgeBookmark.edge_parent_id) {
      // 查找对应的服务器 ID
      parentServerId = tempIdToServerId.get(edgeBookmark.edge_parent_id) || null;
    } else if (targetParentId) {
      // 根书签使用目标父文件夹
      parentServerId = targetParentId;
    }

    // 检查服务器上是否已存在相同书签（在同一父文件夹下，相同标题和URL）
    const existingBookmark = serverBookmarks.find(b =>
      b.parent_id === parentServerId &&
      b.title === edgeBookmark.title &&
      b.url === edgeBookmark.url
    );

    if (existingBookmark) {
      // 更新现有书签
      request.update.bookmarks.push({
        id: existingBookmark.id,
        parent_id: parentServerId,
        title: edgeBookmark.title,
        url: edgeBookmark.url,
        favicon_url: edgeBookmark.favicon_url,
        position: edgeBookmark.position
      });
    } else {
      // 创建新书签
      request.create.bookmarks.push({
        parent_id: parentServerId,
        title: edgeBookmark.title,
        url: edgeBookmark.url,
        favicon_url: edgeBookmark.favicon_url,
        position: edgeBookmark.position
      });
    }
  });

  // Replace 模式：只删除目标范围内 Edge 已不存在的书签
  if (syncMode === 'replace') {
    // Edge 侧书签的 "父文件夹服务器ID_标题_url" 集合
    const edgeBookmarkKeys = new Set(edgeData.bookmarks.map(b => {
      const parentSrvId = b.edge_parent_id
        ? (tempIdToServerId.get(b.edge_parent_id) ?? null)
        : (targetParentId ?? null);
      return `${parentSrvId}_${b.title}_${b.url}`;
    }));

    // 确定服务器侧书签的比对范围：targetParentId 及其所有子孙文件夹
    const scopeFolderIds = targetParentId !== null
      ? collectScopeFolderIds(targetParentId, serverFolders)
      : null; // null 表示全库范围（未指定目录时）

    serverBookmarks.forEach(serverBookmark => {
      // 限定范围：仅处理 scopeFolderIds 内的书签
      if (scopeFolderIds !== null && !scopeFolderIds.has(serverBookmark.parent_id)) {
        return; // 不在范围内，跳过
      }
      const key = `${serverBookmark.parent_id ?? null}_${serverBookmark.title}_${serverBookmark.url}`;
      if (!edgeBookmarkKeys.has(key)) {
        request.delete.bookmark_ids.push(serverBookmark.id);
      }
    });
  }

  return request;
}

// 获取文件夹深度
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

// 生成批量操作请求（兼容旧版本，现在只用于删除操作）
function generateBatchRequest(edgeData, serverBookmarks, serverFolders, syncMode) {
  const request = {
    create: { bookmarks: [], folders: [] },
    update: { bookmarks: [], folders: [] },
    delete: { bookmark_ids: [], folder_ids: [] }
  };

  // Replace 模式：删除服务器上有但 Edge 没有的内容
  if (syncMode === 'replace') {
    const edgeBookmarkKeys = new Set(edgeData.bookmarks.map(b =>
      `${b.edge_parent_id || 'null'}_${b.title}_${b.url}`
    ));
    const edgeFolderKeys = new Set(edgeData.folders.map(f =>
      `${f.edge_parent_id || 'null'}_${f.title}`
    ));

    serverBookmarks.forEach(serverBookmark => {
      const key = `${serverBookmark.parent_id || 'null'}_${serverBookmark.title}_${serverBookmark.url}`;
      if (!edgeBookmarkKeys.has(key)) {
        request.delete.bookmark_ids.push(serverBookmark.id);
      }
    });

    serverFolders.forEach(serverFolder => {
      const key = `${serverFolder.parent_id || 'null'}_${serverFolder.title}`;
      if (!edgeFolderKeys.has(key)) {
        request.delete.folder_ids.push(serverFolder.id);
      }
    });
  }

  return request;
}

// 调用批量操作接口
async function callBatchAPI(batchRequest) {
  console.log('调用 /api/sync/batch 接口...');
  const response = await fetch(`${config.serverUrl}/api/sync/batch`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': config.apiKey
    },
    body: JSON.stringify(batchRequest)
  });

  console.log('后端响应状态:', response.status, response.ok);

  if (!response.ok) {
    const errorText = await response.text();
    console.error('后端API错误响应:', errorText);
    throw new Error(`后端API错误: ${response.status}, ${errorText}`);
  }

  const result = await response.json();
  console.log('后端同步结果:', result);
  return result;
}

// 收集 parentId 下所有子孙文件夹（不含 parentId 自身）
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

// 收集 parentId 下所有子孙文件夹的 id 集合（含自身），用于限定书签范围
function collectScopeFolderIds(parentId, allFolders) {
  const ids = collectDescendantFolderIds(parentId, allFolders);
  if (parentId !== null) ids.add(parentId);
  return ids;
}

// 同步到后端（使用新的 /api/sync/batch 接口）
async function syncToBackend(edgeBookmarks) {
  try {
    console.log('syncToBackend 调用');

    // 检查 API Key
    if (!config.apiKey) {
      console.error('未配置 API Key');
      throw new Error('请先配置 API Key');
    }

    // 1. 获取服务器端现有数据
    console.log('获取服务器端现有数据...');
    const serverBookmarks = await fetchServerBookmarks();
    const serverFolders = await fetchServerFolders();

    // 2. 确定应用侧目标父目录
    let targetParentId = null;
    if (config.appFolderMode === 'select' && config.appFolderId) {
      targetParentId = parseInt(config.appFolderId, 10);
      console.log('同步到应用指定文件夹 ID:', targetParentId);
    }

    // 3. 将原始 Edge 书签树转为同步格式
    //    指定了浏览器目录时：edgeBookmarks = [folderNode]，只遍历该目录的子节点，
    //    子节点直接挂在 targetParentId 下，edge_parent_id 初始为 null。
    //    未指定目录时：直接遍历所有根节点。
    let nodesToSync;
    if (config.edgeFolderMode === 'select' && config.edgeFolderId && edgeBookmarks.length === 1 && !edgeBookmarks[0].url) {
      // 指定目录：只取该文件夹的子节点
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

    // 3. 第一步：按层级分批同步文件夹（每层用真实服务器 ID 作 parent_id）
    console.log('=== 第一步：按层级同步文件夹 ===');

    // tempId -> 服务器真实 ID 的映射，贯穿整个文件夹同步过程
    const tempIdToServerId = new Map();

    // 按深度分组：depth 0 = 根文件夹，depth 1 = 一级子文件夹，以此类推
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
        // 父文件夹的真实服务器 ID（此时上一层已处理完，映射里一定有）
        let parentServerId = null;
        if (edgeFolder.edge_parent_id) {
          parentServerId = tempIdToServerId.get(edgeFolder.edge_parent_id) || null;
        } else if (targetParentId) {
          parentServerId = targetParentId;
        }

        // 检查服务器上是否已存在同名文件夹
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
          // 已存在的文件夹直接记入映射
          tempIdToServerId.set(edgeFolder.temp_id, existingFolder.id);
          console.log(`文件夹映射(已存在): temp_${edgeFolder.temp_id} -> server_${existingFolder.id} (${edgeFolder.title})`);
        } else {
          layerRequest.create.folders.push({
            parent_id: parentServerId,
            title: edgeFolder.title,
            position: edgeFolder.position,
            _temp_id: edgeFolder.temp_id  // 仅用于本地匹配，不发送给后端
          });
        }
      });

      // Replace 模式：只删除目标范围内（targetParentId 直接子层）Edge 已不存在的文件夹
      if (depth === maxDepth && config.syncMode === 'replace') {
        // Edge 侧本层文件夹标题集合（全量，用于快速查找）
        const edgeFolderTitles = new Set(edgeData.folders.map(f => f.title));

        // 确定服务器侧需要比对的文件夹范围：只取 targetParentId 下的直接子文件夹
        // 如果没有指定目标目录，则比对 parent_id 为 null 的顶层文件夹
        const scopeParentId = targetParentId;
        serverFolders.forEach(serverFolder => {
          if (serverFolder.parent_id === scopeParentId && !edgeFolderTitles.has(serverFolder.title)) {
            layerRequest.delete.folder_ids.push(serverFolder.id);
          }
        });
      }

      // 发送前去掉本地用的 _temp_id 字段
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
        console.log(`第 ${depth} 层文件夹请求:`, {
          create: sendRequest.create.folders.length,
          update: sendRequest.update.folders.length,
          delete: sendRequest.delete.folder_ids.length
        });

        const layerResult = await callBatchAPI(sendRequest);

        // 新创建的文件夹建立映射
        const createList = layerRequest.create.folders; // 含 _temp_id
        layerResult.created?.folders?.forEach((folder, index) => {
          if (index < createList.length) {
            const tempId = createList[index]._temp_id;
            tempIdToServerId.set(tempId, folder.id);
            console.log(`文件夹映射(创建): temp_${tempId} -> server_${folder.id} (${folder.title})`);
          }
        });
      }
    }

    console.log('文件夹 ID 映射:', Array.from(tempIdToServerId.entries()));

    // 5. 第二步：同步书签（使用正确的文件夹 ID 映射）
    console.log('=== 第二步：同步书签 ===');
    const bookmarkBatchRequest = generateBookmarkBatchRequest(
      edgeData,
      serverBookmarks,
      serverFolders,
      tempIdToServerId,
      targetParentId,
      config.syncMode
    );

    console.log('书签批量操作请求:', {
      create: bookmarkBatchRequest.create.bookmarks.length,
      update: bookmarkBatchRequest.update.bookmarks.length,
      delete: bookmarkBatchRequest.delete.bookmark_ids.length
    });

    const bookmarkResult = await callBatchAPI(bookmarkBatchRequest);

    // 6. 合并结果
    const result = {
      created: {
        folders: [],  // 文件夹已在分层循环中处理
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
      errors: [
        ...(bookmarkResult.errors || [])
      ]
    };

    // 更新统计信息
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

// 获取应用中的文件夹列表（使用新的 /api/sync/folders 接口）
async function getAppFolders() {
  try {
    console.log('开始获取应用文件夹列表...');
    
    // 检查 API Key
    if (!config.apiKey) {
      console.error('未配置 API Key');
      return [];
    }
    
    const response = await fetch(`${config.serverUrl}/api/sync/folders`, {
      headers: {
        'X-API-Key': config.apiKey
      }
    });
    console.log('应用文件夹API响应状态:', response.status);
    
    if (!response.ok) {
      const errorText = await response.text();
      console.error('获取应用文件夹失败:', errorText);
      throw new Error(`获取应用文件夹失败: ${response.status}, ${errorText}`);
    }
    
    const data = await response.json();
    const folders = data.folders || [];
    console.log('获取到应用文件夹数量:', folders.length);
    
    // 构建文件夹路径（扁平化列表显示完整路径）
    const folderMap = new Map();
    folders.forEach(folder => {
      folderMap.set(folder.id, folder);
    });
    
    // 构建完整路径
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
    
    const foldersWithPath = folders.map(folder => ({
      id: folder.id,
      title: buildFullPath(folder)
    }));
    
    console.log('提取到的文件夹列表:', foldersWithPath);
    return foldersWithPath;
  } catch (error) {
    console.error('获取应用文件夹失败:', error);
    return [];
  }
}

async function manualSync() {
  console.log('开始手动同步');
  try {
    const result = await syncBookmarks();
    console.log('手动同步完成:', result);
    return { status: 'success', result };
  } catch (error) {
    console.error('手动同步失败:', error);
    return { status: 'error', error: error.message };
  }
}

async function updateConfig(newConfig) {
  config = { ...config, ...newConfig };
  await chrome.storage.local.set({ config });
  
  chrome.alarms.clear('syncBookmarks');
  registerSyncAlarm();
  
  console.log('配置已更新:', config);

  // 广播配置更新通知，让已打开的页面（如 sync-window）实时刷新
  chrome.runtime.sendMessage({ action: 'configUpdated', config }).catch(() => {});
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  console.log('收到消息:', message.action, message);
  
  switch (message.action) {
    case 'sync':
      console.log('处理 sync 消息');
      manualSync().then((result) => {
        console.log('同步完成，发送响应:', result);
        sendResponse(result);
      }).catch((error) => {
        console.error('同步失败，发送错误响应:', error);
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'getConfig':
      console.log('处理 getConfig 消息');
      console.log('返回配置:', config);
      sendResponse({ config });
      break;
    case 'updateConfig':
      console.log('处理 updateConfig 消息:', message.config);
      updateConfig(message.config).then(() => {
        console.log('updateConfig 成功');
        sendResponse({ status: 'success' });
      }).catch((error) => {
        console.error('updateConfig 失败:', error);
        sendResponse({ status: 'error', error: error.message });
      });
      return true;
    case 'getSyncStatus':
      console.log('处理 getSyncStatus 消息');
      sendResponse({
        syncInProgress,
        syncProgress,
        lastSyncTime: config.lastSyncTime,
        lastError: config.lastError
      });
      break;
    case 'getAppFolders':
      console.log('处理 getAppFolders 消息');
      getAppFolders().then(folders => {
        console.log('getAppFolders 成功，返回文件夹数量:', folders.length);
        sendResponse({ folders });
      }).catch(error => {
        console.error('getAppFolders 失败:', error);
        sendResponse({ folders: [], error: error.message });
      });
      return true;
    case 'getEdgeFolders':
      console.log('处理 getEdgeFolders 消息');
      getEdgeFolders().then(folders => {
        console.log('getEdgeFolders 成功，返回文件夹数量:', folders.length);
        sendResponse({ folders });
      }).catch(error => {
        console.error('getEdgeFolders 失败:', error);
        sendResponse({ folders: [], error: error.message });
      });
      return true;
    case 'openSyncWindow':
      console.log('处理 openSyncWindow 消息');
      try {
        openSyncWindow();
        sendResponse({ status: 'success' });
      } catch (error) {
        console.error('打开同步窗口失败:', error);
        sendResponse({ status: 'error', error: error.message });
      }
      break;
    default:
      console.log('未知消息:', message.action);
      sendResponse({ status: 'unknown action' });
      break;
  }
});

// 广播同步进度（通过 runtime.sendMessage 让所有扩展页面都能收到）
function sendProgressToWindow() {
  chrome.runtime.sendMessage({
    action: 'syncProgress',
    progress: { ...syncProgress }
  }, () => {
    if (chrome.runtime.lastError) {
      // 没有接收者时会报错，属于正常情况，忽略
    }
  });
}

// 广播同步完成通知
function sendSyncCompleteToWindow() {
  chrome.runtime.sendMessage({
    action: 'syncComplete',
    progress: { ...syncProgress }
  }, () => {
    if (chrome.runtime.lastError) {
      // 忽略无接收者错误
    }
  });
}

// 广播同步错误通知
function sendSyncErrorToWindow(errorMsg) {
  chrome.runtime.sendMessage({
    action: 'syncError',
    error: errorMsg
  }, () => {
    if (chrome.runtime.lastError) {
      // 忽略无接收者错误
    }
  });
}

// 初始化
initialize();

// 打开同步窗口
function openSyncWindow() {
  console.log('开始打开同步窗口...');
  console.log('URL:', chrome.runtime.getURL('sync-window.html'));
  
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
        console.log('同步窗口已打开:', window.id);
      } else {
        console.warn('窗口创建成功但未返回窗口对象');
      }
    });
  } catch (error) {
    console.error('打开同步窗口异常:', error);
  }
}

// 监听安装事件
chrome.runtime.onInstalled.addListener((details) => {
  console.log('插件安装/更新:', details);
  
  if (details.reason === 'install') {
    console.log('插件首次安装');
    chrome.tabs.create({ url: chrome.runtime.getURL('sync-window.html') });
  } else if (details.reason === 'update') {
    console.log('插件已更新到版本:', chrome.runtime.getManifest().version);
  }
});

// 点击插件图标时，打开或切换到同步页面标签
chrome.action.onClicked.addListener(() => {
  const syncWindowUrl = chrome.runtime.getURL('sync-window.html');
  // 查询所有标签页，找到已打开的同步页面
  chrome.tabs.query({}, (tabs) => {
    const existing = tabs.find(t => t.url === syncWindowUrl);
    if (existing) {
      // 已有标签页则直接激活
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
