'use strict';
(() => {
const header=document.querySelector('.conductor-detail-header'),panel=document.querySelector('#conductor-live-detail');
if(!header||!panel||location.pathname==='/archive')return;
const seen=new Map();let busy=false;
async function read(path){const response=await fetch('/api/conductor/'+path);const json=await response.json();if(!response.ok||json.ok===false)throw Error(json.error?.message||'Activity unavailable');return json.data;}
async function poll(){if(busy||document.hidden)return;busy=true;try{const runs=await read('runs');const owned=runs.filter(r=>r.project_path===header.dataset.project&&r.plan===header.dataset.plan);for(const r of owned){const old=seen.get(r.run_id)||{cursor:0,events:[]},page=await read('events?'+new URLSearchParams({run_id:r.run_id,after:old.cursor,limit:100}));const known=new Set(old.events.map(e=>e.sequence));seen.set(r.run_id,{cursor:page.cursor||old.cursor,events:(page.expired?[]:old.events).concat((page.events||[]).filter(e=>page.expired||!known.has(e.sequence))).slice(-100)});}panel.replaceChildren();if(owned.length){const h=document.createElement('h2');h.textContent='Conductor activity';panel.append(h);for(const r of owned){const label=document.createElement('p');label.textContent=r.sequence_name+' · '+r.status;panel.append(label);for(const e of seen.get(r.run_id)?.events||[]){const row=document.createElement('p');row.textContent=new Date(e.at).toLocaleTimeString()+' · '+e.kind+' · '+(e.message||'');panel.append(row);}}}}catch(e){panel.textContent='Activity reconnecting · '+e.message;}finally{busy=false}}
poll();setInterval(poll,3000);document.addEventListener('visibilitychange',poll);
})();
