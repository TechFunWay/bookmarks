const { createApp } = Vue;

const BookmarkNode = {
  name: "BookmarkNode",
  props: {
    node: { type: Object, required: true },
    level: { type: Number, default: 0 },
    selectedId: { type: Number, required: true },
    globalActionsVisible: { type: Boolean, default: false },
  },
  emits: ["add-folder", "add-bookmark", "edit", "delete", "select", "move", "context"],
  data() {
    return {
      collapsed: false, // 文件夹折叠状态
    };
  },
  computed: {
    isFolder() {
      return this.node.type === "folder";
    },
    isSelected() {
      return this.selectedId === this.node.id;
    },
    indentStyle() {
      return {
        paddingLeft: `${this.level * 16}px`,
      };
    },
    bookmarkCount() {
      // 计算当前文件夹及其子文件夹中的书签数量
      return this.countBookmarks(this.node);
    },
    hasChildren() {
      // 检查是否有子文件夹
      if (!this.node.children || this.node.children.length === 0) {
        return false;
      }
      // 检查是否至少有一个子文件夹
      for (const child of this.node.children) {
        if (child.type === 'folder') {
          return true;
        }
      }
      return false;
    },
  },
  methods: {
    countBookmarks(node) {
      // 递归计算书签数量
      let count = 0;
      if (node.type === "bookmark") {
        count = 1;
      } else if (node.type === "folder" && node.children && node.children.length) {
        for (const child of node.children) {
          count += this.countBookmarks(child);
        }
      }
      return count;
    },
    onSelect() {
      this.$emit("select", this.node);
    },
    onAddFolder() {
      this.$emit("add-folder", this.node);
    },
    onAddBookmark() {
      this.$emit("add-bookmark", this.node);
    },
    onEdit() {
      this.$emit("edit", this.node);
    },
    onDelete() {
      this.$emit("delete", this.node);
    },
    onMove(direction) {
      this.$emit("move", { node: this.node, direction });
    },
    onContextMenu(event) {
      if (this.isFolder) {
        // 防止右键时触发选择事件
        this.$emit("context", {
          node: this.node,
          x: event.clientX,
          y: event.clientY,
        });
      }
    },
    startLongPress(event) {
      // 长按事件，在移动端模拟右键菜单
      this.longPressTimer = setTimeout(() => {
        if (this.isFolder) {
          this.$emit("context", {
            node: this.node,
            x: event.touches[0].clientX,
            y: event.touches[0].clientY,
          });
        }
      }, 500); // 500ms长按
    },
    endLongPress() {
      // 清除长按定时器
      if (this.longPressTimer) {
        clearTimeout(this.longPressTimer);
        this.longPressTimer = null;
      }
    },
    toggleCollapse(event) {
      // 切换文件夹折叠状态
      event.stopPropagation(); // 防止触发select事件
      this.collapsed = !this.collapsed;
    },
  },
  template: `
    <li class="tree-item" v-if="isFolder">
      <div :class="['tree-row', { selected: isSelected }]" :style="indentStyle" @contextmenu.prevent="onContextMenu" @touchstart="startLongPress" @touchend="endLongPress" @touchcancel="endLongPress">
        <button class="node-main" type="button" @click="onSelect">
            <span class="node-icon" @click="hasChildren && toggleCollapse($event)">
              <template v-if="isFolder">
                <span v-if="hasChildren">
                  {{ collapsed ? '▶️' : '▼️' }}
                </span>
                <span v-else>
                  {{ level === 0 ? '📁' : '🗂️' }}
                </span>
              </template>
            </span>
            <span class="node-title" :title="node.title">
              {{ node.title }}
              <span class="node-count" v-if="isFolder">({{ bookmarkCount }})</span>
            </span>
          </button>
        <div class="tree-actions inline" v-if="globalActionsVisible">
          <button type="button" title="上移" @click="onMove('up')">
            <span class="action-icon">⬆️</span>
          </button>
          <button type="button" title="下移" @click="onMove('down')">
            <span class="action-icon">⬇️</span>
          </button>
          <button v-if="isFolder" type="button" title="添加子文件夹" @click="onAddFolder">
            <span class="action-icon">📂</span>
          </button>
          <button type="button" title="编辑" @click="onEdit">
            <span class="action-icon">✏️</span>
          </button>
          <button 
            type="button" 
            title="删除" 
            @click="onDelete"
          >
            <span class="action-icon">🗑️</span>
          </button>
        </div>
      </div>
      <ul v-if="hasChildren && !collapsed" class="children">
        <bookmark-node
          v-for="child in node.children"
          :key="child.id"
          :node="child"
          :level="level + 1"
          :selected-id="selectedId"
          :global-actions-visible="globalActionsVisible"
          :get-favicon-url="getFaviconUrl"
          @add-folder="$emit('add-folder', $event)"
          @add-bookmark="$emit('add-bookmark', $event)"
          @edit="$emit('edit', $event)"
          @delete="$emit('delete', $event)"
          @select="$emit('select', $event)"
          @move="$emit('move', $event)"
          @context="$emit('context', $event)"
        ></bookmark-node>
      </ul>
    </li>
  `,
};

