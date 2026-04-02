import React from "react";
import { createRoot } from "react-dom/client";
import { LivenessCapture } from "./LivenessCapture.jsx";

// ── State ─────────────────────────────────────────────────────────────────────
// idle | creating | capturing | polling | liveness_done | uploading | verified | error
let appState = "idle";
let appError = null;
let sessionId = null;
let userId = null;
let livenessResult = null;
let verifiedResult = null;
let selectedFile = null;
let previewURL = null;
let reactRoot = null;

// ── DOM ───────────────────────────────────────────────────────────────────────
document.getElementById("app").innerHTML = `
  <div style="
    font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    max-width: 640px; margin: 48px auto; padding: 0 20px;
  ">
    <h1 style="font-size:1.5rem;font-weight:600;margin-bottom:4px;">Identity Verification</h1>
    <p style="color:#666;margin-top:0;margin-bottom:24px;font-size:.9rem;">Powered by AWS Rekognition + Textract</p>

    <!-- Step indicator -->
    <div id="steps" style="display:flex;gap:0;margin-bottom:28px;"></div>

    <div id="sdk-container"></div>
    <div id="status-card" style="
      border:1px solid #e5e7eb;border-radius:16px;
      padding:32px;text-align:center;background:#fff;
    ">
      <div id="card-content"></div>
    </div>
  </div>
`;

const sdkContainer  = document.getElementById("sdk-container");
const statusCard    = document.getElementById("status-card");
const cardContent   = document.getElementById("card-content");
const stepsEl       = document.getElementById("steps");

// ── Steps indicator ───────────────────────────────────────────────────────────
const STEPS = ["Liveness", "Document", "Result"];

