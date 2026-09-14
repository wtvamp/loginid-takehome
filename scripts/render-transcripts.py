#!/usr/bin/env python3
"""Render Claude Code session transcripts for the LoginID take-home into a
static, public-safe HTML archive under docs/transcripts/.

Stdlib only. Usage: python3 scripts/render-transcripts.py
"""

import collections
import datetime
import glob
import html
import json
import os
import re
import sys
from zoneinfo import ZoneInfo

PROJECTS = "/Users/warrenthompson/.claude/projects"
REPO = "/Users/warrenthompson/Source/LoginID"
OUT = os.path.join(REPO, "docs", "transcripts")
PDT = ZoneInfo("America/Los_Angeles")

PM_SESSION = "e179bddc-bee5-4165-90cf-dc54e411a7d6"

LEADS = {
    "01-product-industry-research-design": ("naomi-voss", "Naomi Voss", "Product Owner, lead 01", "01"),
    "02-ai-security-architecture": ("marcus-ilori", "Marcus Ilori", "Security & AI architecture lead", "02"),
    "03-engineering-delivery": ("renata-cole", "Renata Cole", "Engineering lead", "03"),
    "04-infra-devops": ("theo-bergman", "Theo Bergman", "Infra & DevOps lead", "04"),
    "05-data-ops": ("priya-nandakumar", "Priya Nandakumar", "Data ops lead", "05"),
}
LEAD_SLUGS = {v[0] for v in LEADS.values()} | {"dana-whitfield"}

HIRES = {
    "wesley-okonkwo": ("Wesley Okonkwo", "04"),
    "bree-sandoval": ("Bree Sandoval", "04"),
    "callum-ferreira": ("Callum Ferreira", "04"),
    "imogen-hale": ("Imogen Hale", "01"),
    "desmond-okafor": ("Desmond Okafor", "01"),
    "tobias-lindqvist": ("Tobias Lindqvist", "01"),
    "oren-castellan": ("Oren Castellan", "03"),
    "nolan-reyes": ("Nolan Reyes", "03"),
    "marisol-ferran": ("Marisol Ferran", "03"),
    "ines-dabrowski": ("Ines Dabrowski", "03"),
    "tomasz-wrede": ("Tomasz Wrede", "02"),
    "helena-marsh": ("Helena Marsh", "02"),
    "felix-adebayo": ("Felix Adebayo", "02"),
    "ingrid-solano": ("Ingrid Solano", "02"),
    "anders-vogel": ("Anders Vogel", "05"),
    "beatriz-achterberg": ("Beatriz Achterberg", "05"),
    "yusuf-karadag": ("Yusuf Karadag", "05"),
    "saoirse-byrne": ("Saoirse Byrne", "05"),
}
ALL_SLUGS = dict(HIRES)
for k, v in LEADS.items():
    ALL_SLUGS[v[0]] = (v[1], v[3])
ALL_SLUGS["dana-whitfield"] = ("Dana Whitfield", "PM")

# --------------------------------------------------------------------------
# Redaction
# --------------------------------------------------------------------------

COUNTS = collections.Counter()


def _c(name, n=1):
    COUNTS[name] += n


RE_PEM = re.compile(
    r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----",
    re.S,
)
RE_JWT = re.compile(r"eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}")
RE_JWT_FRAG = re.compile(r"eyJ[A-Za-z0-9_-]{40,}")
RE_BEARER_HDR = re.compile(r"(?i)(authorization\s*:\s*bearer\s+)([^\s\"',]+)")
RE_BEARER = re.compile(r"Bearer\s+[A-Za-z0-9._~+/=-]{16,}")
RE_BASIC = re.compile(r"Basic\s+[A-Za-z0-9+/=]{12,}")
RE_ARGON2 = re.compile(r"\$argon2id\$[^\s\"'`]+")
RE_DSN = re.compile(r"(postgres(?:ql)?|mysql|redis)://([^:/\s]+):([^@\s]+)@")
RE_PWEQ = re.compile(r"(?i)\b(PGPASSWORD|sslpassword|password)=([^\s\"',;&]{4,})")
RE_TOKENS = re.compile(
    r"(hvs\.[A-Za-z0-9]{20,}|hvb\.[A-Za-z0-9]{20,}|gh[pousr]_[A-Za-z0-9]{30,}"
    r"|github_pat_[A-Za-z0-9_]{20,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9]{32,}"
    r"|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16})"
)
RE_K8S_FIELD = re.compile(
    r"(?i)\b(client-key-data|client-certificate-data|certificate-authority-data|token)"
    r"(\s*:\s*)([A-Za-z0-9+/=._-]{40,})"
)
RE_B64 = re.compile(r"[A-Za-z0-9+/=]{200,}")
RE_HEX = re.compile(r"\b[0-9a-fA-F]{32,}\b")
RE_KV = re.compile(
    r"(?i)(client_secret|clientsecret|secret|password|passwd|pwd|api[_-]?key|token|signing[_-]?key)"
    r"([\"']?\s*[:=]\s*[\"']?)([^\s\"',;&]{8,})"
)
RE_IP = re.compile(r"\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b")
RE_EMAIL = re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b")

