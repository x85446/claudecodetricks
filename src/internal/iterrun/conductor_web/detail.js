'use strict';
(() => {
const header=document.querySelector('.conductor-detail-header'),panel=document.querySelector('#conductor-live-detail');
if(!header||!panel||location.pathname==='/archive')return;
const seen=new Map();let busy=false,lastPaint='';
async function read(path){const response=await fetch('/api/conductor/'+path);const json=await response.json();if(!response.ok||json.ok===false)throw Error(json.error?.message||'Activity unavailable');return json.data;}
function paragraph(parent,text){const p=document.createElement('p');p.textContent=text;parent.append(p);}
async function poll(){
 if(busy||document.hidden)return;busy=true;
 try{
  const [runs,visibility]=await Promise.all([read('runs?'+new URLSearchParams({project:header.dataset.project,plan:header.dataset.plan})),read('visibility?'+new URLSearchParams({project:header.dataset.project}))]);
  for(const r of runs){const old=seen.get(r.run_id)||{cursor:0,events:[]},page=await read('events?'+new URLSearchParams({run_id:r.run_id,project_id:r.project_id,plan:header.dataset.plan,after:old.cursor,limit:100}));const known=new Set(old.events.map(e=>e.sequence));seen.set(r.run_id,{cursor:page.cursor||old.cursor,events:(page.expired?[]:old.events).concat((page.events||[]).filter(e=>page.expired||!known.has(e.sequence))).slice(-100)});}
  const signature=JSON.stringify([runs,visibility,[...seen]]);
  const notice=panel.querySelector('[data-connection]');if(notice)notice.remove();
  if(signature!==lastPaint){
   const fragment=document.createDocumentFragment(),h=document.createElement('h2');h.textContent='Conductor activity';fragment.append(h);
   for(const v of visibility){paragraph(fragment,'Project state: '+v.state+' · Effective sequence: '+v.effective_sequence);paragraph(fragment,'Next opportunity: '+v.next_opportunity);paragraph(fragment,'Reason: '+v.reason);for(const u of v.usage)paragraph(fragment,u.subscription_id+' usage: '+(u.available?u.age_seconds+' seconds old · '+u.source+' · observed '+new Date(u.observed_at).toLocaleString():'unavailable'));}
   for(const r of runs){paragraph(fragment,r.sequence_name+' · '+r.status);for(const e of seen.get(r.run_id)?.events||[])paragraph(fragment,new Date(e.at).toLocaleTimeString()+' · '+e.kind+' · '+(e.message||''));}
   if(!runs.length)paragraph(fragment,'No conductor run recorded for this plan.');
   const y=scrollY;panel.replaceChildren(fragment);scrollTo(0,y);lastPaint=signature;
  }
 }catch(e){let notice=panel.querySelector('[data-connection]');if(!notice){notice=document.createElement('p');notice.dataset.connection='';panel.prepend(notice);}notice.textContent='Activity reconnecting · '+e.message;}
 finally{busy=false}
}
poll();setInterval(poll,3000);document.addEventListener('visibilitychange',poll);
})();
