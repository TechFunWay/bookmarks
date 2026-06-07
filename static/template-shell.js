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
    getCurrentTemplate(){
      const m=window.location.pathname.match(/\/template-(magazine|masonry|timeline|minimal|grid)\.html/);
      return m?m[1]:null;
    },
    getRememberedTemplate(sys){
      const s=localStorage.getItem('bookmark_template');
      if(s&&['magazine','masonry','timeline','minimal','grid'].includes(s))return s;
      if(sys&&sys.default_template)return sys.default_template;
      return 'magazine';
    },
    setUserTemplate(n){
      if(['magazine','masonry','timeline','minimal','grid'].includes(n)){
        localStorage.setItem('bookmark_template',n);
        window.location.href=`/template-${n}.html`;
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
        {id:'grid',n:'卡片网格',i:'<svg viewBox="0 0 24 24" fill="currentColor"><rect x="3" y="3" width="8" height="8" rx="1"/><rect x="13" y="3" width="8" height="8" rx="1"/><rect x="3" y="13" width="8" height="8" rx="1"/><rect x="13" y="13" width="8" height="8" rx="1"/></svg>'}
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
    }
  };
})();