import React from "react";
import { createRoot } from "react-dom/client";
import { LivenessCapture } from "./LivenessCapture.jsx";
import { AzureLivenessCapture } from "./AzureLivenessCapture.jsx";

// Persist a stable user ID across page loads
const USER_ID = localStorage.getItem("dev_user_id") || (() => {
  const id = crypto.randomUUID();
  localStorage.setItem("dev_user_id", id);
  return id;
})();

const jsonHeaders = () => ({ "Content-Type": "application/json", "X-User-ID": USER_ID });
const authHeaders = () => ({ "X-User-ID": USER_ID });

// ── Identity verification state ────────────────────────────────────────────
let appState          = "idle";
let appError          = null;
let sessionId         = null;
let providerSessionId = null;
let provider          = null;
let authToken         = null;
let livenessResult    = null;
let livenessImageURL  = null;
let verifiedResult    = null;
let selectedFile      = null;
let previewURL        = null;
let reactRoot         = null;
let consentStored     = false;

// ── Profession verification state ──────────────────────────────────────────
let profVerificationId  = null;
let profType            = null;
let profScore           = 0;
let profLevel           = "none";
let profEvidence        = [];
let profEmailEvidenceId = null;
let profSelectedFile    = null;
let profPreviewURL      = null;
let profError           = null;

