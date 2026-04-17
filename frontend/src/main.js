import React from "react";
import { createRoot } from "react-dom/client";
import { LivenessCapture } from "./LivenessCapture.jsx";
import { AzureLivenessCapture } from "./AzureLivenessCapture.jsx";

// Hardcoded test user — set VITE_USER_ID in frontend/.env
const USER_ID = import.meta.env.VITE_USER_ID || "";

// X-User-ID is sent on every API request. For multipart (documents) we
// omit Content-Type so the browser sets it with the boundary.
const jsonHeaders = () => ({ "Content-Type": "application/json", "X-User-ID": USER_ID });
const authHeaders = () => ({ "X-User-ID": USER_ID });

// ── State ──────────────────────────────────────────────────────────────────
// idle | creating | capturing | polling | liveness_done |
// uploading | processing_doc | consenting | storing_consent | verified | duplicate | error
let appState          = "idle";
let appError          = null;
let sessionId         = null;   // our internal UUID
let providerSessionId = null;   // AWS/Azure session ID — passed to the liveness SDK
let provider          = null;   // "aws" | "azure" — determines which SDK to render
let authToken         = null;   // Azure only — passed to AzureLivenessCapture
let livenessResult    = null;   // { livenessStatus, livenessImage }
let livenessImageURL  = null;   // data URL of captured liveness face
let verifiedResult    = null;   // { decisionStatus, document, faceMatch }
let selectedFile      = null;
let previewURL        = null;
let reactRoot         = null;
let consentStored     = false;