PLACEHOLDERS = {"testpass", "changeme", "example", "password", "redacted", "null", "none"}


def _kv_sub(m):
    key, sep, val = m.group(1), m.group(2), m.group(3)
    v = val.strip("\"'")
    low = v.lower()
    if (not v) or v[0] in "$<{«" or v.startswith("{{") or low in PLACEHOLDERS:
        return m.group(0)
    if re.fullmatch(r"[a-z][a-z-]*", v):  # k8s object name / placeholder
        return m.group(0)
    has_digit = any(ch.isdigit() for ch in v)
    looks = re.search(r"[A-Za-z0-9+/=_-]{12,}", v)
    if has_digit and looks:
        _c("9 key/value secret")
        return "%s%s«REDACTED»" % (key, sep)
    return m.group(0)


def _hex_sub(m, s):
    v = m.group(0)
    n = len(v)
    a, b = m.start(), m.end()
    nb = s[a - 1] if a > 0 else ""
    na = s[b] if b < len(s) else ""
    if nb == "-" or na == "-":  # dash-joined => UUID-ish
        return v
    if n == 40:  # git SHA
        return v
    if n == 32 or n >= 44:
        _c("3 long hex")
        return "«REDACTED-HEX»"
    return v


def _ip_sub(m):
    o = [int(x) for x in m.groups()]
    if any(x > 255 for x in o):
        return m.group(0)
    if o[0] in (10, 127) or (o[0] == 192 and o[1] == 168) or (o[0] == 172 and 16 <= o[1] <= 31) or o[0] == 0:
        return m.group(0)
    _c("10 public IP")
    return "«IP»"


def _email_sub(m):
    v = m.group(0)
    lv = v.lower()
    if lv.endswith("@agents.loginid-takehome.invalid") or lv == "noreply@anthropic.com":
        return v
    _c("11 email")
    return "«email»"


def _count_sub(pattern, repl, s, name):
    out, n = pattern.subn(repl, s)
    if n:
        _c(name, n)
    return out


def redact(s):
    if not s:
        return s
    s = _count_sub(RE_PEM, "«REDACTED-PRIVATE-KEY»", s, "6 private key block")
    s = _count_sub(RE_JWT, "«REDACTED-JWT»", s, "1 JWT")
    s = _count_sub(RE_JWT_FRAG, "«REDACTED-JWT»", s, "1 JWT fragment")
    s = _count_sub(RE_TOKENS, "«REDACTED-TOKEN»", s, "8 known token prefix")
    s = _count_sub(RE_ARGON2, "«REDACTED-ARGON2»", s, "4 argon2 hash")
    s = _count_sub(RE_K8S_FIELD, lambda m: m.group(1) + m.group(2) + "«REDACTED»", s, "7 k8s secret field")
    s = _count_sub(RE_B64, "«REDACTED-BASE64»", s, "7 long base64 run")
    s = _count_sub(RE_BEARER_HDR, lambda m: m.group(1) + "«REDACTED»", s, "2 Authorization header")
    s = _count_sub(RE_BEARER, "Bearer «REDACTED»", s, "2 bearer token")
    s = _count_sub(RE_BASIC, "Basic «REDACTED»", s, "2 basic auth")
    s = _count_sub(RE_DSN, lambda m: "%s://%s:«REDACTED»@" % (m.group(1), m.group(2)), s, "5 DSN password")
    s = _count_sub(RE_PWEQ, lambda m: m.group(1) + "=«REDACTED»", s, "5 password= value")
    src = s
    s = RE_HEX.sub(lambda m: _hex_sub(m, src), s)
    s = RE_KV.sub(_kv_sub, s)
    s = RE_IP.sub(_ip_sub, s)
    s = RE_EMAIL.sub(_email_sub, s)
    return s


# --------------------------------------------------------------------------
# Parsing
# --------------------------------------------------------------------------

SKIP_TYPES = {
    "attachment", "pr-link", "mode", "permission-mode", "atis-latch", "last-prompt",
    "bridge-session", "ai-title", "queue-operation", "agent-color", "agent-name",
    "cost-state", "custom-title", "frame-link", "agent-setting",
    "artifact-comment-monitor", "artifact-autoreact-ledger",
}
SKIP_TYPES |= {t for t in ("file-history-snapshot", "file-history-delta")}
SYSTEM_SHOW = {"local_command", "compact_boundary", "informational", "scheduled_task_fire", "away_summary"}