// ── DOM ────────────────────────────────────────────────────────────────────
document.getElementById("app").innerHTML = `
  <div style="
    font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    max-width: 680px; margin: 48px auto; padding: 0 20px;
  ">
    <div style="margin-bottom:24px;">
      <h1 style="font-size:1.5rem;font-weight:600;margin:0 0 4px;">Identity &amp; Profession Verification</h1>
      <p style="color:#666;margin:0;font-size:.9rem;">Privacy-first identity layer</p>
    </div>

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

// ── Step indicators ────────────────────────────────────────────────────────
const ID_STEPS   = ["Liveness", "Document", "Result"];
const PROF_STEPS = ["Profession", "Verify Email", "Score"];

function renderSteps(steps, active) {
  stepsEl.innerHTML = steps.map((label, i) => {
    const done    = i < active;
    const current = i === active;
    const color   = done ? "#16a34a" : current ? "#7c3aed" : "#9ca3af";
    return `
      <div style="flex:1;text-align:center;position:relative;">
        <div style="
          width:28px;height:28px;border-radius:50%;
          background:${done ? "#16a34a" : current ? "#7c3aed" : "#e5e7eb"};
          color:#fff;font-size:.8rem;font-weight:600;
          display:flex;align-items:center;justify-content:center;margin:0 auto 4px;
        ">${done ? "✓" : i + 1}</div>
        <div style="font-size:.75rem;color:${color};font-weight:${current ? 600 : 400};">${label}</div>
        ${i < steps.length - 1 ? `<div style="
          position:absolute;top:14px;left:60%;width:80%;height:2px;
          background:${done ? "#16a34a" : "#e5e7eb"};
        "></div>` : ""}
      </div>`;
  }).join("");
}

// ── Helpers ────────────────────────────────────────────────────────────────
function btn(bg, extra = "") {
  return `display:inline-block;padding:11px 24px;border:none;border-radius:10px;
    background:${bg};color:#fff;font-size:.95rem;font-weight:500;cursor:pointer;${extra}`;
}
function btnOutline(color, extra = "") {
  return `display:inline-block;padding:10px 22px;border:2px solid ${color};border-radius:10px;
    background:transparent;color:${color};font-size:.95rem;font-weight:500;cursor:pointer;${extra}`;
}
function spinner(msg, color = "#7c3aed") {
  return `
    <div style="display:flex;justify-content:center;gap:6px;margin-bottom:12px;">
      ${[0, 0.15, 0.3].map(d => `<div style="
        width:10px;height:10px;border-radius:50%;background:${color};
        animation:bounce 1.2s ease-in-out ${d}s infinite both;
      "></div>`).join("")}
    </div>
    <style>@keyframes bounce{0%,80%,100%{transform:scale(0);opacity:.5}40%{transform:scale(1);opacity:1}}</style>
    <p style="color:#374151;margin:0;">${msg}</p>`;
}
function escapeHTML(str) {
  return String(str ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
function inputStyle() {
  return `width:100%;box-sizing:border-box;padding:10px 12px;border:1px solid #d1d5db;
    border-radius:8px;font-size:.95rem;outline:none;`;
}
function scoreBar(score, level) {
  const colors = { none: "#e5e7eb", low: "#f59e0b", medium: "#3b82f6", high: "#16a34a" };
  const labels = { none: "No confidence", low: "Low confidence", medium: "Medium confidence", high: "High confidence" };
  const color  = colors[level] || "#e5e7eb";
  return `
    <div style="margin:12px 0 4px;">
      <div style="display:flex;justify-content:space-between;font-size:.8rem;margin-bottom:4px;">
        <span style="color:#6b7280;">Confidence score</span>
        <span style="color:${color};font-weight:600;">${score}/100 — ${labels[level]}</span>
      </div>
      <div style="background:#e5e7eb;border-radius:4px;height:8px;overflow:hidden;">
        <div style="background:${color};height:8px;border-radius:4px;width:${score}%;transition:width .4s ease;"></div>
      </div>
    </div>`;
}
function levelBadge(level) {
  const map = {
    none:   { bg: "#f3f4f6", color: "#6b7280", label: "Unverified" },
    low:    { bg: "#fef3c7", color: "#92400e", label: "Low confidence" },
    medium: { bg: "#dbeafe", color: "#1e40af", label: "Medium confidence" },
    high:   { bg: "#dcfce7", color: "#166534", label: "High confidence" },
  };
  const s = map[level] || map.none;
  return `<span style="background:${s.bg};color:${s.color};padding:3px 10px;border-radius:20px;font-size:.8rem;font-weight:600;">${s.label}</span>`;
}

// ── Render ─────────────────────────────────────────────────────────────────
function render() {
  const isProfState = appState.startsWith("prof_");
  const capturing   = appState === "capturing";

  sdkContainer.style.display = capturing ? "block" : "none";
  statusCard.style.display   = capturing ? "none"  : "block";

  if (!isProfState) {
    const stepIndex =
      ["idle","creating","capturing","polling","liveness_done"].includes(appState) ? 0
      : ["uploading","processing_doc"].includes(appState) ? 1
      : 2;
    renderSteps(ID_STEPS, stepIndex);
  } else {
    const profStep =
      ["prof_start","prof_creating"].includes(appState) ? 0
      : appState === "prof_done" ? 2
      : 1;
    renderSteps(PROF_STEPS, profStep);
  }

  switch (appState) {

    // ──────────────────────────────────────────────────────────────────────
    // IDENTITY VERIFICATION FLOW
    // ──────────────────────────────────────────────────────────────────────

    case "idle":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">🪪</div>
        <h2 style="margin:0 0 8px;font-size:1.2rem;color:#111;">Verify Your Identity</h2>
        <p style="color:#6b7280;font-size:.9rem;margin:0 0 24px;">
          Complete face liveness detection and upload a photo ID to get verified.
        </p>
        <button onclick="window.__start()" style="${btn("#2563eb")}">Start →</button>
      `;
      break;

    case "creating":
      cardContent.innerHTML = spinner("Creating session…", "#2563eb");
      break;

    case "liveness_done":
      cardContent.innerHTML = `
        ${livenessImageURL ? `<img src="${livenessImageURL}" style="
          width:120px;height:120px;object-fit:cover;border-radius:50%;
          border:3px solid #2563eb;display:block;margin:0 auto 16px;" />` : ""}
        <p style="color:#16a34a;font-weight:600;font-size:1rem;margin:0 0 4px;">✅ Liveness Verified</p>
        <p style="color:#374151;font-size:.95rem;margin:0 0 20px;">Now upload a photo of your ID document.</p>
        <button onclick="window.__goUpload()" style="${btn("#2563eb")}">Upload ID Document →</button>
      `;
      break;

    case "polling":
      cardContent.innerHTML = spinner("Fetching liveness result…", "#2563eb");
      break;

    case "uploading":
      cardContent.innerHTML = `
        <p style="color:#374151;margin-bottom:16px;font-weight:500;">Upload a photo of your ID (passport or driver's license)</p>
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
        const dz = document.getElementById("drop-zone");
        const fi = document.getElementById("file-input");
        dz?.addEventListener("dragover", e => { e.preventDefault(); dz.style.borderColor = "#2563eb"; });
        dz?.addEventListener("dragleave", () => { dz.style.borderColor = "#d1d5db"; });
        dz?.addEventListener("drop", e => { e.preventDefault(); const f = e.dataTransfer?.files?.[0]; if (f) handleFile(f); });
        fi?.addEventListener("change", e => { const f = e.target.files?.[0]; if (f) handleFile(f); });
      }, 0);
      break;

    case "processing_doc":
      cardContent.innerHTML = spinner("Analyzing document &amp; matching face…", "#2563eb");
      break;

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
        ${(livenessImageURL || previewURL) ? `<div style="display:flex;gap:12px;justify-content:center;margin-bottom:16px;">
          ${livenessImageURL ? `<div style="text-align:center;"><img src="${livenessImageURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #2563eb;" /><p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Liveness</p></div>` : ""}
          ${previewURL ? `<div style="text-align:center;"><img src="${previewURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #16a34a;" /><p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Document</p></div>` : ""}
        </div>` : ""}
        <div style="font-size:2rem;margin-bottom:8px;">🔒</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">Consent to store your data</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">
          Face match: <strong>${verifiedResult?.faceMatch?.similarity?.toFixed(1)}%</strong> ✅ &nbsp;|&nbsp; Select fields to securely store:
        </p>
        <div style="text-align:left;margin-bottom:20px;">
          ${fields.map(f => `<label style="display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid #e5e7eb;border-radius:8px;margin-bottom:8px;cursor:pointer;font-size:.9rem;">
            <input type="checkbox" class="consent-field" value="${f.key}" checked style="width:16px;height:16px;cursor:pointer;accent-color:#2563eb;" />
            <span style="flex:1;color:#374151;">${f.label}</span>
            <span style="color:#6b7280;font-family:monospace;font-size:.8rem;">${escapeHTML(f.value)}</span>
          </label>`).join("")}
        </div>
        <p style="color:#9ca3af;font-size:.78rem;margin:0 0 16px;">Encrypted with AES-256. Unchecked fields are not saved.</p>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <button onclick="window.__submitConsent()" style="${btn("#2563eb")}">Consent &amp; Store</button>
          <button onclick="window.__skipConsent()" style="${btn("#6b7280")}">Skip</button>
        </div>
      `;
      break;
    }

    case "storing_consent":
      cardContent.innerHTML = spinner("Storing securely…", "#2563eb");
      break;

    case "verified": {
      const { decisionStatus, document: doc, faceMatch } = verifiedResult;
      const passed = decisionStatus === "verified";
      cardContent.innerHTML = `
        ${(livenessImageURL || previewURL) ? `<div style="display:flex;gap:12px;justify-content:center;margin-bottom:16px;">
          ${livenessImageURL ? `<div style="text-align:center;"><img src="${livenessImageURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #2563eb;" /><p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Liveness</p></div>` : ""}
          ${previewURL ? `<div style="text-align:center;"><img src="${previewURL}" style="width:90px;height:90px;object-fit:cover;border-radius:50%;border:3px solid #16a34a;" /><p style="font-size:.72rem;color:#6b7280;margin:4px 0 0;">Document</p></div>` : ""}
        </div>` : ""}
        <div style="font-size:3rem;margin-bottom:12px;">${passed ? "🎉" : "❌"}</div>
        <h2 style="margin:0 0 8px;font-size:1.2rem;color:${passed ? "#16a34a" : "#dc2626"};">${passed ? "Identity Verified" : "Verification Failed"}</h2>
        ${faceMatch ? `<p style="color:#6b7280;font-size:.9rem;margin:4px 0;">Face match: <strong>${faceMatch.similarity?.toFixed(1)}%</strong> ${faceMatch.passed ? "✅" : "❌"}</p>` : ""}
        ${consentStored ? `<p style="display:inline-block;background:#dcfce7;color:#16a34a;border-radius:6px;padding:4px 10px;font-size:.85rem;margin:8px 0;">✅ Data securely stored</p>` : ""}
        ${doc ? `<div style="background:#f9fafb;border:1px solid #e5e7eb;border-radius:10px;padding:16px;margin-top:20px;text-align:left;font-size:.9rem;">
          <p style="font-weight:600;margin:0 0 10px;color:#374151;">Extracted from document</p>
          ${doc.firstName ? `<p style="margin:4px 0;color:#6b7280;">First Name: <strong style="color:#111;">${escapeHTML(doc.firstName)}</strong></p>` : ""}
          ${doc.lastName ? `<p style="margin:4px 0;color:#6b7280;">Last Name: <strong style="color:#111;">${escapeHTML(doc.lastName)}</strong></p>` : ""}
          ${doc.dob ? `<p style="margin:4px 0;color:#6b7280;">Date of Birth: <strong style="color:#111;">${escapeHTML(doc.dob)}</strong></p>` : ""}
          ${doc.idNumber ? `<p style="margin:4px 0;color:#6b7280;">Document Number: <strong style="color:#111;">${escapeHTML(doc.idNumber)}</strong></p>` : ""}
          ${doc.expiry ? `<p style="margin:4px 0;color:#6b7280;">Expiry: <strong style="color:#111;">${escapeHTML(doc.expiry)}</strong></p>` : ""}
          ${doc.issuingCountry ? `<p style="margin:4px 0;color:#6b7280;">Country: <strong style="color:#111;">${escapeHTML(doc.issuingCountry)}</strong></p>` : ""}
        </div>` : ""}
        <div style="display:flex;gap:10px;justify-content:center;margin-top:24px;flex-wrap:wrap;">
          ${faceMatch && !faceMatch.passed ? `<button onclick="window.__retryDoc()" style="${btn("#2563eb")}">Retry with different photo</button>` : ""}
          ${passed && !consentStored ? `<button onclick="window.__goConsent()" style="${btn("#059669")}">Store My Data</button>` : ""}
          ${passed ? `<button onclick="window.__profStart()" style="${btn("#7c3aed")}">Verify Profession →</button>` : ""}
          <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
        </div>
      `;
      break;
    }

    case "duplicate":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">🚫</div>
        <h2 style="margin:0 0 8px;font-size:1.2rem;color:#dc2626;">Identity Already Exists</h2>
        <p style="color:#6b7280;font-size:.9rem;margin:0 0 24px;">
          The identity on this document (name + date of birth) is already linked to another account.
        </p>
        <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
      `;
      break;

    case "error":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">⚠️</div>
        <h2 style="margin:0 0 8px;font-size:1.1rem;color:#dc2626;">Something went wrong</h2>
        <pre style="background:#fef2f2;color:#7f1d1d;padding:12px;border-radius:8px;text-align:left;font-size:.8rem;white-space:pre-wrap;overflow:auto;">${escapeHTML(appError)}</pre>
        <button onclick="window.__reset()" style="${btn("#6b7280")} margin-top:16px;">Try Again</button>
      `;
      break;

    // ──────────────────────────────────────────────────────────────────────
    // PROFESSION VERIFICATION FLOW
    // ──────────────────────────────────────────────────────────────────────

    case "prof_start":
      cardContent.innerHTML = `
        <div style="font-size:2rem;margin-bottom:8px;">🧑‍💼</div>
        <h2 style="margin:0 0 4px;font-size:1.15rem;color:#111;">Verify Your Profession</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">We'll send a one-time code to your work email to verify your role.</p>
        <div style="text-align:left;margin-bottom:20px;">
          <p style="font-size:.85rem;font-weight:600;color:#374151;margin:0 0 8px;">Profession type</p>
          <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:8px;margin-bottom:16px;">
            ${[
              { val: "corporate", icon: "🏢", label: "Corporate" },
              { val: "freelance", icon: "💻", label: "Freelance" },
              { val: "regulated", icon: "⚕️",  label: "Regulated" },
            ].map(t => `
              <label style="border:2px solid #e5e7eb;border-radius:10px;padding:10px 6px;cursor:pointer;text-align:center;"
                onmouseover="this.style.borderColor='#7c3aed'" onmouseout="this.querySelector('input').checked||(this.style.borderColor='#e5e7eb')">
                <input type="radio" name="profType" value="${t.val}" class="prof-type-radio" ${profType===t.val?"checked":""} style="display:none;"
                  onchange="document.querySelectorAll('label[onmouseover]').forEach(l=>l.style.borderColor='#e5e7eb');this.closest('label').style.borderColor='#7c3aed';" />
                <div style="font-size:1.4rem;">${t.icon}</div>
                <p style="font-weight:600;font-size:.82rem;margin:4px 0 0;color:#111;">${t.label}</p>
              </label>`).join("")}
          </div>
          <div style="margin-bottom:12px;">
            <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:4px;">Job Title <span style="color:#9ca3af;font-weight:400;">(optional)</span></label>
            <input id="prof-title" type="text" placeholder="e.g. Senior Software Engineer" style="${inputStyle()}"
              onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'" />
          </div>
          <div style="margin-bottom:14px;">
            <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:4px;">Employer <span style="color:#9ca3af;font-weight:400;">(optional)</span></label>
            <input id="prof-employer" type="text" placeholder="e.g. Acme Corp" style="${inputStyle()}"
              onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'" />
          </div>
          <div>
            <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:4px;">Work Email</label>
            <input id="prof-email" type="email" placeholder="you@company.com" style="${inputStyle()}"
              onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'"
              onkeydown="if(event.key==='Enter') window.__profStartAndVerify()" />
          </div>
        </div>
        <div style="display:flex;gap:10px;justify-content:center;">
          <button onclick="window.__reset()" style="${btnOutline("#9ca3af")}">← Back</button>
          <button onclick="window.__profStartAndVerify()" style="${btn("#7c3aed")}">Send Code →</button>
        </div>
      `;
      setTimeout(() => {
        if (profType) {
          document.querySelectorAll(".prof-type-radio").forEach(r => {
            if (r.value === profType) { r.checked = true; r.closest("label").style.borderColor = "#7c3aed"; }
          });
        }
        document.getElementById("prof-email")?.focus();
      }, 0);
      break;

    case "prof_creating":
      cardContent.innerHTML = spinner("Setting up profession verification…");
      break;

    case "prof_email_otp": {
      const storedEmail = sessionStorage.getItem("prof_pending_email") || "";
      const domain = storedEmail.split("@")[1] || "your email";
      const devOTP = sessionStorage.getItem("prof_dev_otp") || "";
      cardContent.innerHTML = `
        <div style="font-size:2rem;margin-bottom:8px;">📬</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">Enter Verification Code</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 4px;">Check your inbox at <strong>*@${escapeHTML(domain)}</strong></p>
        <p style="color:#6b7280;font-size:.8rem;margin:0 0 20px;">Code expires in 10 minutes.</p>
        ${devOTP ? `<div style="background:#fef9c3;border:1px solid #fde047;border-radius:8px;padding:10px 14px;margin-bottom:16px;font-size:.82rem;">
          <strong>Dev mode</strong> — SMTP not configured.<br/>OTP: <strong style="font-size:1.1rem;letter-spacing:.1em;">${escapeHTML(devOTP)}</strong>
        </div>` : ""}
        <div style="text-align:left;margin-bottom:20px;">
          <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:6px;">6-digit code</label>
          <input id="prof-otp" type="text" inputmode="numeric" maxlength="6" placeholder="000000"
            style="${inputStyle()}font-size:1.2rem;letter-spacing:.2em;text-align:center;"
            onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'"
            onkeydown="if(event.key==='Enter') window.__profVerifyOTP()" />
        </div>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <button onclick="window.__profStart()" style="${btnOutline("#9ca3af")}">← Resend</button>
          <button onclick="window.__profVerifyOTP()" style="${btn("#7c3aed")}">Verify Code →</button>
        </div>
      `;
      setTimeout(() => document.getElementById("prof-otp")?.focus(), 50);
      break;
    }

    case "prof_email_verifying":
      cardContent.innerHTML = spinner("Verifying code…");
      break;

    case "prof_methods": {
      const methods = [
        { id: "document_upload",  icon: "📄", label: "License / Certificate", pts: profType === "regulated" ? "45 pts" : "35 pts", desc: "Professional license, degree, or certificate" },
        { id: "portfolio_social", icon: "🔗", label: "Portfolio / Profile",   pts: "20 pts", desc: "GitHub, Dribbble, portfolio URL" },
        { id: "linkedin_oauth",   icon: "💼", label: "LinkedIn",              pts: "40 pts", desc: "Connect your LinkedIn account" },
      ];
      const verifiedIds = new Set(profEvidence.filter(e => e.status === "verified").map(e => e.method));
      const pendingIds  = new Set(profEvidence.filter(e => e.status === "pending").map(e => e.method));
      cardContent.innerHTML = `
        <div style="margin-bottom:20px;">
          ${scoreBar(profScore, profLevel)}
          ${profScore > 0 ? `<div style="margin-top:8px;">${levelBadge(profLevel)}</div>` : ""}
        </div>
        <p style="text-align:left;font-size:.8rem;font-weight:600;color:#6b7280;text-transform:uppercase;letter-spacing:.04em;margin:0 0 10px;">Add more evidence</p>
        <div style="display:flex;flex-direction:column;gap:8px;margin-bottom:20px;">
          ${methods.map(m => {
            const ok = verifiedIds.has(m.id);
            return `<button onclick="window.__profSelectMethod('${m.id}')" ${ok ? "disabled" : ""}
              style="display:flex;align-items:center;gap:12px;padding:12px 14px;
                border:1px solid ${ok?"#bbf7d0":"#e5e7eb"};border-radius:10px;
                background:${ok?"#f0fdf4":"#fff"};cursor:${ok?"default":"pointer"};
                text-align:left;width:100%;"
              onmouseover="if(!${ok}) this.style.borderColor='#7c3aed'"
              onmouseout="this.style.borderColor='${ok?"#bbf7d0":"#e5e7eb"}'">
              <span style="font-size:1.4rem;flex-shrink:0;">${m.icon}</span>
              <span style="flex:1;"><span style="display:block;font-weight:600;font-size:.9rem;color:#111;">${m.label}</span>
              <span style="display:block;font-size:.75rem;color:#6b7280;margin-top:2px;">${m.desc}</span></span>
              <span style="font-size:.8rem;font-weight:600;color:${ok?"#16a34a":"#7c3aed"};flex-shrink:0;">
                ${ok ? "✅ done" : pendingIds.has(m.id) ? "⏳ pending" : m.pts}
              </span>
            </button>`;
          }).join("")}
        </div>
        <div style="display:flex;gap:10px;justify-content:center;">
          <button onclick="window.__profFinish()" style="${btn("#16a34a")}">Done →</button>
        </div>
      `;
      break;
    }

    case "prof_doc_upload":
      cardContent.innerHTML = `
        <div style="font-size:2rem;margin-bottom:8px;">📄</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">Upload License or Certificate</h2>
        <p style="color:#9ca3af;font-size:.78rem;margin:0 0 16px;">Document bytes are never stored — only metadata is saved.</p>
        <div style="text-align:left;margin-bottom:12px;">
          <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:6px;">Document type</label>
          <select id="prof-doc-type" style="${inputStyle()}background:#fff;cursor:pointer;"
            onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'">
            <option value="professional_document">General professional document</option>
            <option value="medical_license">Medical license (doctor)</option>
            <option value="bar_certificate">Bar certificate (lawyer)</option>
            <option value="ca_certificate">CA / CPA certificate (accountant)</option>
            <option value="engineering_license">Engineering license</option>
            <option value="degree_certificate">Degree / diploma</option>
            <option value="work_permit">Work permit / employment certificate</option>
          </select>
        </div>
        <label id="prof-drop-zone" style="display:block;border:2px dashed #d1d5db;border-radius:12px;padding:20px;cursor:pointer;margin-bottom:14px;">
          ${profPreviewURL
            ? `<img src="${profPreviewURL}" style="max-height:150px;max-width:100%;border-radius:8px;display:block;margin:0 auto;" /><p style="color:#6b7280;font-size:.8rem;margin:8px 0 0;text-align:center;">Click to change</p>`
            : `<div style="text-align:center;"><div style="color:#9ca3af;font-size:2rem;margin-bottom:6px;">📋</div><p style="color:#6b7280;margin:0;font-size:.9rem;">Click or drag to upload</p><p style="color:#9ca3af;margin:4px 0 0;font-size:.78rem;">JPEG, PNG, or PDF, max 10 MB</p></div>`}
          <input id="prof-file-input" type="file" accept="image/jpeg,image/png,application/pdf" style="display:none;" />
        </label>
        <div style="display:flex;gap:10px;justify-content:center;">
          <button onclick="window.__profBackToMethods()" style="${btnOutline("#9ca3af")}">← Back</button>
          <button onclick="window.__profSubmitDoc()" ${!profSelectedFile ? "disabled" : ""}
            style="${btn(profSelectedFile ? "#7c3aed" : "#d1d5db")}${!profSelectedFile ? "cursor:not-allowed;" : ""}">
            Submit Document
          </button>
        </div>
      `;
      setTimeout(() => {
        const dz = document.getElementById("prof-drop-zone");
        const fi = document.getElementById("prof-file-input");
        dz?.addEventListener("dragover", e => { e.preventDefault(); dz.style.borderColor = "#7c3aed"; });
        dz?.addEventListener("dragleave", () => { dz.style.borderColor = "#d1d5db"; });
        dz?.addEventListener("drop", e => { e.preventDefault(); const f = e.dataTransfer?.files?.[0]; if (f) handleProfFile(f); });
        fi?.addEventListener("change", e => { const f = e.target.files?.[0]; if (f) handleProfFile(f); });
      }, 0);
      break;

    case "prof_doc_processing":
      cardContent.innerHTML = spinner("Processing document…");
      break;

    case "prof_portfolio_input":
      cardContent.innerHTML = `
        <div style="font-size:2rem;margin-bottom:8px;">🔗</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">Portfolio / Profile URL</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">
          Add a publicly visible profile. Earns <strong>20 pts</strong>. Supported: GitHub, GitLab, Dribbble, Behance, Medium, Dev.to.
        </p>
        <div style="text-align:left;margin-bottom:20px;">
          <label style="display:block;font-size:.85rem;font-weight:500;color:#374151;margin-bottom:6px;">URL</label>
          <input id="prof-portfolio-url" type="url" placeholder="https://github.com/you" style="${inputStyle()}"
            onfocus="this.style.borderColor='#7c3aed'" onblur="this.style.borderColor='#d1d5db'"
            oninput="window.__profDetectPlatform(this.value)"
            onkeydown="if(event.key==='Enter') window.__profSubmitPortfolio()" />
          <p id="prof-platform-hint" style="color:#7c3aed;font-size:.78rem;margin:4px 0 0;"></p>
        </div>
        <div style="display:flex;gap:10px;justify-content:center;">
          <button onclick="window.__profBackToMethods()" style="${btnOutline("#9ca3af")}">← Back</button>
          <button onclick="window.__profSubmitPortfolio()" style="${btn("#7c3aed")}">Submit →</button>
        </div>
      `;
      setTimeout(() => document.getElementById("prof-portfolio-url")?.focus(), 50);
      break;

    case "prof_portfolio_saving":
      cardContent.innerHTML = spinner("Saving portfolio link…");
      break;

    case "prof_linkedin_redirect": {
      const linkedInURL = sessionStorage.getItem("prof_linkedin_url") || "";
      cardContent.innerHTML = `
        <div style="font-size:2rem;margin-bottom:8px;">💼</div>
        <h2 style="margin:0 0 4px;font-size:1.1rem;color:#111;">LinkedIn Verification</h2>
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">Confirm your professional identity via LinkedIn OAuth. Earns <strong>40 pts</strong>.</p>
        ${linkedInURL ? `
        <a href="${escapeHTML(linkedInURL)}" target="_blank"
          style="${btn("#0077b5","text-decoration:none;display:inline-block;margin-bottom:16px;")}">
          Authorize with LinkedIn →
        </a>
        <p style="color:#9ca3af;font-size:.78rem;margin:0 0 16px;">Opens in a new tab. After authorizing, paste the code below.</p>
        <div style="text-align:left;margin-bottom:16px;">
          <label style="display:block;font-size:.82rem;font-weight:500;color:#374151;margin-bottom:6px;">Paste auth code from the redirect URL:</label>
          <input id="prof-li-code" type="text" placeholder="Paste code here…" style="${inputStyle()}"
            onfocus="this.style.borderColor='#0077b5'" onblur="this.style.borderColor='#d1d5db'" />
        </div>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <button onclick="window.__profBackToMethods()" style="${btnOutline("#9ca3af")}">← Back</button>
          <button onclick="window.__profLinkedInCallback()" style="${btn("#0077b5")}">Confirm →</button>
        </div>` : `
        <p style="color:#dc2626;font-size:.85rem;">LinkedIn OAuth is not configured on this server.<br/>Set <code>LINKEDIN_CLIENT_ID</code> to enable it.</p>
        <button onclick="window.__profBackToMethods()" style="${btnOutline("#9ca3af")} margin-top:16px;">← Back</button>`}
      `;
      break;
    }

    case "prof_done": {
      const verified = profEvidence.filter(e => e.status === "verified");
      const multiBonus = verified.length >= 2 ? 10 : 0;
      const levelEmoji = { none: "⬜", low: "🟡", medium: "🔵", high: "🟢" };
      const levelMsg   = { none: "No evidence was verified.", low: "Low confidence — consider adding more evidence.", medium: "Medium confidence — profession likely verified.", high: "High confidence — profession strongly verified." };
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:8px;">${levelEmoji[profLevel]||"⬜"}</div>
        <h2 style="margin:0 0 4px;font-size:1.2rem;color:#111;">Profession Verification Complete</h2>
        <div style="margin-bottom:16px;">${levelBadge(profLevel)}</div>
        ${scoreBar(profScore, profLevel)}
        ${verified.length > 0 ? `<div style="background:#f9fafb;border:1px solid #e5e7eb;border-radius:10px;padding:16px;margin:16px 0;text-align:left;">
          <p style="font-weight:600;font-size:.85rem;color:#374151;margin:0 0 10px;">Verified evidence</p>
          ${verified.map(e => {
            const icons  = { work_email:"✉️", document_upload:"📄", portfolio_social:"🔗", linkedin_oauth:"💼" };
            const labels = { work_email:"Work Email", document_upload:"Document", portfolio_social:"Portfolio", linkedin_oauth:"LinkedIn" };
            const meta = e.metadata || {};
            const detail = meta.email_domain || meta.platform || meta.document_type || meta.headline || "";
            return `<div style="display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid #f3f4f6;">
              <span>${icons[e.method]||"📋"}</span>
              <span style="flex:1;font-size:.85rem;color:#374151;">${labels[e.method]||e.method}${detail?`<span style="color:#9ca3af;"> · ${escapeHTML(detail)}</span>`:""}</span>
              <span style="color:#16a34a;font-weight:600;font-size:.85rem;">+${e.scoreContribution} pts</span>
            </div>`;
          }).join("")}
          ${multiBonus > 0 ? `<div style="display:flex;align-items:center;gap:8px;padding:6px 0;">
            <span>🎯</span><span style="flex:1;font-size:.85rem;color:#374151;">Multi-method bonus</span>
            <span style="color:#16a34a;font-weight:600;font-size:.85rem;">+${multiBonus} pts</span>
          </div>` : ""}
        </div>` : ""}
        <p style="color:#6b7280;font-size:.85rem;margin:0 0 20px;">${levelMsg[profLevel]}</p>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap;">
          <button onclick="window.__profBackToMethods()" style="${btnOutline("#7c3aed")}">Add More Evidence</button>
          <button onclick="window.__reset()" style="${btn("#6b7280")}">Done</button>
        </div>
      `;
      break;
    }

    case "prof_error":
      cardContent.innerHTML = `
        <div style="font-size:3rem;margin-bottom:12px;">⚠️</div>
        <h2 style="margin:0 0 8px;font-size:1.1rem;color:#dc2626;">Something went wrong</h2>
        <pre style="background:#fef2f2;color:#7f1d1d;padding:12px;border-radius:8px;text-align:left;font-size:.8rem;white-space:pre-wrap;overflow:auto;">${escapeHTML(profError)}</pre>
        <div style="display:flex;gap:10px;justify-content:center;margin-top:16px;">
          <button onclick="window.__profBackToMethods()" style="${btnOutline("#7c3aed")}">← Back</button>
          <button onclick="window.__reset()" style="${btn("#6b7280")}">Start Over</button>
        </div>
      `;
      break;
  }
}

