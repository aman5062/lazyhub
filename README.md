# lazyhub

A **terminal UI for GitHub Projects**. Browse your project boards, see tickets
grouped by status column, **assign them to people**, and **move them between
columns** — all from the keyboard. Log in once, then never again.

```
 lazyhub   @aman5062  ·  Projects
  Product Roadmap  (@aman5062)

  Todo
  ○ #14 Wire billing webhooks         unassigned    acme/gateway
▶ ○ #15 Design onboarding flow        @aman5062     acme/home
  In Progress
  ⇄ #22 Refactor auth middleware      @teammate     acme/gateway
  Done
  ○ #9  Set up CI                      @aman5062     acme/tools
  ↑/↓ move · ↵ details · a assign · s column · o open · esc back
```

## What it does

- **Projects list** — every board you own, plus your orgs' boards.
- **Kanban board** — tickets in colored columns (your real Status options), a
  distribution bar with `% done`, and avatar chips per assignee.
- **`enter` details** — open a ticket to read its **full description**, labels,
  milestone, who opened it and when, and the **latest comments** — the whole
  story of a task without leaving the terminal. Scroll with `↑`/`↓`; comment
  with `c` right there.
- **`n` new** — create a draft ticket straight onto the board.
- **`i` issue** — open a **real repo issue** (pick the repo) and drop it onto
  the board in one step — not just a draft.
- **`a` assign** — toggle who's assigned to the selected ticket.
- **`s` status** — move a ticket to another column.
- **`p` field** — set any single-select field (Priority, Size, …), options
  synced live from your board.
- **`c` comment** — add a comment to the ticket.
- **`f` filter** — narrow the board to one assignee's cards; **`m` mine** is the
  one-key shortcut for your own.
- **Auto-sync** — the board silently refreshes every 30s (with cursor kept in
  place), so a teammate's new ticket shows up on its own.
- **`o` open** — jump to the ticket on github.com.

## Auth — read this

lazyhub needs a **GitHub token** (an SSH key can't talk to the API). The token
is kept in your **OS keychain** when one is available (GNOME Keyring / libsecret
on Linux, Keychain on macOS, Credential Manager on Windows); non-secret metadata
lives at `~/.config/lazyhub/auth.json`. On headless boxes with no keychain,
lazyhub falls back to that same file (perms `0600`). Either way you log in once.

> **Important:** GitHub Projects (v2) are **GraphQL-only** and require the
> **`project`** scope. A `repo`-only token will not see your boards.

### Option A — Personal Access Token (works immediately)

1. Create a token: https://github.com/settings/tokens
   Scopes: **`project`**, `repo`, `read:org`
2. `lazyhub login` → choose **1** → paste it.

### Option B — Browser device flow (no token to paste)

`gh`-style: lazyhub prints a short code, you approve in the browser, done — no
copy-pasting secrets. Works out of the box (a public OAuth App Client ID ships
with lazyhub). To use your own app instead, set `LAZYHUB_CLIENT_ID`.

## Install

Pick whichever fits you — the first needs **neither Go nor git**.

### 1. Prebuilt binary (recommended)

Download the archive for your OS/arch from the
[**latest release**](https://github.com/aman5062/lazyhub/releases/latest),
extract it, and put `lazyhub` on your `PATH`.

**Linux (amd64):**
```bash
curl -L https://github.com/aman5062/lazyhub/releases/latest/download/lazyhub_linux_amd64.tar.gz | tar xz
sudo mv lazyhub /usr/local/bin/
lazyhub
```

**macOS (Apple Silicon):**
```bash
curl -L https://github.com/aman5062/lazyhub/releases/latest/download/lazyhub_darwin_arm64.tar.gz | tar xz
sudo mv lazyhub /usr/local/bin/
lazyhub
```
> On macOS, if Gatekeeper blocks it: `xcode-select --install` isn't needed —
> just run `xattr -d com.apple.quarantine ./lazyhub` once.

**Windows:** download `lazyhub_windows_amd64.zip` from the release, unzip, and
run `lazyhub.exe` (or add its folder to `PATH`).

Available archives: `linux_amd64`, `linux_arm64`, `darwin_amd64`,
`darwin_arm64`, `windows_amd64`, `windows_arm64`. Verify with
`checksums.txt` if you like.

### 2. `go install` (Go 1.23+)

The native one-liner for Go tools — no manual cloning:
```bash
go install github.com/aman5062/lazyhub@latest
lazyhub   # from ~/go/bin (add it to PATH if needed)
```

### 3. Build from source (for developing lazyhub)

```bash
git clone https://github.com/aman5062/lazyhub.git
cd lazyhub
go build -o lazyhub .
./lazyhub
```

Check your version anytime with `lazyhub version`.

## Commands

```
lazyhub          Launch the TUI (prompts login on first run)
lazyhub login    Authenticate
lazyhub logout   Remove stored token
lazyhub whoami   Show current account
lazyhub update   Update to the latest release in place (alias: upgrade)
lazyhub version  Print the version
```

lazyhub checks for a newer release on startup and shows an unobtrusive
`⬆ vX available` notice; run `lazyhub update` to self-update.

On launch you get a brief branded welcome screen (wordmark, a greeting, and a
rotating tip) while your boards load in the background — press any key to jump
straight in once it's ready.

## Keybindings

| Screen | Key | Action |
|---|---|---|
| Projects | `enter` | Open the board |
| Projects | `/` | Filter boards by name |
| Projects | `o` | Open board on github.com |
| Board | `←`/`→` (`h`/`l`) | Move between columns |
| Board | `↑`/`↓` (`k`/`j`) | Move between cards |
| Board | `enter` | Open ticket details (body + comments) |
| Board | `n` | Create a new draft ticket |
| Board | `i` | Open a real repo issue onto the board |
| Board | `a` | Assign / unassign the card |
| Board | `s` | Move the card to another column |
| Board | `p` | Set a field (Priority, Size, …) |
| Board | `c` | Add a comment to the ticket |
| Board | `f` | Filter the board by assignee |
| Board | `m` | Toggle: show only my tickets |
| Board | `o` | Open ticket in browser |
| Board | `r` | Refresh now (also auto every 30s) |
| Board | `esc` | Back to projects |
| Details | `↑`/`↓` (`k`/`j`) | Scroll the body & comments |
| Details | `c` | Add a comment |
| Details | `a` | Assign / unassign |
| Details | `o` | Open the ticket in the browser |
| Details | `r` | Reload the ticket |
| Details | `esc`/`q` | Back to the board |
| Any | `?` | Toggle help overlay |
| Any | `X` | Log out (press twice) |
| Any | `ctrl+c` | Quit |

## Roadmap

- [x] View a ticket's full description & comments inline (press `enter`)
- [x] Filter board by assignee (press `f`; `m` for your own)
- [x] Create a *real* repo issue (not just a draft) from the board (press `i`)
- [x] OS keychain storage (libsecret / Keychain / WinCred, with file fallback)
- [x] Prebuilt release binaries (see Releases)

## License

MIT
