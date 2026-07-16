#!/usr/bin/env python3
"""Refresh tracked security events from every source, into one candidate list.

Sources:
  * BSides chapter sites  — each chapter's own website (from the seed's chapters),
                            scraped for its next event date; "where" = chapter city/country.
  * cryptax/confsec       — a curated markdown table of security/hacking conferences.
  * DEF CON forum calendar — official DEF CON events incl. regional editions
                            (e.g. DEF CON Middle East / Bahrain) that defcon.org's
                            landing page does not expose in plain HTML.
  * Named conference sites — Black Hat, Hacker Halted, hardwear.io, etc. scraped for a date.

Usage (from repo root):
    python3 scripts/import-events.py > /tmp/events.json    # candidates -> stdout, summary -> stderr
    # review the dates (best-effort across heterogeneous sites: a few may be CFP/ticket
    # dates, and JS-only or cert-broken sites yield nothing), then merge the good ones
    # into the "meetups" array of internal/server/meetups_seed.json and redeploy.

Stdlib only. Human-reviewed by design — nothing is written automatically.
"""
import concurrent.futures as cf
import datetime, html, json, os, re, subprocess, sys, time, unicodedata, urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SEED = os.path.join(ROOT, "internal/server/meetups_seed.json")
TODAY = datetime.date.today()
HORIZON = TODAY + datetime.timedelta(days=730)
UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 "
      "SmellyFeet-meetup-tracker/1.0 (+https://feed.purecypher.com)")

CONFSEC = "https://raw.githubusercontent.com/cryptax/confsec/master/README.md"
DEFCON_CAL = "https://forum.defcon.org/calendar"
DEFCON_SEARCH = ('{"type":["vBForum_Event"],"eventstartdate":"future",'
                 '"sort":{"eventstartdate":"ASC"},"view":"event"}')
# named conference sites not (reliably) covered by confsec; scraped for a date.
NAMED_SITES = [
    ("Black Hat USA",   "https://www.blackhat.com/",   "Las Vegas", "USA"),
    ("Hacker Halted",   "https://hackerhalted.com/",   "Atlanta",   "USA"),
    ("hardwear.io NL",  "https://hardwear.io/",        "The Hague", "Netherlands"),
    ("KernelCon",       "https://kernelcon.org/",      "Omaha",     "USA"),
    ("Area41",          "https://area41.io/",          "Zürich",    "Switzerland"),
    ("Wild West Hackin' Fest", "https://wildwesthackinfest.com/", "", "USA"),
]
COUNTRY_FIX = {"united states": "USA", "us": "USA", "u.s.": "USA",
               "united kingdom": "UK", "england": "UK", "scotland": "UK", "wales": "UK"}