// ── Identity API calls ─────────────────────────────────────────────────────

async function apiCreateSession() {
  const res = await fetch("/api/sessions", { method: "POST", headers: jsonHeaders() });
  if (!res.ok) throw new Error(`Create session failed (${res.status}): ${await res.text()}`);
  return (await res.json()).data;
}
async function apiGetLivenessResult(sid) {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sid)}/result`, { headers: authHeaders() });
  if (!res.ok) throw new Error(`Get result failed (${res.status}): ${await res.text()}`);
  return (await res.json()).data;
}
async function apiStoreConsent(sid, fields) {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sid)}/consent`, {
    method: "POST", headers: jsonHeaders(), body: JSON.stringify({ fields }),
  });
  if (res.status === 409) { const b = await res.json().catch(()=>({})); throw new Error(b.message || "Duplicate identity."); }
  if (!res.ok) throw new Error(`Consent failed (${res.status}): ${await res.text()}`);
  return (await res.json()).data;
}
async function apiUploadDocument(sid, file) {
  const fd = new FormData(); fd.append("sessionId", sid); fd.append("file", file);
  const res = await fetch("/api/documents", { method: "POST", headers: authHeaders(), body: fd });
  if (res.status === 409) { const b = await res.json().catch(()=>({})); throw Object.assign(new Error(b.message||"Duplicate identity."), { duplicate: true }); }
  if (!res.ok) throw new Error(`Upload failed (${res.status}): ${await res.text()}`);
  return (await res.json()).data;
}

