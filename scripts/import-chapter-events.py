#!/usr/bin/env python3
"""Refresh tracked meetup events by scraping each BSides chapter's own website
for its next event date. Best-effort, offline, human-reviewed.

Usage (from repo root):
    python3 scripts/import-chapter-events.py > /tmp/events.json    # candidates -> stdout
    # review the dates (heterogeneous sites; a few may be CFP/ticket dates or JS-only
    # pages that yield nothing), then merge the good ones into the "meetups" array of
    # internal/server/meetups_seed.json and redeploy.

Stdlib only. "Where" comes from the chapter record (city/country from bsides.org);
this only scrapes the "when". Uses default TLS verification — chapters whose sites
have expired/invalid certs (or are unreachable, or render the date only via JS) are
skipped and reported on stderr, not guessed.
"""
import concurrent.futures as cf
import datetime, json, os, re, sys, unicodedata, urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SEED = os.path.join(ROOT, "internal/server/meetups_seed.json")
TODAY = datetime.date.today()
HORIZON = TODAY + datetime.timedelta(days=730)
UA = "SmellyFeet-meetup-tracker/1.0 (+https://feed.purecypher.com)"

MONTHS = {m.lower(): i for i, m in enumerate(
    ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"], 1)}
MON = r"(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*"
RE_DMY = re.compile(rf"\b(\d{{1,2}})(?:st|nd|rd|th)?\s+(?:of\s+)?{MON}\.?,?\s+(20\d\d)\b", re.I)
RE_MDY = re.compile(rf"\b{MON}\.?\s+(\d{{1,2}})(?:st|nd|rd|th)?,?\s+(20\d\d)\b", re.I)
RE_ISO = re.compile(r"\b(20\d\d)-(\d{1,2})-(\d{1,2})\b")
# context meaning "this date is NOT the event start" -> drop it
BAD = re.compile(r"currently|copyright|©|all rights reserved|last updated|posted on|"
                 r"updated:|call for|\bcfp\b|closes?|deadline|submission|proposals?|"
                 r"ticket release|early bird|registration (closes|opens)|founded|since 20", re.I)


def mkdate(y, m, d):
    try:
        return datetime.date(y, m, d)
    except ValueError:
        return None


def pick_event_date(text):
    hits = []
    for m in RE_DMY.finditer(text):
        hits.append((m, mkdate(int(m.group(3)), MONTHS[m.group(2).lower()[:3]], int(m.group(1)))))
    for m in RE_MDY.finditer(text):
        hits.append((m, mkdate(int(m.group(3)), MONTHS[m.group(1).lower()[:3]], int(m.group(2)))))
    for m in RE_ISO.finditer(text):
        hits.append((m, mkdate(int(m.group(1)), int(m.group(2)), int(m.group(3)))))
    clean = [d for m, d in hits
             if d and TODAY <= d <= HORIZON and not BAD.search(text[max(0, m.start() - 45):m.end() + 25])]
    return min(clean) if clean else None


def slugify(s):
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()
    return re.sub(r"-+", "-", re.sub(r"[^a-z0-9]+", "-", s.lower())).strip("-")


def fetch_date(chapter):
    try:
        req = urllib.request.Request(chapter["website"], headers={"User-Agent": UA})
        with urllib.request.urlopen(req, timeout=10) as r:
            html = r.read(600_000).decode("utf-8", "replace")
        text = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", html, flags=re.S | re.I)
        text = re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", text))
        return chapter, pick_event_date(text), None
    except Exception as e:
        return chapter, None, type(e).__name__


def main():
    chapters = [c for c in json.load(open(SEED))["chapters"] if c.get("website")]
    events, seen, errors, nodate = [], set(), 0, 0
    with cf.ThreadPoolExecutor(max_workers=10) as ex:
        for chapter, date, err in ex.map(fetch_date, chapters):
            if err:
                errors += 1
                continue
            if not date:
                nodate += 1
                continue
            base = slugify(chapter["name"]) or "bsides"
            slug, n = f"{base}-{date.year}", 2
            while slug in seen:
                slug, n = f"{base}-{date.year}-{n}", n + 1
            seen.add(slug)
            events.append({
                "slug": slug, "title": f"{chapter['name']} {date.year}",
                "summary": f"Security BSides community conference in {chapter['city'] or chapter['country']}.",
                "description": f"{chapter['name']} — a community-run Security BSides conference. "
                               "See the official site for the schedule, tickets, and CFP.",
                "starts_at": date.strftime("%Y-%m-%dT12:00:00Z"), "date_only": True,
                "location_type": "in_person", "venue_name": "", "venue_address": "",
                "city": chapter["city"], "region": "", "country": chapter["country"],
                "online_url": "", "rsvp_url": chapter["website"],
                "chapter_name": chapter["name"], "chapter_url": chapter["website"],
                "tags": ["conference"],
            })
    events.sort(key=lambda e: e["starts_at"])
    print(json.dumps(events, indent=2, ensure_ascii=False))
    print(f"chapters_with_site={len(chapters)} events={len(events)} "
          f"no_date={nodate} errors={errors}", file=sys.stderr)


if __name__ == "__main__":
    main()
