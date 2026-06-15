/**
 * View Shell - Shared Vue app logic for the sidebar/portal homepage pages
 * (view-sidebar.html / view-portal.html). Page markup lives in each page.
 */
(function(){
  const { createApp } = Vue;

  const TreeNode = {
    name: 'TreeNode',
    props: { node: Object, depth: Number, selectedId: Number, foldersOnly: { type: Boolean, default: false } },
    emits: ['select'],
    data(){return{expanded:false}},
    computed:{
      isFolder(){return this.node.type==='folder'},
      isSelected(){return this.selectedId===this.node.id},
      hasChildren(){
        if(!this.isFolder||!this.node.children?.length)return false;
        return this.foldersOnly ? this.node.children.some(c=>c.type==='folder') : true;
      },
      visibleChildren(){
        const ch=this.node.children||[];
        return this.foldersOnly ? ch.filter(c=>c.type==='folder') : ch;
      },
      indentStyle(){return{paddingLeft:(this.depth*14+14)+'px'}},
    },
    methods:{
      onClick(){
        if(this.isFolder){this.expanded=!this.expanded;this.$emit('select',this.node.id)}
        else{this.$emit('select',this.node.id)}
      },
      onSelect(id){this.$emit('select',id)},
      isUrl(s){return s&&(s.startsWith('http')||s.startsWith('/')||s.startsWith('data:'))},
    },
    template:`
      <div v-if="!foldersOnly || isFolder">
        <div @click="onClick"
          class="flex items-center gap-2 px-3.5 py-2 rounded-xl cursor-pointer transition-all duration-200 text-sm mx-0.5 my-0.5"
          :style="indentStyle"
          :class="isSelected ? 'bg-gradient-to-r from-indigo-50 to-purple-50 dark:from-indigo-500/15 dark:to-purple-500/8 text-indigo-600 dark:text-indigo-400 font-semibold shadow-sm' : 'text-slate-600 dark:text-slate-400 hover:bg-indigo-50/60 dark:hover:bg-indigo-500/8 hover:text-indigo-600 dark:hover:text-indigo-400'">
          <span class="text-[10px] w-3 text-center flex-shrink-0 text-slate-400 dark:text-slate-500 transition-transform duration-200" :class="{'rotate-90':expanded}" v-if="hasChildren">▶</span>
          <span class="w-3 flex-shrink-0" v-else></span>
          <span v-if="isFolder">
            <img v-if="node.favicon_url&&isUrl(node.favicon_url)" :src="node.favicon_url" class="w-4 h-4 object-contain flex-shrink-0 rounded" @error="onIconError" />
            <span v-else class="text-sm flex-shrink-0">{{node.favicon_url||'📁'}}</span>
          </span>
          <span v-else class="text-sm flex-shrink-0">🔗</span>
          <span class="flex-1 truncate">{{node.title}}</span>
          <span v-if="isFolder&&node.bookmark_count!=null"
            class="text-xs font-semibold rounded-full px-2.5 py-0.5 transition-colors"
            :class="isSelected?'bg-indigo-500/15 text-indigo-600 dark:text-indigo-300':'bg-slate-100 dark:bg-white/8 text-slate-400'">{{node.bookmark_count}}</span>
        </div>
        <div v-if="isFolder&&expanded&&hasChildren" class="overflow-hidden transition-all duration-200">
          <tree-node v-for="child in visibleChildren" :key="child.id" :node="child" :depth="depth+1" :selected-id="selectedId" :folders-only="foldersOnly" @select="onSelect" />
        </div>
      </div>
    `
  };

  window.createViewApp = function(){
    const app = createApp({
      components:{TreeNode},
      data(){
        return{
          token:localStorage.getItem('token')||'',
          currentUser:null, tree:[], selectedId:null, loading:true,
          searchQuery:'', searchTimer:null, isSearching:false, searchResults:[],
          theme:localStorage.getItem('bookmark_theme')==='light'?'light':'dark', itemSize:1,
        }
      },
      computed:{
        totalCount(){
          let c=0; const count=nodes=>{for(const n of nodes){if(n.type==='bookmark')c++;if(n.children)count(n.children)}};
          count(this.tree); return c;
        },
        bookmarks(){
          let items=[];
          const collect=(nodes,path=[])=>{
            for(const n of nodes){
              if(n.type==='folder'){
                if(this.selectedId===null||this.selectedId===n.id||this.isChildOf(n.id,this.selectedId))
                  collect(n.children||[],[...path,n.title]);
              }else if(n.type==='bookmark'){
                if(this.selectedId===null||this.isInFolder(n.id))
                  items.push({...n,path:path.join(' / ')});
              }
            }
          };
          if(this.selectedId===null) collect(this.tree);
          else if(this.selectedId){
            const f=this.findNode(this.selectedId,this.tree);
            if(f){if(f.type==='folder')collect(f.children||[],[f.title]);else items.push({...f})}
          }
          if(this.searchQuery){
            const q=this.searchQuery.toLowerCase();
            items=items.filter(i=>(i.title&&i.title.toLowerCase().includes(q))||(i.url&&i.url.toLowerCase().includes(q)));
          }
          return items;
        },
        filteredBookmarks(){
          if(!this.searchQuery)return[];
          const q=this.searchQuery.toLowerCase(); const all=[];
          const collect=nodes=>{for(const n of nodes){if(n.type==='bookmark'&&((n.title&&n.title.toLowerCase().includes(q))||(n.url&&n.url.toLowerCase().includes(q))))all.push(n);if(n.children)collect(n.children)}};
          collect(this.tree); return all;
        },
        currentFolderTitle(){
          if(this.searchQuery)return '搜索结果';
          if(this.selectedId===null)return '所有书签';
          const n=this.findNode(this.selectedId,this.tree); return n?n.title:'所有书签';
        },
        currentFolderPath(){
          if(this.searchQuery||this.selectedId===null)return '';
          const find=(nodes,targetId,path=[])=>{
            for(const n of nodes){
              if(n.id===targetId)return[...path,n.title];
              if(n.children){const r=find(n.children,targetId,[...path,n.title]);if(r)return r}
            }return null;
          };
          const p=find(this.tree,this.selectedId); return p?p.join(' / '):'';
        },
        displaySections(){
          if(this.isSearching){
            if(this.searchResults.length===0)return[];
            return[{id:'search',icon:'🔍',title:'搜索结果 ('+this.searchResults.length+')',bookmarks:this.searchResults}];
          }
          const r=[];
          const walk=(nodes,pp)=>{
            for(const n of nodes){
              if(n.type==='folder'){
                const p=pp?pp+' / '+n.title:n.title;
                // 只收集直接子书签，子文件夹的书签由递归 walk 负责
                const bm=(n.children||[]).filter(c=>c.type==='bookmark');
                if(bm.length>0)r.push({id:n.id,icon:n.favicon_url||'📁',title:p,bookmarks:bm});
                if(n.children)walk(n.children,p);
              }
            }
          };
          walk(this.tree,''); return r;
        },
      },
      methods:{
        getHeaders(ct){const h={};if(ct)h['Content-Type']=ct;if(this.token)h['Authorization']=this.token;return h},
        faviconUrl(item){
          if(item.favicon_url)return item.favicon_url;
          try{const u=new URL(item.url);return u.origin+'/favicon.ico'}catch{return''}
        },
        getDomain(url){try{const u=new URL(url);return u.hostname.replace(/^www\./,'')}catch{return url}},
        async checkAuth(){
          // 已登录：拉取用户信息；token 失效则清除并按匿名处理
          if(this.token){
            try{
              const r=await fetch('/api/auth/me',{headers:this.getHeaders()});
              if(r.ok){this.currentUser=await r.json();return}
              localStorage.removeItem('token');this.token='';
            }catch{localStorage.removeItem('token');this.token=''}
          }
          // 无 token：仅当管理员开启免登录模式时才允许匿名浏览，否则跳转登录
          this.currentUser=null;
          try{
            const r=await fetch('/api/config/system');
            const sys=r.ok?await r.json():{};
            if(sys.require_login!=='false'){window.location.href='/login.html';return false}
          }catch{window.location.href='/login.html';return false}
          return true;
        },
        async loadTree(){this.loading=true;try{const r=await fetch('/api/tree',{headers:this.getHeaders()});if(r.ok){this.tree=await r.json();if(this.selectedId===null&&this.tree.length>0)this.selectedId=this.tree[0].id}}catch{};this.loading=false},
        async loadConfig(){
          try{const r=await fetch('/api/config',{headers:this.getHeaders()});if(r.ok){const c=await r.json();if(c['portal_view_scale']){const v=parseFloat(c['portal_view_scale']);if(v>=0.6&&v<=1.6)this.itemSize=v}}}catch{}
        },
        async saveSetting(k,v){try{await fetch('/api/config',{method:'POST',headers:this.getHeaders('application/json'),body:JSON.stringify({key:k,value:v})})}catch{}},
        getSiteName(item){let n=(item.title||'').trim();for(const s of[' - ',' – ',' | ',' — ',' | ']){if(n.includes(s)){n=n.split(s)[0].trim();break}}return n},
        onSizeChange(){this.saveSetting('portal_view_scale',this.itemSize.toString())},
        findNode(id,nodes){for(const n of nodes){if(n.id===id)return n;if(n.children){const f=this.findNode(id,n.children);if(f)return f}}return null},
        isChildOf(pid,cid){const p=this.findNode(pid,this.tree);if(!p||!p.children)return false;return!!this.findNode(cid,p.children)},
        isInFolder(bid){if(this.selectedId===null)return true;const walk=nodes=>{for(const n of nodes){if(n.id===bid)return true;if(n.children&&walk(n.children))return true}return false};const f=this.findNode(this.selectedId,this.tree);if(!f)return false;return walk(f.children||[])},
        selectFolder(id){this.selectedId=id;this.searchQuery=''},
        handleSearch(){},
        clearSearch(){this.searchQuery='';this.isSearching=false;this.searchResults=[]},
        doSearch(){const q=this.searchQuery.toLowerCase().trim();if(!q){this.clearSearch();return};const all=[];const collect=nodes=>{for(const n of nodes){if(n.type==='bookmark')all.push(n);if(n.children)collect(n.children)}};collect(this.tree);this.searchResults=all.filter(b=>b.title.toLowerCase().includes(q)||(b.url||'').toLowerCase().includes(q));this.isSearching=true},
        debouncedSearch(){if(this.searchTimer)clearTimeout(this.searchTimer);this.searchTimer=setTimeout(()=>this.doSearch(),300)},
        toggleTheme(){this.theme=this.theme==='dark'?'light':'dark';document.documentElement.className=this.theme;document.documentElement.setAttribute('data-theme',this.theme);localStorage.setItem('bookmark_theme',this.theme)},
        onImgError(e){e.target.style.display='none';e.target.parentElement.innerHTML='🌏'},
        onFaviconError(e){e.target.style.display='none';e.target.parentElement.innerHTML='🌐'},
        isIconUrl(s){return s&&(s.startsWith('http')||s.startsWith('/')||s.startsWith('data:'))},
        onIconError(e){e.target.style.display='none';e.target.parentElement.innerHTML='📁'},
        scrollToSection(id){const el=document.getElementById('section-'+id);if(el)el.scrollIntoView({behavior:'smooth',block:'start'})},
        handleMouseMove(e,target){
          const rect=target.getBoundingClientRect();
          target.style.setProperty('--mouse-x',((e.clientX-rect.left)/rect.width*100)+'%');
          target.style.setProperty('--mouse-y',((e.clientY-rect.top)/rect.height*100)+'%');
        },
      },
      mounted(){
        // 与其他模板共用 bookmark_theme，默认暗色
        const saved=localStorage.getItem('bookmark_theme')==='light'?'light':'dark';
        this.theme=saved; document.documentElement.className=saved;
        document.documentElement.setAttribute('data-theme',saved);
        this.checkAuth().then(ok=>{if(ok===false)return;this.loadConfig();this.loadTree()});
        // Track mouse for shine effect on cards
        document.addEventListener('mousemove',e=>{
          document.querySelectorAll('[data-shine]').forEach(el=>{
            const rect=el.getBoundingClientRect();
            el.style.setProperty('--mouse-x',((e.clientX-rect.left)/rect.width*100)+'%');
            el.style.setProperty('--mouse-y',((e.clientY-rect.top)/rect.height*100)+'%');
          });
        });
      },
    });
    app.mount('#app');
    return app;
  };
})();
