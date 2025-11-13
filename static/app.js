const { createApp } = Vue;

const BookmarkNode = {
  name: "BookmarkNode",
  props: {
    node: { type: Object, required: true },
    level: { type: Number, default: 0 },
    selectedId: { type: Number, default: null },
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
        this.$emit("select", this.node);
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
            <template v-if="isFolder">📁</template>
            <template v-else>
              <img v-if="node.favicon_url" :src="node.favicon_url" alt="favicon" />
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
          <button v-if="isFolder" type="button" title="添加网址" @click="onAddBookmark">
            <span class="action-icon">➕</span>
          </button>
          <button type="button" title="编辑" @click="onEdit">
            <span class="action-icon">✏️</span>
          </button>
          <button type="button" title="删除" @click="onDelete">
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
    this.loadTree();
    window.addEventListener("scroll", this.hideContextMenu, true);
    window.addEventListener("resize", this.hideContextMenu);
  },
  beforeUnmount() {
    window.removeEventListener("scroll", this.hideContextMenu, true);
    window.removeEventListener("resize", this.hideContextMenu);
  },
  methods: {
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
    openAddBookmark(parent) {
      this.hideContextMenu();
      this.modal.visible = true;
      this.modal.type = "add-bookmark";
      this.modal.parentId = parent ? parent.id : null;
      this.modal.nodeId = null;
      this.modal.form.title = "";
      this.modal.form.url = "";
      this.modal.form.favicon_url = "";
      this.metadataError = "";
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
      const padding = 16;
      const menuWidth = 220;
      const menuHeight = 240;
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
      if (!this.treeActionsVisible) {
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
            path: trail.join(" / "),
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
        if (data.title && !this.modal.form.title) {
          this.modal.form.title = data.title;
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
  },
});

app.mount("#app");