def ts_parse(s):
    if not s:
        return None
    try:
        return datetime.datetime.fromisoformat(s.replace("Z", "+00:00"))
    except Exception:
        return None


def fmt_ts(dt):
    if not dt:
        return ""
    return dt.astimezone(PDT).strftime("%b %-d, %-I:%M %p PDT")


def fmt_ts_long(dt):
    if not dt:
        return ""
    return dt.astimezone(PDT).strftime("%b %-d, %Y %-I:%M %p PDT")


def blocks_of(msg):
    c = msg.get("content")
    if isinstance(c, str):
        return [{"type": "text", "text": c}]
    if isinstance(c, list):
        return [b for b in c if isinstance(b, dict)]
    return []


def parse_file(path):
    """Return dict with items (render list), stats, model, start, end."""
    records = []
    with open(path, "r", errors="replace") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                r = json.loads(line)
            except Exception:
                continue
            t = r.get("type")
            if t in SKIP_TYPES:
                continue
            if t in ("user", "assistant", "system"):
                records.append(r)

    # pass 1: tool results
    results = {}
    for r in records:
        if r.get("type") != "user":
            continue
        m = r.get("message")
        if not isinstance(m, dict):
            continue
        for b in blocks_of(m):
            if b.get("type") == "tool_result":
                results[b.get("tool_use_id")] = b

    items = []
    merged = {}  # message.id -> item
    models = collections.Counter()
    turns = 0
    tool_calls = 0
    thinking_blocks = 0
    first = last = None

    for r in records:
        t = r.get("type")
        dt = ts_parse(r.get("timestamp"))
        if dt:
            first = dt if first is None else min(first, dt)
            last = dt if last is None else max(last, dt)
        m = r.get("message") if isinstance(r.get("message"), dict) else None

        if t == "system":
            sub = r.get("subtype")
            if sub not in SYSTEM_SHOW:
                continue
            txt = r.get("content")
            if not isinstance(txt, str) or not txt.strip():
                continue
            items.append({"kind": "system", "ts": dt, "sub": sub, "text": txt})
            continue

        if t == "user":
            if not m:
                continue
            bs = blocks_of(m)
            if bs and all(b.get("type") == "tool_result" for b in bs):
                continue  # rendered under its tool_use
            texts = []
            for b in bs:
                if b.get("type") == "text":
                    texts.append(b.get("text") or "")
                elif b.get("type") == "tool_result":
                    pass
                elif b.get("type") == "image":
                    texts.append("[image]")
            txt = "\n".join(x for x in texts if x)
            if not txt.strip():
                continue
            items.append({"kind": "human", "ts": dt, "text": txt,
                          "meta": r.get("userType") or r.get("promptSource") or ""})
            continue

        # assistant
        if not m:
            continue
        mid = m.get("id") or r.get("uuid")
        if m.get("model"):
            models[m.get("model")] += 1
        item = merged.get(mid)
        if item is None:
            item = {"kind": "assistant", "ts": dt, "blocks": [], "seen": set(), "model": m.get("model")}
            merged[mid] = item
            items.append(item)
            turns += 1
        for b in blocks_of(m):
            bt = b.get("type")
            if bt == "thinking":
                thinking_blocks += 1
                continue
            try:
                key = json.dumps(b, sort_keys=True, default=str)
            except Exception:
                key = repr(b)
            if key in item["seen"]:
                continue
            item["seen"].add(key)
            if bt == "tool_use":
                tool_calls += 1
                b = dict(b)
                b["_result"] = results.get(b.get("id"))
            item["blocks"].append(b)

    # drop empty assistant items
    items = [i for i in items if i["kind"] != "assistant" or i["blocks"]]
    for i in items:
        i.pop("seen", None)
    turns = sum(1 for i in items if i["kind"] == "assistant")

    return {
        "items": items, "turns": turns, "tool_calls": tool_calls,
        "thinking": thinking_blocks,
        "model": models.most_common(1)[0][0] if models else "unknown",
        "start": first, "end": last, "path": path,
    }


def scan_slugs(path, exclude_leads=False):
    """Count `agents/<slug>` and `profiles/<slug>/` markers across the WHOLE file."""
    counts = collections.Counter()
    try:
        with open(path, "r", errors="replace") as fh:
            data = fh.read()
    except Exception:
        return counts
    for slug in ALL_SLUGS:
        if exclude_leads and slug in LEAD_SLUGS:
            continue
        n = data.count("agents/" + slug) + data.count("profiles/" + slug + "/")
        if n:
            counts[slug] = n
    return counts


