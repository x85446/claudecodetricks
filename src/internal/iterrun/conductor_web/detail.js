'use strict';
(() => {
const header=document.querySelector('.conductor-detail-header'),panel=document.querySelector('#conductor-live-detail');
if(!header||!panel||location.pathname==='/archive')return;
const status=panel.querySelector('[data-status]'),usage=panel.querySelector('[data-usage]'),log=panel.querySelector('.conductor-event-scroll');
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
   const summary=document.createDocumentFragment(),observations=document.createDocumentFragment(),events=document.createDocumentFragment();
   for(const v of visibility){paragraph(summary,'Project state: '+v.state+' · '+v.effective_sequence);paragraph(summary,v.reason);paragraph(summary,'Next opportunity: '+v.next_opportunity);for(const u of v.usage||[])paragraph(observations,u.subscription_id+' usage: '+(u.available?u.age_seconds+' seconds old · '+u.source+' · observed '+new Date(u.observed_at).toLocaleString():'unavailable'));}
   for(const r of [...runs].sort((a,b)=>String(a.started||'').localeCompare(String(b.started||'')))){const h=document.createElement('h3');h.textContent=r.sequence_name+' · '+r.status;events.append(h);for(const e of seen.get(r.run_id)?.events||[])paragraph(events,new Date(e.at).toLocaleString()+' · '+e.kind+' · '+(e.message||''));}
   if(!runs.length)paragraph(events,'No conductor run recorded for this plan.');
   const top=log.scrollTop,follow=log.scrollHeight-log.clientHeight-top<24;
   if(status.textContent!==summary.textContent)status.replaceChildren(summary);
   usage.replaceChildren(observations);
   // Keep the event nodes stable when only usage observations change.
   if(log.textContent!==events.textContent){log.replaceChildren(events);log.scrollTop=follow?log.scrollHeight:top;}
   lastPaint=signature;
  }
 }catch(e){let notice=panel.querySelector('[data-connection]');if(!notice){notice=document.createElement('p');notice.dataset.connection='';notice.setAttribute('role','status');panel.prepend(notice);}notice.textContent='Activity reconnecting · '+e.message;}
 finally{busy=false}
}
poll();setInterval(poll,3000);document.addEventListener('visibilitychange',poll);
})();
