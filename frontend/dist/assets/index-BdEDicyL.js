(function(){const n=document.createElement("link").relList;if(n&&n.supports&&n.supports("modulepreload"))return;for(const t of document.querySelectorAll('link[rel="modulepreload"]'))o(t);new MutationObserver(t=>{for(const r of t)if(r.type==="childList")for(const d of r.addedNodes)d.tagName==="LINK"&&d.rel==="modulepreload"&&o(d)}).observe(document,{childList:!0,subtree:!0});function a(t){const r={};return t.integrity&&(r.integrity=t.integrity),t.referrerPolicy&&(r.referrerPolicy=t.referrerPolicy),t.crossOrigin==="use-credentials"?r.credentials="include":t.crossOrigin==="anonymous"?r.credentials="omit":r.credentials="same-origin",r}function o(t){if(t.ep)return;t.ep=!0;const r=a(t);fetch(t.href,r)}})();const u=document.getElementById("app");u.innerHTML=`
  <div style="font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;max-width:1100px;margin:24px auto;padding:0 16px;">
    <h2>Azure Face Liveness (separate frontend)</h2>
    <p style="color:#555">
      This frontend calls your Go backend to create a liveness session, then (optionally) starts the Azure web component.
      The backend polls the final decision.
    </p>

    <div style="display:grid;grid-template-columns:360px 1fr;gap:16px;align-items:start;">
      <div style="border:1px solid #eee;border-radius:12px;padding:12px;">
        <h3>Camera preview</h3>
        <video id="preview" autoplay playsinline muted style="width:100%;border-radius:12px;background:#111;"></video>

        <div style="margin-top:12px;display:flex;gap:10px;flex-wrap:wrap;">
          <button id="btnStartCamera">Start camera</button>
          <button id="btnCreateSession">Create liveness session</button>
          <button id="btnPoll">Poll result (once)</button>
        </div>

        <p style="color:#666;margin-top:12px">
          Full liveness capture on Web requires the official SDK <code>@azure/ai-vision-face-ui</code> (not included here).
        </p>
      </div>

      <div style="border:1px solid #eee;border-radius:12px;padding:12px;">
        <h3>Liveness UI</h3>
        <div id="livenessContainer"></div>
        <p style="color:#666">
          Once you install/register the web component, it will render above and guide the user.
        </p>

        <h3>Debug</h3>
        <pre id="out" style="background:#0b1020;color:#d7e1ff;padding:12px;border-radius:12px;overflow:auto;">{}</pre>
      </div>
    </div>
  </div>
`;const p=document.getElementById("out"),f=document.getElementById("preview"),m=document.getElementById("livenessContainer");let s=null,l=null;function i(e){p.textContent=JSON.stringify(e,null,2)}async function h(){const e=await navigator.mediaDevices.getUserMedia({video:{facingMode:"user"},audio:!1});f.srcObject=e}async function y(){const e=await fetch("/api/liveness/session",{method:"POST"});if(!e.ok)throw new Error(await e.text());const n=await e.json();return s=n.sessionId,l=n.authToken,i({createdSession:n}),n}async function c(){if(!s)throw new Error("Create a session first");const e=await fetch("/api/liveness/session/"+encodeURIComponent(s));if(!e.ok)throw new Error(await e.text());const n=await e.json();return i({polledSession:n}),n}async function v({intervalMs:e=2e3,timeoutMs:n=18e4}={}){if(!s)throw new Error("Create a session first");const a=Date.now();for(;Date.now()-a<n;){const o=await c(),t=o?.status;if(t==="Succeeded"||t==="Failed"||t==="Canceled")return o;await new Promise(r=>setTimeout(r,e))}throw new Error("Polling timed out waiting for final session result")}async function g(){if(!l)throw new Error("Create a session first");const e=document.createElement("azure-ai-vision-face-ui");m.replaceChildren(e);const n=await e.start(l);i({sdkFinished:n,note:"Now poll /api/liveness/session/{id} for the liveness decision."})}document.getElementById("btnStartCamera").addEventListener("click",()=>h().catch(e=>i({error:String(e)})));document.getElementById("btnCreateSession").addEventListener("click",async()=>{try{await y(),await g(),await v()}catch(e){i({error:String(e),hint:"If Azure returns 403 UnsupportedFeature/LivenessDetection, your Face resource needs approval: https://aka.ms/facerecognition. If you see 'el.start is not a function', install/bundle @azure/ai-vision-face-ui so the custom element is registered."})}});document.getElementById("btnPoll").addEventListener("click",()=>c().catch(e=>i({error:String(e)})));
