# Security

Poiesis keeps people's recordings and words. A flaw that could expose them matters more than
any other bug.

## Report a problem privately

Use GitHub's private report: the **Security** tab of this repository, then **Report a
vulnerability**. Only the maintainer sees it. Please do not open a public issue for a
security problem.

Say what you found, how to see it happen, and which version (`poiesis version`). You get an
answer within a week; a fix for a real problem goes out as a new release, and every installed
copy picks it up within the hour.

## What is in scope

- Anything that sends a recording, a transcript or a claim anywhere the person did not choose.
- The update and install path: a way to make Poiesis install files that are not the
  release's, or to skip the checksum.
- The local socket between the window and the core, and the AI connection (MCP), which must
  stay read-only.
- Links (`poiesis://`): a link that starts a recording, changes the log, or does anything
  but open the record screen ready or an entry at a moment.

## Supported versions

The newest release. Poiesis updates itself, so fixes go out as new releases only.
