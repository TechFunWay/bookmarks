/**
 * Template Shell - Shared JS for read-only public display templates.
 */
(function(){
  window.TemplateShell={
    async loadSystemConfig(){
      try{
        const r=await fetch('/api/config/system');
        if(!r.ok)return{};
        const d=await r.json();
        if(d&&typeof d==='object'&&!Array.isArray(d))return d;
        return(d&&d.data&&typeof d.data==='object')?d.data:{};
      }catch(e){return{};}
    },
    async loadPublicTree(){
      try{
        const r=await fetch('/api/public-tree');
        if(!r.ok)return[];
        const d=await r.json();
        if(Array.isArray(d))return d;
        return Array.isArray(d&&d.data)?d.data:[];
      }catch(e){return[];}
    },
    async loadUserTree(){
      const token=localStorage.getItem('token');
      if(token){
        try{
          const r=await fetch('/api/tree',{headers:{'Authorization':token}});
          if(r.ok){const d=await r.json();return Array.isArray(d)?d:[];}
        }catch(e){}
      }
      return this.loadPublicTree();
    },
    getCurrentTemplate(){
      const m=window.location.pathname.match(/\/template-(magazine|masonry|timeline|minimal|grid|table|nav)\.html/);
      return m?m[1]:null;
    },
    getRememberedTemplate(sys){
      const s=localStorage.getItem('bookmark_template');
      if(s&&['magazine','masonry','timeline','minimal','grid','table','nav'].includes(s))return s;
      if(sys&&sys.default_template)return sys.default_template;
      return 'magazine';
    },
    setUserTemplate(n){
      if(['magazine','masonry','timeline','minimal','grid','table','nav'].includes(n)){
        localStorage.setItem('bookmark_template',n);
        window.location.href=`/template-${n}.html`;
      }else if(n==='view-sidebar'||n==='view-portal'){
        localStorage.setItem('bookmark_template',n);
        window.location.href=`/${n}.html`;
      }
    },
    flattenTree(nodes){
      const res=[];
      function tr(list,bc){
        if(!list||!Array.isArray(list))return;
        for(const n of list){
          if(n.type==='folder'){
            if(n.children&&n.children.length)tr(n.children,[...bc,n.title]);
          }else if(n.type==='bookmark'){
            res.push({id:n.id,title:n.title,url:n.url,favicon_url:n.favicon_url,remark:n.remark,visibility:n.visibility,created_at:n.created_at,updated_at:n.updated_at,breadcrumb:bc});
          }
        }
      }
      tr(nodes,[]);
      return res;
    },
    groupByFolder(nodes){
      const res={};
      function tr(list){
        if(!list||!Array.isArray(list))return;
        for(const n of list){
          if(n.type==='folder'){
            const fn=n.title;
            if(!res[fn])res[fn]=[];
            if(n.children){
              for(const c of n.children){
                if(c.type==='bookmark'){
                  res[fn].push({id:c.id,title:c.title,url:c.url,favicon_url:c.favicon_url,remark:c.remark,visibility:c.visibility,created_at:c.created_at,updated_at:c.updated_at,breadcrumb:[fn]});
                }
              }
              tr(n.children.filter(c=>c.type==='folder'));
            }
          }
        }
      }
      tr(nodes);
      return res;
    },
    groupByMonth(bms){
      if(!bms||!Array.isArray(bms))return[];
      const grps={};
      for(const b of bms){
        const ds=b.updated_at||b.created_at;
        if(!ds)continue;
        try{
          const d=new Date(ds);
          if(isNaN(d.getTime()))continue;
          const y=d.getFullYear(),m=d.getMonth()+1;
          const sk=`${y}-${m.toString().padStart(2,'0')}`;
          if(!grps[sk])grps[sk]={label:`${y}年${m}月`,sk,items:[]};
          grps[sk].items.push(b);
        }catch(e){}
      }
      const s=Object.values(grps).sort((a,b)=>b.sk.localeCompare(a.sk));
      for(const g of s){
        g.items.sort((a,b)=>new Date(b.updated_at||b.created_at||0).getTime()-new Date(a.updated_at||a.created_at||0).getTime());
        delete g.sk;
      }
      return s;
    },
    mountSwitcher(tgt,cur){
      if(!tgt||tgt.querySelector('.t-sw'))return;
      const c=document.createElement('div');
      c.className='t-sw';
      const st=document.createElement('style');
      st.textContent=`
.t-sw{position:fixed;bottom:24px;right:24px;z-index:9999;font-family:system-ui,sans-serif}
.t-sw-b{width:48px;height:48px;border-radius:50%;background:var(--bg,#1a1a1a);color:var(--tc,#fff);border:1px solid var(--bc,#333);cursor:pointer;display:flex;align-items:center;justify-content:center;box-shadow:0 4px 12px rgba(0,0,0,.2);transition:transform .2s}
.t-sw-b:hover{transform:scale(1.05)}
.t-sw-b svg{width:24px;height:24px;fill:currentColor}
.t-sw-m{position:absolute;bottom:88px;right:0;width:280px;background:var(--bg,#1a1a1a);border:1px solid var(--bc,#333);border-radius:12px;box-shadow:0 8px 24px rgba(0,0,0,.3);padding:8px;display:none;flex-direction:column;gap:4px}
.t-sw-m.open{display:flex}
.t-sw-i{display:flex;align-items:center;gap:12px;padding:12px;border-radius:8px;cursor:pointer;color:var(--tc,#fff);background:0 0;border:none;width:100%;text-align:left;font-size:14px}
.t-sw-i:hover,.t-sw-i:focus{background:var(--hc,#333);outline:0}
.t-sw-i[aria-current="page"]{background:var(--ab,#2a2a2a);font-weight:600}
.t-sw-ic{width:24px;height:24px;display:flex;align-items:center;justify-content:center;opacity:.7}
.t-sw-i[aria-current="page"] .t-sw-ic{opacity:1;color:var(--pc,#3b82f6)}
.t-sw-ck{margin-left:auto;width:16px;height:16px;opacity:0}
.t-sw-i[aria-current="page"] .t-sw-ck{opacity:1;color:var(--pc,#3b82f6)}
html[data-theme="light"] .t-sw{--bg:#fff;--tc:#111;--bc:#e5e5e5;--hc:#f5f5f5;--ab:#f0f7ff;--pc:#2563eb}
`;
      c.appendChild(st);
      const gi=`<svg viewBox="0 0 24 24"><path d="M4 4h4v4H4zm6 0h4v4h-4zm6 0h4v4h-4zM4 10h4v4H4zm6 0h4v4h-4zm6 0h4v4h-4zM4 16h4v4H4zm6 0h4v4h-4zm6 0h4v4h-4z"/></svg>`;
      const ci=`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
      const tpls=[
        {id:'magazine',n:'杂志',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="18" height="8" rx="1"/><rect x="3" y="13" width="8" height="8" rx="1"/><rect x="13" y="13" width="8" height="8" rx="1"/></svg>'},
        {id:'masonry',n:'瀑布流',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="8" height="12" rx="1"/><rect x="13" y="3" width="8" height="6" rx="1"/><rect x="3" y="17" width="8" height="4" rx="1"/><rect x="13" y="11" width="8" height="10" rx="1"/></svg>'},
        {id:'timeline',n:'时间线',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="8" y="4" width="12" height="4" rx="1"/><rect x="8" y="10" width="12" height="4" rx="1"/><rect x="8" y="16" width="12" height="4" rx="1"/><circle cx="4" cy="6" r="2"/><circle cx="4" cy="12" r="2"/><circle cx="4" cy="18" r="2"/><path d="M4 8v2M4 14v2" stroke="currentColor" stroke-width="2"/></svg>'},
        {id:'minimal',n:'极简',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="6" width="18" height="2" rx="1"/><rect x="3" y="11" width="18" height="2" rx="1"/><rect x="3" y="16" width="18" height="2" rx="1"/></svg>'},
        {id:'grid',n:'卡片网格',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="8" height="8" rx="1"/><rect x="13" y="3" width="8" height="8" rx="1"/><rect x="3" y="13" width="8" height="8" rx="1"/><rect x="13" y="13" width="8" height="8" rx="1"/></svg>'},
        {id:'table',n:'表格',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="5" width="16" height="3" rx="1"/><rect x="4" y="10" width="16" height="3" rx="1"/><rect x="4" y="15" width="16" height="3" rx="1"/><line x1="10" y1="5" x2="10" y2="18" stroke="currentColor" stroke-width="0.5"/><line x1="16" y1="5" x2="16" y2="18" stroke="currentColor" stroke-width="0.5"/></svg>'},
        {id:'nav',n:'导航',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="6" height="18" rx="1"/><rect x="11" y="3" width="10" height="5" rx="1"/><rect x="11" y="10" width="10" height="5" rx="1"/><rect x="11" y="17" width="10" height="4" rx="1"/></svg>'},
        {id:'view-sidebar',n:'侧边',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="4" width="6" height="16" rx="1"/><rect x="11" y="4" width="10" height="4" rx="1"/><rect x="11" y="10" width="10" height="4" rx="1"/><rect x="11" y="16" width="10" height="4" rx="1"/></svg>'},
        {id:'view-portal',n:'门户',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="18" height="6" rx="1"/><rect x="3" y="11" width="4" height="4" rx="1"/><rect x="9" y="11" width="4" height="4" rx="1"/><rect x="15" y="11" width="4" height="4" rx="1"/><rect x="3" y="17" width="4" height="4" rx="1"/><rect x="9" y="17" width="4" height="4" rx="1"/><rect x="15" y="17" width="4" height="4" rx="1"/></svg>'}
      ];
      const b=document.createElement('button');
      b.className='t-sw-b';
      b.setAttribute('aria-haspopup','menu');
      b.setAttribute('aria-expanded','false');
      b.innerHTML=gi;
      c.appendChild(b);
      const m=document.createElement('div');
      m.className='t-sw-m';
      m.setAttribute('role','menu');
      tpls.forEach(t=>{
        const i=document.createElement('button');
        i.className='t-sw-i';
        i.setAttribute('role','menuitem');
        i.setAttribute('data-template',t.id);
        i.setAttribute('tabindex','-1');
        if(t.id===cur)i.setAttribute('aria-current','page');
        i.innerHTML=`<span class="t-sw-ic">${t.i}</span><span class="t-sw-n"></span><span class="t-sw-ck">${ci}</span>`;
        i.querySelector('.t-sw-n').textContent=t.n;
        m.appendChild(i);
      });
      c.appendChild(m);
      tgt.appendChild(c);
      let op=false;
      const tg=s=>{
        op=s!==undefined?s:!op;
        b.setAttribute('aria-expanded',op.toString());
        if(op){
          m.classList.add('open');
          const a=m.querySelector('[aria-current="page"]')||m.firstElementChild;
          if(a)a.focus();
        }else{
          m.classList.remove('open');
          b.focus();
        }
      };
      b.addEventListener('click',e=>{e.stopPropagation();tg();});
      document.addEventListener('click',e=>{if(op&&!c.contains(e.target))tg(false);});
      c.addEventListener('keydown',e=>{
        if(e.key==='Escape'){tg(false);return;}
        if(!op&&(e.key==='Enter'||e.key===' ')){
          if(e.target===b){e.preventDefault();tg(true);}
          return;
        }
        if(op){
          const is=Array.from(m.querySelectorAll('[role="menuitem"]'));
          const ci=is.indexOf(document.activeElement);
          if(e.key==='ArrowDown'){e.preventDefault();is[ci<is.length-1?ci+1:0].focus();}
          else if(e.key==='ArrowUp'){e.preventDefault();is[ci>0?ci-1:is.length-1].focus();}
          else if(e.key==='Home'){e.preventDefault();is[0].focus();}
          else if(e.key==='End'){e.preventDefault();is[is.length-1].focus();}
          else if(e.key==='Enter'||e.key===' '){
            e.preventDefault();
            const t=e.target.closest('[data-template]');
            if(t)window.TemplateShell.setUserTemplate(t.getAttribute('data-template'));
          }
        }
      });
      m.addEventListener('click',e=>{
        const t=e.target.closest('[data-template]');
        if(t)window.TemplateShell.setUserTemplate(t.getAttribute('data-template'));
      });
    },
    initTheme(){
      const s=localStorage.getItem('bookmark_theme');
      document.documentElement.setAttribute('data-theme',s==='light'?'light':'dark');
    },
    toggleTheme(){
      const n=document.documentElement.getAttribute('data-theme')==='light'?'dark':'light';
      document.documentElement.setAttribute('data-theme',n);
      localStorage.setItem('bookmark_theme',n);
    },
    escapeHtml(str){
      if(!str)return'';
      return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#039;');
    },
    searchBookmarks(bms,query){
      if(!bms||!Array.isArray(bms))return[];
      if(!query||!query.trim())return bms;
      const q=query.trim().toLowerCase();
      return bms.filter(b=>{
        const hay=(b.title||'')+' '+(b.url||'')+' '+(b.remark||'')+' '+(b.breadcrumb||[]).join(' ');
        return hay.toLowerCase().includes(q);
      });
    },
    highlightText(text,query){
      if(!query||!query.trim()||!text)return TemplateShell.escapeHtml(text);
      const q=query.trim();
      if(!q)return TemplateShell.escapeHtml(text);
      const parts=text.split(new RegExp(`(${q.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')})`,'gi'));
      return parts.map(p=>p.toLowerCase()===q.toLowerCase()?'<mark>'+TemplateShell.escapeHtml(p)+'</mark>':TemplateShell.escapeHtml(p)).join('');
    },
    sortBookmarks(bms,mode){
      if(!bms||!Array.isArray(bms))return[];
      if(!mode||mode==='default')return bms;
      const desc=mode.endsWith('-desc');
      const base=mode.replace(/-asc$|-desc$/,'');
      const validBases=['folder','bookmark','folder-bookmark','time-created','time-updated'];
      if(!validBases.includes(base))return bms;
      const dir=desc?-1:1;
      const path=b=>(b.breadcrumb&&b.breadcrumb.length)?b.breadcrumb.join(' / '):'';
      const title=b=>(b.title||'').toLowerCase();
      const c=new Intl.Collator('zh-CN',{sensitivity:'base',numeric:true});
      if(base==='folder')return[...bms].sort((a,b)=>dir*c.compare(path(a),path(b)));
      if(base==='bookmark')return[...bms].sort((a,b)=>dir*c.compare(title(a),title(b)));
      if(base==='folder-bookmark')return[...bms].sort((a,b)=>{const p=dir*c.compare(path(a),path(b));if(p!==0)return p;return dir*c.compare(title(a),title(b));});
      if(base==='time-created'||base==='time-updated'){
        const field=base==='time-created'?'created_at':'updated_at';
        const withTime=[],withoutTime=[];
        for(const b of bms){
          const t=b[field]?new Date(b[field]).getTime():null;
          if(t===null||isNaN(t))withoutTime.push(b);
          else withTime.push({b,t});
        }
        withTime.sort((a,b)=>dir*(a.t-b.t));
        return[...withTime.map(x=>x.b),...withoutTime];
      }
      return bms;
    },
    sortFolderNames(names,mode){
      if(!names||!Array.isArray(names))return[];
      const desc=mode&&mode.endsWith('-desc');
      const base=mode?mode.replace(/-asc$|-desc$/,''):'';
      if(base!=='folder'&&base!=='folder-bookmark')return names;
      const c=new Intl.Collator('zh-CN',{sensitivity:'base',numeric:true});
      const sorted=[...names].sort((a,b)=>c.compare(a,b));
      return desc?sorted.reverse():sorted;
    },
    mountSortControl(target,onChange){
      if(!target||target.querySelector('.t-sort'))return'default';
      const wrap=document.createElement('div');
      wrap.className='t-sort';
      wrap.innerHTML='<svg class="t-sort-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18M6 12h12M10 18h4"/></svg><select class="t-sort-select" aria-label="排序方式"><option value="default">默认顺序</option><option value="folder-asc">按文件夹 ↑</option><option value="folder-desc">按文件夹 ↓</option><option value="bookmark-asc">按书签 ↑</option><option value="bookmark-desc">按书签 ↓</option><option value="folder-bookmark-asc">文件夹+书签 ↑</option><option value="folder-bookmark-desc">文件夹+书签 ↓</option><option value="time-created-asc">创建时间 ↑</option><option value="time-created-desc">创建时间 ↓</option><option value="time-updated-asc">更新时间 ↑</option><option value="time-updated-desc">更新时间 ↓</option></select>';
      target.appendChild(wrap);
      const sel=wrap.querySelector('.t-sort-select');
      const saved=localStorage.getItem('bookmark_sort')||'default';
      sel.value=saved;
      sel.addEventListener('change',()=>{localStorage.setItem('bookmark_sort',sel.value);if(onChange)onChange(sel.value);});
      return sel.value;
    },
    mountAuthButton(target){
      if(!target||target.querySelector('.t-auth'))return;
      const el=document.createElement('div');
      el.className='t-auth';
      el.style.cssText='display:flex;align-items:center;flex-shrink:0;';
      const link=document.createElement('a');
      link.style.cssText='font-size:13px;font-weight:500;color:var(--text-muted);text-decoration:none;padding:6px 12px;border-radius:20px;background:var(--bg-elev-1);border:1px solid var(--border);transition:all .2s ease;white-space:nowrap;cursor:pointer;';
      link.addEventListener('mouseenter',()=>{link.style.background='var(--bg-elev-2)';link.style.borderColor='var(--border-strong)';});
      link.addEventListener('mouseleave',()=>{link.style.background='var(--bg-elev-1)';link.style.borderColor='var(--border)';});
      const update=()=>{
        const token=localStorage.getItem('token');
        if(token){link.href='admin.html';link.textContent='后台管理';}else{link.href='login.html';link.textContent='登录';}
      };
      update();
      document.addEventListener('visibilitychange',()=>{if(!document.hidden)update();});
      window.addEventListener('focus',update);
      el.appendChild(link);
      target.appendChild(el);
    },
    mountVersionFooter(){
      const els=document.querySelectorAll('.t-version');
      if(!els.length)return;
      fetch('/api/version').then(r=>r.json()).then(d=>{
        const v=d&&d.version?d.version:'';
        els.forEach(el=>{el.textContent='网址收藏夹 '+v;});
      }).catch(()=>{});
    }
  };
})();