function renderSteps(active) {
  stepsEl.innerHTML = STEPS.map((label, i) => {
    const done    = i < active;
    const current = i === active;
    const color   = done ? "#16a34a" : current ? "#2563eb" : "#9ca3af";
    return `
      <div style="flex:1;text-align:center;position:relative;">
        <div style="
          width:28px;height:28px;border-radius:50%;
          background:${done ? "#16a34a" : current ? "#2563eb" : "#e5e7eb"};
          color:#fff;font-size:.8rem;font-weight:600;
          display:flex;align-items:center;justify-content:center;
          margin:0 auto 4px;
        ">${done ? "✓" : i + 1}</div>
        <div style="font-size:.75rem;color:${color};font-weight:${current ? 600 : 400};">${label}</div>
        ${i < STEPS.length - 1 ? `<div style="
          position:absolute;top:14px;left:60%;width:80%;height:2px;
          background:${done ? "#16a34a" : "#e5e7eb"};
        "></div>` : ""}
      </div>
    `;
  }).join("");
}

// ── Render ────────────────────────────────────────────────────────────────────
function render() {
  const capturing = appState === "capturing";
  sdkContainer.style.display = capturing ? "block" : "none";
  statusCard.style.display   = capturing ? "none"  : "block";

  const stepIndex = ["idle","creating","capturing","polling","liveness_done"].includes(appState) ? 0
    : ["uploading"].includes(appState) ? 1
    : 2;
  renderSteps(stepIndex);

  switch (appState) {
    // ── Step 1: Enter name ──────────────────────────────────────────────────
    case "idle":
      cardContent.innerHTML = `
        <p style="color:#374151;margin-bottom:20px;">Enter your name to begin.</p>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <input id="username-input" type="text" placeholder="Your name"
            style="padding:11px 16px;border:1px solid #d1d5db;border-radius:10px;font-size:1rem;width:200px;outline:none;" />
          <button onclick="window.__start()" style="${btn("#2563eb")}">Start Verification</button>
        </div>
      `;
      setTimeout(() => {
        const inp = document.getElementById("username-input");
        inp?.focus();
        inp?.addEventListener("keydown", e => { if (e.key === "Enter") window.__start(); });
      }, 0);
      break;

    case "creating":
      cardContent.innerHTML = spinner("Creating session…");
      break;

    // ── Step 1: Liveness done ───────────────────────────────────────────────
    case "liveness_done": {
      const { livenessConfidence, referenceImage } = livenessResult;
      cardContent.innerHTML = `
        ${referenceImage ? `<img src="${referenceImage}" style="
          width:120px;height:120px;object-fit:cover;border-radius:50%;
          border:3px solid #16a34a;margin-bottom:16px;
        " />` : ""}
        <p style="color:#16a34a;font-weight:600;font-size:1rem;margin:0 0 4px;">
          ✅ Liveness Verified
        </p>
        <p style="color:#6b7280;font-size:.9rem;margin:0 0 24px;">
          Confidence: <strong>${livenessConfidence?.toFixed(1)}%</strong>
        </p>
        <p style="color:#374151;font-size:.95rem;margin:0 0 20px;">
          Now upload a photo of your ID document.
        </p>
        <button onclick="window.__goUpload()" style="${btn("#2563eb")}">
          Upload ID Document →
        </button>
      `;
      break;
    }

    // ── Step 2: Upload document ─────────────────────────────────────────────
    case "uploading":
      cardContent.innerHTML = `
        <p style="color:#374151;margin-bottom:16px;font-weight:500;">
          Upload a photo of your ID (passport or driver's license)
        </p>

        <!-- Drop zone -->
        <label id="drop-zone" style="
          display:block;border:2px dashed #d1d5db;border-radius:12px;
          padding:24px;cursor:pointer;transition:border-color .2s;margin-bottom:16px;
        ">
          ${previewURL ? `
            <img src="${previewURL}" style="max-height:180px;max-width:100%;border-radius:8px;display:block;margin:0 auto;" />
            <p style="color:#6b7280;font-size:.8rem;margin:8px 0 0;">Click to change</p>
          ` : `
            <div style="color:#9ca3af;font-size:2rem;margin-bottom:8px;">📄</div>
            <p style="color:#6b7280;margin:0;font-size:.9rem;">Click or drag to upload your ID</p>
            <p style="color:#9ca3af;margin:4px 0 0;font-size:.8rem;">JPEG or PNG, max 5 MB</p>
          `}
          <input id="file-input" type="file" accept="image/jpeg,image/png" style="display:none;" />
        </label>

        <div style="display:flex;gap:10px;justify-content:center;">
          <button onclick="window.__back()" style="${btn("#9ca3af")}">← Back</button>
          <button onclick="window.__submitDoc()" ${!selectedFile ? "disabled" : ""}
            style="${btn(selectedFile ? "#2563eb" : "#d1d5db")}${!selectedFile ? "cursor:not-allowed;" : ""}">
            Verify Document
          </button>
        </div>
      `;
      // Wire up file input
      setTimeout(() => {
        const dropZone = document.getElementById("drop-zone");
        const fileInput = document.getElementById("file-input");
        dropZone?.addEventListener("dragover", e => { e.preventDefault(); dropZone.style.borderColor = "#2563eb"; });
        dropZone?.addEventListener("dragleave", () => { dropZone.style.borderColor = "#d1d5db"; });
        dropZone?.addEventListener("drop", e => {
          e.preventDefault();
          const f = e.dataTransfer?.files?.[0];
          if (f) handleFile(f);
        });
        fileInput?.addEventListener("change", e => {
          const f = e.target.files?.[0];
          if (f) handleFile(f);
        });
      }, 0);
      break;

    case "processing_doc":
      cardContent.innerHTML = spinner("Analyzing document & matching face…");
      break;

    // ── Step 3: Final result ────────────────────────────────────────────────
    case "verified": {
      const { verificationStatus, document: doc, faceMatch } = verifiedResult;
      const passed = verificationStatus === "verified";

      cardContent.innerHTML = `
        <div style="font-size:2.5rem;margin-bottom:8px;">${passed ? "✅" : "❌"}</div>
        <h2 style="margin:0 0 20px;font-size:1.2rem;color:${passed ? "#16a34a" : "#dc2626"};">
          ${passed ? "Identity Verified" : "Verification Failed"}
        </h2>

        <!-- Two-column summary -->
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:24px;text-align:left;">

          <!-- Liveness -->
          <div style="background:#f0fdf4;border-radius:10px;padding:14px;">
            <div style="font-size:.75rem;color:#16a34a;font-weight:600;text-transform:uppercase;margin-bottom:6px;">Liveness</div>
            <div style="font-size:1.1rem;font-weight:700;color:#15803d;">
              ${livenessResult?.livenessConfidence?.toFixed(1)}%
            </div>
            <div style="font-size:.8rem;color:#4b5563;margin-top:2px;">Confidence</div>
          </div>

          <!-- Face match -->
          <div style="background:${faceMatch?.passed ? "#f0fdf4" : "#fef2f2"};border-radius:10px;padding:14px;">
            <div style="font-size:.75rem;color:${faceMatch?.passed ? "#16a34a" : "#dc2626"};font-weight:600;text-transform:uppercase;margin-bottom:6px;">Face Match</div>
            <div style="font-size:1.1rem;font-weight:700;color:${faceMatch?.passed ? "#15803d" : "#dc2626"};">
              ${faceMatch?.similarity?.toFixed(1)}%
            </div>
            <div style="font-size:.8rem;color:#4b5563;margin-top:2px;">Similarity</div>
          </div>
        </div>

        <!-- Document fields -->
        ${doc ? `
          <div style="background:#f9fafb;border-radius:10px;padding:16px;text-align:left;margin-bottom:24px;">
            <div style="font-size:.75rem;color:#6b7280;font-weight:600;text-transform:uppercase;margin-bottom:10px;">Document Details</div>
            ${docField("Name",       [doc.firstName, doc.lastName].filter(Boolean).join(" "))}
            ${docField("Date of Birth", doc.dob)}
            ${docField("ID Number",  doc.idNumber)}
            ${docField("Expiry",     doc.expiry)}
            ${docField("Address",    doc.address)}
          </div>
        ` : ""}

        <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
      `;
      break;
    }

    case "error": {
      const isTimeout = appError?.includes("CONNECTION_TIMEOUT") || appError?.includes("Could not connect");
      cardContent.innerHTML = `
        <div style="font-size:2.5rem;margin-bottom:8px;">${isTimeout ? "📡" : "⚠️"}</div>
        <h2 style="margin:0 0 8px;font-size:1.1rem;color:#dc2626;">
          ${isTimeout ? "Connection timed out" : "Something went wrong"}
        </h2>
        ${isTimeout ? `
          <p style="color:#6b7280;font-size:.9rem;margin:0 0 20px;">
            Could not connect to AWS Rekognition.<br>
            This is usually a temporary network issue — please try again.
          </p>
        ` : `
          <pre style="background:#fef2f2;color:#7f1d1d;padding:12px;border-radius:8px;
            text-align:left;font-size:.8rem;white-space:pre-wrap;overflow:auto;margin-bottom:16px;">${escHTML(appError)}</pre>
        `}
        <div style="display:flex;gap:10px;justify-content:center;">
          ${isTimeout ? `<button onclick="window.__retry()" style="${btn("#2563eb")}">Retry</button>` : ""}
          <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
        </div>
      `;
      break;
    }
      break;
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function btn(bg) {
  return `display:inline-block;padding:11px 22px;border:none;border-radius:10px;background:${bg};color:#fff;font-size:.95rem;font-weight:500;cursor:pointer;`;
}
function spinner(msg) {
  return `
    <div style="display:flex;justify-content:center;gap:6px;margin-bottom:12px;">
      ${[0,0.15,0.3].map(d=>`<div style="width:10px;height:10px;border-radius:50%;background:#2563eb;
        animation:bounce 1.2s ease-in-out ${d}s infinite both;"></div>`).join("")}
    </div>
    <style>@keyframes bounce{0%,80%,100%{transform:scale(0);opacity:.5}40%{transform:scale(1);opacity:1}}</style>
    <p style="color:#374151;margin:0;">${msg}</p>`;
}
function escHTML(s) {
  return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
}
function docField(label, value) {
  if (!value) return "";
  return `<div style="display:flex;justify-content:space-between;padding:4px 0;border-bottom:1px solid #e5e7eb;font-size:.875rem;">
    <span style="color:#6b7280;">${label}</span>
    <span style="color:#111827;font-weight:500;">${escHTML(value)}</span>
  </div>`;
}

function handleFile(f) {
  if (f.size > 5 * 1024 * 1024) { alert("File must be under 5 MB"); return; }
  selectedFile = f;
  if (previewURL) URL.revokeObjectURL(previewURL);
  previewURL = URL.createObjectURL(f);
  render();
}

// ── API ───────────────────────────────────────────────────────────────────────
async function apiCreateSession(username) {
  const res = await fetch("/api/sessions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username }),
  });
  if (!res.ok) throw new Error(`Create session failed (${res.status}): ${await res.text()}`);
  return res.json();
}

async function apiGetResult(sid) {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sid)}/result`);
  if (!res.ok) throw new Error(`Get result failed (${res.status}): ${await res.text()}`);
  return res.json();
}

async function apiUploadDocument(uid, file) {
  const fd = new FormData();
  fd.append("userId", uid);
  fd.append("file", file);
  const res = await fetch("/api/documents", { method: "POST", body: fd });
  if (!res.ok) throw new Error(`Document upload failed (${res.status}): ${await res.text()}`);
  return res.json();
}

// ── Flow ──────────────────────────────────────────────────────────────────────
async function start() {
  const username = document.getElementById("username-input")?.value?.trim();
  if (!username) { document.getElementById("username-input")?.focus(); return; }

  appState = "creating"; render();
  try {
    const session = await apiCreateSession(username);
    sessionId = session.sessionId;
    userId    = session.userId;
  } catch (e) { appError = e.message; appState = "error"; render(); return; }

  appState = "capturing"; render();
  reactRoot = createRoot(sdkContainer);
  reactRoot.render(React.createElement(LivenessCapture, {
    sessionId,
    onComplete: async () => {
      reactRoot.unmount(); reactRoot = null;
      appState = "polling"; render();
      try {
        livenessResult = await apiGetResult(sessionId);
        appState = "liveness_done"; render();
      } catch (e) { appError = e.message; appState = "error"; render(); }
    },
    onCancel: () => { reactRoot.unmount(); reactRoot = null; appState = "idle"; render(); },
    onError:  (e) => { reactRoot.unmount(); reactRoot = null; appError = e?.message ?? JSON.stringify(e,null,2); appState = "error"; render(); },
  }));
}

async function submitDoc() {
  if (!selectedFile) return;
  appState = "processing_doc"; render();
  try {
    verifiedResult = await apiUploadDocument(userId, selectedFile);
    appState = "verified"; render();
  } catch (e) { appError = e.message; appState = "error"; render(); }
}

function reset() {
  sessionId = null; userId = null; livenessResult = null;
  verifiedResult = null; selectedFile = null;
  if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = null; }
  appError = null; appState = "idle"; render();
}

async function retry() {
  // Create a fresh session but keep the same username/userId context.
  appError = null;
  appState = "creating"; render();

  const username = (await fetch(`/api/users/${userId}`).then(r => r.json()).catch(() => null))?.username;
  if (!username) { reset(); return; }

  try {
    const session = await apiCreateSession(username);
    sessionId = session.sessionId;
    userId    = session.userId;
  } catch (e) { appError = e.message; appState = "error"; render(); return; }

  appState = "capturing"; render();
  reactRoot = createRoot(sdkContainer);
  reactRoot.render(React.createElement(LivenessCapture, {
    sessionId,
    onComplete: async () => {
      reactRoot.unmount(); reactRoot = null;
      appState = "polling"; render();
      try {
        livenessResult = await apiGetResult(sessionId);
        appState = "liveness_done"; render();
      } catch (e) { appError = e.message; appState = "error"; render(); }
    },
    onCancel: () => { reactRoot.unmount(); reactRoot = null; appState = "idle"; render(); },
    onError:  (e) => {
      reactRoot.unmount(); reactRoot = null;
      appError = JSON.stringify(e, null, 2); appState = "error"; render();
    },
  }));
}

window.__start     = start;
window.__retry     = retry;
window.__goUpload  = () => { appState = "uploading"; render(); };
window.__back      = () => { appState = "liveness_done"; render(); };
window.__submitDoc = submitDoc;
window.__reset     = reset;

render();