const app = createApp({
  components: {
    BookmarkNode,
  },
  data() {
      return {
        currentUser: null,
        token: localStorage.getItem('token') || null,
        tree: [],
        loading: false,
        selectedNodeId: null,
        treeActionsVisible: false,
        version: '',
        contextMenu: {
          visible: false,
          x: 0,
          y: 0,
          nodeId: null,
        },
        longPressTimer: null,
        modal: {
          visible: false,
          type: "",
          parentId: null,
          nodeId: null,
          form: {
            title: "",
            url: "",
            favicon_url: "",
          },
        },
        metadataLoading: false,
        metadataError: "",
        toast: {
          visible: false,
          message: "",
          type: "success",
          timer: null,
        },
        confirmDialog: {
          visible: false,
          title: "确认操作",
          message: "",
          type: "warning",
          confirmText: "确认",
          callback: null
        },
        bookmarkEditMode: false,
        selectedBookmarks: new Set(),
        moveModal: {
          visible: false,
          targetParentId: null,
          nodeId: null,
        },
        bookmarkFolderSelectorVisible: false,
        selectedFolderId: null,
        rightClickNode:{},
        // 导入功能相关状态
        importDialog: {
          visible: false
        },
        importMode: 'merge', // 导入模式：merge 或 replace
        importFileInput: null,
        importParentId: null, // 导入JSON书签的父文件夹ID
        importFolderSelectorVisible: false, // 导入文件夹选择器可见状态
        // Edge导入功能相关状态
        edgeImportDialog: {
          visible: false
        },
        edgeImportMode: 'merge', // Edge导入模式：merge 或 replace
        edgeImportFileInput: null,
        edgeImportParentId: null, // 导入Edge书签的父文件夹ID
        edgeFolderSelectorVisible: false, // Edge导入文件夹选择器可见状态
        edgeImportFile: null, // 选中的Edge导入文件
        edgeConfirmImportVisible: false, // Edge导入确认对话框可见状态
        importFile: null, // 选中的JSON导入文件
        confirmImportVisible: false, // JSON导入确认对话框可见状态
        // 导出菜单状态
        exportMenuVisible: false,
        // 更新书签信息相关状态
        updateMetadataDialog: {
          visible: false,
          targetFolderId: null,
          targetFolderName: '',
          updating: false,
          currentIndex: 0,
          totalCount: 0,
          currentBookmarkTitle: ''
        },
        searchQuery: "",
        clearSearchBtnVisible: false,
        searchResultVisible: false,
        searchResultCount: 0,
        searchResults: [],
        isSearching: false,
        // 背景设置相关状态
        backgroundModal: {
          visible: false
        },
        backgroundSettings: {
          color: '',
          image: '',
          type: 'default', // default, color, image
          panelOpacity: 90 // 面板透明度，默认90%
        },
        // 保存原始背景设置，用于取消时恢复
        originalBackgroundSettings: null,
        // 预设背景颜色
        presetColors: [
          '#0c0c0c',
          '#1a1a2e',
          '#16213e',
          '#0f3460',
          '#533483',
          '#f8fafc',
          '#f1f5f9',
          '#e2e8f0',
          '#cbd5e1',
          '#94a3b8'
        ],
        // 用户下拉菜单状态
        userDropdownVisible: false,
        // 修改密码模态框状态
        changePasswordModal: {
          visible: false,
          oldPassword: '',
          newPassword: '',
          confirmPassword: '',
          loading: false,
          error: ''
        },
        // 每行显示数量设置
        itemsPerRow: 1,
        // 配置相关
        config: {
          showUrlInList: true, // 默认显示URL
          showFolderPath: true, // 默认显示文件夹路径
          showUpdatedAt: true, // 默认显示更新时间
          showFullTitle: true, // 默认显示完整标题（可换行）
          allowRegister: true // 默认允许注册
        },
        configModal: {
          visible: false
        },
        // 导出相关状态
        exporting: false,
        exportMessage: "",
        exportProgress: 0,
        totalExportItems: 0,
        exportResults: [],
        exportSuccessCount: 0,
        exportFailCount: 0,
        showExportDetails: false
      };
    },
  computed: {
    totalBookmarks() {
      // 计算所有书签的总数
      return this.collectBookmarks(this.tree).length;
    },
    selectedNode() {
      if (!this.selectedNodeId) return null;
      return this.findNodeById(this.selectedNodeId, this.tree);
    },
    contextNode() {
      if (!this.contextMenu.visible || this.contextMenu.nodeId === null) {
        return null;
      }
      return this.findNodeById(this.contextMenu.nodeId, this.tree);
    },
    contextMenuStyle() {
      if (!this.contextMenu.visible) {
        return {};
      }
      return {
        top: `${this.contextMenu.y}px`,
        left: `${this.contextMenu.x}px`,
      };
    },
    bookmarkList() {
      return this.collectBookmarks(this.tree);
    },
    displayBookmarks() {
      // 如果处于搜索模式，返回搜索结果
      if (this.isSearching) {
        return this.searchResults;
      }

      this.searchResultVisible = false;
      
      // 如果选择的是【所有网址】文件夹，显示所有书签
      if (this.selectedNodeId === 'all-bookmarks') {
        return this.bookmarkList;
      }
      
      const current = this.selectedNode;
      if (!current) {
        return this.bookmarkList;
      }
      if (current.type === "folder") {
        const ids = new Set(this.collectBookmarkIds(current));
        return this.bookmarkList.filter((item) => ids.has(item.id));
      }
      if (current.type === "bookmark") {
        return this.bookmarkList.filter((item) => item.id === current.id);
      }
      return this.bookmarkList;
    },
    listTitle() {
      // 如果处于搜索模式，显示搜索结果数量
      if (this.isSearching) {
        return `显示搜索结果（${this.searchResults.length} 个）`;
      }
      // 如果选择的是【所有网址】文件夹，显示对应标题
      if (this.selectedNodeId === 'all-bookmarks') {
        return "显示所有收藏的网址";
      }
      
      const current = this.selectedNode;
      if (!current) {
        return "显示全部收藏的网址";
      }
      if (current.type === "folder") {
        return `显示文件夹「${current.title}」及其子层级中的网址`;
      }
      return `已选网址「${current.title}」`;
    },
    modalTitle() {
      switch (this.modal.type) {
        case "add-folder":
          return "新建文件夹";
        case "add-bookmark":
          return "添加网址";
        case "edit-folder":
          return "编辑文件夹";
        case "edit-bookmark":
          return "编辑网址";
        default:
          return "";
      }
    },
  },
  beforeUnmount() {
    window.removeEventListener("scroll", this.hideContextMenu, true);
    window.removeEventListener("resize", this.hideContextMenu);
  },
  methods: {
    checkAuth() {
      const token = localStorage.getItem('token');
      if (!token) {
        window.location.href = 'login.html';
        return false;
      }
      this.token = token;
      this.loadCurrentUser();
      return true;
    },
    async loadCurrentUser() {
      try {
        const response = await fetch('/api/auth/me', {
          headers: {
            'Authorization': this.token
          }
        });
        if (response.ok) {
          const user = await response.json();
          this.currentUser = user;
        } else {
          localStorage.removeItem('token');
          localStorage.removeItem('currentUser');
          window.location.href = 'login.html';
        }
      } catch (error) {
        console.error('加载用户信息失败:', error);
        localStorage.removeItem('token');
        localStorage.removeItem('currentUser');
        window.location.href = 'login.html';
      }
    },
    async logout() {
      try {
        await fetch('/api/auth/logout', {
          headers: {
            'Authorization': this.token
          }
        });
      } catch (error) {
        console.error('登出失败:', error);
      } finally {
        localStorage.removeItem('token');
        localStorage.removeItem('currentUser');
        this.token = null;
        this.currentUser = null;
        window.location.href = 'login.html';
      }
    },
    toggleUserDropdown() {
      this.userDropdownVisible = !this.userDropdownVisible;
    },
    openChangePasswordModal() {
      this.userDropdownVisible = false;
      this.changePasswordModal.visible = true;
      this.changePasswordModal.oldPassword = '';
      this.changePasswordModal.newPassword = '';
      this.changePasswordModal.confirmPassword = '';
      this.changePasswordModal.error = '';
      this.changePasswordModal.loading = false;
    },
    openUserManagement() {
      this.userDropdownVisible = false;
      window.location.href = '/users.html';
    },
    closeChangePasswordModal() {
      this.changePasswordModal.visible = false;
      this.changePasswordModal.oldPassword = '';
      this.changePasswordModal.newPassword = '';
      this.changePasswordModal.confirmPassword = '';
      this.changePasswordModal.error = '';
      this.changePasswordModal.loading = false;
    },
    async handleChangePassword() {
      this.changePasswordModal.error = '';

      if (!this.changePasswordModal.oldPassword || !this.changePasswordModal.newPassword || !this.changePasswordModal.confirmPassword) {
        this.changePasswordModal.error = '请填写所有字段';
        return;
      }

      if (this.changePasswordModal.newPassword.length < 6) {
        this.changePasswordModal.error = '新密码长度不能少于6位';
        return;
      }

      if (this.changePasswordModal.newPassword !== this.changePasswordModal.confirmPassword) {
        this.changePasswordModal.error = '两次输入的新密码不一致';
        return;
      }

      if (this.changePasswordModal.oldPassword === this.changePasswordModal.newPassword) {
        this.changePasswordModal.error = '新密码不能与旧密码相同';
        return;
      }

      this.changePasswordModal.loading = true;

      try {
        const response = await fetch('/api/auth/change-password', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': this.token
          },
          body: JSON.stringify({
            old_password: this.changePasswordModal.oldPassword,
            new_password: this.changePasswordModal.newPassword
          })
        });

        const data = await response.json();

        if (response.ok) {
          this.showToast('密码修改成功，请重新登录', 'success');
          this.closeChangePasswordModal();
          setTimeout(() => {
            this.logout();
          }, 1500);
        } else {
          this.changePasswordModal.error = data.error || '密码修改失败';
        }
      } catch (error) {
        console.error('修改密码失败:', error);
        this.changePasswordModal.error = '网络错误，请稍后重试';
      } finally {
        this.changePasswordModal.loading = false;
      }
    },
    showFolderSelector() {
      this.selectedFolderId = this.modal.parentId || null;
      this.bookmarkFolderSelectorVisible = true;
    },
    selectFolder(folderId) {
      this.selectedFolderId = folderId;
      // 选择文件夹时退出搜索模式
      this.isSearching = false;
      this.searchResults = [];
    },
    confirmFolderSelection() {
      // 如果是普通书签文件夹选择器
      if (this.bookmarkFolderSelectorVisible) {
        this.modal.parentId = this.selectedFolderId;
        this.bookmarkFolderSelectorVisible = false;
      } 
      // 如果是导入文件夹选择器
      else if (this.importFolderSelectorVisible) {
        this.importFolderSelectorVisible = false;
      }
    },
    cancelFolderSelection() {
      // 关闭所有可能的文件夹选择器
      this.bookmarkFolderSelectorVisible = false;
      this.importFolderSelectorVisible = false;
    },
    showImportFolderSelector() {
      this.importFolderSelectorVisible = true;
    },
    selectImportParentFolder(folderId) {
      this.importParentId = folderId;
    },
    getAllFolders() {
      // 获取所有文件夹节点
      const folders = [];
      const collectFolders = (nodes, path = '') => {
        nodes.forEach(node => {
          if (node.type === 'folder') {
            folders.push(node);
            if (node.children && node.children.length > 0) {
              collectFolders(node.children);
            }
          }
        });
      };
      collectFolders(this.tree);
      return folders;
    },
    toggleTheme() {
      const html = document.documentElement;
      const currentTheme = html.getAttribute('data-theme');
      const newTheme = currentTheme === 'light' ? 'dark' : 'light';
      
      // 保存主题到localStorage
      localStorage.setItem('bookmark-manager-theme', newTheme);
      
      // 应用新主题
      html.setAttribute('data-theme', newTheme);
      
      // 重新应用面板透明度
      this.applyPanelOpacity(this.backgroundSettings.panelOpacity);
    },
    async exportBookmarks() {
      try {
        // 重置导出状态
        this.exporting = true;
        this.exportMessage = '正在准备导出数据...';
        this.exportProgress = 0;
        this.exportResults = [];
        this.exportSuccessCount = 0;
        this.exportFailCount = 0;
        
        // 复制数据，避免修改原始数据
        const exportData = JSON.parse(JSON.stringify(this.tree));
        
        // 计算总书签数量
        this.totalExportItems = this.countTotalBookmarks(exportData);
        
        this.exportMessage = '正在转换图片为base64...';
        // 转换所有书签的图片为base64
        await this.convertBookmarksImages(exportData);
        
        this.exportMessage = '正在生成导出文件...';
        // 生成导出文件
        const json = JSON.stringify(exportData, null, 2);
        const blob = new Blob([json], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `bookmarks-${new Date().toISOString().slice(0, 10)}.json`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        
        // 显示导出结果
        this.exportMessage = '导出完成！';
        // 导出完成后，不自动关闭，等待用户手动关闭
        this.exportMenuVisible = false; // 关闭导出菜单
      } catch (error) {
        console.error('导出失败:', error);
        this.exportMessage = `导出失败: ${error.message}`;
        // 导出失败后，不自动关闭，等待用户手动关闭
      }
    },
    // 手动关闭导出提示
    closeExportProgress() {
      this.exporting = false;
      this.exportMessage = '';
      this.exportProgress = 0;
      this.exportResults = [];
      this.exportSuccessCount = 0;
      this.exportFailCount = 0;
      this.showExportDetails = false;
    },
    async convertBookmarksImages(nodes) {
      for (const node of nodes) {
        if (node.type === 'bookmark') {
          // 处理书签
          let result = {
            title: node.title,
            success: true,
            message: ''
          };
          
          // 如果有favicon_url，尝试转换为base64
          if (node.favicon_url) {
            try {
              // 转换图片为base64
              node.favicon_url = await this.imageToBase64(node.favicon_url);
              result.message = '图片转换成功';
            } catch (error) {
              console.error(`转换书签图片失败: ${node.title}`, error);
              result.success = false;
              result.message = `图片转换失败: ${error.message}`;
              // 转换失败时保留原始URL
            }
          } else {
            result.message = '无图片需要转换';
          }
          
          // 更新导出进度
          this.exportProgress++;
          
          // 更新成功和失败统计
          if (result.success) {
            this.exportSuccessCount++;
          } else {
            this.exportFailCount++;
          }
          
          // 保存结果
          this.exportResults.push(result);
          
          // 自动滚动结果列表到底部，显示最新结果
          this.$nextTick(() => {
            const resultsList = this.$refs.resultsList;
            if (resultsList) {
              resultsList.scrollTop = resultsList.scrollHeight;
            }
          });
        } else if (node.type === 'folder' && node.children && node.children.length > 0) {
          // 递归处理子文件夹
          await this.convertBookmarksImages(node.children);
        }
      }
    },
    async exportEdgeBookmarks() {
      try {
        // 重置导出状态
        this.exporting = true;
        this.exportMessage = '正在准备导出数据...';
        this.exportProgress = 0;
        this.exportResults = [];
        this.exportSuccessCount = 0;
        this.exportFailCount = 0;
        
        // 复制数据，避免修改原始数据
        const exportData = JSON.parse(JSON.stringify(this.tree));
        
        // 计算总书签数量
        this.totalExportItems = this.countTotalBookmarks(exportData);
        
        this.exportMessage = '正在转换图片为base64...';
        // 转换所有书签的图片为base64
        await this.convertBookmarksImages(exportData);
        
        this.exportMessage = '正在生成HTML文件...';
        // 生成导出文件
        const html = this.generateEdgeBookmarksHTML(exportData);
        const blob = new Blob([html], { type: 'text/html;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `bookmarks-${new Date().toISOString().slice(0, 10)}.html`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        
        // 显示导出结果
        this.exportMessage = '导出完成！';
        // 导出完成后，不自动关闭，等待用户手动关闭
        this.exportMenuVisible = false; // 关闭导出菜单
      } catch (error) {
        console.error('导出Edge书签失败:', error);
        this.exportMessage = `导出失败: ${error.message}`;
        // 导出失败后，不自动关闭，等待用户手动关闭
      }
    },
    generateEdgeBookmarksHTML(nodes) {
      // 生成Edge兼容的HTML格式书签，包含图标信息
      const now = Math.floor(Date.now() / 1000);
      let html = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<!-- This is an automatically generated file.
     It will be read and overwritten.
     DO NOT EDIT! -->
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>`;
      
      // 递归生成HTML内容
      html += this.generateEdgeBookmarksHTMLRecursive(nodes, 0, now);
      
      html += `
</DL><p>`;
      return html;
    },
    generateEdgeBookmarksHTMLRecursive(nodes, level, now) {
      // 递归生成书签HTML，包含图标信息
      let html = '';
      
      for (const node of nodes) {
        const indent = '  '.repeat(level);
        
        if (node.type === 'folder') {
          // 文件夹
          html += `
${indent}<DT><H3 ADD_DATE="${now}" LAST_MODIFIED="0">${node.title}</H3>
${indent}<DL><p>`;
          
          if (node.children && node.children.length > 0) {
            html += this.generateEdgeBookmarksHTMLRecursive(node.children, level + 1, now);
          }
          
          html += `
${indent}</DL><p>`;
        } else if (node.type === 'bookmark') {
          // 书签
          const href = node.url || '';
          const title = node.title || '';
          const favicon = node.favicon_url || '';
          let iconAttr = '';
          if (favicon) {
            iconAttr = ` ICON="${favicon}"`;
          }
          html += `
${indent}<DT><A HREF="${href}" ADD_DATE="${now}"${iconAttr}>${title}</A>`;
        }
      }
      
      return html;
    },
    toggleExportMenu() {
      // 切换导出菜单的显示状态
      this.exportMenuVisible = !this.exportMenuVisible;
    },
    importBookmarks(event) {
      const file = event.target.files[0];
      if (!file) return;
      
      const reader = new FileReader();
      reader.onload = async (e) => {
        try {
          const json = e.target.result;
          const data = JSON.parse(json);
          
          // 发送导入请求，添加parent_id参数
          const response = await fetch('/api/import', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': this.token
            },
            body: JSON.stringify({
              bookmarks: data,
              mode: this.importMode,
              parent_id: this.importParentId
            }),
          });
          
          if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || '导入失败');
          }
          
          // 导入成功，重新加载树结构
          await this.loadTree();
          this.showToast('导入成功', 'success');
          this.closeImportDialog();
          
        } catch (error) {
          this.showToast(error.message || '导入失败，请检查文件格式', 'error');
        } finally {
          // 重置文件输入
          event.target.value = '';
        }
      };
      reader.readAsText(file);
    },
    showImportDialog() {
      // 显示导入选项对话框
      this.importDialog.visible = true;
      this.importMode = 'merge'; // 默认选择合并模式
      this.importParentId = null; // 默认导入到根目录
      this.importFolderSelectorVisible = false; // 确保导入文件夹选择器初始状态为隐藏
    },
    closeImportDialog() {
      // 关闭导入选项对话框
      this.importDialog.visible = false;
      // 重置文件输入
      const fileInput = document.getElementById('import-file-input');
      if (fileInput) {
        fileInput.value = '';
      }
    },
    handleFileUpload() {
      // 触发文件选择
      const fileInput = document.getElementById('import-file-input');
      if (fileInput) {
        fileInput.click();
      }
    },
    showEdgeImportDialog() {
      // 显示Edge导入选项对话框
      this.edgeImportDialog.visible = true;
      this.edgeImportMode = 'merge'; // 默认选择合并模式
      this.edgeImportParentId = null; // 默认导入到根目录
    },
    closeEdgeImportDialog() {
      // 关闭Edge导入选项对话框
      this.edgeImportDialog.visible = false;
      // 重置文件输入
      const fileInput = document.getElementById('edge-import-file-input');
      if (fileInput) {
        fileInput.value = '';
      }
    },
    handleEdgeFileUpload() {
      // 触发Edge文件选择
      const fileInput = document.getElementById('edge-import-file-input');
      if (fileInput) {
        fileInput.click();
      }
    },
    // Edge导入文件夹选择相关方法
    showEdgeFolderSelector() {
      // 显示Edge导入文件夹选择器
      this.edgeFolderSelectorVisible = true;
    },
    closeEdgeFolderSelector() {
      // 关闭Edge导入文件夹选择器
      this.edgeFolderSelectorVisible = false;
    },
    selectEdgeImportParentFolder(folderId) {
      // 选择导入目标文件夹
      this.edgeImportParentId = folderId;
    },
    confirmEdgeFolderSelection() {
      // 确认Edge导入文件夹选择
      this.edgeFolderSelectorVisible = false;
    },
    cancelEdgeFolderSelection() {
      // 取消Edge导入文件夹选择
      this.edgeFolderSelectorVisible = false;
    },
    showUpdateMetadataDialog() {
      // 显示更新书签信息对话框
      this.updateMetadataDialog.visible = true;
      this.updateMetadataDialog.targetFolderId = null;
      this.updateMetadataDialog.targetFolderName = '';
    },
    closeUpdateMetadataDialog() {
      // 关闭更新书签信息对话框
      this.updateMetadataDialog.visible = false;
    },
    handleStartUpdate() {
      // 处理开始更新按钮点击
      console.log('handleStartUpdate 被调用，targetFolderId:', this.updateMetadataDialog.targetFolderId);
      this.updateFolderMetadata(this.updateMetadataDialog.targetFolderId);
    },
    async updateFolderMetadata(folderId) {
      console.log('updateFolderMetadata 被调用，folderId:', folderId);
      
      // 更新指定文件夹及其子文件夹中的所有书签信息
      const folder = this.findNodeById(folderId, this.tree);
      if (!folder || folder.type !== 'folder') {
        this.showToast('请选择文件夹', 'error');
        return;
      }

      const bookmarks = this.collectBookmarks([folder]);
      console.log('找到书签数量:', bookmarks.length);
      
      if (bookmarks.length === 0) {
        this.showToast('该文件夹下没有书签', 'warning');
        return;
      }

      // 开始更新
      this.updateMetadataDialog.updating = true;
      this.updateMetadataDialog.currentIndex = 0;
      this.updateMetadataDialog.totalCount = bookmarks.length;
      this.updateMetadataDialog.currentBookmarkTitle = '';
      
      console.log('开始更新，总数:', this.updateMetadataDialog.totalCount);
      console.log('updating状态:', this.updateMetadataDialog.updating);
      
      let successCount = 0;
      let failCount = 0;
      let skipCount = 0;

      for (const bookmark of bookmarks) {
        // 跳过没有URL的书签
        if (!bookmark.url || bookmark.url.trim() === '') {
          console.log('跳过没有URL的书签:', bookmark.title);
          skipCount++;
          this.updateMetadataDialog.currentIndex++;
          continue;
        }

        // 验证URL格式
        try {
          new URL(bookmark.url);
        } catch (error) {
          console.error('URL格式无效:', bookmark.url, error);
          failCount++;
          this.updateMetadataDialog.currentIndex++;
          continue;
        }

        // 更新当前书签标题
        this.updateMetadataDialog.currentBookmarkTitle = bookmark.title;

        try {
          console.log('正在更新书签:', bookmark.title, 'ID:', bookmark.id, 'URL:', bookmark.url);
          await this.updateBookmarkMetadata(bookmark.id, bookmark.url);
          successCount++;
        } catch (error) {
          console.error('更新书签失败:', bookmark.title, error);
          failCount++;
        }

        this.updateMetadataDialog.currentIndex++;
      }

      // 更新完成
      this.updateMetadataDialog.updating = false;
      this.updateMetadataDialog.currentBookmarkTitle = '';

      if (successCount > 0) {
        this.showToast(`成功更新 ${successCount} 个书签的信息`, 'success');
      }
      if (failCount > 0) {
        this.showToast(`更新失败 ${failCount} 个书签`, 'error');
      }
      if (skipCount > 0) {
        this.showToast(`跳过 ${skipCount} 个无效书签`, 'warning');
      }

      this.closeUpdateMetadataDialog();
    },
    async updateBookmarkMetadata(nodeId, url) {
      const response = await fetch(`/api/nodes/${nodeId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': this.token
        },
        body: JSON.stringify({
          url: url
        }),
      });

      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || '更新失败');
      }

      return await response.json();
    },
    async importEdgeBookmarks(event) {
      const file = event.target.files[0];
      if (!file) return;
      
      // 保存选中的文件，显示确认对话框
      this.edgeImportFile = file;
      this.edgeConfirmImportVisible = true;
      
      // 重置文件输入
      event.target.value = '';
    },
    async confirmEdgeImportBookmarks() {
      if (!this.edgeImportFile) return;
      
      const reader = new FileReader();
      reader.onload = async (e) => {
        try {
          const html = e.target.result;
          
          // 发送导入Edge书签请求
          const response = await fetch('/api/import-edge', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': this.token
            },
            body: JSON.stringify({
              html: html,
              mode: this.edgeImportMode,
              parent_id: this.edgeImportParentId
            }),
          });
          
          if (!response.ok) {
            const err = await response.json().catch(() => ({}));
            throw new Error(err.error || '导入失败');
          }
          
          // 导入成功，重新加载树结构
          await this.loadTree();
          this.showToast('Edge书签导入成功', 'success');
          this.closeEdgeImportDialog();
          this.closeEdgeConfirmImportDialog();
          
        } catch (error) {
          this.showToast(error.message || '导入失败，请检查文件格式', 'error');
          this.closeEdgeConfirmImportDialog();
        }
      };
      reader.readAsText(this.edgeImportFile);
    },
    closeEdgeConfirmImportDialog() {
      // 关闭Edge导入确认对话框
      this.edgeConfirmImportVisible = false;
      this.edgeImportFile = null;
    },
    closeConfirmImportDialog() {
      // 关闭JSON导入确认对话框
      this.confirmImportVisible = false;
      this.importFile = null;
    },
    loadSavedTheme() {
      const savedTheme = localStorage.getItem('bookmark-manager-theme');
      if (savedTheme) {
        document.documentElement.setAttribute('data-theme', savedTheme);
      }
    },
    // 背景设置相关方法
    async loadBackgroundSettings() {
      try {
        const response = await fetch('/api/config', {
          headers: {
            'Authorization': this.token
          }
        });
        if (response.ok) {
          const config = await response.json();
          if (config['background']) {
            try {
              const settings = JSON.parse(config['background']);
              // 确保有 panelOpacity 属性
              if (typeof settings.panelOpacity === 'undefined') {
                settings.panelOpacity = 90;
              }
              this.backgroundSettings = settings;
              this.applyBackgroundSettings(settings);
            } catch (e) {
              console.error('Failed to parse background settings:', e);
            }
          }
        }
      } catch (e) {
        console.error('Failed to load background settings:', e);
        // 如果从服务器加载失败，尝试从localStorage加载作为备份
        const saved = localStorage.getItem('bookmark-manager-background');
        if (saved) {
          try {
            const settings = JSON.parse(saved);
            // 确保有 panelOpacity 属性
            if (typeof settings.panelOpacity === 'undefined') {
              settings.panelOpacity = 90;
            }
            this.backgroundSettings = settings;
            this.applyBackgroundSettings(settings);
            // 将localStorage中的设置保存到数据库
            this.saveBackgroundSettings();
          } catch (e) {
            console.error('Failed to parse background settings from localStorage:', e);
          }
        }
      }
    },
    async saveBackgroundSettings() {
      try {
        const response = await fetch('/api/config', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': this.token
          },
          body: JSON.stringify({
            key: 'background',
            value: JSON.stringify(this.backgroundSettings)
          })
        });
        if (!response.ok) {
          throw new Error('Failed to save background settings');
        }
        // 同时保存到localStorage作为备份
        localStorage.setItem('bookmark-manager-background', JSON.stringify(this.backgroundSettings));
      } catch (e) {
        console.error('Failed to save background settings:', e);
        // 如果保存到服务器失败，只保存到localStorage
        localStorage.setItem('bookmark-manager-background', JSON.stringify(this.backgroundSettings));
      }
    },
    applyBackgroundSettings(settings) {
      const body = document.body;
      switch (settings.type) {
        case 'color':
          // 直接设置background简写属性，包含所有背景相关设置
          body.style.background = settings.color;
          // 确保没有其他背景属性干扰
          body.style.backgroundImage = 'none';
          body.style.backgroundSize = 'auto';
          body.style.backgroundRepeat = 'repeat';
          body.style.backgroundPosition = '0 0';
          body.style.backgroundAttachment = 'scroll';
          break;
        case 'image':
          body.style.background = `url('${settings.image}') center center / cover no-repeat fixed`;
          break;
        default: // default
          // 使用img目录下的background.jpg作为默认背景
          body.style.background = `url('/img/background.jpg') center center / cover no-repeat fixed`;
          break;
      }
      // 应用面板透明度
      this.applyPanelOpacity(settings.panelOpacity);
    },
    applyPanelOpacity(opacity) {
      const opacityValue = opacity / 100;
      const panels = document.querySelectorAll('.tree-panel, .list-panel');
      const isLightTheme = document.documentElement.getAttribute('data-theme') === 'light';
      
      panels.forEach(panel => {
        if (isLightTheme) {
          panel.style.backgroundColor = `rgba(255, 255, 255, ${opacityValue * 0.9})`;
        } else {
          panel.style.backgroundColor = `rgba(0, 0, 0, ${opacityValue * 0.35})`;
        }
      });
    },
    openBackgroundModal() {
      // 保存原始背景设置，用于取消时恢复
      this.originalBackgroundSettings = JSON.parse(JSON.stringify(this.backgroundSettings));
      this.backgroundModal.visible = true;
    },
    closeBackgroundModal() {
      this.backgroundModal.visible = false;
    },
    saveBackgroundSettingsAndClose() {
      // 保存背景设置并关闭模态框
      this.saveBackgroundSettings();
      this.backgroundModal.visible = false;
    },
    cancelBackgroundSettings() {
      // 恢复原始背景设置
      if (this.originalBackgroundSettings) {
        this.backgroundSettings = JSON.parse(JSON.stringify(this.originalBackgroundSettings));
        this.applyBackgroundSettings(this.backgroundSettings);
      }
      this.backgroundModal.visible = false;
    },
    handleImageUpload(event) {
      const file = event.target.files[0];
      if (!file) return;
      
      const reader = new FileReader();
      reader.onload = (e) => {
        const imageUrl = e.target.result;
        this.backgroundSettings = {
          type: 'image',
          color: '',
          image: imageUrl
        };
        this.applyBackgroundSettings(this.backgroundSettings);
      };
      reader.readAsDataURL(file);
      
      // 重置文件输入
      event.target.value = '';
    },
    resetBackground() {
      this.backgroundSettings = {
        type: 'default',
        color: '',
        image: ''
      };
      this.applyBackgroundSettings(this.backgroundSettings);
      this.saveBackgroundSettings();
    },
    isIntranetUrl(url) {
      if (!url) return false;
      
      try {
        const urlObj = new URL(url);
        const hostname = urlObj.hostname.toLowerCase();
        
        // 检测内网地址模式
        const intranetPatterns = [
          /^localhost$/,
          /^127\.0\.0\.1$/,
          /^::1$/,
          /^192\.168\./,
          /^10\./,
          /^172\.(1[6-9]|2[0-9]|3[01])\./
        ];
        
        return intranetPatterns.some(pattern => pattern.test(hostname));
      } catch (e) {
        return false;
      }
    },
    countTotalBookmarks(nodes) {
      // 递归计算总书签数量
      let count = 0;
      for (const node of nodes) {
        if (node.type === 'bookmark') {
          count++;
        } else if (node.type === 'folder' && node.children && node.children.length > 0) {
          count += this.countTotalBookmarks(node.children);
        }
      }
      return count;
    },
    imageToBase64(url) {
      return new Promise((resolve, reject) => {
        // 如果已经是base64格式，直接返回
        if (url.startsWith('data:')) {
          resolve(url);
          return;
        }
        
        // 创建Image对象
        const img = new Image();
        // 允许跨域图片
        img.crossOrigin = 'anonymous';
        
        img.onload = () => {
          // 创建Canvas对象
          const canvas = document.createElement('canvas');
          canvas.width = img.width;
          canvas.height = img.height;
          
          // 绘制图片到Canvas
          const ctx = canvas.getContext('2d');
          ctx.drawImage(img, 0, 0);
          
          try {
            // 转换为base64
            const base64 = canvas.toDataURL('image/png');
            resolve(base64);
          } catch (error) {
            console.error('转换图片到base64失败:', error);
            reject(error);
          }
        };
        
        img.onerror = () => {
          console.error('加载图片失败:', url);
          reject(new Error('加载图片失败'));
        };
        
        // 处理本地路径
        let imageUrl = url;
        if (url.startsWith('/') && !url.startsWith('//')) {
          // 本地绝对路径，添加当前域名
          imageUrl = window.location.origin + url;
        }
        
        img.src = imageUrl;
      });
    },
    getFaviconUrl(item) {
      // 如果有明确的favicon_url，优先使用，无论是否是内网地址
      if (item.favicon_url && item.favicon_url.trim()) {
        return item.favicon_url.trim();
      }
      
      // 如果是内网地址且没有favicon_url，不显示favicon，返回空字符串让前端显示默认图标
      if (this.isIntranetUrl(item.url)) {
        return '';
      }
      
      // 如果没有favicon_url且不是内网地址，则使用默认的 /favicon.ico 路径
      try {
        const url = new URL(item.url);
        return `${url.protocol}//${url.host}/favicon.ico`;
      } catch (error) {
        // 如果URL解析失败，返回空字符串
        return '';
      }
    },
    async loadTree() {
      this.loading = true;
      try {
        const response = await fetch("/api/tree", {
          headers: {
            "Authorization": this.token,
            "Cache-Control": "no-cache, no-store, must-revalidate",
            "Pragma": "no-cache",
            "Expires": "0"
          }
        });
        if (!response.ok) {
          throw new Error("加载失败");
        }
        const data = await response.json();
        console.log("加载树结构成功，节点数量:", data.length);
        this.tree = Array.isArray(data) ? data : [];
        if (this.selectedNodeId) {
          const current = this.findNodeById(this.selectedNodeId, this.tree);
          if (!current && this.selectedNodeId !== 'all-bookmarks') {
            // 默认选中【所有网址】文件夹
            this.selectedNodeId = 'all-bookmarks';
          }
        } else {
          // 默认选中【所有网址】文件夹
          this.selectedNodeId = 'all-bookmarks';
        }
        if (this.contextMenu.visible) {
          const node = this.contextNode;
          if (!node || node.type !== "folder") {
            this.hideContextMenu();
          }
        }
      } catch (error) {
        console.error("加载树结构失败:", error);
        this.showToast(error.message || "加载树结构失败", "error");
      } finally {
        this.loading = false;
      }
    },
    findFirstFolder(list) {
      const queue = [...list];
      while (queue.length) {
        const item = queue.shift();
        if (item.type === "folder") {
          return item;
        }
        if (item.children && item.children.length) {
          queue.push(...item.children);
        }
      }
      return null;
    },
    findNodeById(id, list) {
      for (const item of list) {
        if (item.id === id) {
          return item;
        }
        if (item.children && item.children.length) {
          const found = this.findNodeById(id, item.children);
          if (found) return found;
        }
      }
      return null;
    },
    selectNode(node) {
      // 点击文件夹时退出搜索模式
      this.isSearching = false;
      this.searchResults = [];
      
      this.selectedNodeId = node.id;
      if (!this.treeActionsVisible) {
        this.hideContextMenu();
      }
    },
    
    // 选择【所有网址】文件夹
    selectAllBookmarksFolder() {
      // 点击所有网址文件夹时退出搜索模式
      this.isSearching = false;
      this.searchResults = [];
      
      this.selectedNodeId = 'all-bookmarks';
      // 确保编辑模式下也不会对所有网址文件夹进行操作
      this.treeActionsVisible = false;
      this.hideContextMenu();
    },
    openAddFolder(parent) {
      this.hideContextMenu();
      this.modal.visible = true;
      this.modal.type = "add-folder";
      this.modal.parentId = parent ? parent.id : null;
      this.modal.nodeId = null;
      this.modal.form.title = "";
      this.modal.form.url = "";
      this.modal.form.favicon_url = "";
      this.metadataError = "";
    },
    openAddBookmark(parent = null) {
      this.hideContextMenu();
      this.modal.visible = true;
      this.modal.type = "add-bookmark";
      this.modal.parentId = parent ? parent.id : null;
      this.modal.nodeId = null;
      this.modal.form.title = "";
      this.modal.form.url = "";
      this.modal.form.favicon_url = "";
      this.metadataError = "";
      // 从列表顶部按钮调用时，确保不强制设置parentId
      // 这样用户可以先选择文件夹，再填写信息
    },
    openEdit(node) {
      this.hideContextMenu();
      this.modal.visible = true;
      this.modal.type = node.type === "folder" ? "edit-folder" : "edit-bookmark";
      this.modal.parentId = node.parent_id ?? null;
      this.modal.nodeId = node.id;
      this.modal.form.title = node.title;
      this.modal.form.url = node.url || "";
      this.modal.form.favicon_url = node.favicon_url || "";
      this.metadataError = "";
    },
    async lookupMetadata() {
      const url = this.modal.form.url.trim();
      if (!url) return;
      this.metadataLoading = true;
      this.metadataError = "";
      try {
        const res = await fetch(`/api/metadata?url=${encodeURIComponent(url)}`);
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          throw new Error(err.error || "获取失败");
        }
        const data = await res.json();
        
        // 区分添加和编辑模式
        const isEditMode = this.modal.type === "edit-bookmark";
        
        if (data.title) {
          // 编辑模式下总是更新title，添加模式下只有title为空时才更新
          if (isEditMode || !this.modal.form.title) {
            this.modal.form.title = data.title;
          }
        }
        if (data.favicon_url) {
          this.modal.form.favicon_url = data.favicon_url;
        }
        if (data.url) {
          this.modal.form.url = data.url;
        }
      } catch (error) {
        this.metadataError = error.message || "无法获取网站信息";
      } finally {
        this.metadataLoading = false;
      }
    },
    closeModal() {
      this.modal.visible = false;
      this.modal.type = "";
      this.metadataLoading = false;
      this.metadataError = "";
    },
    async submitModal() {
      try {
        if (this.modal.type === "add-folder") {
          await this.createFolder();
        } else if (this.modal.type === "add-bookmark") {
          await this.createBookmark();
        } else if (this.modal.type === "edit-folder") {
          await this.updateFolder();
        } else if (this.modal.type === "edit-bookmark") {
          await this.updateBookmark();
        }
        this.closeModal();
        await this.loadTree();
        this.showToast("操作成功", "success");
      } catch (error) {
        this.showToast(error.message || "操作失败", "error");
      }
    },
    async createFolder() {
      const payload = {
        title: this.modal.form.title.trim(),
        parent_id: this.modal.parentId,
      };
      const res = await fetch("/api/folders", {
        method: "POST",
        headers: { 
          "Content-Type": "application/json",
          "Authorization": this.token
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "创建文件夹失败");
      }
      const data = await res.json();
      this.selectedNodeId = data.id;
    },
    async createBookmark() {
      // 验证是否选中了文件夹
      if (!this.modal.parentId) {
        throw new Error("请先选择一个文件夹");
      }
      
      const payload = {
        url: this.modal.form.url.trim(),
        title: this.modal.form.title.trim(),
        parent_id: this.modal.parentId,
        favicon_url: this.modal.form.favicon_url.trim() || undefined,
      };
      const res = await fetch("/api/bookmarks", {
        method: "POST",
        headers: { 
          "Content-Type": "application/json",
          "Authorization": this.token
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "创建网址失败");
      }
      const data = await res.json();
      this.selectedNodeId = this.modal.parentId ?? this.selectedNodeId;
    },
    async updateFolder() {
      const payload = {
        title: this.modal.form.title.trim(),
      };
      const res = await fetch(`/api/nodes/${this.modal.nodeId}`, {
        method: "PUT",
        headers: { 
          "Content-Type": "application/json",
          "Authorization": this.token
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "更新文件夹失败");
      }
    },
    async updateBookmark() {
      // 验证是否选中了文件夹
      if (!this.modal.parentId) {
        throw new Error("请先选择一个文件夹");
      }
      
      const payload = {
        title: this.modal.form.title.trim(),
        url: this.modal.form.url.trim(),
        parent_id: this.modal.parentId,
      };
      const favicon = this.modal.form.favicon_url.trim();
      if (favicon) {
        payload.favicon_url = favicon;
      }
      const res = await fetch(`/api/nodes/${this.modal.nodeId}`, {
        method: "PUT",
        headers: { 
          "Content-Type": "application/json",
          "Authorization": this.token
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "更新网址失败");
      }
    },
    // 自定义确认对话框方法
    async customConfirm(title, message, type = 'warning', confirmText = '确认') {
      return new Promise((resolve) => {
        this.confirmDialog = {
          visible: true,
          title,
          message,
          type,
          confirmText,
          callback: resolve
        };
      });
    },
    
    handleConfirmOk() {
      const callback = this.confirmDialog.callback;
      this.confirmDialog.visible = false;
      if (typeof callback === 'function') {
        callback(true);
      }
    },
    
    handleConfirmCancel() {
      const callback = this.confirmDialog.callback;
      this.confirmDialog.visible = false;
      if (typeof callback === 'function') {
        callback(false);
      }
    },
    
    async confirmDelete(node) {
      const ok = await this.customConfirm(
        "确认删除",
        `确定删除「${node.title}」吗？该操作不可撤销。`,
        "warning",
        "删除"
      );
      if (!ok) return;
      try {
        const res = await fetch(`/api/nodes/${node.id}`, { 
          method: "DELETE",
          headers: {
            "Authorization": this.token
          }
        });
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          throw new Error(err.error || "删除失败");
        }
        if (this.selectedNodeId === node.id) {
          this.selectedNodeId = null;
        }
        if (this.contextMenu.visible && this.contextMenu.nodeId === node.id) {
          this.hideContextMenu();
        }
        await this.loadTree();
        this.showToast("删除成功", "success");
      } catch (error) {
        this.showToast(error.message || "删除失败", "error");
      }
    },
    async reorderNode({ node, direction }) {
      const siblings = this.getSiblings(node.parent_id ?? null);
      if (!siblings.length) return;
      const index = siblings.findIndex((item) => item.id === node.id);
      if (index === -1) return;
      if (direction === "up" && index === 0) return;
      if (direction === "down" && index === siblings.length - 1) return;

      const reordered = siblings.slice();
      const [item] = reordered.splice(index, 1);
      const targetIndex = direction === "up" ? index - 1 : index + 1;
      reordered.splice(targetIndex, 0, item);

      try {
        const payload = {
          parent_id: node.parent_id ?? null,
          ordered_ids: reordered.map((n) => n.id),
        };
        const res = await fetch("/api/nodes/reorder", {
          method: "POST",
          headers: { 
            "Content-Type": "application/json",
            "Authorization": this.token
          },
          body: JSON.stringify(payload),
        });
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          throw new Error(err.error || "排序失败");
        }
        await this.loadTree();
      } catch (error) {
        this.showToast(error.message || "排序失败", "error");
      }
    },
    getSiblings(parentId) {
      if (parentId === null || parentId === undefined) {
        return this.tree;
      }
      const parent = this.findNodeById(parentId, this.tree);
      if (!parent) return [];
      return parent.children || [];
    },
    toggleTreeActions() {
      this.treeActionsVisible = !this.treeActionsVisible;
      if (this.treeActionsVisible) {
        this.hideContextMenu();
      }
    },
     showFolderActions({ node, x, y }) {
        // 如果是【所有网址】文件夹，不显示右键菜单
        if (!node || node.id === 'all-bookmarks' || node.type !== 'folder') {
          return;
        }
        this.treeActionsVisible = false;
        this.contextMenu.visible = true;
        this.contextMenu.x = x;
        this.contextMenu.y = y;
        this.contextMenu.nodeId = node.id;
        this.rightClickNode = node;
      },
    showBookmarkActions(node, event) {
      this.treeActionsVisible = false;
      this.showContextMenu(node, event.clientX, event.clientY);
    },
    startBookmarkLongPress(node, event) {
      // 长按事件，在移动端模拟右键菜单
      this.longPressTimer = setTimeout(() => {
        this.treeActionsVisible = false;
        this.showContextMenu(node, event.touches[0].clientX, event.touches[0].clientY);
      }, 500); // 500ms长按
    },
    endBookmarkLongPress() {
      // 清除长按定时器
      if (this.longPressTimer) {
        clearTimeout(this.longPressTimer);
        this.longPressTimer = null;
      }
    },
    showContextMenu(node, x, y) {
      const padding = 16;
      const menuWidth = 220;
      // 根据节点类型动态计算菜单高度
      const menuHeight = node.type === 'folder' ? 240 : 180;
      const viewportWidth = window.innerWidth;
      const viewportHeight = window.innerHeight;

      let left = x;
      let top = y;

      if (left + menuWidth + padding > viewportWidth) {
        left = viewportWidth - menuWidth - padding;
      }
      if (top + menuHeight + padding > viewportHeight) {
        top = viewportHeight - menuHeight - padding;
      }

      this.contextMenu.visible = true;
      this.contextMenu.nodeId = node.id;
      this.contextMenu.x = Math.max(padding, left);
      this.contextMenu.y = Math.max(padding, top);
    },
    hideContextMenu() {
      this.contextMenu.visible = false;
      this.contextMenu.nodeId = null;
    },
    handleRootClick() {
      this.hideContextMenu();
      this.userDropdownVisible = false;
    },
    handleHeaderClick(event) {
      // 处理头部区域的点击事件，避免意外隐藏右键菜单
      // 点击按钮时正常执行按钮功能，但不隐藏菜单
      // 只有点击空白区域时才隐藏菜单
      const isButtonClick = event.target.closest('button');
      if (!isButtonClick && event.target === event.currentTarget) {
        // 点击的是头部空白区域，隐藏菜单
        this.hideContextMenu();
      }
    },
    handleTreePanelClick(event) {
      // 专门处理左边结构面板的点击
      // 如果点击的是树节点相关的元素，不隐藏菜单
      const isTreeNodeClick = event.target.closest('.tree-node') || 
                             event.target.closest('.tree-toggle') || 
                             event.target.closest('.tree-actions');
      if (!isTreeNodeClick) {
        this.hideContextMenu();
      }
    },
    handleListPanelClick(event) {
      // 处理右边网址列表面板的点击
      // 如果点击的是书签项，不隐藏菜单
      const isBookmarkItemClick = event.target.closest('.bookmark-item') || 
                                 event.target.closest('.bookmark-title') ||
                                 event.target.closest('.bookmark-url');
      if (!isBookmarkItemClick) {
        this.hideContextMenu();
      }
    },
    contextReorder(direction) {
      const node = this.contextNode;
      if (!node) return;
      this.hideContextMenu();
      this.reorderNode({ node, direction });
    },
    contextAddFolder() {
      const node = this.contextNode;
      if (!node) return;
      this.openAddFolder(node);
    },
    contextAddBookmark() {
      const node = this.contextNode;
      if (!node) return;
      this.openAddBookmark(node);
    },
    contextEdit() {
      const node = this.contextNode;
      if (!node) return;
      this.openEdit(node);
    },
    contextDelete() {
      const node = this.contextNode;
      if (!node) return;
      this.hideContextMenu();
      this.confirmDelete(node);
    },
    contextMoveTo() {
      const node = this.contextNode;
      if (!node) return;
      this.moveModal.visible = true;
      this.moveModal.targetParentId = node.parent_id ?? null;
      this.rightClickNode = node;
      this.hideContextMenu();
    },
    closeMoveModal() {
      this.moveModal.visible = false;
      this.moveModal.targetParentId = null;
      this.hideContextMenu();
    },
    selectMoveTarget(parentId) {
      this.moveModal.targetParentId = parentId;
    },
    getAllFolders() {
      const folders = [];
      const collectFolders = (nodes) => {
        for (const node of nodes) {
          if (node.type === "folder") {
            folders.push(node);
            if (node.children && node.children.length) {
              collectFolders(node.children);
            }
          }
        }
      };
      collectFolders(this.tree);
      return folders;
    },
    getFolderPath(folderId) {
      const findPath = (nodes, targetId, currentPath = []) => {
        for (const node of nodes) {
          if (node.id === targetId) {
            return currentPath.concat(node.title);
          }
          if (node.children && node.children.length) {
            const found = findPath(node.children, targetId, currentPath.concat(node.title));
            if (found.length > currentPath.length) {
              return found;
            }
          }
        }
        return [];
      };
      
      const path = findPath(this.tree, folderId);
      return path.join(" / ");
    },
    isFolderOrChild(folderId, nodeId) {
      console.log('isFolderOrChild 参数folderId：', folderId, 'nodeId：',nodeId);
      if (!nodeId) return false;
      
      // 检查是否是同一个文件夹
      if (folderId === nodeId) {
        return true;
      }
      
      // 检查是否是子文件夹
      const node = this.findNodeById(nodeId, this.tree);
      if (!node || node.type !== "folder") {
        return false;
      }
      
      const checkIsDescendant = (parentNode, targetId) => {
        if (!parentNode.children) return false;
        
        for (const child of parentNode.children) {
          if (child.id === targetId) {
            return true;
          }
          if (child.type === "folder" && checkIsDescendant(child, targetId)) {
            return true;
          }
        }
        return false;
      };
      
      return checkIsDescendant(node, folderId);
    },
    async confirmMove() {
      const node = this.rightClickNode;
      if (!node) return;
      
      const newParentId = this.moveModal.targetParentId;
      if (newParentId === node.parent_id) {
        this.showToast("未选择不同的文件夹", "warning");
        return;
      }
      
      try {
        const response = await fetch(`/api/nodes/${node.id}`, {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            "Authorization": this.token
          },
          body: JSON.stringify({
            parent_id: newParentId,
          }),
        });
        
        if (!response.ok) {
          throw new Error("移动操作失败");
        }
        
        this.closeMoveModal();
        await this.loadTree();
        this.showToast("已移动到指定文件夹", "success");
      } catch (error) {
        this.showToast(error.message || "移动操作失败", "error");
      }
    },
    collectBookmarks(nodes, trail = []) {
      const list = [];
      for (const node of nodes) {
        if (node.type === "folder") {
          const nextTrail = trail.concat(node.title);
          if (node.children && node.children.length) {
            list.push(...this.collectBookmarks(node.children, nextTrail));
          }
        } else if (node.type === "bookmark") {
          list.push({
            id: node.id,
            title: node.title,
            url: node.url,
            favicon_url: node.favicon_url,
            path: trail.length > 0 ? trail.join(" / ") : "",
            updated_at: node.updated_at,
            raw: node,
          });
        }
      }
      return list;
    },
    collectBookmarkIds(root) {
      const result = [];
      const stack = [root];
      while (stack.length) {
        const current = stack.pop();
        if (current.type === "bookmark") {
          result.push(current.id);
        }
        if (current.children && current.children.length) {
          stack.push(...current.children);
        }
      }
      return result;
    },
    showToast(message, type = "success") {
      // 清除之前的定时器
      if (this.toast.timer) {
        clearTimeout(this.toast.timer);
      }
      
      // 设置新的消息
      this.toast.visible = true;
      this.toast.message = message;
      this.toast.type = type;
      
      // 根据类型设置不同的显示时间
      const duration = type === "error" ? 5000 : type === "warning" ? 3500 : 2200;
      
      this.toast.timer = setTimeout(() => {
        this.toast.visible = false;
        this.toast.timer = null;
      }, duration);
    },
    // 编辑模式相关方法
    toggleBookmarkEdit() {
      if (this.bookmarkEditMode) {
        // 退出编辑模式，清除选择状态
        this.bookmarkEditMode = false;
        this.selectedBookmarks.clear();
      } else {
        // 进入编辑模式
        this.bookmarkEditMode = true;
        this.hideContextMenu();
      }
    },
    toggleBookmarkSelection(id) {
      if (this.selectedBookmarks.has(id)) {
        this.selectedBookmarks.delete(id);
      } else {
        this.selectedBookmarks.add(id);
      }
    },
    selectAllBookmarks() {
      if (this.selectedBookmarks.size === this.displayBookmarks.length) {
        // 取消全选
        this.selectedBookmarks.clear();
      } else {
        // 全选
        this.selectedBookmarks.clear();
        this.displayBookmarks.forEach(bookmark => {
          this.selectedBookmarks.add(bookmark.id);
        });
      }
    },
    async deleteSelectedBookmarks() {
      if (this.selectedBookmarks.size === 0) {
        return;
      }
      const count = this.selectedBookmarks.size;
      const ok = await this.customConfirm(
        "批量删除",
        `确定删除选中的 ${count} 个网址吗？该操作不可撤销。`,
        "warning",
        "删除"
      );
      if (!ok) return;
      
      try {
        const ids = Array.from(this.selectedBookmarks);
        console.log("开始批量删除，IDs:", ids);
        
        // 使用新的批量删除API，添加缓存控制
        const response = await fetch(`/api/nodes/batch-delete`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "Authorization": this.token,
            "Cache-Control": "no-cache, no-store, must-revalidate",
            "Pragma": "no-cache",
            "Expires": "0"
          },
          body: JSON.stringify({
            ids: ids
          })
        });
        
        if (!response.ok) {
          const err = await response.json().catch(() => ({}));
          throw new Error(err.error || "批量删除失败");
        }
        
        const result = await response.json();
        console.log("批量删除结果:", result);
        
        // 检查删除结果
        if (result.deleted_count === 0) {
          throw new Error("没有删除任何网址，请检查选中的网址是否存在");
        }
        
        // 清理状态
        this.selectedBookmarks.clear();
        this.bookmarkEditMode = false;
        await this.loadTree();
        
        // 显示结果
        if (result.deleted_count === result.requested_count) {
          this.showToast(`成功删除 ${result.deleted_count} 个网址`, "success");
        } else {
          this.showToast(`成功删除 ${result.deleted_count} 个网址，${result.requested_count - result.deleted_count} 个网址删除失败`, "warning");
        }
      } catch (error) {
        this.showToast(error.message || "批量删除失败", "error");
      }
    },
    async moveSelectedBookmarks(direction) {
      if (this.selectedBookmarks.size === 0) return;
      
      // 获取选中的书签并按当前显示顺序排列
      const selectedItems = this.displayBookmarks.filter(bookmark => 
        this.selectedBookmarks.has(bookmark.id)
      );
      
      try {
        let successCount = 0;
        const totalCount = selectedItems.length;
        
        // 上移：从上到下依次移动每个选中的项目
        if (direction === 'up') {
          for (const item of selectedItems) {
            try {
              await this.reorderNode({ node: item.raw, direction: 'up' });
              successCount++;
            } catch (error) {
              console.warn(`移动项目 ${item.id} 失败:`, error);
              // 单个项目移动失败不影响其他项目
            }
          }
        } 
        // 下移：从下到上依次移动每个选中的项目
        else if (direction === 'down') {
          for (const item of [...selectedItems].reverse()) {
            try {
              await this.reorderNode({ node: item.raw, direction: 'down' });
              successCount++;
            } catch (error) {
              console.warn(`移动项目 ${item.id} 失败:`, error);
              // 单个项目移动失败不影响其他项目
            }
          }
        }
        
        // 重新加载树结构
        await this.loadTree();
        
        if (successCount === 0) {
          this.showToast("无法移动选中的项目", "error");
        } else if (successCount === totalCount) {
          this.showToast(`已${direction === 'up' ? '上移' : '下移'} ${successCount} 个项目`, "success");
        } else {
          this.showToast(`成功移动 ${successCount}/${totalCount} 个项目`, "warning");
        }
      } catch (error) {
        this.showToast(error.message || "批量移动操作失败", "error");
      }
    },
    editBookmark(node) {
      this.openEdit(node);
    },
    deleteBookmark(node) {
      this.confirmDelete(node);
    },
    async moveSingleBookmark(node, direction) {
      try {
        await this.reorderNode({ node, direction });
        this.showToast(`已${direction === 'up' ? '上移' : '下移'}该项目`, "success");
      } catch (error) {
        this.showToast(error.message || "移动操作失败", "error");
      }
    },
    handleSearchInput(){
      this.searchQuery = this.searchQuery.trim();
      this.clearSearchBtnVisible = this.searchQuery.length > 0;
    },
    clearSearch() {
      this.searchQuery = "";
      this.clearSearchBtnVisible = false;
      this.searchResultVisible = false;
      this.searchResults = [];
      this.isSearching = false;
    },
    handleSearch() {
      if (this.searchQuery.trim().length === 0) {
        this.showToast("请输入搜索内容", "warning");
        return;
      }

      this.searchBookmarks();
    },
    async searchBookmarks() {
      try {
        const query = this.searchQuery.toLowerCase().trim();
        if (!query) return;
        
        // 收集所有书签
        const allBookmarks = this.collectBookmarks(this.tree);
        
        // 过滤符合条件的书签（搜索标题和URL）
        this.searchResults = allBookmarks.filter(bookmark => 
          bookmark.title.toLowerCase().includes(query) || 
          bookmark.url.toLowerCase().includes(query)
        );

        // 更新搜索结果数量
        this.searchResultCount = this.searchResults.length;
        this.searchResultVisible = true;
        
        // 开启搜索模式
        this.isSearching = true;
        this.listTitle = `搜索结果（${this.searchResults.length} 个）`
        
        // 显示搜索结果数量
        this.showToast(`找到 ${this.searchResults.length} 个结果`, "info");
      } catch (error) {
        this.showToast(error.message || "搜索失败", "error");
      }
    },
    // 设置每行显示数量
    setItemsPerRow(num) {
      this.itemsPerRow = num;
      // 保存到数据库
      this.saveItemsPerRow(num);
    },
    async saveItemsPerRow(num) {
      try {
        const response = await fetch('/api/config', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': this.token
          },
          body: JSON.stringify({
            key: 'items_per_row',
            value: num.toString(),
          }),
        });
        if (!response.ok) {
          throw new Error('保存每页显示数量失败');
        }
      } catch (error) {
        console.error('保存每页显示数量失败:', error);
        // 保存到localStorage作为备份
        localStorage.setItem('bookmarks_itemsPerRow', num.toString());
      }
    },
    // 加载配置
    async loadConfig() {
      try {
        const response = await fetch('/api/config', {
          headers: {
            'Authorization': this.token
          }
        });
        if (response.ok) {
          const configData = await response.json();
          // 更新配置，使用默认值作为回退
          this.config.showUrlInList = configData.show_url_in_list !== undefined ? 
            configData.show_url_in_list === 'true' : true;
          this.config.showFolderPath = configData.show_folder_path !== undefined ?
            configData.show_folder_path === 'true' : true;
          this.config.showUpdatedAt = configData.show_updated_at !== undefined ?
            configData.show_updated_at === 'true' : true;
          this.config.showFullTitle = configData.show_full_title !== undefined ?
            configData.show_full_title === 'true' : true;
          // 加载每页显示数量
          if (configData.items_per_row !== undefined) {
            const num = parseInt(configData.items_per_row);
            if (num >= 1 && num <= 6) {
              this.itemsPerRow = num;
            }
          } else {
            // 如果数据库中没有，尝试从localStorage加载
            const savedItemsPerRow = localStorage.getItem('bookmarks_itemsPerRow');
            if (savedItemsPerRow) {
              const num = parseInt(savedItemsPerRow);
              if (num >= 1 && num <= 6) {
                this.itemsPerRow = num;
              }
            }
          }
        }
      } catch (error) {
        console.error('加载配置失败:', error);
        // 使用默认配置
        this.config.showUrlInList = true;
        this.config.showFolderPath = true;
        this.config.showUpdatedAt = true;
        this.config.showFullTitle = true;
        // 尝试从localStorage加载每页显示数量
        const savedItemsPerRow = localStorage.getItem('bookmarks_itemsPerRow');
        if (savedItemsPerRow) {
          const num = parseInt(savedItemsPerRow);
          if (num >= 1 && num <= 6) {
            this.itemsPerRow = num;
          }
        }
      }

      // 加载系统配置（无需认证）
      try {
        const sysResponse = await fetch('/api/config/system');
        if (sysResponse.ok) {
          const sysConfigData = await sysResponse.json();
          this.config.allowRegister = sysConfigData.allow_register !== undefined ?
            sysConfigData.allow_register === 'true' : true;
        }
      } catch (error) {
        console.error('加载系统配置失败:', error);
        // 使用默认配置
      }
    },
    // 保存配置
    async saveConfig() {
      try {
        const configs = [
          { key: 'show_url_in_list', value: this.config.showUrlInList.toString() },
          { key: 'show_folder_path', value: this.config.showFolderPath.toString() },
          { key: 'show_updated_at', value: this.config.showUpdatedAt.toString() },
          { key: 'show_full_title', value: this.config.showFullTitle.toString() }
        ];

        for (const config of configs) {
          const response = await fetch('/api/config', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': this.token
            },
            body: JSON.stringify(config),
          });
          if (!response.ok) {
            throw new Error('保存配置失败');
          }
        }

        // 保存 allow_register 配置（仅管理员）
        if (this.currentUser && this.currentUser.is_admin) {
          const response = await fetch('/api/config', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': this.token
            },
            body: JSON.stringify({
              key: 'allow_register',
              value: this.config.allowRegister.toString()
            }),
          });
          if (!response.ok) {
            const errorData = await response.json();
            throw new Error(errorData.error || '保存允许注册配置失败');
          }
        }

        this.showToast('配置保存成功', 'success');
      } catch (error) {
        console.error('保存配置失败:', error);
        this.showToast(error.message || '配置保存失败', 'error');
      }
    },
    // 打开配置模态框
    openConfigModal() {
      this.configModal.visible = true;
    },
    // 关闭配置模态框
    closeConfigModal() {
      this.configModal.visible = false;
    },
    // 保存配置并关闭模态框
    async saveConfigAndClose() {
      await this.saveConfig();
      this.closeConfigModal();
    },
    // 获取版本号
    async loadVersion() {
      try {
        const response = await fetch('/api/version');
        if (response.ok) {
          const data = await response.json();
          this.version = data.version;
        }
      } catch (error) {
        console.error('获取版本号失败:', error);
      }
    }
  },
  mounted() {
    this.loading = true;
    this.loadSavedTheme();
    this.loadBackgroundSettings();
    this.loadConfig();
    this.checkAuth();
    this.loadTree();
    this.loadVersion();
    window.addEventListener("scroll", this.hideContextMenu, true);
    window.addEventListener("resize", this.hideContextMenu);
  },
});

app.mount("#app");