// ── Profession API calls ───────────────────────────────────────────────────

const profAPI = (path, opts = {}) =>
  fetch(path, { headers: opts.body instanceof FormData ? authHeaders() : jsonHeaders(), ...opts })
    .then(async r => { if (!r.ok) throw new Error(`${opts.method||"GET"} ${path} failed (${r.status}): ${await r.text()}`); return (await r.json()).data; });

const apiProfStart           = (t,ti,e,c)       => profAPI("/api/professions", { method:"POST", body: JSON.stringify({professionType:t,professionTitle:ti,employerName:e,consentStoreData:c}) });
const apiProfSubmitEmail     = (vid,email)       => profAPI(`/api/profession/${vid}/submit/email`, { method:"POST", body: JSON.stringify({workEmail:email}) });
const apiProfConfirmEmail    = (vid,eid,otp)     => profAPI(`/api/profession/${vid}/confirm/email`, { method:"POST", body: JSON.stringify({evidenceId:eid,otp}) });
const apiProfSubmitPortfolio = (vid,url,platform)=> profAPI(`/api/profession/${vid}/submit/portfolio`, { method:"POST", body: JSON.stringify({url,platform}) });
const apiProfGetStatus       = (vid)             => profAPI(`/api/profession/${vid}/status`);
const apiProfGetLinkedInURL  = (vid)             => profAPI(`/api/profession/${vid}/linkedin/auth-url`);
const apiProfLinkedInCb      = (vid,code,state)  => profAPI(`/api/profession/${vid}/linkedin/callback`, { method:"POST", body: JSON.stringify({authCode:code,state}) });
async function apiProfSubmitDocument(vid, file, docType) {
  const fd = new FormData(); fd.append("file", file); fd.append("documentType", docType);
  return profAPI(`/api/profession/${vid}/submit/document`, { method:"POST", body: fd });
}

