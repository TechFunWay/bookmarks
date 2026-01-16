// 配置默认值
const DEFAULT_CONFIG = {
  serverUrl: 'http://localhost:8901',
  syncInterval: 5, // 分钟
  syncDirection: 'bidirectional', // unidirectional, bidirectional
  firstSyncMode: 'merge', // merge, replace
  lastSyncTime: null
};

// 全局变量
let config = { ...DEFAULT_CONFIG };
let syncInProgress = false;

// 初始化插件
async function initialize() {
  console.log('书签同步助手初始化...');
  
  // 加载配置
  await loadConfig();
  
  // 注册同步定时器
  registerSyncAlarm();
  
  // 注册书签变更监听器
  registerBookmarkListeners();
  
  // 首次同步
  await syncBookmarks();
  
  console.log('书签同步助手初始化完成');
}

// 加载配置
async function loadConfig() {
  try {
    const storedConfig = await chrome.storage.local.get('config');
    if (storedConfig.config) {
      config = { ...DEFAULT_CONFIG, ...storedConfig.config };
      console.log('配置加载成功:', config);
    } else {
      // 保存默认配置
      await chrome.storage.local.set({ config });
      console.log('使用默认配置:', config);
    }
  } catch (error) {
    console.error('加载配置失败:', error);
  }
}

// 注册同步定时器
function registerSyncAlarm() {
  chrome.alarms.create('syncBookmarks', {
    periodInMinutes: config.syncInterval
  });
  
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === 'syncBookmarks') {
      console.log('执行定时同步');
      syncBookmarks();
    }
  });
  
  console.log(`同步定时器已注册，间隔: ${config.syncInterval}分钟`);
}

// 注册书签变更监听器
function registerBookmarkListeners() {
  // 监听书签创建
  chrome.bookmarks.onCreated.addListener((id, bookmark) => {
    console.log('书签创建:', id, bookmark);
    // 延迟执行，确保书签树完整
    setTimeout(() => {
      syncBookmarks();
    }, 1000);
  });
  
  // 监听书签删除
  chrome.bookmarks.onRemoved.addListener((id, removeInfo) => {
    console.log('书签删除:', id, removeInfo);
    syncBookmarks();
  });
  
  // 监听书签修改
  chrome.bookmarks.onChanged.addListener((id, changeInfo) => {
    console.log('书签修改:', id, changeInfo);
    syncBookmarks();
  });
  
  // 监听书签移动
  chrome.bookmarks.onMoved.addListener((id, moveInfo) => {
    console.log('书签移动:', id, moveInfo);
    syncBookmarks();
  });
  
  // 监听书签重排序
  chrome.bookmarks.onChildrenReordered.addListener((id, reorderInfo) => {
    console.log('书签重排序:', id, reorderInfo);
    syncBookmarks();
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
  
  try {
    console.log('开始同步书签...');
    
    // 获取Edge浏览器书签
    const edgeBookmarks = await getEdgeBookmarks();
    console.log('获取到Edge书签数量:', countBookmarks(edgeBookmarks));
    
    // 转换为后端API格式
    const apiFormatBookmarks = convertToApiFormat(edgeBookmarks);
    console.log('转换为API格式完成');
    
    // 同步到后端
    await syncToBackend(apiFormatBookmarks);
    console.log('同步到后端完成');
    
    // 如果是双向同步，从后端获取并同步到浏览器
    if (config.syncDirection === 'bidirectional') {
      await syncFromBackend();
      console.log('从后端同步到浏览器完成');
    }
    
    // 更新最后同步时间
    config.lastSyncTime = new Date().toISOString();
    await chrome.storage.local.set({ config });
    
    console.log('书签同步完成');
  } catch (error) {
    console.error('同步书签失败:', error);
  } finally {
    syncInProgress = false;
  }
}

// 获取Edge浏览器书签
function getEdgeBookmarks() {
  return new Promise((resolve, reject) => {
    chrome.bookmarks.getTree((tree) => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve(tree);
      }
    });
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
      // 可以添加favicon_url的处理
    }
    
    if (edgeNode.children && edgeNode.children.length > 0) {
      node.children = edgeNode.children.map(child => traverse(child, node.id));
    }
    
    return node;
  }
  
  edgeBookmarks.forEach(root => {
    // 跳过根节点，只处理子节点
    if (root.children) {
      root.children.forEach(child => {
        result.push(traverse(child));
      });
    }
  });
  
  return result;
}