// ── DOM ────────────────────────────────────────────────────────────────────
document.getElementById("app").innerHTML = `
  <div style="
    font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    max-width: 640px; margin: 48px auto; padding: 0 20px;
  ">
    <h1 style="font-size:1.5rem;font-weight:600;margin-bottom:4px;">Identity Verification</h1>
    <p style="color:#666;margin-top:0;margin-bottom:24px;font-size:.9rem;">
      Powered by AWS Rekognition + Textract
    </p>

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

const sdkContainer = document.getElementById("sdk-container");
const statusCard   = document.getElementById("status-card");
const cardContent  = document.getElementById("card-content");
const stepsEl      = document.getElementById("steps");

// ── Step indicator ─────────────────────────────────────────────────────────
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
          display:flex;align-items:center;justify-content:center;margin:0 auto 4px;
        ">${done ? "✓" : i + 1}</div>
        <div style="font-size:.75rem;color:${color};font-weight:${current ? 600 : 400};">${label}</div>
        ${i < STEPS.length - 1 ? `<div style="
          position:absolute;top:14px;left:60%;width:80%;height:2px;
          background:${done ? "#16a34a" : "#e5e7eb"};
        "></div>` : ""}
      </div>`;
  }).join("");
}

// ── Render ─────────────────────────────────────────────────────────────────
function render() {
  const capturing = appState === "capturing";
  sdkContainer.style.display = capturing ? "block" : "none";
  statusCard.style.display   = capturing ? "none"  : "block";

  const stepIndex =
    ["idle","creating","capturing","polling","liveness_done"].includes(appState) ? 0
    : ["uploading","processing_doc"].includes(appState) ? 1
    : 2;
  renderSteps(stepIndex);

  switch (appState) {

    // ── Start ───────────────────────────────────────────────────────────────
    case "idle":
      cardContent.innerHTML = `
        ${!USER_ID ? `<div style="
          background:#fef9c3;border:1px solid #fde047;border-radius:8px;
          padding:12px 16px;margin-bottom:20px;font-size:.85rem;text-align:left;
        ">Set <code>VITE_USER_ID</code> in <code>frontend/.env</code> then restart the dev server.</div>` : ""}
        <p style="color:#374151;margin-bottom:24px;">
          Click below to start your identity verification.
        </p>
        <button onclick="window.__start()" ${!USER_ID ? "disabled" : ""} style="${btn("#2563eb")}">
          Start Verification
        </button>
      `;
      break;

    case "creating":
      cardContent.innerHTML = spinner("Creating session…");
      break;

    // ── Liveness done ───────────────────────────────────────────────────────
    case "liveness_done":
      cardContent.innerHTML = `
        ${livenessImageURL ? `<img src="${livenessImageURL}" style="
          width:120px;height:120px;object-fit:cover;border-radius:50%;
          border:3px solid #2563eb;display:block;margin:0 auto 16px;
        " />` : ""}
        <p style="color:#16a34a;font-weight:600;font-size:1rem;margin:0 0 4px;">✅ Liveness Verified</p>
        <p style="color:#374151;font-size:.95rem;margin:0 0 20px;">
          Now upload a photo of your ID document.
        </p>
        <button onclick="window.__goUpload()" style="${btn("#2563eb")}">
          Upload ID Document →
        </button>
      `;
      break;

    case "polling":
      cardContent.innerHTML = spinner("Fetching liveness result…");
      break;

    // ── Document upload ─────────────────────────────────────────────────────
    case "uploading":
      cardContent.innerHTML = `
        <p style="color:#374151;margin-bottom:16px;font-weight:500;">
          Upload a photo of your ID (passport or driver's license)
        </p>
        <label id="drop-zone" style="
          display:block;border:2px dashed #d1d5db;border-radius:12px;
          padding:24px;cursor:pointer;margin-bottom:16px;
        ">
          ${previewURL
            ? `<img src="${previewURL}" style="max-height:180px;max-width:100%;border-radius:8px;display:block;margin:0 auto;" />
               <p style="color:#6b7280;font-size:.8rem;margin:8px 0 0;">Click to change</p>`
            : `<div style="color:#9ca3af;font-size:2rem;margin-bottom:8px;">📄</div>
               <p style="color:#6b7280;margin:0;font-size:.9rem;">Click or drag to upload your ID</p>
               <p style="color:#9ca3af;margin:4px 0 0;font-size:.8rem;">JPEG or PNG, max 5 MB</p>`}
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
      setTimeout(() => {
        const dropZone  = document.getElementById("drop-zone");
        const fileInput = document.getElementById("file-input");
        dropZone?.addEventListener("dragover", e => { e.preventDefault(); dropZone.style.borderColor = "#2563eb"; });
        dropZone?.addEventListener("dragleave", () => { dropZone.style.borderColor = "#d1d5db"; });
        dropZone?.addEventListener("drop", e => { e.preventDefault(); const f = e.dataTransfer?.files?.[0]; if (f) handleFile(f); });
        fileInput?.addEventListener("change", e => { const f = e.target.files?.[0]; if (f) handleFile(f); });
      }, 0);
      break;

    case "processing_doc":
      cardContent.innerHTML = spinner("Analyzing document & matching face…");
      break;

    // ── Consent ─────────────────────────────────────────────────────────────
    case "consenting": {
      const doc = verifiedResult?.document || {};
      const fields = [
        { key: "first_name",      label: "First Name",      value: doc.firstName },
        { key: "last_name",       label: "Last Name",       value: doc.lastName },
        { key: "dob",             label: "Date of Birth",   value: doc.dob },
        { key: "doc_number",      label: "Document Number", value: doc.idNumber },
        { key: "expiry_date",     label: "Expiry Date",     value: doc.expiry },
        { key: "issuing_country", label: "Issuing Country", value: doc.issuingCountry },
      ].filter(f => f.value);

      cardContent.innerHTML = `
        ${(livenessImageURL || previewURL) ? `
        <div style="display:flex;gap:12px;justify-content:center;margin-bottom:16px;">
          ${livenessImageURL ? `<div style="text-align:center;">
            <img src="${livenessImageURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #2563eb;" />
            <p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Liveness capture</p>
          </div>` : ""}
          ${previewURL ? `<div style="text-align:center;">
            <img src="${previewURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #16a34a;" />
            <p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">ID document</p>
          </div>` : ""}
        </div>` : ""}
        <div style="font-size:2rem;margin-bottom:8px;">🔒</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">Consent to store your data</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">
          Face match: <strong>${verifiedResult?.faceMatch?.similarity?.toFixed(1)}%</strong> ✅ &nbsp;|&nbsp;
          Select which fields to securely store:
        </p>

        <div style="text-align:left;margin-bottom:20px;">
          ${fields.map(f => `
          <label style="
            display:flex;align-items:center;gap:10px;padding:10px 12px;
            border:1px solid #e5e7eb;border-radius:8px;margin-bottom:8px;
            cursor:pointer;font-size:.9rem;
          ">
            <input type="checkbox" class="consent-field" value="${f.key}" checked
              style="width:16px;height:16px;cursor:pointer;accent-color:#2563eb;" />
            <span style="flex:1;color:#374151;">${f.label}</span>
            <span style="color:#6b7280;font-family:monospace;font-size:.8rem;">${escapeHTML(f.value)}</span>
          </label>`).join("")}
        </div>

        <p style="color:#9ca3af;font-size:.78rem;margin:0 0 16px;">
          Data is encrypted (AES-256) and stored securely. Unchecked fields will not be saved.
        </p>

        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <button onclick="window.__submitConsent()" style="${btn("#2563eb")}">
            Consent &amp; Store My Data
          </button>
          <button onclick="window.__skipConsent()" style="${btn("#6b7280")}">
            Skip
          </button>
        </div>
      `;
      break;
    }

    case "storing_consent":
      cardContent.innerHTML = spinner("Storing your data securely…");
      break;

    // ── Verified ────────────────────────────────────────────────────────────
    case "verified": {
      const { decisionStatus, document: doc, faceMatch } = verifiedResult;
      const passed = decisionStatus === "verified";
      cardContent.innerHTML = `
        ${(livenessImageURL || previewURL) ? `
        <div style="display:flex;gap:12px;justify-content:center;margin-bottom:16px;">
          ${livenessImageURL ? `<div style="text-align:center;">
            <img src="${livenessImageURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #2563eb;" />
            <p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Liveness capture</p>
          </div>` : ""}
          ${previewURL ? `<div style="text-align:center;">
            <img src="${previewURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #16a34a;" />
            <p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">ID document</p>
          </div>` : ""}
        </div>` : ""}
        <div style="font-size:3rem;margin-bottom:12px;">${passed ? "🎉" : "❌"}</div>
        <h2 style="margin:0 0 8px;font-size:1.2rem;color:${passed ? "#16a34a" : "#dc2626"};">
          ${passed ? "Identity Verified" : "Verification Failed"}
        </h2>

        ${faceMatch ? `<p style="color:#6b7280;font-size:.9rem;margin:4px 0;">
          Face match: <strong>${faceMatch.similarity?.toFixed(1)}%</strong>
          ${faceMatch.passed ? "✅" : "❌"}
        </p>` : ""}

        ${consentStored ? `<p style="
          display:inline-block;background:#dcfce7;color:#16a34a;
          border-radius:6px;padding:4px 10px;font-size:.85rem;margin:8px 0;
        ">✅ Data securely stored</p>` : ""}

        ${doc ? `
        <div style="
          background:#f9fafb;border:1px solid #e5e7eb;border-radius:10px;
          padding:16px;margin-top:20px;text-align:left;font-size:.9rem;
        ">
          <p style="font-weight:600;margin:0 0 10px;color:#374151;">Extracted from document</p>
          ${doc.firstName      ? `<p style="margin:4px 0;color:#6b7280;">First Name: <strong style="color:#111;">${escapeHTML(doc.firstName)}</strong></p>` : ""}
          ${doc.lastName       ? `<p style="margin:4px 0;color:#6b7280;">Last Name: <strong style="color:#111;">${escapeHTML(doc.lastName)}</strong></p>` : ""}
          ${doc.dob            ? `<p style="margin:4px 0;color:#6b7280;">Date of Birth: <strong style="color:#111;">${escapeHTML(doc.dob)}</strong></p>` : ""}
          ${doc.idNumber       ? `<p style="margin:4px 0;color:#6b7280;">Document Number: <strong style="color:#111;">${escapeHTML(doc.idNumber)}</strong></p>` : ""}
          ${doc.expiry         ? `<p style="margin:4px 0;color:#6b7280;">Expiry: <strong style="color:#111;">${escapeHTML(doc.expiry)}</strong></p>` : ""}
          ${doc.issuingCountry ? `<p style="margin:4px 0;color:#6b7280;">Issuing Country: <strong style="color:#111;">${escapeHTML(doc.issuingCountry)}</strong></p>` : ""}
        </div>` : ""}

        <div style="display:flex;gap:10px;justify-content:center;margin-top:24px;flex-wrap:wrap;">
          ${faceMatch && !faceMatch.passed ? `
          <button onclick="window.__retryDoc()" style="${btn("#2563eb")}">
            Retry with different photo
          </button>` : ""}
          ${passed && !consentStored ? `
          <button onclick="window.__goConsent()" style="${btn("#059669")}">
            Store My Data
          </button>` : ""}
          <button onclick="window.__reset()" style="${btn("#6b7280")}">
            Start Over
          </button>
        </div>
      `;
      break;
    }

    // ── Duplicate ───────────────────────────────────────────────────────────
    case "duplicate":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">🚫</div>
        <h2 style="margin:0 0 8px;font-size:1.2rem;color:#dc2626;">Identity Already Exists</h2>
        <p style="color:#6b7280;font-size:.9rem;margin:0 0 24px;">
          The identity on this document (name + date of birth) is already linked to another account.
          Each document identity can only verify one account.
        </p>
        <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
      `;
      break;

    // ── Error ───────────────────────────────────────────────────────────────
    case "error":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">⚠️</div>
        <h2 style="margin:0 0 8px;font-size:1.1rem;color:#dc2626;">Something went wrong</h2>
        <pre style="
          background:#fef2f2;color:#7f1d1d;padding:12px;border-radius:8px;
          text-align:left;font-size:.8rem;white-space:pre-wrap;overflow:auto;
        ">${escapeHTML(appError)}</pre>
        <button onclick="window.__reset()" style="${btn("#6b7280")} margin-top:16px;">
          Try Again
        </button>
      `;
      break;
  }
}

// ── API calls ──────────────────────────────────────────────────────────────

// POST /api/sessions → { sessionId, providerSessionId, userId }
async function apiCreateSession() {
  const res = await fetch("/api/sessions", {
    method: "POST",
    headers: jsonHeaders(),
  });
  if (!res.ok) throw new Error(`Create session failed (${res.status}): ${await res.text()}`);
  return res.json();
}

// GET /api/sessions/:sessionId/result → { sessionId, livenessStatus, livenessConfidence, referenceImage }
async function apiGetLivenessResult(sid) {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sid)}/result`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(`Get result failed (${res.status}): ${await res.text()}`);
  return res.json();
}

// POST /api/sessions/:sessionId/consent → { stored: true }
async function apiStoreConsent(sid, fields) {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sid)}/consent`, {
    method: "POST",
    headers: jsonHeaders(),
    body: JSON.stringify({ fields }),
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message || "Duplicate identity detected.");
  }
  if (!res.ok) throw new Error(`Consent failed (${res.status}): ${await res.text()}`);
  return res.json();
}

// POST /api/documents (multipart) → { decisionStatus, document, faceMatch }
async function apiUploadDocument(sid, file) {
  const fd = new FormData();
  fd.append("sessionId", sid);
  fd.append("file", file);
  const res = await fetch("/api/documents", {
    method: "POST",
    headers: authHeaders(), // no Content-Type — browser sets multipart boundary
    body: fd,
  });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    throw Object.assign(new Error(body.message || "Duplicate identity detected."), { duplicate: true });
  }
  if (!res.ok) throw new Error(`Document upload failed (${res.status}): ${await res.text()}`);
  return res.json();
}

// ── Helpers ────────────────────────────────────────────────────────────────

function handleFile(f) {
  if (previewURL) URL.revokeObjectURL(previewURL);
  selectedFile = f;
  previewURL   = URL.createObjectURL(f);
  render();
}

function btn(bg) {
  return `display:inline-block;padding:12px 28px;border:none;border-radius:10px;
    background:${bg};color:#fff;font-size:1rem;font-weight:500;cursor:pointer;`;
}

