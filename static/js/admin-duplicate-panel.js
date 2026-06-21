// 去重复 —— admin 控制台标签页组件
window.DuplicatePanel = {
  name: 'duplicate-panel',
  template: `
  <div>
    <div class="card" style="margin-bottom:16px;">
      <div class="card-bd" style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;">
        <button class="btn btn-primary" :disabled="loading" @click="check">{{ loading ? '检测中...' : '开始检测' }}</button>
        <div style="flex:1;min-width:200px;color:var(--text-secondary);font-size:13px;" v-if="checkedOnce">
          共 {{ stats.total }} 个书签，发现 {{ stats.groups }} 组重复（{{ stats.dupBookmarks }} 个）
        </div>
        <div style="color:var(--text-muted);font-size:13px;" v-else>按网址查找重复书签，逐个删除多余项。</div>
      </div>
    </div>

    <div class="card" v-if="groups.length">
      <div class="card-bd">
        <div v-for="g in groups" :key="g.url" style="padding:12px 0;border-bottom:1px solid var(--border);">
          <a class="dup-url" :href="g.url" target="_blank">{{ g.url }}</a>
          <span class="badge" style="margin-left:8px;background:var(--bg-elev-3);color:var(--text-secondary);">{{ g.bookmarks.length }} 个</span>
          <div v-for="b in g.bookmarks" :key="b.id" class="dup-row">
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
    return { loading: false, checkedOnce: false, groups: [], stats: { total: 0, groups: 0, dupBookmarks: 0 } };
  },
  methods: {
    authHeaders() {
      const h = { 'Content-Type': 'application/json' };
      const t = localStorage.getItem('token');
      if (t) h['Authorization'] = t;
      return h;
    },
    async check() {
      this.loading = true; this.checkedOnce = true;
      try {
        const r = await fetch('/api/check-duplicates', { headers: this.authHeaders() });
        const data = await r.json();
        const d = data.data || {};
        this.groups = (d.duplicates || []).map(x => ({ url: x.url, bookmarks: x.bookmarks || [] }));
        this.stats = { total: d.totalBookmarks || 0, groups: d.duplicateCount || 0, dupBookmarks: d.duplicateBookmarksCount || 0 };
      } catch (e) { console.error(e); alert('检测失败'); }
      this.loading = false;
    },
    async del(g, b) {
      if (!confirm('确定删除该书签？\n' + b.title)) return;
      const r = await fetch('/api/nodes/' + b.id, { method: 'DELETE', headers: this.authHeaders() });
      if (r.ok) {
        g.bookmarks = g.bookmarks.filter(x => x.id !== b.id);
        if (g.bookmarks.length <= 1) this.groups = this.groups.filter(x => x.url !== g.url);
      } else alert('删除失败');
    },
  },
};

(function(){
  const css = `
  .dup-url{color:var(--accent);font-size:13px;word-break:break-all;text-decoration:none;font-weight:600;}
  .dup-row{display:flex;align-items:center;gap:10px;padding:6px 0 6px 14px;}
  .dup-main{flex:1;min-width:0;}
  .dup-title{color:var(--text-primary);font-size:14px;}
  .dup-meta{color:var(--text-muted);font-size:12px;}`;
  const s = document.createElement('style'); s.textContent = css; document.head.appendChild(s);
})();