// ── Identity flow ──────────────────────────────────────────────────────────

function handleFile(f) { if (previewURL) URL.revokeObjectURL(previewURL); selectedFile = f; previewURL = URL.createObjectURL(f); render(); }

async function start() {
  appState = "creating"; render();
  try {
    const s = await apiCreateSession();
    sessionId = s.sessionId; providerSessionId = s.providerSessionId; provider = s.provider; authToken = s.authToken;
  } catch (e) { appError = e.message; appState = "error"; render(); return; }

  appState = "capturing"; render();
  reactRoot = createRoot(sdkContainer);
  const onComplete = async () => {
    reactRoot.unmount(); reactRoot = null; appState = "polling"; render();
    try {
      livenessResult = await apiGetLivenessResult(sessionId);
      const img = livenessResult.livenessImage || livenessResult.referenceImage;
      if (img) livenessImageURL = img;
      if (!livenessResult.passed) {
        appError = "Liveness check failed — please ensure good lighting and follow the on-screen instructions, then try again.";
        appState = "error"; render(); return;
      }
      appState = "liveness_done"; render();
    } catch (e) { appError = e.message; appState = "error"; render(); }
  };
  const onCancel = () => { reactRoot.unmount(); reactRoot = null; appState = "idle"; render(); };
  const onError  = (e) => {
    reactRoot.unmount(); reactRoot = null;
    const raw = e?.message || e?.error?.Message || e?.error?.message || JSON.stringify(e, null, 2);
    const azureMessages = {
      ExcessiveImageBlurDetected:   "Camera image is too blurry. Move closer to the camera, ensure good lighting, and try again.",
      FaceNotDetected:              "No face detected. Make sure your face is fully visible and well-lit.",
      MultipleFacesDetected:        "Multiple faces detected. Please ensure only you are in the frame.",
      LivenessFailed:               "Liveness check failed. Follow the on-screen instructions carefully and try again.",
      TimedOut:                     "Liveness session timed out. Please try again.",
    };
    let friendlyMsg = raw;
    try {
      const inner = JSON.parse(raw.replace(/^Azure liveness error:\s*/, ""));
      const code = inner?.livenessError || inner?.code;
      if (code && azureMessages[code]) friendlyMsg = azureMessages[code];
    } catch (_) {}
    appError = friendlyMsg;
    appState = "error"; render();
  };
  if (provider === "azure") reactRoot.render(React.createElement(AzureLivenessCapture, { authToken, onComplete, onCancel, onError }));
  else                      reactRoot.render(React.createElement(LivenessCapture,       { providerSessionId, onComplete, onCancel, onError }));
}

