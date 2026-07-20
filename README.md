# lazyhub

A **terminal UI for GitHub Projects**. Browse your project boards, see tickets
grouped by status column, **assign them to people**, and **move them between
columns** — all from the keyboard. Log in once, then never again.

```
 lazyhub   @aman5062  ·  Projects
  Ananta HQ Roadmap  (@aman5062)

  Todo
  ○ #14 Wire billing webhooks         unassigned    ananta-hq/gateway
▶ ○ #15 Design onboarding flow        @aman5062  ananta-hq/home
  In Progress
  ⇄ #22 Refactor auth middleware      @teammate     ananta-hq/gateway
  Done
  ○ #9  Set up CI                      @aman5062  ananta-hq/tools
  ↑/↓ move · a assign · s move column · o open · r refresh · esc back
```

## What it does

- **Projects list** — every board you own, plus your orgs' boards.
- **Kanban board** — tickets in colored columns (your real Status options), a
  distribution bar with `% done`, and avatar chips per assignee.
- **`n` new** — create a draft ticket straight onto the board.
- **`a` assign** — toggle who's assigned to the selected ticket.
- **`s` status** — move a ticket to another column.
- **`p` field** — set any single-select field (Priority, Size, …), options
  synced live from your board.
- **`c` comment** — add a comment to the ticket.
- **`m` mine** — filter to just your tickets.
- **Auto-sync** — the board silently refreshes every 30s (with cursor kept in
  place), so a teammate's new ticket shows up on its own.
- **`o` open** — jump to the ticket on github.com.

## Auth — read this

lazyhub needs a **GitHub token** (an SSH key can't talk to the API). It's
stored at `~/.config/lazyhub/auth.json` (perms `0600`), so you log in once.

> **Important:** GitHub Projects (v2) are **GraphQL-only** and require the
> **`project`** scope. A `repo`-only token will not see your boards.

### Option A — Personal Access Token (works immediately)

1. Create a token: https://github.com/settings/tokens
   Scopes: **`project`**, `repo`, `read:org`
2. `lazyhub login` → choose **1** → paste it.

### Option B — Browser device flow (the polished path)

`gh`-style: lazyhub prints a code, you approve in the browser. Needs a one-time
GitHub **OAuth App** (Device Flow enabled). Set its Client ID via
`LAZYHUB_CLIENT_ID` or bake it into `internal/auth/auth.go`.

## Install

```bash
git clone git@github.com:aman5062/lazyhub.git
cd lazyhub
go build -o lazyhub .   # needs Go 1.23+
./lazyhub
```

## Commands

```
lazyhub          Launch the TUI (prompts login on first run)
lazyhub login    Authenticate
lazyhub logout   Remove stored token
lazyhub whoami   Show current account
```

## Keybindings

| Screen | Key | Action |
|---|---|---|
| Projects | `enter` | Open the board |
| Projects | `/` | Filter boards by name |
| Projects | `o` | Open board on github.com |
| Board | `←`/`→` (`h`/`l`) | Move between columns |
| Board | `↑`/`↓` (`k`/`j`) | Move between cards |
| Board | `n` | Create a new draft ticket |
| Board | `a` | Assign / unassign the card |
| Board | `s` | Move the card to another column |
| Board | `p` | Set a field (Priority, Size, …) |
| Board | `c` | Add a comment to the ticket |
| Board | `m` | Toggle: show only my tickets |
| Board | `o` | Open ticket in browser |
| Board | `r` | Refresh now (also auto every 30s) |
| Board | `esc` | Back to projects |
| Any | `?` | Toggle help overlay |
| Any | `X` | Log out (press twice) |
| Any | `ctrl+c` | Quit |

## Roadmap

- [ ] View existing comments inline (currently add-only)
- [ ] Filter board by assignee / status
- [ ] Create a *real* repo issue (not just a draft) from the board
- [ ] OS keychain storage (libsecret / Keychain / WinCred)
- [x] Prebuilt release binaries (see Releases)

## License

MIT