def agent_setting(path):
    """The definitive persona for a named-teammate session, if the log declares one."""
    settings = collections.Counter()
    names = collections.Counter()
    try:
        fh = open(path, "r", errors="replace")
    except Exception:
        return None, None
    with fh:
        for line in fh:
            if '"agent-setting"' not in line and '"agent-name"' not in line:
                continue
            try:
                r = json.loads(line)
            except Exception:
                continue
            if r.get("type") == "agent-setting" and r.get("agentSetting"):
                settings[r["agentSetting"]] += 1
            elif r.get("type") == "agent-name" and r.get("agentName"):
                names[r["agentName"]] += 1
    st = settings.most_common(1)[0][0] if settings else None
    nm = names.most_common(1)[0][0] if names else None
    return st, nm


# --------------------------------------------------------------------------
# Discovery
# --------------------------------------------------------------------------

def persona_for_slug(slug):
    if slug == "dana-whitfield":
        return "Dana Whitfield", "PM"
    if slug in HIRES:
        return HIRES[slug]
    for k, v in LEADS.items():
        if v[0] == slug:
            return v[1], v[3]
    return slug, "-"


def attribute_top_level(counts, track):
    """Attribute a top-level session that declares no hire `agent-setting`.

    A lead's own session names its hires constantly (it drafted and spawned them),
    so raw slug frequency alone points at the wrong person. Resolution order:
      1. a track directory implies its lead;
      2. otherwise, if hire slugs dominate and they all belong to one track, the
         file is that lead's hiring session;
      3. otherwise the single most-mentioned lead slug (`dana-whitfield` excluded —
         it appears in every file via the root CLAUDE.md persona reference);
      4. a tie between leads with no hire signal means a cross-track session the PM
         directed (the consistency pass, the Jira scribe), not any one lead.
    """
    if track and LEADS[track][0] in counts:
        slug, name, role, tr = LEADS[track]
        return dict(slug=slug, persona=name, role=role, track=tr, group="lead")

    hire_counts = {k: v for k, v in counts.items() if k in HIRES}
    if hire_counts:
        per_track = collections.Counter()
        for k, v in hire_counts.items():
            per_track[HIRES[k][1]] += v
        tr, top = per_track.most_common(1)[0]
        others = sum(per_track.values()) - top
        # A hiring session names its OWN hires and few others; a cross-track pass
        # names all eighteen. Only a concentrated signal identifies a lead.
        if others < top * 0.25:
            for key, v in LEADS.items():
                if v[3] == tr:
                    return dict(slug=v[0], persona=v[1], role=v[2] + " — hiring session",
                                track=tr, group="planning")

    lead_counts = sorted(((v, k) for k, v in counts.items()
                          if k in LEAD_SLUGS and k != "dana-whitfield"), reverse=True)
    if lead_counts and (len(lead_counts) == 1 or lead_counts[0][0] > lead_counts[1][0]):
        slug = lead_counts[0][1]
        for key, v in LEADS.items():
            if v[0] == slug:
                return dict(slug=slug, persona=v[1], role=v[2] + " — planning-phase run",
                            track=v[3], group="planning")
    if counts.get("dana-whitfield") and not lead_counts:
        return dict(slug="dana-whitfield", persona="Dana Whitfield",
                    role="PM teammate run", track="PM", group="planning")
    return dict(slug="pm-utility",
                persona="PM-directed utility session",
                role="Cross-track pass / Jira scribe (spawned by Dana Whitfield)",
                track="PM", group="planning")