async function submitDoc() {
  if (!selectedFile) return;
  appState = "processing_doc"; render();
  try {
    verifiedResult = await apiUploadDocument(sessionId, selectedFile);
    const doc = verifiedResult?.document || {};
    const hasDocFields = ["firstName","lastName","dob","idNumber","expiry","issuingCountry"].some(k => doc[k]);
    appState = (verifiedResult?.faceMatch?.passed && hasDocFields) ? "consenting" : "verified"; render();
  } catch (e) {
    if (e.duplicate) { appState = "duplicate"; render(); }
    else             { appError = e.message; appState = "error"; render(); }
  }
}

async function submitConsent() {
  const checked = [...document.querySelectorAll(".consent-field:checked")].map(el => el.value);
  if (checked.length === 0) { appState = "verified"; render(); return; }
  appState = "storing_consent"; render();
  try { await apiStoreConsent(sessionId, checked); consentStored = true; appState = "verified"; render(); }
  catch (e) { appError = e.message; appState = "error"; render(); }
}

function reset() {
  sessionId = null; providerSessionId = null; provider = null; authToken = null;
  livenessResult = null; livenessImageURL = null; verifiedResult = null;
  selectedFile = null; consentStored = false;
  if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = null; }
  appError = null;
  profVerificationId = null; profType = null; profScore = 0; profLevel = "none"; profEvidence = [];
  profEmailEvidenceId = null; profSelectedFile = null;
  if (profPreviewURL) { URL.revokeObjectURL(profPreviewURL); profPreviewURL = null; }
  profError = null;
  sessionStorage.removeItem("prof_pending_email");
  sessionStorage.removeItem("prof_dev_otp");
  sessionStorage.removeItem("prof_linkedin_url");
  appState = "idle"; render();
}