function spinner(msg) {
  return `
    <div style="display:flex;justify-content:center;gap:6px;margin-bottom:12px;">
      ${[0, 0.15, 0.3].map(d => `<div style="
        width:10px;height:10px;border-radius:50%;background:#2563eb;
        animation:bounce 1.2s ease-in-out ${d}s infinite both;
      "></div>`).join("")}
    </div>
    <style>@keyframes bounce{0%,80%,100%{transform:scale(0);opacity:.5}40%{transform:scale(1);opacity:1}}</style>
    <p style="color:#374151;margin:0;">${msg}</p>`;
}

function escapeHTML(str) {
  return String(str).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// ── Flow ───────────────────────────────────────────────────────────────────

async function start() {
  appState = "creating"; render();

  try {
    const session     = await apiCreateSession();
    sessionId         = session.sessionId;
    providerSessionId = session.providerSessionId;
    provider          = session.provider;   // "aws" | "azure"
    authToken         = session.authToken;  // Azure only
  } catch (e) {
    appError = e.message; appState = "error"; render(); return;
  }

  appState = "capturing"; render();
  reactRoot = createRoot(sdkContainer);

  const onComplete = async () => {
    reactRoot.unmount(); reactRoot = null;
    appState = "polling"; render();
    try {
      livenessResult = await apiGetLivenessResult(sessionId);
      if (livenessResult.livenessImage) livenessImageURL = livenessResult.livenessImage;
      appState = "liveness_done"; render();
    } catch (e) {
      appError = e.message; appState = "error"; render();
    }
  };
  const onCancel = () => { reactRoot.unmount(); reactRoot = null; appState = "idle"; render(); };
  const onError  = (e) => {
    reactRoot.unmount(); reactRoot = null;
    appError = e?.message ?? JSON.stringify(e, null, 2);
    appState = "error"; render();
  };

  // Load the appropriate liveness SDK based on provider returned by backend.
  if (provider === "azure") {
    reactRoot.render(React.createElement(AzureLivenessCapture, {
      authToken, onComplete, onCancel, onError,
    }));
  } else {
    reactRoot.render(React.createElement(LivenessCapture, {
      providerSessionId, onComplete, onCancel, onError,
    }));
  }
}

async function submitDoc() {
  if (!selectedFile) return;
  appState = "processing_doc"; render();
  try {
    verifiedResult = await apiUploadDocument(sessionId, selectedFile);
    // Route to consent step if face match passed; otherwise show result directly.
    if (verifiedResult?.faceMatch?.passed) {
      appState = "consenting";
    } else {
      appState = "verified";
    }
    render();
  } catch (e) {
    if (e.duplicate) { appState = "duplicate"; render(); }
    else { appError = e.message; appState = "error"; render(); }
  }
}

async function submitConsent() {
  const checked = [...document.querySelectorAll(".consent-field:checked")].map(el => el.value);
  appState = "storing_consent"; render();
  try {
    await apiStoreConsent(sessionId, checked);
    consentStored = true;
    appState = "verified"; render();
  } catch (e) {
    appError = e.message; appState = "error"; render();
  }
}

function reset() {
  sessionId = null; providerSessionId = null;
  provider = null; authToken = null;
  livenessResult = null; livenessImageURL = null; verifiedResult = null;
  selectedFile = null; consentStored = false;
  if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = null; }
  appError = null; appState = "idle"; render();
}

function retryDoc() {
  verifiedResult = null;
  selectedFile = null;
  if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = null; }
  appError = null;
  appState = "uploading"; render();
}

window.__start          = start;
window.__goUpload       = () => { appState = "uploading"; render(); };
window.__back           = () => { appState = "liveness_done"; render(); };
window.__submitDoc      = submitDoc;
window.__reset          = reset;
window.__retryDoc       = retryDoc;
window.__submitConsent  = submitConsent;
window.__skipConsent    = () => { appState = "verified"; render(); };
window.__goConsent      = () => { appState = "consenting"; render(); };

render();