def discover():
    """Identify every transcript file's persona.

    Precedence:
      1. an `agent-setting` record naming a known hire/lead slug  (named teammate session)
      2. the dominant `agents/<slug>` / `profiles/<slug>/` marker across the whole file
      3. the directory-implies-lead fallback
    """
    files = []
    tops = {}   # session id -> descriptor, for attributing orphan subagents
    subs = []
    for d in sorted(glob.glob(os.path.join(PROJECTS, "*LoginID*"))):
        dirslug = os.path.basename(d)
        track = None
        for key in LEADS:
            if dirslug.endswith(key):
                track = key
                break
        for p in sorted(glob.glob(os.path.join(d, "*.jsonl"))):
            sid = os.path.basename(p)[:-6]
            st, nm = agent_setting(p)
            fd = None
            if sid == PM_SESSION:
                fd = dict(slug="dana-whitfield", persona="Dana Whitfield",
                          role="Project manager (PM)", track="PM", group="pm")
            elif st in ALL_SLUGS:
                nm2, tr = persona_for_slug(st)
                if st in HIRES:
                    fd = dict(slug=st, persona=nm2, role="Hire (track %s)" % tr,
                              track=tr, group="hire")
                else:
                    fd = dict(slug=st, persona=nm2, role="Teammate session",
                              track=tr, group="planning")
            else:
                counts = scan_slugs(p)
                fd = attribute_top_level(counts, track)
            fd.update(path=p, sid=sid)
            files.append(fd)
            tops[sid] = fd
        for p in sorted(glob.glob(os.path.join(d, "*", "subagents", "*.jsonl"))):
            subs.append((p, os.path.basename(os.path.dirname(os.path.dirname(p))), track))

    for p, parent, track in subs:
        sid = os.path.basename(p)[:-6]
        st, nm = agent_setting(p)
        if st in HIRES:
            slug = st
        else:
            counts = scan_slugs(p, exclude_leads=True)
            slug = counts.most_common(1)[0][0] if counts else None
        if slug and slug in HIRES:
            nm2, tr = HIRES[slug]
            fd = dict(slug=slug, persona=nm2, role="Hire (track %s)" % tr, track=tr,
                      group="hire")
        else:
            owner = tops.get(parent)
            oname = owner["persona"] if owner else "an unidentified session"
            otrack = owner["track"] if owner else (LEADS[track][3] if track else "-")
            oslug = owner["slug"] if owner else "unknown"
            fd = dict(slug="research-subagent-" + oslug,
                      persona="Research subagent (%s's session)" % oname,
                      role="Unnamed research / consistency subagent",
                      track=otrack, group="hire")
        fd.update(path=p, sid=sid, parent=parent)
        files.append(fd)
    return files


# --------------------------------------------------------------------------
# Rendering
# --------------------------------------------------------------------------

CSS = """
:root{--bg:#0A0F1E;--fg:#E6EAF2;--mut:#8A93A8;--teal:#3DD6C8;--orange:#FF7A59;
--panel:#121a2e;--line:#1e2a44;}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);
font-family:Manrope,system-ui,-apple-system,sans-serif;font-size:15px;line-height:1.6;
overflow-wrap:anywhere;}
code,pre,.mono{font-family:"JetBrains Mono",ui-monospace,Menlo,monospace;}
a{color:var(--teal)}
.wrap{max-width:900px;margin:0 auto;padding:16px;}
header.sticky{position:sticky;top:0;z-index:5;background:rgba(10,15,30,.95);
border-bottom:1px solid var(--line);backdrop-filter:blur(6px);}
header.sticky .wrap{padding:10px 16px;}
h1{font-size:20px;margin:0 0 4px}
h2{font-size:17px;margin:28px 0 8px;color:var(--teal)}
h3{font-size:15px;margin:18px 0 6px}
.sub{color:var(--mut);font-size:12.5px}
.nav{margin-top:6px;font-size:13px}
.nav a{margin-right:12px}
.msg{border:1px solid var(--line);border-radius:10px;padding:10px 12px;margin:14px 0;background:var(--panel);}
.msg.human{border-left:4px solid var(--orange);margin-left:8%;}
.msg.assistant{border-left:4px solid var(--teal);margin-right:4%;}
.msg.system{border-left:4px solid var(--mut);background:transparent;font-size:13px;color:var(--mut)}
.who{font-weight:700;font-size:13px;letter-spacing:.02em}
.msg.assistant .who{color:var(--teal)}
.msg.human .who{color:var(--orange)}
.when{float:right;color:var(--mut);font-size:11px;font-family:"JetBrains Mono",ui-monospace,Menlo,monospace}
.body{margin-top:6px;white-space:pre-wrap;font-size:14px}
details{margin:8px 0;border:1px solid var(--line);border-radius:8px;background:#0d1425}
summary{cursor:pointer;padding:6px 10px;font-size:12.5px;color:var(--mut);
font-family:"JetBrains Mono",ui-monospace,Menlo,monospace}
summary .tn{color:var(--teal);font-weight:700}
details .inner{padding:0 10px 10px}
pre{white-space:pre-wrap;font-size:12px;background:#070b16;border:1px solid var(--line);
border-radius:6px;padding:8px;margin:6px 0;max-height:640px;overflow:auto}
.lbl{color:var(--mut);font-size:11px;text-transform:uppercase;letter-spacing:.08em;margin-top:8px}
.err{color:#ff8b8b}
details.sr{display:inline-block;background:transparent;border-style:dashed}
table{border-collapse:collapse;width:100%;font-size:13px;margin:8px 0}
th,td{border-bottom:1px solid var(--line);padding:6px 8px;text-align:left;vertical-align:top}
th{color:var(--mut);font-weight:600;font-size:12px}
.note{color:var(--mut);font-size:13.5px;border:1px solid var(--line);border-radius:10px;padding:12px;background:var(--panel)}
.pill{display:inline-block;border:1px solid var(--line);border-radius:999px;padding:1px 8px;
font-size:11px;color:var(--mut);margin-left:6px;font-family:"JetBrains Mono",ui-monospace,Menlo,monospace}
@media(max-width:520px){.msg.human{margin-left:0}.msg.assistant{margin-right:0}}
"""

