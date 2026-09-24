# The vault, explained

Your log is a folder. Everything in it is a plain file you can open, copy, grep, put in
git, or hand to any program. This page shows what is there and how to read it, with real
examples. The contract the app follows is `SCHEMA.md` inside the vault itself.

## The folder

    Poiesis Vault/
      config.json                 settings: language, extractor, theme, entry limit …
      _index.md                   the map of the vault, rewritten after every entry
      _log.md                     what the app did, one line per action, never edited
      inbox/                      drop a video here to process it
      raw/2026/09/                the recordings, never touched again:
        2026-09-02T14-34_288b….mp4        the video, named by time and content hash
        2026-09-02T14-34_288b….json       what was known at recording time
        2026-09-02T14-34_288b….words….json  every word with its second
        2026-09-02T14-34_288b….wav          the sound the words were heard from, a cache
        ….silent  ….log  ….extract.jsonl    no words were heard; what the speech model and the sorting did
      episodes/
        2026-09-02-a.md                   one page per entry
        2026-09-02-a.claims.jsonl         that entry's claims, one per line
      entities/
        erik.md                           one page per person, project, mission, place …
      trash/                      entries you removed: page, video, claims, moved not deleted

Three ideas hold it together. **Time is the join key:** every entry has a recorded time,
every claim carries it, every claim points at a second in the video. **The video is the
truth:** pages and claims are derived from it and can be rebuilt. **Nothing is edited in
place:** claims are appended, a newer claim can supersede an older one, and the entity
pages are rewritten from the claims after every entry.

## An entry page

`episodes/2026-09-02-a.md` starts with a header any tool can parse, then the words with
their times, then the claims:

    ---
    id: 2026-09-02-a
    day: 1
    recorded_at: "2026-09-02T14:34:56+02:00"
    duration_s: 90.0
    media: raw/2026/09/2026-09-02T14-34_288b865b854833c4.mp4
    transcript: raw/2026/09/2026-09-02T14-34_288b865b854833c4.words.ggml-large-v3-turbo-q5_0.auto.json
    language: auto
    detected: sv
    mission: bageriet
    entities: [bageriet, erik, hotell, ugn]
    claims: 14
    extractor: ollama:qwen3.5:4b/extract-v5
    ---

The id is the date plus a letter. `day` counts days since the first entry. `mission` is
the mission's id, or empty for a free run. `prompt` is there only when the entry was opened
with a line to talk about (see Links): what the person set out to talk about.

## A claim

One line of `episodes/2026-09-02-a.claims.jsonl`:

    {"id":"clm:01M1H47NZ8NQMZWJKPMTBA0RSF",
     "kind":"event",
     "text":"I will meet Erik tomorrow to talk about the bakery in Uppsala.",
     "quote":"Imorgon träffar jag Erik för att prata om bageriet i Uppsala.",
     "about":["bageriet","erik"],
     "stated_at":"2026-09-02T14:34:56+02:00",
     "source":{"episode":"2026-09-02-a","start":6.4,"end":8.7},
     "supersedes":null, "valid_to":null, "due":null, "outcome":null,
     "confidence":1,
     "extracted_at":"2026-09-02T13:16:48Z",
     "extractor":"ollama:qwen3.5:4b/extract-v5"}

- `text` is the claim in plain English, said in the first person.
- `quote` is the exact words spoken, in the language spoken; the app checks they exist
  in the transcript, or the claim is dropped.
- `kind` is one of eight: event, state, belief, decision, intention, prediction,
  question, lesson.
- `about` names the pages this claim belongs to.
- `stated_at` is when it was said; `extracted_at` and `extractor` say who wrote it down
  and when. Two times, so a re-extraction is never confused with a new statement.
- `supersedes`, `valid_to`, `due` and `outcome` are kept for a claim that replaces, ends or
  settles an older one. They are part of the format; the app does not fill them in yet.
- `source` is the moment: the entry and the seconds. `poiesis://2026-09-02-a?t=6.4` opens it.

## A page for a person or thing

`entities/erik.md` is rewritten from the claims after every entry:

    ---
    id: erik
    kind: person
    aliases: [Erik]
    first_seen: "2026-09-01"
    last_seen: "2026-09-02"
    mentions: 6
    ---
    # Erik
    person · 6 claims · 2026-09-01 → 2026-09-02
    ## Timeline
    - 2026-09-02 · **event** · I will meet Erik tomorrow … — "Imorgon träffar jag Erik …" · [[2026-09-02-a]] [00:06](poiesis://2026-09-02-a?t=6.4)

Kinds: person, project, mission, theme, place, org, habit. Missions are entities too:
`entities/bageriet.md` lists every entry recorded under it. The special page `self`
collects claims about you: moods, states, beliefs.

## Reading it without the app

Obsidian opens the folder as a vault; the graph view draws the links. From a shell:

    # every claim about erik, newest first
    cat episodes/*.claims.jsonl | jq -c 'select(.about | index("erik"))' | sort -r

    # what did I decide in August?
    cat episodes/2026-08-*.claims.jsonl | jq -r 'select(.kind=="decision") | .text'

    # all my beliefs, with the second they were said
    cat episodes/*.claims.jsonl | jq -r 'select(.kind=="belief") | "\(.stated_at[:10])  \(.text)  →  \(.source.episode) @\(.source.start)"'

In Python, `json.loads` per line and `yaml` for the page headers is all it takes.

## Reading it from an AI

`poiesis mcp` serves the vault to any AI app that speaks MCP, on your machine. It never writes.
`poiesis setup --mcp` connects Claude Code in one step; `poiesis mcp --connect` prints the lines
for Claude Desktop and Cursor. Five verbs:

| verb | what it gives | when to use it |
|---|---|---|
| `orient` | the shape of the vault: how many entries, which missions and people, the last days, the contract | first, always, once |
| `search` | claims and transcript lines that match words, with filters for dates, kind, person, mission | "what did I say about the oven in August" |
| `read` | one page as it is: an entry, a person, the index, the schema, the log | when a hit needs its context |
| `moment` | the words around one second of one entry, and the link that opens it | "show me the moment I said that" |
| `record` | opens Poiesis on the record screen, ready, with a line to talk about and the camera off. The person's first key turns it on; space records | "I want to log how the launch went" |

Answers are capped so the AI never drowns: 16,000 characters, 50 hits. A good first
question: "Orient yourself in my log, then tell me what I said about X and show me the
moment."

## Links

Two links open Poiesis from anywhere: a note, a shortcut, a calendar event, an AI.

    poiesis://record?about=how%20the%20launch%20went   open ready to record, with a line to talk about
    poiesis://2026-09-02-a?t=63.2                      open an entry at that second

On a Mac with the app they work wherever links work. Everywhere, `poiesis open <link>` does
the same. `about` is one line of at most 120 letters: shown on the record screen and saved as
the entry's `prompt`. Anything else is refused.

A link never starts a recording, and never turns the camera on. Opening the record screen,
with the camera off, is as far as any link, script or AI goes: the person's first key turns
the camera on, and space records. A link that arrives while an entry is being recorded is
not followed.

## Changing the format

The format is MIT licensed so other tools can use it. Changes are additive: new fields
may appear, existing ones keep their meaning, old vaults always open. `SCHEMA.md` inside
each vault explains the format as it was when that vault was made.
