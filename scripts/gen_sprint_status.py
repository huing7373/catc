#!/usr/bin/env python3
"""Generate sprint-status.yaml from epics.md.

Parses '## Epic N: Title' and '### Story N.M: Title' headers, converts each
story title to a kebab-case slug, and writes an ordered sprint-status.yaml.

Usage:
  python scripts/gen_sprint_status.py \
      _bmad-output/planning-artifacts/epics.md \
      _bmad-output/implementation-artifacts/sprint-status.yaml
"""
from __future__ import annotations

import os
import re
import sys
from datetime import datetime, timezone

EPIC_RE = re.compile(r"^## Epic (\d+): (.+?)\s*$")
STORY_RE = re.compile(r"^### Story (\d+)\.(\d+): (.+?)\s*$")

# Chars to strip entirely (parenthetical contents handled separately).
PUNCT_STRIP = "".join(["`", '"', "'", "“", "”", "‘", "’", "《", "》", "<", ">", "!", "！", "?", "？", "。", ".", ":", "：", ";", "；"])


def strip_parentheticals(s: str) -> str:
    # Remove half-width and full-width parenthetical annotations (non-nested).
    prev = None
    out = s
    while prev != out:
        prev = out
        out = re.sub(r"\([^()]*\)", " ", out)
        out = re.sub(r"（[^（）]*）", " ", out)
    return out


def to_slug(title: str) -> str:
    s = strip_parentheticals(title)
    # Replace punctuation / separators with space.
    for ch in PUNCT_STRIP:
        s = s.replace(ch, " ")
    # Common separators become dashes via space.
    s = re.sub(r"[\\/+,，、&#@*|~—§%]+", " ", s)
    # Arrows, brackets, and other YAML-unfriendly chars become space.
    s = re.sub(r"[→←↑↓⇒⇐\[\]\{\}()（）<>《》]+", " ", s)
    # Keep ASCII alphanumerics, underscores, CJK, and dashes/spaces; drop the rest.
    s = re.sub(r"[^0-9A-Za-z_\-\s一-鿿]", " ", s)
    # Collapse whitespace to single dash, lowercase ASCII.
    s = s.strip().lower()
    s = re.sub(r"\s+", "-", s)
    # Collapse repeated dashes.
    s = re.sub(r"-+", "-", s)
    return s.strip("-")


def parse(epics_path: str):
    epics: list[dict] = []
    current: dict | None = None
    seen_stories: set[tuple[int, int]] = set()

    with open(epics_path, "r", encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            m = EPIC_RE.match(line)
            if m:
                num = int(m.group(1))
                title = m.group(2).strip()
                current = {"num": num, "title": title, "stories": []}
                epics.append(current)
                continue
            m = STORY_RE.match(line)
            if m:
                epic_n = int(m.group(1))
                story_n = int(m.group(2))
                title = m.group(3).strip()
                key = (epic_n, story_n)
                if key in seen_stories:
                    # Duplicate story id — skip silently (shouldn't happen).
                    continue
                seen_stories.add(key)
                # Route story to its epic by number (handles out-of-section stories).
                target = next((e for e in epics if e["num"] == epic_n), None)
                if target is None:
                    # Epic header not yet seen — create placeholder (skipped if later emerges).
                    target = {"num": epic_n, "title": f"Epic {epic_n}", "stories": []}
                    epics.append(target)
                target["stories"].append({"epic": epic_n, "num": story_n, "title": title})
                continue

    # Sort each epic's stories by story number.
    for e in epics:
        e["stories"].sort(key=lambda s: s["num"])
    # Sort epics by number.
    epics.sort(key=lambda e: e["num"])
    return epics


def build_yaml(epics, *, project_name: str, project_key: str, tracking_system: str, story_location: str) -> str:
    date_iso = datetime.now(timezone.utc).astimezone().replace(microsecond=0).isoformat()
    header_comments = f"""# generated: {date_iso}
# last_updated: {date_iso}
# project: {project_name}
# project_key: {project_key}
# tracking_system: {tracking_system}
# story_location: {story_location}

# STATUS DEFINITIONS:
# ==================
# Epic Status:
#   - backlog: Epic not yet started
#   - in-progress: Epic actively being worked on
#   - done: All stories in epic completed
#
# Epic Status Transitions:
#   - backlog → in-progress: Automatically when first story is created (via create-story)
#   - in-progress → done: Manually when all stories reach 'done' status
#
# Story Status:
#   - backlog: Story only exists in epic file
#   - ready-for-dev: Story file created in stories folder
#   - in-progress: Developer actively working on implementation
#   - review: Ready for code review (via Dev's code-review workflow)
#   - done: Story completed
#
# Retrospective Status:
#   - optional: Can be completed but not required
#   - done: Retrospective has been completed
#
# WORKFLOW NOTES:
# ===============
# - Epic transitions to 'in-progress' automatically when first story is created
# - Stories can be worked in parallel if team capacity allows
# - SM typically creates next story after previous one is 'done' to incorporate learnings
# - Dev moves story to 'review', then runs code-review (fresh context, different LLM recommended)
"""

    meta_block = (
        f"generated: {date_iso}\n"
        f"last_updated: {date_iso}\n"
        f"project: {project_name}\n"
        f"project_key: {project_key}\n"
        f"tracking_system: {tracking_system}\n"
        f'story_location: "{story_location}"\n'
    )

    lines: list[str] = []
    lines.append(header_comments)
    lines.append(meta_block)
    lines.append("development_status:")

    used_keys: set[str] = set()
    for epic in epics:
        epic_key = f"epic-{epic['num']}"
        lines.append(f"  {epic_key}: backlog")
        for story in epic["stories"]:
            slug = to_slug(story["title"])
            base = f"{story['epic']}-{story['num']}-{slug}" if slug else f"{story['epic']}-{story['num']}"
            key = base
            suffix = 2
            while key in used_keys:
                key = f"{base}-{suffix}"
                suffix += 1
            used_keys.add(key)
            lines.append(f"  {key}: backlog")
        lines.append(f"  {epic_key}-retrospective: optional")

    return "\n".join(lines) + "\n"


def main(argv: list[str]) -> int:
    if len(argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    epics_path = argv[1]
    out_path = argv[2]

    epics = parse(epics_path)
    story_count = sum(len(e["stories"]) for e in epics)
    epic_count = len(epics)

    content = build_yaml(
        epics,
        project_name="cat",
        project_key="NOKEY",
        tracking_system="file-system",
        story_location="{project-root}/_bmad-output/implementation-artifacts",
    )

    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        f.write(content)

    print(f"epics={epic_count} stories={story_count} output={out_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