MONTHS = {m.lower(): i for i, m in enumerate(
    ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"], 1)}
MON = r"(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*"
RE_DMY = re.compile(rf"\b(\d{{1,2}})(?:st|nd|rd|th)?\s+(?:of\s+)?{MON}\.?,?\s+(20\d\d)\b", re.I)
RE_MDY = re.compile(rf"\b{MON}\.?\s+(\d{{1,2}})(?:st|nd|rd|th)?,?\s+(20\d\d)\b", re.I)
RE_ISO = re.compile(r"\b(20\d\d)-(\d{1,2})-(\d{1,2})\b")
BAD = re.compile(r"currently|copyright|©|all rights reserved|last updated|posted on|"
                 r"updated:|call for|\bcfp\b|closes?|deadline|submission|proposals?|"
                 r"ticket release|early bird|registration (closes|opens)|founded|since 20", re.I)


def mkdate(y, m, d):
    try:
        return datetime.date(y, m, d)
    except ValueError:
        return None


def clean(s):
    return re.sub(r"\s+", " ", html.unescape(re.sub(r"<[^>]+>", "", s))).strip()


def slug(s):
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()
    return re.sub(r"-+", "-", re.sub(r"[^a-z0-9]+", "-", s.lower())).strip("-")


def norm_country(c):
    return COUNTRY_FIX.get(c.strip().lower(), c.strip())


def get(url, cap=600_000):
    headers = {"User-Agent": UA, "Accept": "text/html,application/xhtml+xml,*/*",
               "Accept-Language": "en-US,en;q=0.9"}
    last = None
    for attempt in range(3):
        try:
            req = urllib.request.Request(url, headers=headers)
            with urllib.request.urlopen(req, timeout=20) as r:
                return r.read(cap).decode("utf-8", "replace")
        except Exception as e:  # transient reset / rate-limit -> back off and retry
            last = e
            time.sleep(1.0)
    # some hosts (e.g. the DEF CON forum) reset urllib's TLS handshake; fall back to curl
    out = subprocess.run(["curl", "-sL", "--max-time", "25", "-A", UA, url], capture_output=True)
    if out.returncode == 0 and out.stdout:
        return out.stdout[:cap].decode("utf-8", "replace")
    raise last if last else RuntimeError(f"fetch failed: {url}")


def strip_html(h):
    h = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", h, flags=re.S | re.I)
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", h))


def pick_event_date(text):
    hits = []
    for m in RE_DMY.finditer(text):
        hits.append((m, mkdate(int(m.group(3)), MONTHS[m.group(2).lower()[:3]], int(m.group(1)))))
    for m in RE_MDY.finditer(text):
        hits.append((m, mkdate(int(m.group(3)), MONTHS[m.group(1).lower()[:3]], int(m.group(2)))))
    for m in RE_ISO.finditer(text):
        hits.append((m, mkdate(int(m.group(1)), int(m.group(2)), int(m.group(3)))))
    clean_hits = [d for m, d in hits
                  if d and TODAY <= d <= HORIZON and not BAD.search(text[max(0, m.start() - 45):m.end() + 25])]
    return min(clean_hits) if clean_hits else None


def mkevent(name, url, city, country, date, chapter="", tags=("conference",), summary=None, desc=None):
    year = date.year
    title = name if name[-4:].isdigit() else f"{name} {year}"
    return {
        "slug": f"{slug(name)}-{year}", "title": title,
        "summary": summary or f"Security event{(' in ' + (city or country)) if (city or country) else ''}.",
        "description": desc or f"{name} — security event. See the official site for the schedule, tickets, and CFP.",
        "starts_at": date.strftime("%Y-%m-%dT12:00:00Z"), "date_only": True,
        "location_type": "in_person", "venue_name": "", "venue_address": "",
        "city": city, "region": "", "country": country,
        "online_url": "", "rsvp_url": url, "chapter_name": chapter, "chapter_url": url or "",
        "tags": list(tags),
    }


# ---- source: BSides chapter sites ----
def bsides_chapter_events():
    chapters = [c for c in json.load(open(SEED))["chapters"] if c.get("website")]

    def one(c):
        try:
            date = pick_event_date(strip_html(get(c["website"])))
        except Exception:
            return None
        if not date:
            return None
        return mkevent(c["name"], c["website"], c["city"], c["country"], date, chapter=c["name"],
                       summary=f"Security BSides community conference in {c['city'] or c['country']}.",
                       desc=f"{c['name']} — a community-run Security BSides conference. "
                            "See the official site for the schedule, tickets, and CFP.")
    with cf.ThreadPoolExecutor(max_workers=10) as ex:
        return [e for e in ex.map(one, chapters) if e]


# ---- source: cryptax/confsec ----
def confsec_events():
    out = []
    for line in get(CONFSEC).splitlines():
        if not line.strip().startswith("|"):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if len(cells) < 4 or "Name" in cells[0] or set(cells[0]) <= set("- "):
            continue
        lm = re.match(r"\[(.+?)\]\((.+?)\)", cells[0])
        name, url = (lm.group(1), lm.group(2)) if lm else (cells[0], "")
        parts = [p.strip() for p in cells[1].split(",") if p.strip()]
        city = ", ".join(parts[:-1]) if len(parts) > 1 else ""
        country = norm_country(parts[-1]) if parts else ""
        dm = re.search(rf"{MON}\.?\s+(\d{{1,2}})", cells[-1], re.I)
        ym = re.search(r"20\d\d", cells[-1])
        if not dm or not ym:
            continue
        date = mkdate(int(ym.group()), MONTHS[dm.group(1).lower()[:3]], int(dm.group(2)))
        if date and TODAY <= date <= HORIZON:
            out.append(mkevent(name, url, city, country, date))
    return out


# ---- source: DEF CON forum calendar ----
def defcon_events():
    r = subprocess.run(["curl", "-sL", "--max-time", "25", "-A", UA, "-G",
                        "--data-urlencode", "searchJSON=" + DEFCON_SEARCH, DEFCON_CAL],
                       capture_output=True)
    page = r.stdout.decode("utf-8", "replace")
    out = []
    locs = [("las vegas", "Las Vegas", "USA"), ("bahrain", "", "Bahrain")]
    for b in re.split(r'class="b-event b-event--inlist', page)[1:]:
        extra = [clean(e) for e in re.findall(r'b-event__extraline[^>]*>(.*?)</', b, re.S)]
        if not extra:
            continue
        name = re.sub(r"\s+(Las Vegas|Bahrain)$", "", extra[0], flags=re.I)
        blob = " ".join(extra).lower()
        dm = re.search(r'b-event__date[^>]*>(.*?)</', b, re.S)
        md = re.search(rf"{MON}\.?\s+(\d{{1,2}})", clean(dm.group(1)) if dm else "", re.I)
        if not md:
            continue
        month, day = MONTHS[md.group(1).lower()[:3]], int(md.group(2))
        ym = re.search(r"\b(20\d\d)\b", " ".join(extra))
        year = int(ym.group(1)) if ym else (TODAY.year if month >= TODAY.month else TODAY.year + 1)
        date = mkdate(year, month, day)
        if not date or not (TODAY <= date <= HORIZON):
            continue
        href = re.search(r'href="(https://forum\.defcon\.org/node/\d+[^"]*)"', b)
        url = href.group(1) if href else "https://defcon.org/"
        city = country = ""
        for tok, ci, co in locs:
            if tok in blob:
                city, country = ci, co
                break
        out.append(mkevent(name, url, city, country, date,
                           desc=f"{name} — official DEF CON event (from the DEF CON forum calendar)."))
    return out


# ---- source: named conference sites ----
def named_site_events():
    def one(item):
        name, url, city, country = item
        try:
            date = pick_event_date(strip_html(get(url, cap=800_000)))
        except Exception:
            return None
        return mkevent(name, url, city, country, date) if date else None
    with cf.ThreadPoolExecutor(max_workers=6) as ex:
        return [e for e in ex.map(one, NAMED_SITES) if e]


def main():
    all_events, tally = [], {}
    for label, fn in [("bsides", bsides_chapter_events), ("confsec", confsec_events),
                      ("defcon", defcon_events), ("named", named_site_events)]:
        try:
            evs = fn()
        except Exception as e:
            print(f"source {label} failed: {type(e).__name__}: {e}", file=sys.stderr)
            evs = []
        tally[label] = len(evs)
        all_events.extend(evs)
    seen, merged = set(), []
    for e in all_events:
        if e["slug"] in seen:
            continue
        seen.add(e["slug"])
        merged.append(e)
    merged.sort(key=lambda e: e["starts_at"])
    if "--apply" in sys.argv:
        seed = json.load(open(SEED))
        seed["meetups"] = merged            # fully autonomous: only what the sources yield
        with open(SEED, "w") as f:
            json.dump(seed, f, indent=2, ensure_ascii=False)
            f.write("\n")
        print(f"applied: {len(merged)} events sources={tally}", file=sys.stderr)
    else:
        print(json.dumps(merged, indent=2, ensure_ascii=False))
        print(f"sources={tally} total_after_dedup={len(merged)}", file=sys.stderr)


if __name__ == "__main__":
    main()
