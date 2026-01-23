// 配置默认值
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

// 全局变量
let config = { ...DEFAULT_CONFIG };
let syncInProgress = false;

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
  
  try {
    console.log('开始同步书签...');
    console.log('同步配置:', {
      enableAutoSync: config.enableAutoSync,
      syncScope: config.syncScope,
      syncMode: config.syncMode,
      edgeFolderId: config.edgeFolderId,
      appFolderId: config.appFolderId
    });
    
    let edgeBookmarks;
    if (config.syncScope === 'edge-to-app' && config.edgeFolderId) {
      console.log('从Edge指定目录同步，Edge文件夹 ID:', config.edgeFolderId);
      edgeBookmarks = await getEdgeBookmarks(config.edgeFolderId);
    } else {
      console.log('同步全部书签');
      edgeBookmarks = await getEdgeBookmarks();
    }
    console.log('获取到Edge书签数量:', countBookmarks(edgeBookmarks));
    
    const apiFormatBookmarks = convertToApiFormat(edgeBookmarks);
    console.log('转换为API格式完成，节点数:', apiFormatBookmarks.length);
    
    if (apiFormatBookmarks.length === 0) {
      console.warn('没有书签需要同步');
    }
    
    const syncResult = await syncToBackend(apiFormatBookmarks);
    console.log('同步到后端完成，结果:', syncResult);
    
    if (syncResult && syncResult.stats) {
      config.syncResult = syncResult.stats;
      console.log('保存同步结果:', syncResult.stats);
    }
    
    config.lastSyncTime = new Date().toISOString();
    await chrome.storage.local.set({ config });
    
    console.log('书签同步完成');
  } catch (error) {
    console.error('同步书签失败:', error);
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

// 同步到后端
async function syncToBackend(bookmarks) {
  try {
    console.log('syncToBackend 调用，书签数量:', bookmarks.length);
    console.log('第一个书签示例:', JSON.stringify(bookmarks[0], null, 2));
    
    let parentId = null;
    if (config.syncScope === 'folder' && config.appFolderId) {
      parentId = parseInt(config.appFolderId, 10);
      console.log('同步到后端，目标文件夹 ID:', parentId);
    } else if (config.syncScope === 'edge-to-app' && config.appFolderId) {
      parentId = parseInt(config.appFolderId, 10);
      console.log('从Edge指定目录同步到应用指定目录，目标文件夹 ID:', parentId);
    } else {
      console.log('同步到后端，目标文件夹: 根目录');
    }
    
    if (bookmarks.length === 0) {
      console.warn('没有书签需要同步到后端');
      return null;
    }
    
    const requestBody = {
      bookmarks,
      mode: config.syncMode,
      parent_id: parentId
    };
    console.log('发送到后端的请求体:', JSON.stringify(requestBody, null, 2));
    
    const response = await fetch(`${config.serverUrl}/api/import`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(requestBody)
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
  } catch (error) {
    console.error('同步到后端失败:', error);
    return null;
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

async function getAppFolders() {
  try {
    console.log('开始获取应用文件夹，服务器地址:', config.serverUrl);
    const response = await fetch(`${config.serverUrl}/api/tree`);
    console.log('应用文件夹API响应状态:', response.status, response.ok);
    
    if (!response.ok) {
      const errorText = await response.text();
      console.error('应用文件夹API错误响应:', errorText);
      throw new Error(`后端API错误: ${response.status}, ${errorText}`);
    }
    const bookmarks = await response.json();
    console.log('应用文件夹API返回数据:', bookmarks);
    const folders = [];
    
    function traverse(node, path = '') {
      if (node.type === 'folder') {
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
    
    bookmarks.forEach(root => traverse(root));
    console.log('应用文件夹遍历完成，共', folders.length, '个文件夹');
    return folders;
  } catch (error) {
    console.error('获取应用文件夹失败:', error);
    return [];
  }
}

async function manualSync() {
  console.log('手动触发同步');
  try {
    await syncBookmarks();
    console.log('手动同步完成');
  } catch (error) {
    console.error('手动同步失败:', error);
    throw error;
  }
}

async function updateConfig(newConfig) {
  config = { ...config, ...newConfig };
  await chrome.storage.local.set({ config });
  
  chrome.alarms.clear('syncBookmarks');
  registerSyncAlarm();
  
  console.log('配置已更新:', config);
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  console.log('收到消息:', message.action, message);
  
  switch (message.action) {
    case 'sync':
      console.log('处理 sync 消息');
      manualSync().then(() => {
        console.log('同步完成，发送响应');
        sendResponse({ status: 'success' });
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
        lastSyncTime: config.lastSyncTime
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
    default:
      console.log('未知消息:', message.action);
      sendResponse({ status: 'unknown action' });
      break;
  }
});

// 初始化
initialize();

// 监听安装事件
chrome.runtime.onInstalled.addListener((details) => {
  console.log('插件安装/更新:', details);
  
  if (details.reason === 'install') {
    console.log('插件首次安装');
    chrome.tabs.create({ url: 'options.html' });
  } else if (details.reason === 'update') {
    console.log('插件已更新到版本:', chrome.runtime.getManifest().version);
  }
});

chrome.runtime.onStartup.addListener(() => {
  console.log('浏览器启动，初始化【网址收藏夹】同步助手');
  initialize();
});
