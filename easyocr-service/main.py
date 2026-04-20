"""
EasyOCR microservice — extracts structured identity fields from document images.
POST /analyze   multipart: file=<image>
Returns DocumentData JSON compatible with the Go provider interface.
"""

import re
import logging
from typing import Optional

import easyocr
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("easyocr-service")

app = FastAPI(title="EasyOCR Document Service")

# Initialise once at startup (downloads model on first run)
reader: Optional[easyocr.Reader] = None


@app.on_event("startup")
def _init_reader():
    global reader
    log.info("Loading EasyOCR model (english)…")
    reader = easyocr.Reader(["en"], gpu=False)
    log.info("EasyOCR model ready")


# ── MRZ helpers ───────────────────────────────────────────────────────────────

MRZ_TD3_RE = re.compile(r'^[A-Z0-9<]{44}$')  # passport (2 lines x 44 chars)
MRZ_TD1_RE = re.compile(r'^[A-Z0-9<]{30}$')  # ID card  (3 lines x 30 chars)


def _digits_only(s: str) -> str:
    """Fix common OCR confusion in digit-only MRZ fields."""
    return s.replace("O", "0").replace("I", "1").replace("B", "8").replace("L", "1")


def _format_yymmdd(raw: str) -> str:
    """Convert YYMMDD → YYYY-MM-DD. Century pivot: >30 → 19xx, else 20xx."""
    raw = _digits_only(raw)
    if len(raw) != 6 or not raw.isdigit():
        return raw
    yy, mm, dd = raw[:2], raw[2:4], raw[4:]
    year = ("19" if int(yy) > 30 else "20") + yy
    return f"{year}-{mm}-{dd}"


def _parse_mrz_td3(lines: list) -> dict:
    """Parse TD3 (passport) MRZ — 2 lines of 44 chars."""
    L1, L2 = lines[0], lines[1]
    issuing    = L1[2:5].replace("<", "")
    name_parts = L1[5:].split("<<")
    last_name  = name_parts[0].replace("<", " ").strip() if len(name_parts) > 0 else ""
    first_name = name_parts[1].replace("<", " ").strip() if len(name_parts) > 1 else ""
    return {
        "firstName":      first_name,
        "lastName":       last_name,
        "dob":            _format_yymmdd(L2[13:19]),
        "idNumber":       L2[0:9].replace("<", ""),
        "expiry":         _format_yymmdd(L2[20:26]),
        "issuingCountry": issuing,
        "address":        "",
        "documentType":   "PASSPORT" if L1[0] == "P" else "TRAVEL_DOCUMENT",
    }


def _parse_mrz_td1(lines: list) -> dict:
    """Parse TD1 (ID card) MRZ — 3 lines of 30 chars."""
    L1, L2, L3 = lines[0], lines[1], lines[2]
    name_parts = L3.split("<<")
    last_name  = name_parts[0].replace("<", " ").strip() if len(name_parts) > 0 else ""
    first_name = name_parts[1].replace("<", " ").strip() if len(name_parts) > 1 else ""
    return {
        "firstName":      first_name,
        "lastName":       last_name,
        "dob":            _format_yymmdd(L2[0:6]),
        "idNumber":       L1[5:14].replace("<", ""),
        "expiry":         _format_yymmdd(L2[8:14]),
        "issuingCountry": L1[2:5].replace("<", ""),
        "address":        "",
        "documentType":   "ID_CARD",
    }


def _find_mrz(lines: list):
    """Return MRZ line list if found, else None."""
    cleaned = [l.upper().replace(" ", "") for l in lines]
    for i in range(len(cleaned) - 1):
        if MRZ_TD3_RE.match(cleaned[i]) and MRZ_TD3_RE.match(cleaned[i + 1]):
            return cleaned[i:i + 2]
    for i in range(len(cleaned) - 2):
        if all(MRZ_TD1_RE.match(cleaned[i + j]) for j in range(3)):
            return cleaned[i:i + 3]
    return None


# ── Spatial key-value extraction ──────────────────────────────────────────────

FIELD_PATTERNS = {
    "firstName":      [r"elector.?s\s*name", r"^name$", r"first\s*name", r"given\s*name", r"forename"],
    "lastName":       [r"father.?s\s*name", r"husband.?s\s*name", r"s[./\\-]?[io][./\\-]?w",
                       r"s[./\\-]?o\b", r"d[./\\-]?o\b", r"last\s*name", r"surname", r"family\s*name"],
    "dob":            [r"^dob$", r"date\s+of\s+birth", r"d\.?o\.?b", r"birth\s*date"],
    "idNumber":       [r"^number$", r"licence\s*(no|number|#)", r"license\s*(no|number|#)",
                       r"id\s*(no|number|#)", r"document\s*(no|number|#)", r"dl\s*(no|number)"],
    "expiry":         [r"expir(y|ation|es)", r"exp\.?\s*date", r"valid\s*(until|thru)"],
    "issuingCountry": [r"country", r"issued\s*by", r"issuing"],
    "address":        [r"^address$", r"\baddr\.?\b", r"residence"],
    "documentType":   [r"\btype\b", r"\bclass\b", r"category"],
}

