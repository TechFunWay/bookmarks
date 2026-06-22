// 去重复 —— admin 控制台标签页组件
window.DuplicatePanel = {
  name: 'duplicate-panel',
  template: `
  <div>
    <!-- 自定义确认弹窗 -->
    <transition name="fade">
      <div v-if="dlg.show" class="dup-overlay" @click.self="dlg.show=false">
        <div class="dup-dialog">
          <div class="dup-dlg-title">{{ dlg.title }}</div>
          <div class="dup-dlg-body">{{ dlg.message }}</div>
          <div class="dup-dlg-actions">
            <button v-if="dlg.type!=='alert'" class="btn btn-secondary" @click="dlg.show=false">取消</button>
            <button class="btn" :class="dlg.type==='danger'?'btn-danger':'btn-primary'" @click="dlg.show=false; dlg.onOk()">{{ dlg.okText || '确定' }}</button>
          </div>
        </div>
      </div>
    </transition>

    <div class="card" style="margin-bottom:16px;">
      <div class="card-bd">
        <div style="display:flex;flex-wrap:wrap;gap:8px 16px;align-items:center;font-size:13px;color:var(--text-secondary);">
          <span style="font-weight:600;">重复判定规则：</span>
          <label style="display:flex;align-items:center;gap:4px;cursor:pointer;"><input type="checkbox" v-model="opts.ignoreScheme" :disabled="loading"> HTTP和HTTPS计重复</label>
          <label style="display:flex;align-items:center;gap:4px;cursor:pointer;"><input type="checkbox" v-model="opts.ignoreWWW" :disabled="loading"> www.与无www计重复</label>
          <label style="display:flex;align-items:center;gap:4px;cursor:pointer;"><input type="checkbox" v-model="opts.ignoreTrailingSlash" :disabled="loading"> 末尾有无/计重复</label>
          <label style="display:flex;align-items:center;gap:4px;cursor:pointer;"><input type="checkbox" v-model="opts.ignoreQuery" :disabled="loading"> ?后参数计重复</label>
          <label style="display:flex;align-items:center;gap:4px;cursor:pointer;"><input type="checkbox" v-model="opts.crossFolder" :disabled="loading"> 不同文件夹计重复</label>
        </div>
        <div style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin-top:12px;">
          <button class="btn btn-primary" :disabled="loading" @click="check">{{ loading ? '检测中...' : '开始检测' }}</button>
          <div style="flex:1;min-width:200px;color:var(--text-secondary);font-size:13px;" v-if="checkedOnce">
            共 {{ stats.total }} 个书签，发现 {{ stats.groups }} 组重复（{{ stats.dupBookmarks }} 个）
          </div>
          <div style="color:var(--text-muted);font-size:13px;" v-else>选择判定规则后开始检测，默认可区分 HTTP/HTTPS 同网址重复。</div>
        </div>
      </div>
    </div>

    <div class="card" v-if="groups.length">
      <div class="card-hd" style="gap:10px;flex-wrap:wrap;">
        <button class="btn btn-sm btn-secondary" @click="toggleSelectAll">{{ allSelected ? '取消全选' : '全选' }}</button>
        <button class="btn btn-sm btn-danger" :disabled="selectedCount===0" @click="deleteSelected">删除选中 ({{ selectedCount }})</button>
        <button class="btn btn-sm btn-danger" @click="cleanAll">一键清理（保留每组一个）</button>
      </div>
      <div class="card-bd">
        <div v-for="g in groups" :key="g.url" style="padding:12px 0;border-bottom:1px solid var(--border);">
          <a class="dup-url" :href="g.url" target="_blank">{{ g.url }}</a>
          <span class="badge" style="margin-left:8px;background:var(--bg-elev-3);color:var(--text-secondary);">{{ g.bookmarks.length }} 个</span>
          <span class="badge" v-if="g.hasSchemeMismatch" style="margin-left:4px;background:var(--amber);color:var(--text-primary);">⚠️ HTTP/HTTPS 混合</span>
          <div v-for="(b,bi) in g.bookmarks" :key="b.id" class="dup-row">
            <input type="checkbox" v-model="b.checked">
            <div class="dup-main">
              <div class="dup-title">{{ b.title }}</div>
              <div class="dup-meta">📁 {{ b.path }}</div>
            </div>
            <button class="btn btn-sm btn-ghost" @click="del(g, b)">删除</button>
          </div>
        </div>
      </div>
    </div>
    <div class="card" v-else-if="checkedOnce && !loading">
      <div class="card-bd" style="color:var(--text-muted);">未发现重复书签 🎉</div>
    </div>
  </div>`,
  data() {
    return {
      loading: false, checkedOnce: false, groups: [], stats: { total: 0, groups: 0, dupBookmarks: 0 },
      opts: { ignoreScheme: true, ignoreWWW: false, ignoreTrailingSlash: false, ignoreQuery: false, crossFolder: false },
      dlg: { show: false, title: '', message: '', type: 'alert', okText: '', onOk() {} },
    };
  },
  computed: {
    allSelected() {
      const all = this.groups.flatMap(g => g.bookmarks);
      return all.length > 0 && all.every(b => b.checked);
    },
    selectedCount() {
      return this.groups.flatMap(g => g.bookmarks).filter(b => b.checked).length;
    },
  },
  methods: {
    authHeaders() {
      const h = { 'Content-Type': 'application/json' };
      const t = localStorage.getItem('token');
      if (t) h['Authorization'] = t;
      return h;
    },
    showAlert(msg) {
      this.dlg = { show: true, title: '提示', message: msg, type: 'alert', okText: '确定', onOk() {} };
    },
    showConfirm(title, msg, danger, onOk) {
      this.dlg = { show: true, title, message: msg, type: danger ? 'danger' : 'confirm', okText: danger ? '确认删除' : '确定', onOk };
    },
    async check() {
      this.loading = true; this.checkedOnce = true;
      try {
        const params = new URLSearchParams();
        if (this.opts.ignoreScheme) params.set('ignore_scheme', 'true');
        if (this.opts.ignoreWWW) params.set('ignore_www', 'true');
        if (this.opts.ignoreTrailingSlash) params.set('ignore_trailing_slash', 'true');
        if (this.opts.ignoreQuery) params.set('ignore_query', 'true');
        if (this.opts.crossFolder) params.set('cross_folder', 'true');
        const r = await fetch('/api/check-duplicates?' + params.toString(), { headers: this.authHeaders() });
        const data = await r.json();
        const d = data.data || {};
        this.groups = (d.duplicates || []).map(x => ({
          url: x.url, hasSchemeMismatch: x.hasSchemeMismatch,
          bookmarks: (x.bookmarks || []).map((b, i) => ({ ...b, checked: i > 0 })),
        }));
        this.stats = { total: d.totalBookmarks || 0, groups: d.duplicateCount || 0, dupBookmarks: d.duplicateBookmarksCount || 0 };
      } catch (e) { console.error(e); this.showAlert('检测失败，请重试'); }
      this.loading = false;
    },
    del(g, b) {
      this.showConfirm('删除书签', '确定删除「' + b.title + '」？', true, async () => {
        const r = await fetch('/api/nodes/' + b.id, { method: 'DELETE', headers: this.authHeaders() });
        if (r.ok) {
          g.bookmarks = g.bookmarks.filter(x => x.id !== b.id);
          if (g.bookmarks.length <= 1) this.groups = this.groups.filter(x => x.url !== g.url);
        } else this.showAlert('删除失败');
      });
    },
    toggleSelectAll() {
      const target = !this.allSelected;
      this.groups.forEach(g => g.bookmarks.forEach(b => b.checked = target));
    },
    deleteSelected() {
      const ids = this.groups.flatMap(g => g.bookmarks).filter(b => b.checked).map(b => b.id);
      if (ids.length === 0) { this.showAlert('未选中任何书签'); return; }
      this.showConfirm('批量删除', '确定删除选中的 ' + ids.length + ' 个书签？', true, () => this.batchDelete(ids));
    },
    cleanAll() {
      const ids = this.groups.flatMap(g => g.bookmarks.slice(1)).map(b => b.id);
      if (ids.length === 0) { this.showAlert('没有可清理的重复书签'); return; }
      this.showConfirm('一键清理', '确定一键清理 ' + ids.length + ' 个重复书签？\n每组将保留一个，其余全部删除。', true, () => this.batchDelete(ids));
    },
    async batchDelete(ids) {
      const r = await fetch('/api/nodes/batch-delete', {
        method: 'POST', headers: this.authHeaders(),
        body: JSON.stringify({ ids }),
      });
      if (r.ok) {
        const set = new Set(ids);
        this.groups = this.groups.map(g => ({
          ...g, bookmarks: g.bookmarks.filter(b => !set.has(b.id)),
        })).filter(g => g.bookmarks.length > 1);
      } else this.showAlert('批量删除失败');
    },
  },
};