// ── Profession flow ────────────────────────────────────────────────────────

function handleProfFile(f) { if (profPreviewURL) URL.revokeObjectURL(profPreviewURL); profSelectedFile = f; profPreviewURL = URL.createObjectURL(f); render(); }

async function profStartAndVerify() {
  const typeInput = document.querySelector(".prof-type-radio:checked");
  if (!typeInput) { alert("Please select a profession type."); return; }
  const email = document.getElementById("prof-email")?.value?.trim();
  if (!email) { alert("Please enter your work email."); return; }
  profType = typeInput.value;
  const title    = document.getElementById("prof-title")?.value?.trim() || "";
  const employer = document.getElementById("prof-employer")?.value?.trim() || "";
  sessionStorage.setItem("prof_pending_email", email);
  appState = "prof_creating"; render();
  try {
    const r = await apiProfStart(profType, title, employer, false);
    profVerificationId = r.verificationId; profScore = r.confidenceScore || 0; profLevel = r.confidenceLevel || "none"; profEvidence = [];
    const emailRes = await apiProfSubmitEmail(profVerificationId, email);
    profEmailEvidenceId = emailRes.evidenceId;
    if (emailRes.dev_otp) sessionStorage.setItem("prof_dev_otp", emailRes.dev_otp);
    else sessionStorage.removeItem("prof_dev_otp");
    appState = "prof_email_otp"; render();
  } catch (e) { profError = e.message; appState = "prof_error"; render(); }
}

