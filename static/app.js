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
  },
  methods: {
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
  },
  template: `
    <li class="tree-item" v-if="isFolder">
      <div :class="['tree-row', { selected: isSelected }]" :style="indentStyle" @contextmenu.prevent="onContextMenu">
        <button class="node-main" type="button" @click="onSelect">
          <span class="node-icon">
            <template v-if="isFolder">
              <span v-if="level === 0">📁</span>
              <span v-else>🗂️</span>
            </template>
            <template v-else>
              <img v-if="getFaviconUrl && getFaviconUrl(node)" :src="getFaviconUrl(node)" alt="favicon" />
              <span v-else>🔖</span>
            </template>
          </span>
          <span class="node-title" :title="node.title">{{ node.title }}</span>
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
      <ul v-if="node.children && node.children.length" class="children">
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
      tree: [],
      loading: false,
      selectedNodeId: null,
      treeActionsVisible: false,
      contextMenu: {
        visible: false,
        x: 0,
        y: 0,
        nodeId: null,
      },
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
      bookmarkEditMode: false,
      selectedBookmarks: new Set(),
      moveModal: {
        visible: false,
        targetParentId: null,
        nodeId: null,
      },
      folderSelectorVisible: false,
      selectedFolderId: null,
      rightClickNode:{

      }
    };
  },
  computed: {
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
  mounted() {
    this.loadSavedTheme(); // 加载保存的主题
    this.loadTree();
    window.addEventListener("scroll", this.hideContextMenu, true);
    window.addEventListener("resize", this.hideContextMenu);
  },
  beforeUnmount() {
    window.removeEventListener("scroll", this.hideContextMenu, true);
    window.removeEventListener("resize", this.hideContextMenu);
  },
  methods: {
    showFolderSelector() {
      this.selectedFolderId = this.modal.parentId || null;
      this.folderSelectorVisible = true;
    },
    selectFolder(folderId) {
      this.selectedFolderId = folderId;
    },
    confirmFolderSelection() {
      this.modal.parentId = this.selectedFolderId;
      this.folderSelectorVisible = false;
    },
    cancelFolderSelection() {
      this.folderSelectorVisible = false;
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
    },
    loadSavedTheme() {
      const savedTheme = localStorage.getItem('bookmark-manager-theme');
      if (savedTheme) {
        document.documentElement.setAttribute('data-theme', savedTheme);
      }
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
    getFaviconUrl(item) {
      // 如果是内网地址，不显示favicon，返回空字符串让前端显示默认图标
      if (this.isIntranetUrl(item.url)) {
        return '';
      }
      
      // 如果有明确的favicon_url，优先使用
      if (item.favicon_url && item.favicon_url.trim()) {
        return item.favicon_url.trim();
      }
      
      // 如果没有favicon_url，则使用默认的 /favicon.ico 路径
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
        const response = await fetch("/api/tree");
        if (!response.ok) {
          throw new Error("加载失败");
        }
        const data = await response.json();
        this.tree = Array.isArray(data) ? data : [];
        if (this.selectedNodeId) {
          const current = this.findNodeById(this.selectedNodeId, this.tree);
          if (!current) {
            const firstFolder = this.findFirstFolder(this.tree);
            this.selectedNodeId = firstFolder ? firstFolder.id : null;
          }
        } else {
          const firstFolder = this.findFirstFolder(this.tree);
          this.selectedNodeId = firstFolder ? firstFolder.id : null;
        }
        if (this.contextMenu.visible) {
          const node = this.contextNode;
          if (!node || node.type !== "folder") {
            this.hideContextMenu();
          }
        }
      } catch (error) {
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
      this.selectedNodeId = node.id;
      if (!this.treeActionsVisible) {
        this.hideContextMenu();
      }
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
        headers: { "Content-Type": "application/json" },
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
      // 允许添加到根目录（parentId为null）
      const payload = {
        url: this.modal.form.url.trim(),
        title: this.modal.form.title.trim(),
        parent_id: this.modal.parentId,
        favicon_url: this.modal.form.favicon_url.trim() || undefined,
      };
      const res = await fetch("/api/bookmarks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
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
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "更新文件夹失败");
      }
    },
    async updateBookmark() {
      const payload = {
        title: this.modal.form.title.trim(),
        url: this.modal.form.url.trim(),
      };
      const favicon = this.modal.form.favicon_url.trim();
      if (favicon) {
        payload.favicon_url = favicon;
      }
      const res = await fetch(`/api/nodes/${this.modal.nodeId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || "更新网址失败");
      }
    },
    async confirmDelete(node) {
      const ok = window.confirm(`确定删除「${node.title}」吗？该操作不可撤销。`);
      if (!ok) return;
      try {
        const res = await fetch(`/api/nodes/${node.id}`, { method: "DELETE" });
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
          headers: { "Content-Type": "application/json" },
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
      this.treeActionsVisible = false;
      if (!node || node.type !== "folder") {
        return;
      }
      this.showContextMenu(node, x, y);
    },
    showBookmarkActions(node, event) {
      this.treeActionsVisible = false;
      this.showContextMenu(node, event.clientX, event.clientY);
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
      // 点击根元素时总是隐藏右键菜单
      this.hideContextMenu();
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
      this.toast.visible = true;
      this.toast.message = message;
      this.toast.type = type;
      if (this.toast.timer) {
        clearTimeout(this.toast.timer);
      }
      this.toast.timer = setTimeout(() => {
        this.toast.visible = false;
        this.toast.timer = null;
      }, 2200);
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
      const ok = window.confirm(`确定删除选中的 ${count} 个网址吗？该操作不可撤销。`);
      if (!ok) return;
      
      try {
        const promises = Array.from(this.selectedBookmarks).map(id => 
          fetch(`/api/nodes/${id}`, { method: "DELETE" })
        );
        const results = await Promise.all(promises);
        
        // 检查是否有失败的请求
        const failedRequests = results.filter(res => !res.ok);
        if (failedRequests.length > 0) {
          throw new Error(`${failedRequests.length} 个删除操作失败`);
        }
        
        // 清理状态
        this.selectedBookmarks.clear();
        this.bookmarkEditMode = false;
        await this.loadTree();
        this.showToast(`成功删除 ${count} 个网址`, "success");
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
  },
});

app.mount("#app");

