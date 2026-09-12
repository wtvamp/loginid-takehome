#!/usr/bin/env bash
# Serialized portrait renderer for hires registered in CASTING.md.
# Ops tooling for the planning org, not assignment code.
#
# Walks the hire registry (§8) for rows whose Portrait column is `queued`,
# renders ONE portrait at a time on the shared ComfyUI box using the persona's
# own frontmatter prompt, writes the PNG to the persona's pre-declared image
# path, and flips the row to `done` (or `failed`). Re-runnable; idempotent per row.
#
# Usage:  nohup scripts/portrait-queue.sh > scripts/portrait-queue.log 2>&1 &
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REGISTRY="$ROOT/CASTING.md"
GEN="${PROFILEGEN_DIR:-$HOME/.claude/skills/profile-gen}/scripts/generate_image.py"
WORKFLOW="${PORTRAIT_WORKFLOW:-$ROOT/profiles/dana-whitfield/comfyui-workflow.json}"
export COMFYUI_URL="${COMFYUI_URL:-http://192.168.1.24:8000}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }

# Registry columns (1-based after splitting on '|', field 1 is the empty lead-in):
#  2 Slug · 3 Name · 4 Track · 5 Function · 6 Archetype · 7 Tags · 8 Model
#  9 Patterns · 10 Persona path · 11 Agent def path · 12 Portrait
# All queued rows, one per line (leads may register rows before finishing the persona file,
# so a missing persona is "not ready yet", never a failure).
queued_rows() {
  awk -F'|' '
    /^\|/ && NF >= 12 {
      slug = $2; status = $12
      gsub(/^[ \t`]+|[ \t`]+$/, "", slug); gsub(/^[ \t`]+|[ \t`]+$/, "", status)
      if (slug != "Slug" && slug !~ /^-+$/ && status == "queued") print $0
    }' "$REGISTRY"
}

# Cell text with surrounding whitespace and markdown backticks removed.
field() { echo "$1" | awk -F'|' -v n="$2" '{ v=$n; gsub(/^[ \t`]+|[ \t`]+$/, "", v); print v }'; }

# Pull a frontmatter scalar (quoted or bare) from a profile-gen persona file.
fm() { # fm <file> <key>
  python3 - "$1" "$2" <<'PY'
import sys, re
path, key = sys.argv[1], sys.argv[2]
text = open(path, encoding="utf-8").read()
m = re.match(r"^---\n(.*?)\n---", text, re.S)
fm = m.group(1) if m else ""
for line in fm.splitlines():
    mm = re.match(r"^\s*" + re.escape(key) + r":\s*(.*)$", line)
    if mm:
        v = mm.group(1).strip()
        if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
            v = v[1:-1]
        print(v); break
PY
}

set_status() { # set_status <slug> <status>
  python3 - "$REGISTRY" "$1" "$2" <<'PY'
import sys
path, slug, status = sys.argv[1:4]
out = []
for line in open(path, encoding="utf-8"):
    cells = line.rstrip("\n").split("|")
    if len(cells) >= 13 and cells[1].strip() == slug:
        cells[12] = f" {status} "
        line = "|".join(cells) + "\n"
    out.append(line)
open(path, "w", encoding="utf-8").write("".join(out))
PY
}

log "portrait queue starting (root=$ROOT, workflow=$WORKFLOW)"
# Pass-based: each pass renders every queued row whose persona file exists. Rows whose persona
# is not written yet are left queued and re-checked next pass; when a full pass renders nothing
# and nothing is waiting, exit. WAIT_SECS between passes while rows are waiting on their leads.
WAIT_SECS="${PORTRAIT_QUEUE_WAIT:-120}"
MAX_IDLE_PASSES="${PORTRAIT_QUEUE_MAX_IDLE:-30}"   # ~1 hour of waiting on unwritten personas, then exit
idle_passes=0
while :; do
  rendered=0; waiting=0
  while IFS= read -r row; do
    [ -z "$row" ] && continue
    slug="$(field "$row" 2)"
    persona_rel="$(field "$row" 10)"
    persona="$ROOT/$persona_rel"
    track_dir="$ROOT/${persona_rel%%/*}"
    if [ ! -f "$persona" ]; then log "$slug: persona not written yet ($persona_rel) — leaving queued"; waiting=$((waiting+1)); continue; fi

    image_rel="$(fm "$persona" image)"
    prompt="$(fm "$persona" prompt)"
    negative="$(fm "$persona" negative_prompt)"
    [ -z "$negative" ] && negative="blurry, low quality, distorted, extra limbs, watermark, text, nsfw, nude, nudity, explicit, sexual content"
    out="$track_dir/$image_rel"
    if [ -z "$image_rel" ] || [ -z "$prompt" ]; then log "$slug: persona lacks image path or portrait prompt in frontmatter — marking failed (lead: fix frontmatter, set Portrait back to queued)"; set_status "$slug" failed; continue; fi
    if [ -s "$out" ]; then log "$slug: portrait already exists at $out — marking done"; set_status "$slug" done; continue; fi

    mkdir -p "$(dirname "$out")"
    printf '%s' "$prompt" > "$TMP/$slug.prompt.txt"
    printf '%s' "$negative" > "$TMP/$slug.negative.txt"
    log "$slug: rendering → $out"
    if result="$(python3 "$GEN" --backend comfyui --prompt-file "$TMP/$slug.prompt.txt" --negative-file "$TMP/$slug.negative.txt" --out "$out" --workflow "$WORKFLOW" 2>&1)" && [ -s "$out" ]; then
      seed="$(printf '%s' "$result" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("seed",""))
except Exception: print("")')"
      log "$slug: done (seed=$seed)"
      set_status "$slug" done
    else
      log "$slug: FAILED — $(printf '%s' "$result" | tail -c 400)"
      set_status "$slug" failed
    fi
    rendered=$((rendered+1))
  done <<< "$(queued_rows)"

  if [ "$rendered" -eq 0 ] && [ "$waiting" -eq 0 ]; then log "queue empty — exiting"; break; fi
  if [ "$rendered" -eq 0 ]; then
    idle_passes=$((idle_passes+1))
    if [ "$idle_passes" -ge "$MAX_IDLE_PASSES" ]; then log "$waiting row(s) still waiting on persona files after $idle_passes passes — exiting; re-run later"; break; fi
    sleep "$WAIT_SECS"
  else
    idle_passes=0
  fi
done