(function(){
  const css = `
  .dup-url{color:var(--accent);font-size:13px;word-break:break-all;text-decoration:none;font-weight:600;}
  .dup-row{display:flex;align-items:center;gap:10px;padding:6px 0 6px 14px;}
  .dup-main{flex:1;min-width:0;}
  .dup-title{color:var(--text-primary);font-size:14px;}
  .dup-meta{color:var(--text-muted);font-size:12px;}
  .dup-overlay{position:fixed;inset:0;background:rgba(0,0,0,.7);backdrop-filter:blur(8px);-webkit-backdrop-filter:blur(8px);z-index:9999;display:flex;align-items:center;justify-content:center;padding:20px;}
  .dup-dialog{background:var(--bg-elev-1);border:1px solid var(--border-strong);border-radius:16px;padding:24px;min-width:360px;max-width:480px;box-shadow:0 24px 80px rgba(0,0,0,.5);}
  .dup-dlg-title{font-size:16px;font-weight:700;color:var(--text-primary);margin-bottom:12px;}
  .dup-dlg-body{font-size:14px;color:var(--text-secondary);line-height:1.6;white-space:pre-wrap;margin-bottom:20px;}
  .dup-dlg-actions{display:flex;gap:10px;justify-content:flex-end;}`;
  const s = document.createElement('style'); s.textContent = css; document.head.appendChild(s);
})();