HEAD = """<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;600;700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
<link rel="stylesheet" href="%s">"""

SR_RE = re.compile(r"<system-reminder>(.*?)</system-reminder>", re.S)
XS_RE = re.compile(r'<cross-session-message[^>]*from-name="([^"]*)"', re.S)


def esc(s):
    return html.escape(s if isinstance(s, str) else str(s), quote=False)


def render_text(raw):
    """Escape, collapsing system-reminder blocks into disclosures."""
    raw = redact(raw or "")
    out = []
    pos = 0
    for m in SR_RE.finditer(raw):
        out.append(esc(raw[pos:m.start()]))
        inner = m.group(1)
        out.append('<details class="sr"><summary>system reminder (%d chars)</summary>'
                   '<div class="inner"><pre>%s</pre></div></details>' % (len(inner), esc(inner)))
        pos = m.end()
    out.append(esc(raw[pos:]))
    return "".join(out)


def tool_summary(name, inp):
    if not isinstance(inp, dict):
        return ""
    def g(k):
        v = inp.get(k)
        return v if isinstance(v, str) else ""
    if name == "Bash":
        return g("command")[:100]
    if name in ("Read", "Write", "Edit", "NotebookEdit"):
        return g("file_path")[:120]
    if name == "SendMessage":
        return "to " + g("to")[:60]
    if name == "Agent":
        return g("description")[:100] or g("subagent_type")[:60]
    if name in ("Grep", "Glob"):
        return (g("pattern") or "")[:80]
    if name in ("WebFetch", "WebSearch"):
        return (g("url") or g("query"))[:100]
    if name == "TodoWrite":
        return "%d items" % len(inp.get("todos") or [])
    for k in ("description", "prompt", "query", "url", "path"):
        if g(k):
            return g(k)[:100]
    return ""


def result_text(res, cap=20000):
    if not res:
        return None, False
    c = res.get("content")
    parts = []
    if isinstance(c, str):
        parts.append(c)
    elif isinstance(c, list):
        for b in c:
            if not isinstance(b, dict):
                parts.append(str(b))
            elif b.get("type") == "text":
                parts.append(b.get("text") or "")
            elif b.get("type") == "image":
                parts.append("[image omitted]")
            elif b.get("type") == "tool_reference":
                parts.append("[tool reference: %s]" % (b.get("name") or ""))
            else:
                parts.append(json.dumps(b, default=str)[:2000])
    txt = "\n".join(parts)
    trunc = False
    if len(txt) > cap:
        txt = txt[:cap]
        trunc = True
    return txt, bool(res.get("is_error"))


def render_tool(b):
    name = b.get("name") or "tool"
    inp = b.get("input") or {}
    summ = redact(tool_summary(name, inp))
    try:
        pretty = json.dumps(inp, indent=2, ensure_ascii=False, default=str)
    except Exception:
        pretty = str(inp)
    if len(pretty) > 40000:
        pretty = pretty[:40000] + "\n… truncated"
    pretty = redact(pretty)
    rtxt, is_err = result_text(b.get("_result"))
    out = ['<details><summary><span class="tn">%s</span> %s</summary><div class="inner">'
           % (esc(name), esc(summ))]
    out.append('<div class="lbl">input</div><pre>%s</pre>' % esc(pretty))
    if rtxt is not None:
        trunc = "\n… truncated (20 KB cap)" if len(rtxt) >= 20000 else ""
        out.append('<div class="lbl%s">result%s</div><pre>%s</pre>'
                   % (" err" if is_err else "", " — error" if is_err else "",
                      esc(redact(rtxt) + trunc)))
    out.append("</div></details>")
    return "".join(out)


def human_label(fd, text):
    m = XS_RE.search(text or "")
    if m:
        return "Message from %s" % esc(m.group(1))
    if fd["group"] == "hire":
        return "Lead"
    return "Warren"


def render_item(it, fd):
    if it["kind"] == "system":
        return ('<div class="msg system"><span class="when">%s</span>'
                '<span class="who">system · %s</span><div class="body">%s</div></div>'
                % (esc(fmt_ts(it["ts"])), esc(it.get("sub") or ""), render_text(it["text"][:4000])))
    if it["kind"] == "human":
        return ('<div class="msg human"><span class="when">%s</span>'
                '<span class="who">%s</span><div class="body">%s</div></div>'
                % (esc(fmt_ts(it["ts"])), human_label(fd, it["text"]), render_text(it["text"])))
    parts = ['<div class="msg assistant"><span class="when">%s</span><span class="who">%s</span>'
             % (esc(fmt_ts(it["ts"])), esc(fd["persona"]))]
    for b in it["blocks"]:
        if b.get("type") == "text":
            t = b.get("text") or ""
            if t.strip():
                parts.append('<div class="body">%s</div>' % render_text(t))
        elif b.get("type") == "tool_use":
            parts.append(render_tool(b))
    parts.append("</div>")
    return "".join(parts)


