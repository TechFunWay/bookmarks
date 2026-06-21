// 清理失效书签 —— admin 控制台标签页组件
window.DeadlinksPanel = {
  name: 'deadlinks-panel',
  template: `
  <div>
    <div class="card" style="margin-bottom:16px;">
      <div class="card-bd" style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;">
        <button class="btn btn-primary" :disabled="checking" @click="startCheck">{{ checking ? '检测中…' : '开始检测' }}</button>
        <button class="btn btn-secondary" v-if="checking" @click="stopCheck">停止检测</button>
        <div style="flex:1;min-width:200px;color:var(--text-secondary);font-size:13px;" v-if="total>0">
          已检测 {{ done }} / {{ total }}，发现失效 {{ deadList.length + suspList.length }}
        </div>
        <div style="color:var(--text-muted);font-size:13px;" v-else>检测书签链接是否可访问。确定失效=打不开(404/无法连接)，疑似失效=可能反爬或临时故障(403/5xx)，请自行确认。</div>
      </div>
      <div style="height:6px;background:var(--bg-elev-3);border-radius:3px;overflow:hidden;" v-if="total>0">
        <div :style="{width: (total? Math.round(done/total*100):0)+'%', height:'100%', background:'var(--accent)', transition:'width .2s'}"></div>
      </div>
    </div>

    <div class="card" v-if="deadList.length || suspList.length">
      <div class="card-hd" style="gap:10px;flex-wrap:wrap;">
        <button class="btn btn-sm btn-secondary" @click="toggleSelectAll">{{ allSelected ? '取消全选' : '全选' }}</button>
        <button class="btn btn-sm btn-danger" :disabled="selectedCount===0" @click="deleteSelected">删除选中 ({{ selectedCount }})</button>
        <button class="btn btn-sm btn-danger" :disabled="deadList.length===0" @click="deleteAllDead">一键删除全部确定失效 ({{ deadList.length }})</button>
      </div>
      <div class="card-bd">
        <div v-if="deadList.length">
          <div style="margin:4px 0 10px;"><span class="badge" style="background:var(--rose-soft);color:var(--rose);">确定失效 {{ deadList.length }}</span></div>
          <div v-for="row in deadList" :key="row.id" class="dl-row">
            <input type="checkbox" v-model="row.checked">
            <img v-if="row.favicon_url" :src="row.favicon_url" class="dl-fav" @error="row.favicon_url=null">
            <div class="dl-main">
              <div class="dl-title">{{ row.title }}</div>
              <a class="dl-url" :href="row.url" target="_blank">{{ row.url }}</a>
              <div class="dl-meta">📁 {{ row.path }} · <span style="color:var(--rose);">{{ row.reason || '无法访问' }}</span></div>
            </div>
            <button class="btn btn-sm btn-ghost" @click="deleteOne(row,'deadList')">删除</button>
          </div>
        </div>
        <div v-if="suspList.length" style="margin-top:16px;">
          <div style="margin:4px 0 10px;"><span class="badge" style="background:var(--amber);color:#1a1a2e;">疑似失效 {{ suspList.length }}</span></div>
          <div v-for="row in suspList" :key="row.id" class="dl-row">
            <input type="checkbox" v-model="row.checked">
            <img v-if="row.favicon_url" :src="row.favicon_url" class="dl-fav" @error="row.favicon_url=null">
            <div class="dl-main">
              <div class="dl-title">{{ row.title }}</div>
              <a class="dl-url" :href="row.url" target="_blank">{{ row.url }}</a>
              <div class="dl-meta">📁 {{ row.path }} · <span style="color:var(--amber);">{{ row.reason }}</span></div>
            </div>
            <button class="btn btn-sm btn-ghost" @click="deleteOne(row,'suspList')">删除</button>
          </div>
        </div>
      </div>
    </div>
    <div class="card" v-else-if="!checking && checkedOnce">
      <div class="card-bd" style="color:var(--text-muted);">未发现失效书签 🎉</div>
    </div>
  </div>`,
  data() {
    return { checking: false, stopped: false, checkedOnce: false, total: 0, done: 0, deadList: [], suspList: [] };
  },
  computed: {
    allSelected() {
      const all = [...this.deadList, ...this.suspList];
      return all.length > 0 && all.every(x => x.checked);
    },
    selectedCount() {
      return [...this.deadList, ...this.suspList].filter(x => x.checked).length;
    },
  },
  methods: {
    authHeaders() {
      const h = { 'Content-Type': 'application/json' };
      const t = localStorage.getItem('token');
      if (t) h['Authorization'] = t;
      return h;
    },
    async loadBookmarks() {
      const r = await fetch('/api/tree', { headers: this.authHeaders() });
      const data = await r.json();
      const out = [];
      const walk = (nodes, path) => {
        for (const n of nodes || []) {
          if (n.type === 'folder') walk(n.children, path + '/' + n.title);
          else if (n.url) out.push({ id: n.id, title: n.title, url: n.url, favicon_url: n.favicon_url, path: path || '/' });
        }
      };
      walk(Array.isArray(data) ? data : (data.children || []), '');
      return out;
    },
    async startCheck() {
      this.checking = true; this.stopped = false; this.checkedOnce = true;
      this.deadList = []; this.suspList = [];
      const all = await this.loadBookmarks();
      this.total = all.length; this.done = 0;
      const byId = Object.fromEntries(all.map(b => [b.id, b]));
      const BATCH = 20;
      for (let i = 0; i < all.length; i += BATCH) {
        if (this.stopped) break;
        const batch = all.slice(i, i + BATCH).map(b => ({ id: b.id, url: b.url }));
        try {
          const r = await fetch('/api/check-links', { method: 'POST', headers: this.authHeaders(), body: JSON.stringify({ bookmarks: batch }) });
          const data = await r.json();
          for (const res of (data.data?.results || [])) {
            const b = byId[res.id]; if (!b) continue;
            const row = { ...b, code: res.code, reason: res.reason, checked: res.category === 'dead' };
            if (res.category === 'dead') this.deadList.push(row);
            else if (res.category === 'suspicious') this.suspList.push(row);
          }
        } catch (e) { console.error('batch failed', e); }
        this.done = Math.min(i + BATCH, all.length);
      }
      this.checking = false;
    },
    stopCheck() { this.stopped = true; },
    async deleteOne(row, list) {
      if (!confirm('确定删除该书签？\n' + row.title)) return;
      const r = await fetch('/api/nodes/' + row.id, { method: 'DELETE', headers: this.authHeaders() });
      if (r.ok) { const arr = this[list]; const idx = arr.findIndex(x => x.id === row.id); if (idx >= 0) arr.splice(idx, 1); }
      else alert('删除失败');
    },
    async deleteSelected() {
      const ids = [...this.deadList, ...this.suspList].filter(x => x.checked).map(x => x.id);
      if (ids.length === 0) { alert('未选中任何书签'); return; }
      if (!confirm('确定删除选中的 ' + ids.length + ' 个书签？')) return;
      await this.batchDelete(ids);
    },
    async deleteAllDead() {
      const ids = this.deadList.map(x => x.id);
      if (ids.length === 0) { alert('没有确定失效的书签'); return; }
      if (!confirm('确定删除全部 ' + ids.length + ' 个确定失效的书签？')) return;
      await this.batchDelete(ids);
    },
    async batchDelete(ids) {
      const r = await fetch('/api/nodes/batch-delete', { method: 'POST', headers: this.authHeaders(), body: JSON.stringify({ ids }) });
      if (r.ok) { const set = new Set(ids); this.deadList = this.deadList.filter(x => !set.has(x.id)); this.suspList = this.suspList.filter(x => !set.has(x.id)); }
      else alert('批量删除失败');
    },
    toggleSelectAll() {
      const target = !this.allSelected;
      this.deadList.forEach(x => x.checked = target);
      this.suspList.forEach(x => x.checked = target);
    },
  },
};

(function(){
  const css = `
  .dl-row{display:flex;align-items:flex-start;gap:10px;padding:10px 0;border-bottom:1px solid var(--border);}
  .dl-row:last-child{border-bottom:none;}
  .dl-fav{width:18px;height:18px;border-radius:4px;flex-shrink:0;margin-top:2px;}
  .dl-main{flex:1;min-width:0;}
  .dl-title{font-weight:600;color:var(--text-primary);font-size:14px;}
  .dl-url{display:block;color:var(--accent);font-size:12px;word-break:break-all;text-decoration:none;}
  .dl-meta{color:var(--text-muted);font-size:12px;margin-top:2px;}`;
  const s = document.createElement('style'); s.textContent = css; document.head.appendChild(s);
})();
