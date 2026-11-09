# Gocoin - instructions for Claude Code

## Changelog

Every time you modify the code in this repo, also update `changelog.txt` (in the repo's root) as a part of the same change.

- The newest version is the first section of the file (e.g. `1.11.1:`) - this is the current one.
- Append the new entries at the **end** of the current section, i.e. just before the empty line that precedes the next version's header. Never add them at the top of the section and never touch the older sections.
- One line per change, in the same format as the existing entries: `* <Area>: <description>`
  - `<Area>` names the affected part, e.g. `Client`, `Client/Net`, `Client/WebUI`, `Client/TextUI`, `Client/RPC`, `Lib/chain`, `Lib/btc`, `Lib/Script`, `Wallet`, `Tools`.
  - Describe what has changed from the user's/operator's point of view; for a bug fix say what was wrong and how it could show up.
- Group changes that are about a similar thing into a single line, rather than adding a separate line for each of them. This includes extending a line that you have added earlier in the same (uncommitted) work, instead of appending another one about the same subject.
- Do not create a new version section and do not add a date to the current one - the author does it when releasing.
- Changes that affect nothing in the shipped software (e.g. edits of this file) do not need an entry.
