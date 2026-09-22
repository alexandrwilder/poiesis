package main

const schemaMD = `# SCHEMA.md — the contract for this vault

Read this once. Everything in the vault follows it.

## What is here

    config.json               settings for this vault (language, extractor, model paths)
    _index.md              map of content, rewritten by the app after every ingest
    _log.md                append-only log of every operation
    inbox/                 drop recordings here; ingest moves them into raw/
    raw/YYYY/MM/           the recordings, named <time>_<hash>.mp4, never edited, plus
                           a .json sidecar, a .wav cache and .words.<model>.<lang>.json transcripts
    episodes/<id>.md       one page per recording: frontmatter, transcript with times, claims
    episodes/<id>.claims.jsonl   the facts of that recording, one claim per line, append-only
    entities/<id>.md       one page per person, project, mission, theme, place, org or habit,
                           rewritten from the claims after every ingest
    periods/               week, month and year digests (later)

## Ids

- episode id: date plus a letter, e.g. 2026-09-02-a. Wikilink: [[2026-09-02-a]]
- entity id: lowercase slug, e.g. erik, bageriet-uppsala. Wikilink: [[erik]]
- claim id: clm:<ULID>, time-sortable, never reused
- a moment: log://<episode>?t=<seconds>  (in the pages: [01:03](log://2026-09-02-a?t=63.2))

Time is the join key: every episode has recorded_at with a time zone; every claim carries
stated_at (copied from its episode) and source.start/end in seconds.

## A claim (one JSON line)

    id            clm:<ULID>
    kind          event | state | belief | decision | intention | prediction | question | lesson
    text          one first-person sentence in English
    quote         the exact words from the transcript, original language (the evidence)
    about         one to three entity ids
    stated_at     when it was said (event time)
    valid_to      null until a later claim supersedes this one
    supersedes    the older claim this replaces, or null
    due           intentions and predictions only: the scoring date, or null
    outcome       null | met | missed | abandoned | unknown (written by a later pass)
    source        { episode, start, end }  seconds into the recording
    confidence    0 to 1
    extracted_at  when it was extracted (ingestion time)
    extractor     which model and prompt version produced it

The page "self" holds claims about the person talking and every state with no other topic;
its timeline is the mood and energy series.

Claims are never edited. A changed mind is a new claim with supersedes set.
Delete every page and every claims file, keep raw/, run ingest again: you get it all back.

## Rules for readers (humans and AIs)

1. Start with _index.md, then an entity page, then an episode page around the second you need.
2. Every fact you quote from this vault must carry its episode id and seconds.
3. Nothing in the pages was written by a model except the claims themselves.
`