def page_header(fd, parsed, n, total, files_rel):
    prev = '<a href="%s">← prev</a>' % files_rel[n - 2] if n > 1 else ""
    nxt = '<a href="%s">next →</a>' % files_rel[n] if n < total else ""
    span = "%s – %s" % (fmt_ts_long(parsed["start"]), fmt_ts_long(parsed["end"]))
    return ('<header class="sticky"><div class="wrap"><h1>%s</h1>'
            '<div class="sub">%s · model <span class="mono">%s</span> · %s</div>'
            '<div class="nav">%s %s <a href="../index.html">index</a> '
            '<span class="pill">page %d of %d</span></div></div></header>'
            % (esc(fd["persona"]), esc(fd["role"] + (" · " + fd["label"] if fd.get("label") else "")),
               esc(parsed["model"]), esc(span), prev, nxt, n, total))


def write_session(fd, parsed):
    slugdir = os.path.join(OUT, fd["slug"])
    os.makedirs(slugdir, exist_ok=True)
    short = fd["sid"][:8]
    items = parsed["items"]
    # paginate at 120 assistant messages
    pages = []
    cur = []
    a = 0
    for it in items:
        if it["kind"] == "assistant":
            if a >= 120:
                pages.append(cur)
                cur = []
                a = 0
            a += 1
        cur.append(it)
    if cur:
        pages.append(cur)
    if not pages:
        pages = [[]]
    total = len(pages)
    rels = ["%s-p%d.html" % (short, i + 1) for i in range(total)]
    written = []
    for i, page in enumerate(pages, 1):
        body = [HEAD % (esc(fd["persona"] + " — transcript p%d" % i), "../t.css")]
        body.append(page_header(fd, parsed, i, total, rels))
        body.append('<div class="wrap">')
        for it in page:
            body.append(render_item(it, fd))
        body.append('<div class="nav" style="margin:24px 0">%s %s <a href="../index.html">Back to index</a></div>'
                    % ('<a href="%s">← prev</a>' % rels[i - 2] if i > 1 else "",
                       '<a href="%s">next →</a>' % rels[i] if i < total else ""))
        body.append("</div>")
        p = os.path.join(slugdir, rels[i - 1])
        with open(p, "w") as fh:
            fh.write("\n".join(body))
        written.append("%s/%s" % (fd["slug"], rels[i - 1]))
    return written


INTRO = """This archive is the complete working record of the Claude Code sessions that built this
take-home project: every message, every tool call and every tool result, from the project-manager
session, the five track-lead sessions, and each hire's subagent run. Secrets are redacted in place
and marked with «REDACTED», «REDACTED-JWT», «REDACTED-HEX», «REDACTED-ARGON2», «REDACTED-TOKEN»,
«REDACTED-BASE64», «REDACTED-PRIVATE-KEY», «IP» and «email» so the archive can be published as-is.
The models' private reasoning is not stored in these logs, so it is not shown — only the text,
tool calls and results that were actually exchanged. Human turns are labelled "Warren" in the PM and
lead sessions and "Lead" in hire runs; a turn labelled "Message from …" is a message relayed from
another Claude session. Timestamps are America/Los_Angeles."""