// 同步到后端
async function syncToBackend(bookmarks) {
  try {
    const response = await fetch(`${config.serverUrl}/api/import`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        bookmarks,
        mode: config.firstSyncMode,
        parent_id: null
      })
    });
    
    if (!response.ok) {
      throw new Error(`后端API错误: ${response.status}`);
    }
    
    const result = await response.json();
    console.log('后端同步结果:', result);
    return result;
  } catch (error) {
    console.error('同步到后端失败:', error);
    // 只记录错误，不重新抛出异常，确保同步过程继续
    return null;
  }
}

// 从后端同步到浏览器
async function syncFromBackend() {
  try {
    // 获取后端书签
    const backendBookmarks = await getBackendBookmarks();
    if (!backendBookmarks) {
      console.log('后端书签获取失败，跳过同步到浏览器');
      return;
    }
    console.log('获取到后端书签数量:', countApiFormatBookmarks(backendBookmarks));
    
    // 同步到浏览器
    await syncToBrowser(backendBookmarks);
    console.log('同步到浏览器完成');
  } catch (error) {
    console.error('从后端同步失败:', error);
    // 只记录错误，不重新抛出异常，确保同步过程继续
  }
}

// 获取后端书签
async function getBackendBookmarks() {
  try {
    const response = await fetch(`${config.serverUrl}/api/tree`);
    
    if (!response.ok) {
      throw new Error(`后端API错误: ${response.status}`);
    }
    
    const result = await response.json();
    console.log('后端书签获取成功');
    return result;
  } catch (error) {
    console.error('获取后端书签失败:', error);
    // 只记录错误，不重新抛出异常，确保同步过程继续
    return null;
  }
}

// 计算API格式书签数量
function countApiFormatBookmarks(bookmarks) {
  let count = 0;
  
  function traverse(node) {
    if (node.type === 'bookmark') {
      count++;
    }
    if (node.children) {
      node.children.forEach(child => traverse(child));
    }
  }
  
  bookmarks.forEach(root => traverse(root));
  return count;
}

// 同步到浏览器
async function syncToBrowser(backendBookmarks) {
  try {
    console.log('开始同步后端书签到浏览器...');
    
    // 获取浏览器当前书签
    const browserBookmarks = await getBrowserBookmarks();
    console.log('获取到浏览器书签数量:', countBookmarks(browserBookmarks));
    
    // 比较差异
    const diff = compareBookmarks(backendBookmarks, browserBookmarks);
    console.log('同步差异分析:', diff);
    
    // 执行同步操作
    await executeSyncOperations(diff);
    console.log('浏览器同步操作执行完成');
    
  } catch (error) {
    console.error('同步到浏览器失败:', error);
    // 只记录错误，不重新抛出异常，确保同步过程继续
  }
}

// 获取浏览器书签
function getBrowserBookmarks() {
  return new Promise((resolve, reject) => {
    chrome.bookmarks.getTree((tree) => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve(tree);
      }
    });
  });
}

// 比较后端和浏览器书签的差异
function compareBookmarks(backendBookmarks, browserBookmarks) {
  const diff = {
    create: [],    // 需要创建的项
    update: [],    // 需要更新的项
    delete: []     // 需要删除的项
  };
  
  // 构建后端书签映射
  const backendMap = new Map();
  buildBookmarkMap(backendBookmarks, backendMap, 'backend');
  
  // 构建浏览器书签映射
  const browserMap = new Map();
  buildBookmarkMap(browserBookmarks, browserMap, 'browser');
  
  console.log(`后端书签数量: ${backendMap.size}, 浏览器书签数量: ${browserMap.size}`);
  
  // 分析需要创建和更新的项
  for (const [key, backendItem] of backendMap.entries()) {
    if (!browserMap.has(key)) {
      // 后端有但浏览器没有，需要创建
      diff.create.push(backendItem);
      console.log(`需要创建: ${backendItem.title}`);
    } else {
      // 两边都有，检查是否需要更新
      const browserItem = browserMap.get(key);
      if (needsUpdate(backendItem, browserItem)) {
        diff.update.push({ 
          backend: backendItem, 
          browser: browserItem 
        });
        console.log(`需要更新: ${backendItem.title}`);
      }
      // 从浏览器映射中移除，剩下的就是需要删除的
      browserMap.delete(key);
    }
  }
  
  // 剩下的浏览器书签就是需要删除的
  for (const [key, browserItem] of browserMap.entries()) {
    diff.delete.push(browserItem);
    console.log(`需要删除: ${browserItem.title}`);
  }
  
  console.log(`同步差异: 创建=${diff.create.length}, 更新=${diff.update.length}, 删除=${diff.delete.length}`);
  return diff;
}

