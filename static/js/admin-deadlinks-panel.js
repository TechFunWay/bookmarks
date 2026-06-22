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
        <div class="seg">
          <button :class="['seg-btn',{active:viewMode==='severity'}]" @click="viewMode='severity'">按失效程度</button>
          <button :class="['seg-btn',{active:viewMode==='status'}]" @click="viewMode='status'">按状态码</button>
        </div>
        <button class="btn btn-sm btn-secondary" @click="toggleSelectAll">{{ allSelected ? '取消全选' : '全选' }}</button>
        <button class="btn btn-sm btn-danger" :disabled="selectedCount===0" @click="deleteSelected">删除选中 ({{ selectedCount }})</button>
        <button class="btn btn-sm btn-danger" :disabled="deadList.length===0" @click="deleteAllDead">一键删除全部确定失效 ({{ deadList.length }})</button>
      </div>
      <!-- 按失效程度 -->
      <div class="card-bd" v-if="viewMode==='severity'">
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
            <button class="btn btn-sm btn-ghost" @click="deleteOne(row)">删除</button>
          </div>
        </div>
        <div v-if="suspList.length" style="margin-top:16px;">
          <div style="margin:4px 0 10px;"><span class="badge" style="background:var(--amber);color:var(--text-primary);">疑似失效 {{ suspList.length }}</span></div>
          <div v-for="row in suspList" :key="row.id" class="dl-row">
            <input type="checkbox" v-model="row.checked">
            <img v-if="row.favicon_url" :src="row.favicon_url" class="dl-fav" @error="row.favicon_url=null">
            <div class="dl-main">
              <div class="dl-title">{{ row.title }}</div>
              <a class="dl-url" :href="row.url" target="_blank">{{ row.url }}</a>
              <div class="dl-meta">📁 {{ row.path }} · <span style="color:var(--amber);">{{ row.reason }}</span></div>
            </div>
            <button class="btn btn-sm btn-ghost" @click="deleteOne(row)">删除</button>
          </div>
        </div>
      </div>
      <!-- 按状态码 -->
      <div class="card-bd" v-else>
        <div v-for="group in statusGroups" :key="group.code">
          <div class="dl-group-hd">
            <span class="badge" :style="{background: group.dead?'var(--rose-soft)':'var(--amber)', color: group.dead?'var(--rose)':'var(--text-primary)'}">{{ group.code===0 ? '—' : group.code }} {{ group.label }} · {{ group.rows.length }}</span>
            <button class="btn btn-sm btn-ghost" @click="selectGroup(group)">{{ group.rows.every(r=>r.checked) ? '取消本组' : '全选本组' }}</button>
            <button class="btn btn-sm btn-danger" @click="deleteGroup(group)">删除本组</button>
          </div>
          <div v-for="row in group.rows" :key="row.id" class="dl-row">
            <input type="checkbox" v-model="row.checked">
            <img v-if="row.favicon_url" :src="row.favicon_url" class="dl-fav" @error="row.favicon_url=null">
            <div class="dl-main">
              <div class="dl-title">{{ row.title }}</div>
              <a class="dl-url" :href="row.url" target="_blank">{{ row.url }}</a>
              <div class="dl-meta">📁 {{ row.path }} · <span :style="{color: group.dead?'var(--rose)':'var(--amber)'}">{{ row.reason || '无法访问' }}</span></div>
            </div>
            <button class="btn btn-sm btn-ghost" @click="deleteOne(row)">删除</button>
          </div>
        </div>
      </div>
    </div>
    <div class="card" v-else-if="!checking && checkedOnce">
      <div class="card-bd" style="color:var(--text-muted);">未发现失效书签 🎉</div>
    </div>
  </div>`,
  data() {
    return { checking: false, stopped: false, checkedOnce: false, total: 0, done: 0, deadList: [], suspList: [], viewMode: 'severity' };
  },
  computed: {
    allSelected() {
      const all = [...this.deadList, ...this.suspList];
      return all.length > 0 && all.every(x => x.checked);
    },
    selectedCount() {
      return [...this.deadList, ...this.suspList].filter(x => x.checked).length;
    },
    // 按状态码归组：deadList + suspList 合并后按 code 分组，无法连接(code 0)置顶，其余按状态码升序
    statusGroups() {
      const map = new Map();
      for (const r of [...this.deadList, ...this.suspList]) {
        const code = r.code || 0;
        if (!map.has(code)) map.set(code, []);
        map.get(code).push(r);
      }
      return [...map.entries()]
        .sort((a, b) => (a[0] === 0 ? -1 : b[0] === 0 ? 1 : a[0] - b[0]))
        .map(([code, rows]) => ({ code, rows, label: window.deadlinkStatusLabel(code), dead: window.deadlinkIsDeadCode(code) }));
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
      if (!this.stopped) this.done = this.total;
      this.checking = false;
    },
    stopCheck() { this.stopped = true; },
    async deleteOne(row) {
      if (!confirm('确定删除该书签？\n' + row.title)) return;
      const r = await fetch('/api/nodes/' + row.id, { method: 'DELETE', headers: this.authHeaders() });
      if (r.ok) this.removeIds([row.id]);
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
    selectGroup(group) {
      const target = !group.rows.every(r => r.checked);
      group.rows.forEach(r => r.checked = target);
    },
    async deleteGroup(group) {
      const ids = group.rows.map(r => r.id);
      if (ids.length === 0) return;
      if (!confirm('确定删除「' + group.label + '」分组的 ' + ids.length + ' 个书签？')) return;
      await this.batchDelete(ids);
    },
    async batchDelete(ids) {
      const r = await fetch('/api/nodes/batch-delete', { method: 'POST', headers: this.authHeaders(), body: JSON.stringify({ ids }) });
      if (r.ok) this.removeIds(ids);
      else alert('批量删除失败');
    },
    removeIds(ids) {
      const set = new Set(ids);
      this.deadList = this.deadList.filter(x => !set.has(x.id));
      this.suspList = this.suspList.filter(x => !set.has(x.id));
    },
    toggleSelectAll() {
      const target = !this.allSelected;
      this.deadList.forEach(x => x.checked = target);
      this.suspList.forEach(x => x.checked = target);
    },
  },
};

// HTTP 状态码 → 中文标签（清理失效「按状态码」视图共用，挂到 window 供手机端复用）
window.deadlinkStatusLabel = function(code) {
  const m = { 0: '无法连接/超时', 403: '禁止访问', 404: '链接不存在', 410: '已永久删除', 429: '请求过多', 500: '服务器错误', 502: '网关错误', 503: '服务不可用', 504: '网关超时' };
  return m[code] || ('HTTP ' + code);
};
// 是否归为「确定失效」类（红色）：无法连接、404、410
window.deadlinkIsDeadCode = function(code) {
  return code === 0 || code === 404 || code === 410;
};

(function(){
  const css = `
  .dl-row{display:flex;align-items:flex-start;gap:10px;padding:10px 0;border-bottom:1px solid var(--border);}
  .dl-row:last-child{border-bottom:none;}
  .dl-fav{width:18px;height:18px;border-radius:4px;flex-shrink:0;margin-top:2px;}
  .dl-main{flex:1;min-width:0;}
  .dl-title{font-weight:600;color:var(--text-primary);font-size:14px;}
  .dl-url{display:block;color:var(--accent);font-size:12px;word-break:break-all;text-decoration:none;}
  .dl-meta{color:var(--text-muted);font-size:12px;margin-top:2px;}
  .seg{display:inline-flex;background:var(--bg-elev-3);border-radius:8px;padding:2px;gap:2px;}
  .seg-btn{border:none;background:transparent;color:var(--text-secondary);font-size:13px;padding:5px 12px;border-radius:6px;cursor:pointer;}
  .seg-btn.active{background:var(--bg-elev-1);color:var(--text-primary);font-weight:600;box-shadow:var(--shadow-sm);}
  .dl-group-hd{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:16px 0 8px;}`;
  const s = document.createElement('style'); s.textContent = css; document.head.appendChild(s);
})();
