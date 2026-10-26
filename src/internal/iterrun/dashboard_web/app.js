'use strict';
(() => {
const $=s=>document.querySelector(s),esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
let projects=[],runs=[],route=readRoute(),pollBusy=false,lastPaint='';
const activityExpanded=new Set();
function readRoute(){const q=new URLSearchParams(location.search);return {page:location.pathname==='/project'?'project':'all',project:q.get('project')||'',tab:['plans','graph','runs'].includes(q.get('tab'))?q.get('tab'):'plans',filter:q.get('filter')||'all'};}
const projectURL=(p,tab='plans',filter='all')=>'/project?'+new URLSearchParams({project:p,tab,filter});
function link(url,text,cls=''){return `<a data-nav href="${esc(url)}" class="${cls}">${esc(text)}</a>`;}
function queue(p){return projects.find(x=>x.path===p?.path)?.queue||{plans:[],running_plans:[],running_owners:{}};}
function navigate(url){history.pushState({scroll:0},'',url);route=readRoute();render();scrollTo(0,0);}
async function refresh(){if(pollBusy)return;pollBusy=true;try{const [p,r]=await Promise.all([fetch('/api/projects').then(r=>r.json()),fetch('/api/record/runs').then(r=>r.json())]);if(p.ok===false)throw Error(p.error);projects=p.data;runs=r.runs||[];const key=JSON.stringify([projects,runs]);if(lastPaint!==key){const y=scrollY;render();scrollTo(0,y);lastPaint=key;}if(route.tab==='graph')await refreshGraph(route.project);}catch(e){$('#notices').textContent='Dashboard unavailable: '+e.message;}finally{pollBusy=false}}
function render(){const p=projects.find(p=>p.path===route.project);$('#app').innerHTML=`<header class="app-header">${link('/','iterate-run')}<span id="connection">Live</span></header><main><nav class="breadcrumb" aria-label="Breadcrumb">${link('/','All projects')}${p?' / '+esc(p.name)+' / '+esc(route.tab):''}</nav>${route.page==='all'?allProjects():p?projectView(p):'<h1>Project unavailable</h1><p>This exact project is not registered or available.</p>'}</main>`;}
function allProjects(){return `<h1>All projects</h1><div class="project-table-scroll"><table class="project-table"><thead><tr><th>Project</th><th>Queued</th><th>Running</th><th>Blocked</th><th>Archived</th></tr></thead><tbody>${projects.map(p=>{const lists=projectPlanLists(p);return `<tr><th>${link(projectURL(p.path),p.name)}<div class="path">${esc(p.path)}</div></th>${['queued','running','blocked','archived'].map(f=>`<td>${link(projectURL(p.path,'plans',f),lists[f].length)}</td>`).join('')}</tr>`}).join('')}</tbody></table></div>`;}
function projectView(p){return `<h1>${esc(p.name)}</h1><p class="path">${esc(p.path)}</p><nav class="tabs">${['plans','graph','runs'].map(t=>link(projectURL(p.path,t,route.filter),t[0].toUpperCase()+t.slice(1),route.tab===t?'active':'')).join('')}</nav><div id="project-body">${route.tab==='graph'?graphView(p):route.tab==='runs'?runsView(p):plansView(p)}</div>`;}
function runsView(p){const own=runs.filter(r=>r.project_path===p.path);return `<section class="panel"><h2>Runs</h2>${own.map(r=>`<article class="row">${link('/plan?'+new URLSearchParams({project:p.path,name:r.plan}),r.plan)}<span>${esc(r.state)}</span><span class="meta">${esc(r.opened_at||'Time unavailable')}</span></article>`).join('')||'<p class="empty">No recorded runs.</p>'}</section>`;}
document.addEventListener('click',e=>{const a=e.target.closest('a[data-nav]');if(!a||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;const url=new URL(a.href);if(url.origin===location.origin&&(url.pathname==='/'||url.pathname==='/project')){e.preventDefault();navigate(url.pathname+url.search);}});
document.addEventListener('toggle',e=>{const key=e.target.dataset?.activityDetails;if(key){if(e.target.open)activityExpanded.add(key);else activityExpanded.delete(key);}},true);
addEventListener('popstate',()=>{route=readRoute();render()});
function planState(v,p,c){
  if(v.Archived)return 'archived';
  const q=queue(c||p),state=(v.Status||'').toLowerCase();
  if((q.running_plans||[]).includes(v.Name))return 'running';
  if(/^(blocked|awaiting-human-gate|paused)/.test(state))return 'blocked';
  if((q.plans||[]).some(x=>x.name===v.Name&&x.blocked))return 'blocked';
  if((q.plans||[]).some(x=>x.name===v.Name))return 'queued';
  if(!v.Phase||v.Phase==='planned')return 'draft';
  return 'blocked';
}

function planWait(v,p,c){return (queue(c||p).plans||[]).find(x=>x.name===v.Name&&x.waiting);}
function planStateLabel(v,p,c){const state=planState(v,p,c);if(state==='queued'&&planWait(v,p,c))return 'Waiting';if(state!=='running')return state[0].toUpperCase()+state.slice(1);const owner=queue(c||p).running_owners?.[v.Name];return owner?'Running · '+owner:'Running · Owner unavailable';}
function planBlockReason(v,p,c){const parked=(queue(c||p).plans||[]).find(x=>x.name===v.Name&&x.blocked);return parked?.reason||v.NextAttempt||v.Status||('Not ready to run · '+(v.Phase||'waiting for review'));}
function projectPlanLists(p,c){
  const lists={all:[],draft:[],queued:[],running:[],blocked:[],archived:[]};
  for(const v of [...p.plans,...p.archived])lists[planState(v,p,c)].push(v);
  lists.all=[...lists.blocked,...lists.running,...lists.queued,...lists.draft,...lists.archived];
  return lists;
}

function plansView(p,c){
  const lists=projectPlanLists(p,c),selected=Object.hasOwn(lists,route.filter)?route.filter:'all';
  const explanations={all:'Every plan, including blocked work and archives. Blocked plans appear first.',draft:'Plans awaiting approval to enter the queue.',queued:'Approved or resumable plans waiting to run. Schedules, budgets and project ownership still apply.',running:'Running identifies a live execution owner.',blocked:'Plans waiting for an operator, a cleared blocker or review. They stay visible here until cleared.',archived:'Recorded plan history; these plans are not running.'};
  return `<section class="panel plans-panel"><div class="toolbar heading"><h2>Plans</h2><nav class="actions plan-filters" aria-label="Plan states">${Object.keys(lists).map(f=>`<a data-nav href="${esc(projectURL(p.path,'plans',f))}" class="${selected===f?'active':''}" ${selected===f?'aria-current="page"':''}>${f[0].toUpperCase()+f.slice(1)} · ${lists[f].length}</a>`).join('')}</nav></div><p class="hint">${explanations[selected]}</p>${lists.blocked.length&&selected!=='blocked'?`<p class="blocked-summary">${link(projectURL(p.path,'plans','blocked'),lists.blocked.length+' blocked plan'+(lists.blocked.length===1?'':'s'))} · Other queued work can run. A plan whose depends: predecessor is unfinished waits; ${link(projectURL(p.path,'graph'),'see the graph')}.</p>`:''}<div class="plan-list">${lists[selected].map(v=>{
    const state=planState(v,p,c),run=null;
    const href=(v.Archived?'/archive?':'/plan?')+new URLSearchParams({project:p.path,...(v.Archived?{file:v.ArchiveFile}:{name:v.Name})});
    const detailKey='blocker:'+p.path+':'+v.Name;
    const raw=v.Finished||v.Executing||v.Started||'',date=Date.parse(raw.replace(/\s*\(planned\)$/, '')),dateLabel=v.Finished?'Finished':v.Executing?'Started':'Created';
    return `<article class="plan-row" aria-label="${esc(v.Name)} · ${esc(planStateLabel(v,p,c))}"><span class="badge ${state==='blocked'?'bad':state==='running'||state==='archived'?'good':''}">${esc(planStateLabel(v,p,c))}</span><div class="plan-name">${link(href,v.Name)}</div><div class="plan-goal" title="${esc(v.Goal)}">${esc(v.Goal)}</div><div class="meta plan-date" title="${esc(raw)}">${raw?esc(dateLabel+' '+(Number.isFinite(date)?new Date(date).toLocaleDateString():raw)):''}${v.HasTeams?'<br>Teams '+v.TeamsDone+'/'+v.TeamsTotal:''}</div>${state==='blocked'?`<details class="plan-blocker" data-activity-details="${esc(detailKey)}" ${activityExpanded.has(detailKey)?'open':''}><summary>Why blocked</summary><p>${esc(planBlockReason(v,p,c))}</p></details>`:''}${planWait(v,p,c)?`<p class="plan-wait">${esc(planWait(v,p,c).wait_reason)}${planWait(v,p,c).held_by&&planWait(v,p,c).held_by!==planWait(v,p,c).waiting_on?.[0]?' · held by '+esc(planWait(v,p,c).held_by):''}</p>`:''}</article>`;
  }).join('')||'<p class="empty">No '+(selected==='all'?'':selected+' ')+'plans in this view.</p>'}</div></section>`;
}

const graphs=new Map();
async function refreshGraph(path,force=false){const old=graphs.get(path);if(!force&&old&&Date.now()-old.at<10000)return;try{const r=await fetch('/api/graph?'+new URLSearchParams({project:path}));const body=await r.json();if(!r.ok)throw new Error(body.error||r.statusText);const html=graphMarkup(body,path),changed=!old||old.html!==html;graphs.set(path,{at:Date.now(),html});const el=$('#graph-body');if(el&&changed&&route.project===path&&route.tab==='graph')el.innerHTML=html;graphHint();}catch(e){const el=$('#graph-body');if(el&&!old)el.innerHTML='<p class="error">Graph unavailable: '+esc(e.message)+'</p>';}}
function graphHint(){const box=$('.graph-scroll'),hint=$('.graph-scroll-hint');if(box&&hint)hint.hidden=box.scrollWidth<=box.clientWidth+1;}
addEventListener('resize',graphHint);
function graphView(p){const later=()=>requestAnimationFrame(graphHint);setTimeout(later,0);const cached=graphs.get(p.path);if(!cached||Date.now()-cached.at>=10000)setTimeout(()=>refreshGraph(p.path),0);return `<section class="panel graph-panel" aria-labelledby="graph-title"><div class="toolbar heading"><h2 id="graph-title">Plan graph</h2><span class="meta">Each plan waits until every plan on its depends: line is archived done or landed.</span></div><div id="graph-body">${cached?cached.html:'<p class="empty">Loading graph…</p>'}</div></section>`;}
function graphStatus(n){return n.status==='ready'&&(n.state==='blocked'||n.state==='paused')?'blocked':n.status;}
function graphWord(n){if(n.archived)return ({done:'done',landed:'landed',closed:'closed unfinished',rolled:'rolled'})[n.outcome]||'archived';return n.state;}
function graphMarkup(v,path){
  const nodes=[...(v.nodes||[])].sort((a,b)=>(a.archived?0:1)-(b.archived?0:1)||(a.position||0)-(b.position||0)),edges=(v.edges||[]).filter(e=>!e.implied);
  if(!nodes.length)return '<p class="empty">No plans in this project.</p>';
  const live=nodes.filter(n=>!n.archived),count=s=>live.filter(n=>graphStatus(n)===s).length;
  const W=210,H=52,GX=60,GY=16,PAD=12,layers=new Map();
  for(const n of nodes){const d=n.depth||0;if(!layers.has(d))layers.set(d,[]);layers.get(d).push(n);}
  const pos=new Map();let maxRows=0;
  for(const [d,list] of [...layers.entries()].sort((a,b)=>a[0]-b[0])){list.forEach((n,i)=>pos.set(n.name,{x:PAD+d*(W+GX),y:PAD+i*(H+GY)}));maxRows=Math.max(maxRows,list.length);}
  const lanes=edges.some(e=>{const a=nodes.find(n=>n.name===e.from),b=nodes.find(n=>n.name===e.to);return a&&b&&(b.depth||0)-(a.depth||0)>1;});
  const width=PAD*2+(Math.max(...layers.keys())+1)*(W+GX)-GX,height=PAD*2+maxRows*(H+GY)-GY+(lanes?GY:0);
  const sym={done:'✔',ready:'●',waiting:'◌',blocked:'✖',held:'✖'};
  const paths=edges.map(e=>{const a=pos.get(e.from),b=pos.get(e.to);if(!a||!b)return '';const x1=a.x+W,y1=a.y+H/2,x2=b.x,y2=b.y+H/2,mx=(x1+x2)/2,span=Math.round((b.x-a.x)/(W+GX));let d=`M${x1} ${y1} C${mx} ${y1} ${mx} ${y2} ${x2-6} ${y2}`;if(span>1){const lane=a.y+H+GY/2,h=GX/2;d=`M${x1} ${y1} C${x1+h} ${y1} ${x1+h} ${lane} ${x1+GX} ${lane} L${x2-GX} ${lane} C${x2-h} ${lane} ${x2-h} ${y2} ${x2-6} ${y2}`;}return `<path class="graph-edge ${e.satisfied?'met':'unmet'}" d="${d}" marker-end="url(#graph-arrow)"><title>${esc(e.from)} → ${esc(e.to)}${e.satisfied?' · satisfied':' · '+esc(e.reason||'unfinished')}</title></path>`;}).join('');
  const boxes=nodes.map(n=>{const p=pos.get(n.name),word=graphWord(n),reason=n.status==='waiting'||n.status==='blocked'?n.reason:graphStatus(n)==='blocked'?'':n.status,ms=(n.milestones||[]).join(', '),label=`${n.name}${n.letter?' · '+n.letter:''}${ms?' · '+ms:''} · ${word} · ${reason||''}`;const inner=`<rect class="graph-node ${esc(graphStatus(n))}" x="${p.x}" y="${p.y}" width="${W}" height="${H}" rx="7"/><text class="graph-name" x="${p.x+12}" y="${p.y+21}">${esc(sym[graphStatus(n)]||'●')} ${esc((n.letter?n.letter+' · ':'')+(n.name.length>18?n.name.slice(0,17)+'…':n.name))}</text><text class="graph-sub" x="${p.x+12}" y="${p.y+39}">${esc([n.release,ms,word,n.archived||graphStatus(n)==='blocked'&&n.status!=='blocked'?'':n.status].filter(Boolean).join(' · '))}</text><title>${esc(label)}</title>`;return n.archived?`<g class="graph-item">${inner}</g>`:`<a class="graph-item" href="${esc('/plan?'+new URLSearchParams({project:path,name:n.name}))}" aria-label="${esc(label)}">${inner}</a>`;}).join('');
  const waiting=live.filter(n=>n.status==='waiting'||n.status==='blocked');
  const layerCount=Math.max(...layers.keys())+1;
  return `<p class="graph-summary"><strong>Next: ${esc(v.next||'No ready plan')}</strong> · ${live.length} plans · ${count('ready')} ready · ${count('waiting')} waiting · ${count('blocked')} blocked<span class="graph-scroll-hint" hidden> · ${layerCount} columns, scroll sideways for the rest →</span></p>${(v.errors||[]).length?`<div class="error"><strong>Graph errors</strong><ul>${v.errors.map(e=>'<li>'+esc(e)+'</li>').join('')}</ul></div>`:''}<div class="graph-scroll" tabindex="0" role="region" aria-label="Plan dependency graph, scrolls sideways" data-graph-width="${width}"><svg class="graph" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="graph-title"><defs><marker id="graph-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="9" markerHeight="9" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z"/></marker></defs>${paths}${boxes}</svg></div><p class="hint">Solid arrows are satisfied; dashed arrows still wait. Select a plan to open it.</p>${waiting.length?`<h3>Held plans</h3><ul class="graph-held">${waiting.map(n=>`<li><a data-nav href="${esc('/plan?'+new URLSearchParams({project:path,name:n.name}))}">${esc(n.name)}</a> <span class="meta">${esc(n.reason)}${n.held_by&&n.held_by!==n.waiting_on?.[0]?' · held by '+esc(n.held_by):''}</span></li>`).join('')}</ul>`:''}`;
}

refresh();setInterval(()=>{if(!document.hidden)refresh()},3000);document.addEventListener("visibilitychange",()=>{if(!document.hidden)refresh()});
})();