// 构建书签映射
function buildBookmarkMap(bookmarks, map, source) {
  function traverse(nodes, parentPath = '') {
    nodes.forEach((node, index) => {
      let key;
      if (node.url) {
        // 书签使用URL和标题作为key
        const url = node.url;
        const title = node.title;
        key = `bookmark_${url}_${title}`;
      } else {
        // 文件夹使用路径和标题作为key
        const path = parentPath ? `${parentPath}/${node.title}` : node.title;
        key = `folder_${path}`;
      }
      
      map.set(key, {
        ...node,
        source,
        path: parentPath,
        index
      });
      
      // 递归处理子节点
      if (node.children && node.children.length > 0) {
        const childPath = parentPath ? `${parentPath}/${node.title}` : node.title;
        traverse(node.children, childPath);
      }
    });
  }
  
  traverse(bookmarks);
}

// 检查是否需要更新
function needsUpdate(backendItem, browserItem) {
  // 比较书签属性
  if (backendItem.url) {
    const backendUrl = backendItem.url;
    const browserUrl = browserItem.url;
    const backendTitle = backendItem.title;
    const browserTitle = browserItem.title;
    
    return backendUrl !== browserUrl || backendTitle !== browserTitle;
  }
  
  // 比较文件夹属性
  if (!backendItem.url) {
    return backendItem.title !== browserItem.title;
  }
  
  return false;
}

// 执行同步操作
async function executeSyncOperations(diff) {
  console.log(`执行同步操作: 创建=${diff.create.length}, 更新=${diff.update.length}, 删除=${diff.delete.length}`);
  
  // 性能优化：批量处理操作，避免浏览器卡顿
  const batchSize = 10; // 每批处理的项数
  const delayMs = 100; // 批次之间的延迟
  
  // 先删除不需要的项（批量处理）
  await processBatch(diff.delete, deleteBrowserItem, batchSize, delayMs);
  
  // 再创建新项（批量处理）
  await processBatch(diff.create, createBrowserItem, batchSize, delayMs);
  
  // 最后更新现有项（批量处理）
  await processBatch(diff.update, async (item) => {
    await updateBrowserItem(item.backend, item.browser);
  }, batchSize, delayMs);
}

// 批量处理操作
async function processBatch(items, processor, batchSize, delayMs) {
  if (!items || items.length === 0) {
    return;
  }
  
  console.log(`开始批量处理 ${items.length} 项，每批 ${batchSize} 项`);
  
  for (let i = 0; i < items.length; i += batchSize) {
    const batch = items.slice(i, i + batchSize);
    console.log(`处理批次 ${Math.floor(i / batchSize) + 1}: ${batch.length} 项`);
    
    // 并行处理批次内的所有项
    const promises = batch.map(processor);
    await Promise.all(promises);
    
    // 批次之间添加延迟，避免浏览器卡顿
    if (i + batchSize < items.length) {
      console.log(`批次处理完成，等待 ${delayMs}ms`);
      await new Promise(resolve => setTimeout(resolve, delayMs));
    }
  }
  
  console.log(`批量处理完成，共处理 ${items.length} 项`);
}

// 删除浏览器项
async function deleteBrowserItem(item) {
  try {
    if (item.id) {
      await chrome.bookmarks.removeTree(item.id);
      console.log('删除浏览器项:', item.title);
    }
  } catch (error) {
    console.error('删除浏览器项失败:', error);
  }
}

// 创建浏览器项
async function createBrowserItem(item) {
  try {
    if (item.type === 'bookmark' || item.url) {
      // 创建书签
      const parentId = await findOrCreateParentFolder(item.path);
      const bookmark = {
        parentId,
        title: item.title,
        url: item.url || item.URL
      };
      const created = await chrome.bookmarks.create(bookmark);
      console.log('创建浏览器书签:', item.title);
      return created;
    } else {
      // 创建文件夹
      const parentId = await findOrCreateParentFolder(item.path);
      const folder = {
        parentId,
        title: item.title
      };
      const created = await chrome.bookmarks.create(folder);
      console.log('创建浏览器文件夹:', item.title);
      return created;
    }
  } catch (error) {
    console.error('创建浏览器项失败:', error);
    throw error;
  }
}

