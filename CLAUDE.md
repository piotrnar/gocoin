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

## Investigating an issue with a running node

When asked to debug the state of a client that is currently running (e.g. stuck behind the chain tip, dropping peers, hanging), inspect the live process first, then explain what you see against the source code, and finally propose a patch that stops the issue from happening again.

### Inspecting the live node

The broken state usually lives only in the process' memory, so collect the evidence before anything else - and never restart, quit or kill the node, unless asked to.

- The nodes run inside `screen` sessions (`screen -ls`), started from the `client/` folder. The config file is `client/gocoin.conf`, unless `GOCOIN_CLIENT_CONFIG` points to another one (e.g. `testnet4.conf`, see `client/t4cfg.sh`). It tells the WebUI port (`WebUI.Interface`) and the data folder (`Datadir`).
- WebUI - the JSON endpoints are read-only and need no session: `curl -s 127.0.0.1:<port>/status.json`, and likewise `system.json`, `counts.json`, `netcon.json`, `blocks.json` (the full list is in `client/usif/webui/webui.go`).
  - The node does not support IPv6 yet, so always use IPv4: `127.0.0.1`, not `localhost` (which may resolve to `::1` and then the WebUI returns an empty `200` response).
  - `counts.json` holds the internal debug counters (`common.CountSafe`). They are the best evidence of which code paths were taken - grep the source for the counter's name.
- TextUI - type a command into the screen session and then dump its scrollback to a file:
  - `screen -S <session> -X stuff "<cmd>^M"`, wait a second or two, then `screen -S <session> -X hardcopy -h <file>`
  - Useful commands: `i` (general info), `b` (blockchain stats), `pend` (blocks to get), `cach` (cached blocks and what they link to), `ss` (sync stats), `n` (peers; `n <id>` for details of one), `c [prefix]` (counters). All the commands are registered with `newUi()` in `client/usif/textui/`.
  - Use only the commands that display things. Do not use the ones changing the node's state (`undo`, `redo`, `stop`, `purge`, `mpurge`, `drop`, `ban`, `cfg`, `quit`, `kill`, etc.), unless asked to.
- When it helps to know which chain is the real one, compare the block hashes at the given heights with a public block explorer (e.g. `https://mempool.space/testnet4/api/block-height/<height>`, or without `testnet4/` for mainnet).
- Check what the binary was built from (TextUI command `bi`), as the running code may differ from the working tree.

### From the evidence to a patch

- Follow the observed state and the counters through the source code until the root cause is found - not only what keeps the node in the bad state, but also what has put it there in the first place.
- Say clearly which parts of the diagnosis are confirmed (by the code and the collected evidence) and which are only inferred.
- Implement the fix in the working tree (with the changelog entry), make sure it builds (`cd client && go build`) and passes `go vet` and `go test` for the changed packages. Do not commit it and do not restart the node with it - leave both to the author.
- Say whether a restart alone recovers the node, and whether the patch is needed for that or only prevents the issue from recurring.