async function profVerifyOTP() {
  const otp = document.getElementById("prof-otp")?.value?.trim();
  if (!otp || otp.length !== 6) { alert("Please enter the 6-digit code."); return; }
  appState = "prof_email_verifying"; render();
  try {
    const r = await apiProfConfirmEmail(profVerificationId, profEmailEvidenceId, otp);
    profScore = r.confidenceScore; profLevel = r.confidenceLevel;
    sessionStorage.removeItem("prof_dev_otp"); sessionStorage.removeItem("prof_pending_email");
    await refreshProfEvidence(); appState = "prof_done"; render();
  } catch (e) { profError = e.message; appState = "prof_error"; render(); }
}

async function profSubmitDoc() {
  if (!profSelectedFile) { alert("Please select a file."); return; }
  const docType = document.getElementById("prof-doc-type")?.value || "professional_document";
  appState = "prof_doc_processing"; render();
  try {
    const r = await apiProfSubmitDocument(profVerificationId, profSelectedFile, docType);
    profScore = r.confidenceScore; profLevel = r.confidenceLevel;
    profSelectedFile = null; if (profPreviewURL) { URL.revokeObjectURL(profPreviewURL); profPreviewURL = null; }
    await refreshProfEvidence(); appState = "prof_methods"; render();
  } catch (e) { profError = e.message; appState = "prof_error"; render(); }
}

async function profSubmitPortfolio() {
  const url = document.getElementById("prof-portfolio-url")?.value?.trim();
  if (!url) { alert("Please enter a URL."); return; }
  appState = "prof_portfolio_saving"; render();
  try {
    const r = await apiProfSubmitPortfolio(profVerificationId, url, "");
    profScore = r.confidenceScore; profLevel = r.confidenceLevel;
    await refreshProfEvidence(); appState = "prof_methods"; render();
  } catch (e) { profError = e.message; appState = "prof_error"; render(); }
}

async function profSelectLinkedIn() {
  appState = "prof_linkedin_redirect"; render();
  try {
    const r = await apiProfGetLinkedInURL(profVerificationId);
    sessionStorage.setItem("prof_linkedin_url", r.authUrl || "");
  } catch (_) { sessionStorage.setItem("prof_linkedin_url", ""); }
  render();
}

async function profLinkedInCallback() {
  const code = document.getElementById("prof-li-code")?.value?.trim();
  if (!code) { alert("Please paste the auth code."); return; }
  appState = "prof_creating"; render();
  try {
    const r = await apiProfLinkedInCb(profVerificationId, code, "");
    profScore = r.confidenceScore; profLevel = r.confidenceLevel;
    await refreshProfEvidence(); appState = "prof_methods"; render();
  } catch (e) { profError = e.message; appState = "prof_error"; render(); }
}

async function refreshProfEvidence() {
  try {
    const s = await apiProfGetStatus(profVerificationId);
    profEvidence = s.evidence || []; profScore = s.confidenceScore ?? profScore; profLevel = s.confidenceLevel ?? profLevel;
  } catch (_) {}
}

async function profFinish() {
  await refreshProfEvidence(); appState = "prof_done"; render();
}

// ── Window exports ─────────────────────────────────────────────────────────

window.__start              = start;
window.__goUpload           = () => { appState = "uploading"; render(); };
window.__back               = () => { appState = "liveness_done"; render(); };
window.__submitDoc          = submitDoc;
window.__reset              = reset;
window.__retryDoc           = () => { verifiedResult = null; selectedFile = null; if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = null; } appError = null; appState = "uploading"; render(); };
window.__submitConsent      = submitConsent;
window.__skipConsent        = () => { appState = "verified"; render(); };
window.__goConsent          = () => { appState = "consenting"; render(); };
window.__profStart          = () => { appState = "prof_start"; render(); };
window.__profStartAndVerify = profStartAndVerify;
window.__profBackToMethods  = () => { appState = "prof_methods"; render(); };
window.__profSelectMethod   = (m) => {
  if      (m === "document_upload")  { appState = "prof_doc_upload"; render(); }
  else if (m === "portfolio_social") { appState = "prof_portfolio_input"; render(); }
  else if (m === "linkedin_oauth")   { profSelectLinkedIn(); }
};
window.__profVerifyOTP         = profVerifyOTP;
window.__profSubmitDoc         = profSubmitDoc;
window.__profSubmitPortfolio   = profSubmitPortfolio;
window.__profLinkedInCallback  = profLinkedInCallback;
window.__profFinish            = profFinish;
window.__profDetectPlatform    = (url) => {
  const hint = document.getElementById("prof-platform-hint");
  if (!hint) return;
  const platforms = { "github.com":"GitHub","gitlab.com":"GitLab","dribbble.com":"Dribbble","behance.net":"Behance","medium.com":"Medium","dev.to":"Dev.to","hashnode.com":"Hashnode","linkedin.com":"LinkedIn","stackoverflow":"Stack Overflow" };
  const found = Object.entries(platforms).find(([h]) => url.toLowerCase().includes(h));
  hint.textContent = found ? `Detected: ${found[1]}` : (url.startsWith("http") ? "Platform: Web" : "");
};

// ── LinkedIn callback on redirect back ─────────────────────────────────────
(function checkLinkedInCallback() {
  const params = new URLSearchParams(window.location.search);
  const code   = params.get("code");
  const state  = params.get("state");
  if (!code || !state) return;
  window.history.replaceState({}, "", window.location.pathname);
  profVerificationId = state.split(":")[0];
  appState = "prof_creating";
  apiProfLinkedInCb(profVerificationId, code, state)
    .then(async r => { profScore = r.confidenceScore; profLevel = r.confidenceLevel; await refreshProfEvidence(); appState = "prof_methods"; render(); })
    .catch(e => { profError = e.message; appState = "prof_error"; render(); });
})();

render();