VOTER_ID_RE = re.compile(r'^[A-Z]{3}\d{7}$')


def _kv_extract(raw_results: list) -> dict:
    """
    Spatial label-value pairing using bounding boxes.
    Labels sit in the left column, values to their right on the same Y row.
    Each item in raw_results is (bbox, text, conf).
    bbox = [[x1,y1],[x2,y1],[x2,y2],[x1,y2]]
    """
    result = {k: "" for k in FIELD_PATTERNS}

    blocks = []
    for bbox, text, conf in raw_results:
        x_left  = min(p[0] for p in bbox)
        x_right = max(p[0] for p in bbox)
        y_top   = min(p[1] for p in bbox)
        y_bot   = max(p[1] for p in bbox)
        blocks.append((x_left, y_top, y_bot, x_right, text.strip()))

    for label_block in blocks:
        lx, ly_top, ly_bot, lx_right, label_text = label_block
        label_lower = label_text.lower()

        for field, patterns in FIELD_PATTERNS.items():
            if result[field]:
                continue
            for pat in patterns:
                if not re.search(pat, label_lower):
                    continue

                label_mid_y = (ly_top + ly_bot) / 2
                candidates = []
                for vx, vy_top, vy_bot, vx_right, vtext in blocks:
                    if vx <= lx_right:
                        continue
                    val_mid_y = (vy_top + vy_bot) / 2
                    y_overlap = min(ly_bot, vy_bot) - max(ly_top, vy_top)
                    row_height = ly_bot - ly_top
                    if y_overlap > 0 or abs(val_mid_y - label_mid_y) < row_height:
                        candidates.append((vx, vtext))

                if candidates:
                    candidates.sort(key=lambda c: c[0])
                    value = candidates[0][1]

                    if field == "address":
                        extra = []
                        row_h = ly_bot - ly_top
                        for vx, vy_top, vy_bot, vx_right, vtext in blocks:
                            if vx <= lx_right:
                                continue
                            if vy_top > ly_bot and vy_top < ly_bot + row_h * 2.5:
                                if not any(re.search(p, vtext.lower())
                                           for pats in FIELD_PATTERNS.values()
                                           for p in pats):
                                    extra.append((vy_top, vtext))
                        extra.sort()
                        value = " ".join([value] + [t for _, t in extra])

                    result[field] = value
                break

    # Fallback: scan standalone blocks for known patterns
    for _, _, _, _, text in blocks:
        if not result["idNumber"] and VOTER_ID_RE.match(text.strip()):
            result["idNumber"] = text.strip()
        if not result["issuingCountry"]:
            tl = text.lower()
            if "election commission" in tl or "transport" in tl or "passport" in tl:
                result["issuingCountry"] = text.strip()
        if not result["documentType"]:
            tl = text.lower()
            if "voter" in tl or "elector" in tl or "driving" in tl or "passport" in tl:
                result["documentType"] = text.strip()

    return result


# ── endpoint ──────────────────────────────────────────────────────────────────

@app.post("/analyze")
async def analyze(file: UploadFile = File(...)):
    if reader is None:
        raise HTTPException(status_code=503, detail="OCR model not ready")

    data = await file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty file")

    # Sort top-to-bottom by y-coordinate before processing
    raw_results = reader.readtext(data, detail=1, paragraph=False)
    raw_results.sort(key=lambda x: x[0][0][1])

    log.info("EasyOCR returned %d text regions", len(raw_results))
    for bbox, text, conf in raw_results:
        log.info("  [%.2f] %s", conf, text)

    raw_entries = [{"text": t, "confidence": round(c, 4)} for _, t, c in raw_results]
    texts = [t for _, t, _ in raw_results]

    # MRZ path (most accurate for passports/ID cards with machine-readable zone)
    mrz = _find_mrz(texts)
    if mrz:
        log.info("MRZ detected (%d lines)", len(mrz))
        fields = _parse_mrz_td3(mrz) if len(mrz) == 2 else _parse_mrz_td1(mrz)
    else:
        log.info("No MRZ — using spatial key-value extraction")
        fields = _kv_extract(raw_results)

    log.info("Parsed fields: %s", fields)

    return JSONResponse({
        "firstName":      fields.get("firstName", ""),
        "lastName":       fields.get("lastName", ""),
        "dob":            fields.get("dob", ""),
        "idNumber":       fields.get("idNumber", ""),
        "expiry":         fields.get("expiry", ""),
        "issuingCountry": fields.get("issuingCountry", ""),
        "address":        fields.get("address", ""),
        "documentType":   fields.get("documentType", ""),
        "rawText":        texts,
        "rawOCR":         raw_entries,
    })


@app.get("/health")
def health():
    return {"status": "ok", "model_ready": reader is not None}
