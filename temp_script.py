"""
EasyOCR test script — run on a sample ID document image.

Usage:
    python test.py <path-to-image>

Example:
    python test.py passport.jpg
    python test.py drivers_license.png
"""

import sys
import re
import easyocr

# ── OCR setup ─────────────────────────────────────────────────────────────────
# Downloads ~100MB of models on first run, cached in ~/.EasyOCR/

reader = easyocr.Reader(['en'], gpu=False, verbose=False)

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


# ── Key-value heuristic fallback ──────────────────────────────────────────────

FIELD_PATTERNS = {
    "firstName":      [r"elector.?s\s*name", r"^name$", r"first\s*name", r"given\s*name", r"forename"],
    "lastName":       [r"father.?s\s*name", r"husband.?s\s*name", r"s[./\\-]?[io][./\\-]?w", r"s[./\\-]?o\b", r"d[./\\-]?o\b", r"last\s*name", r"surname", r"family\s*name"],
    "dob":            [r"^dob$", r"date\s+of\s+birth", r"d\.?o\.?b", r"birth\s*date"],
    "idNumber":       [r"^number$", r"licence\s*(no|number|#)", r"license\s*(no|number|#)",
                       r"id\s*(no|number|#)", r"document\s*(no|number|#)", r"dl\s*(no|number)"],
    "expiry":         [r"expir(y|ation|es)", r"exp\.?\s*date", r"valid\s*(until|thru)"],
    "issuingCountry": [r"country", r"issued\s*by", r"issuing"],
    "address":        [r"^address$", r"\baddr\.?\b", r"residence"],
    "documentType":   [r"\btype\b", r"\bclass\b", r"category"],
}

# Regex to detect standalone voter/document ID numbers when no label is nearby
VOTER_ID_RE = re.compile(r'^[A-Z]{3}\d{7}$')


def _kv_extract(raw_results: list) -> dict:
    """
    Spatial label-value pairing using bounding boxes.
    Labels sit in the left column, values to their right on the same Y row.
    Each item in raw_results is (bbox, text, conf).
    bbox = [[x1,y1],[x2,y1],[x2,y2],[x1,y2]] (top-left, top-right, bottom-right, bottom-left)
    """
    result = {k: "" for k in FIELD_PATTERNS}

    # Build list of (x_left, y_top, y_bottom, x_right, text) for each block
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

                # Find all blocks to the right that overlap on Y axis
                label_mid_y = (ly_top + ly_bot) / 2
                candidates = []
                for vx, vy_top, vy_bot, vx_right, vtext in blocks:
                    if vx <= lx_right:          # must be to the right of label
                        continue
                    val_mid_y = (vy_top + vy_bot) / 2
                    y_overlap = min(ly_bot, vy_bot) - max(ly_top, vy_top)
                    row_height = ly_bot - ly_top
                    # Accept if vertically overlapping or within one row height
                    if y_overlap > 0 or abs(val_mid_y - label_mid_y) < row_height:
                        candidates.append((vx, vtext))

                if candidates:
                    # Pick closest on X axis; for address collect all on nearby rows
                    candidates.sort(key=lambda c: c[0])
                    value = candidates[0][1]

                    # For address: also collect continuation lines below
                    if field == "address":
                        extra = []
                        row_h = ly_bot - ly_top
                        for vx, vy_top, vy_bot, vx_right, vtext in blocks:
                            if vx <= lx_right:
                                continue
                            # Only grab lines within 2 row-heights below and
                            # skip blocks that look like other labelled fields
                            if vy_top > ly_bot and vy_top < ly_bot + row_h * 2.5:
                                if not any(re.search(p, vtext.lower())
                                           for pats in FIELD_PATTERNS.values()
                                           for p in pats):
                                    extra.append((vy_top, vtext))
                        extra.sort()
                        value = " ".join([value] + [t for _, t in extra])

                    result[field] = value
                break

    # ── Fallback: scan standalone blocks for known patterns ──────────────────
    for _, _, _, _, text in blocks:
        # Voter ID: 3 uppercase letters + 7 digits (e.g. ZPA1892797)
        if not result["idNumber"] and VOTER_ID_RE.match(text.strip()):
            result["idNumber"] = text.strip()
        # Issuing country/authority: high-signal standalone header text
        if not result["issuingCountry"]:
            tl = text.lower()
            if "election commission" in tl or "transport" in tl or "passport" in tl:
                result["issuingCountry"] = text.strip()
        # Document type from header
        if not result["documentType"]:
            tl = text.lower()
            if "voter" in tl or "elector" in tl or "driving" in tl or "passport" in tl:
                result["documentType"] = text.strip()

    return result


# ── Main ──────────────────────────────────────────────────────────────────────

def run(img_path: str):
    # EasyOCR returns list of (bbox, text, confidence)
    raw_results = reader.readtext(img_path)

    if not raw_results:
        print("No text detected in image.")
        return

    # Sort top-to-bottom by y-coordinate of bounding box top-left
    raw_results.sort(key=lambda x: x[0][0][1])
    texts = [r[1] for r in raw_results]
    confs = [r[2] for r in raw_results]

    # ── Print raw model response ──────────────────────────────────────────────
    print(f"\n{'─' * 64}")
    print(f"  RAW MODEL RESPONSE")
    print(f"{'─' * 64}")
    for bbox, text, conf in raw_results:
        print(f"  bbox={bbox}  text={text!r}  conf={conf:.4f}")

    # ── Print all detected text ───────────────────────────────────────────────
    print(f"\n{'─' * 64}")
    print(f"  RAW OCR OUTPUT")
    print(f"{'─' * 64}")
    print(f"  {'TEXT':<48} {'CONF':>6}")
    print(f"{'─' * 64}")
    for text, conf in zip(texts, confs):
        marker = "  " if conf >= 0.5 else "! "
        print(f"{marker}{text:<48} {conf:>6.1%}")

    # ── MRZ detection ────────────────────────────────────────────────────────
    print(f"\n{'─' * 64}")
    mrz = _find_mrz(texts)

    if mrz:
        print(f"  MRZ DETECTED ({len(mrz)} lines)")
        for l in mrz:
            print(f"    {l}")
        extracted = _parse_mrz_td3(mrz) if len(mrz) == 2 else _parse_mrz_td1(mrz)
        method = "MRZ"
    else:
        print("  No MRZ detected — using key-value heuristic")
        extracted = _kv_extract(raw_results)
        method = "KEY-VALUE"

    # ── Extracted fields ─────────────────────────────────────────────────────
    print(f"\n{'─' * 64}")
    print(f"  EXTRACTED FIELDS  (method: {method})")
    print(f"{'─' * 64}")
    fields = ["firstName", "lastName", "dob", "idNumber", "expiry",
              "issuingCountry", "address", "documentType"]
    for f in fields:
        val = extracted.get(f, "")
        status = "OK" if val else "MISSING"
        print(f"  {f:<20} {val:<30} [{status}]")

    filled = sum(1 for f in fields if extracted.get(f))
    print(f"\n  {filled}/{len(fields)} fields extracted")

    import json
    print(f"\n{'─' * 64}")
    print(f"  FINAL JSON")
    print(f"{'─' * 64}")
    print(json.dumps(extracted, indent=2))
    print()


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python test.py <path-to-image>")
        sys.exit(1)
    run(sys.argv[1])