// 更新浏览器项
async function updateBrowserItem(backendItem, browserItem) {
  try {
    if (browserItem.id) {
      const changes = {};
      
      if (backendItem.title !== browserItem.title) {
        changes.title = backendItem.title;
      }
      
      if ((backendItem.url || backendItem.URL) !== (browserItem.url || browserItem.URL)) {
        changes.url = backendItem.url || backendItem.URL;
      }
      
      if (Object.keys(changes).length > 0) {
        await chrome.bookmarks.update(browserItem.id, changes);
        console.log('更新浏览器项:', browserItem.title);
      }
    }
  } catch (error) {
    console.error('更新浏览器项失败:', error);
  }
}

// 查找或创建父文件夹
async function findOrCreateParentFolder(path) {
  try {
    if (!path) {
      // 根文件夹
      return '1'; // 书签栏的ID
    }
    
    const pathParts = path.split('/');
    let parentId = '1'; // 从书签栏开始
    
    for (const folderName of pathParts) {
      const folderId = await findFolderByName(parentId, folderName);
      if (folderId) {
        parentId = folderId;
      } else {
        // 创建不存在的文件夹
        const folder = {
          parentId,
          title: folderName
        };
        const created = await chrome.bookmarks.create(folder);
        parentId = created.id;
      }
    }
    
    return parentId;
  } catch (error) {
    console.error('查找或创建父文件夹失败:', error);
    return '1'; // 失败时使用根文件夹
  }
}

// 根据名称查找文件夹
async function findFolderByName(parentId, name) {
  return new Promise((resolve) => {
    chrome.bookmarks.getChildren(parentId, (children) => {
      for (const child of children) {
        if (child.title === name && !child.url) {
          resolve(child.id);
          return;
        }
      }
      resolve(null);
    });
  });
}

// 保存原始的chrome.bookmarks方法
const originalBookmarksApi = {
  create: chrome.bookmarks.create,
  update: chrome.bookmarks.update,
  removeTree: chrome.bookmarks.removeTree,
  getChildren: chrome.bookmarks.getChildren
};

// 扩展chrome.bookmarks API为Promise形式
chrome.bookmarks.create = function(createInfo) {
  return new Promise((resolve, reject) => {
    originalBookmarksApi.create(createInfo, (result) => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve(result);
      }
    });
  });
};

chrome.bookmarks.update = function(id, changes) {
  return new Promise((resolve, reject) => {
    originalBookmarksApi.update(id, changes, (result) => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve(result);
      }
    });
  });
};

chrome.bookmarks.removeTree = function(id) {
  return new Promise((resolve, reject) => {
    originalBookmarksApi.removeTree(id, () => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve();
      }
    });
  });
};

chrome.bookmarks.getChildren = function(id) {
  return new Promise((resolve, reject) => {
    originalBookmarksApi.getChildren(id, (children) => {
      if (chrome.runtime.lastError) {
        reject(chrome.runtime.lastError);
      } else {
        resolve(children);
      }
    });
  });
};

// 手动触发同步
function manualSync() {
  console.log('手动触发同步');
  syncBookmarks();
}

// 更新配置
async function updateConfig(newConfig) {
  config = { ...config, ...newConfig };
  await chrome.storage.local.set({ config });
  
  // 重新注册同步定时器
  chrome.alarms.clear('syncBookmarks');
  registerSyncAlarm();
  
  console.log('配置已更新:', config);
}

// 导出功能
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  switch (message.action) {
    case 'sync':
      manualSync();
      sendResponse({ status: 'syncing' });
      break;
    case 'getConfig':
      sendResponse({ config });
      break;
    case 'updateConfig':
      updateConfig(message.config).then(() => {
        sendResponse({ status: 'success' });
      }).catch((error) => {
        sendResponse({ status: 'error', error: error.message });
      });
      return true; // 异步响应
    case 'getSyncStatus':
      sendResponse({ 
        syncInProgress,
        lastSyncTime: config.lastSyncTime
      });
      break;
    default:
      sendResponse({ status: 'unknown action' });
  }
});

// 初始化
initialize();

// 监听安装事件
chrome.runtime.onInstalled.addListener((details) => {
  console.log('插件安装/更新:', details);
  
  if (details.reason === 'install') {
    // 首次安装
    console.log('插件首次安装');
    // 可以打开选项页面引导用户配置
    chrome.tabs.create({ url: 'options.html' });
  } else if (details.reason === 'update') {
    // 插件更新
    console.log('插件已更新到版本:', chrome.runtime.getManifest().version);
  }
});

// 监听启动事件
chrome.runtime.onStartup.addListener(() => {
  console.log('浏览器启动，初始化书签同步助手');
  initialize();
});