def main():
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, "t.css"), "w") as fh:
        fh.write(CSS)

    fds = discover()
    print("discovered %d transcript files" % len(fds))
    parsed_all = []
    for fd in fds:
        p = parse_file(fd["path"])
        if p["turns"] == 0 and not p["items"]:
            continue
        parsed_all.append((fd, p))

    # group lead sessions -> session N by start time
    bykey = collections.defaultdict(list)
    for fd, p in parsed_all:
        bykey[(fd["slug"], fd["group"])].append((fd, p))
    for key, lst in bykey.items():
        lst.sort(key=lambda x: (x[1]["start"] or datetime.datetime.max.replace(tzinfo=datetime.timezone.utc)))
        multi = len(lst) > 1
        for i, (fd, p) in enumerate(lst, 1):
            fd["label"] = ("session %d of %d" % (i, len(lst))) if multi else "session"
            fd["order"] = i

    parsed_all.sort(key=lambda x: (x[1]["start"] or datetime.datetime.max.replace(tzinfo=datetime.timezone.utc)))

    manifest = collections.OrderedDict()
    persona_rows = collections.defaultdict(lambda: {"turns": 0, "tools": 0, "start": None, "end": None,
                                                    "sessions": [], "model": collections.Counter()})
    for fd, p in parsed_all:
        rels = write_session(fd, p)
        key = (fd["group"], fd["slug"])
        row = persona_rows[key]
        row.setdefault("persona", fd["persona"])
        row.setdefault("track", fd["track"])
        row.setdefault("roles", [])
        if fd["role"] not in row["roles"]:
            row["roles"].append(fd["role"])
        row["turns"] += p["turns"]
        row["tools"] += p["tool_calls"]
        row["model"][p["model"]] += p["turns"] or 1
        if p["start"]:
            row["start"] = p["start"] if row["start"] is None else min(row["start"], p["start"])
        if p["end"]:
            row["end"] = p["end"] if row["end"] is None else max(row["end"], p["end"])
        row["sessions"].append({
            "id": fd["sid"], "label": fd.get("label", "session"), "pages": rels,
            "start": p["start"].isoformat() if p["start"] else None,
            "end": p["end"].isoformat() if p["end"] else None,
            "turns": p["turns"], "tool_calls": p["tool_calls"], "model": p["model"],
        })
        print("  %-28s %-10s turns=%-5d tools=%-5d pages=%d" %
              (fd["persona"][:28], fd["slug"][:10], p["turns"], p["tool_calls"], len(rels)))

    # manifest
    mani = []
    for (group, slug), row in persona_rows.items():
        mani.append({"persona": row["persona"], "slug": slug,
                     "role": " · ".join(row["roles"]), "track": row["track"],
                     "group": group,
                     "model": row["model"].most_common(1)[0][0] if row["model"] else "unknown",
                     "turns": row["turns"], "tool_calls": row["tools"],
                     "sessions": row["sessions"]})
    mani.sort(key=lambda r: (["pm", "lead", "hire", "planning"].index(r["group"]), r["track"], r["persona"]))
    with open(os.path.join(OUT, "manifest.json"), "w") as fh:
        json.dump(mani, fh, indent=2)

    # index
    out = [HEAD % ("LoginID take-home — session transcripts", "t.css")]
    out.append('<div class="wrap"><h1>Session transcript archive</h1>'
               '<div class="sub"><a href="../index.html">← Back to the retrospective</a></div>'
               '<div class="note" style="margin-top:14px">%s</div>' % esc(INTRO))

    def section(title, rows):
        if not rows:
            return
        out.append("<h2>%s</h2>" % esc(title))
        out.append("<table><tr><th>Persona</th><th>Role</th><th>Model</th><th>Turns</th>"
                   "<th>Tool calls</th><th>Span</th><th>Transcript</th></tr>")
        for r in rows:
            links = []
            for s in r["sessions"]:
                lbl = s["label"]
                pl = " ".join('<a href="%s">%s</a>' % (esc(pg), "p%d" % (i + 1))
                              for i, pg in enumerate(s["pages"]))
                links.append("%s: %s" % (esc(lbl), pl))
            span = "%s – %s" % (
                fmt_ts(ts_parse(min((s["start"] for s in r["sessions"] if s["start"]), default=None) or "")),
                fmt_ts(ts_parse(max((s["end"] for s in r["sessions"] if s["end"]), default=None) or "")))
            out.append("<tr><td><b>%s</b></td><td>%s</td><td class='mono' style='font-size:11px'>%s</td>"
                       "<td>%d</td><td>%d</td><td class='mono' style='font-size:11px'>%s</td><td>%s</td></tr>"
                       % (esc(r["persona"]), esc(r["role"]), esc(r["model"]), r["turns"],
                          r["tool_calls"], esc(span), "<br>".join(links)))
        out.append("</table>")

    section("Project manager", [r for r in mani if r["group"] == "pm"])
    section("Track leads", sorted([r for r in mani if r["group"] == "lead"], key=lambda r: r["track"]))
    hires = [r for r in mani if r["group"] == "hire"]
    for tr in ["01", "02", "03", "04", "05", "PM", "-"]:
        section("Hires — track %s" % tr, sorted([r for r in hires if r["track"] == tr],
                                                key=lambda r: r["persona"]))
    section("Planning phase (2026-09-12 afternoon)",
            sorted([r for r in mani if r["group"] == "planning"], key=lambda r: r["persona"]))
    out.append('<div class="sub" style="margin:28px 0">Machine-readable index: '
               '<a href="manifest.json">manifest.json</a></div></div>')
    with open(os.path.join(OUT, "index.html"), "w") as fh:
        fh.write("\n".join(out))

    print("\nredaction counts:")
    for k, v in sorted(COUNTS.items()):
        print("  %-28s %d" % (k, v))
    return parsed_all


if __name__ == "__main__":
    main()
