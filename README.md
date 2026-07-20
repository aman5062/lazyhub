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
- **Board view** — tickets grouped by their **Status** column.
- **`a` assign** — toggle who's assigned to the selected ticket.
- **`s` move column** — change a ticket's Status (Todo → In Progress → Done).
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
| Board | `←`/`→` (`h`/`l`) | Move between columns |
| Board | `↑`/`↓` (`k`/`j`) | Move between cards |
| Board | `a` | Assign / unassign the card |
| Board | `s` | Move the card to another column |
| Board | `m` | Toggle: show only my tickets |
| Board | `o` | Open ticket in browser |
| Board | `r` | Refresh |
| Board | `esc` | Back to projects |
| Any | `?` | Toggle help overlay |
| Any | `ctrl+c` | Quit |

## Roadmap

- [ ] Create a ticket (draft or real issue) from the board
- [ ] Edit custom fields beyond Status (priority, iteration, etc.)
- [ ] Comment on a ticket inline
- [ ] Filter board by assignee / status
- [ ] OS keychain storage (libsecret / Keychain / WinCred)
- [ ] Prebuilt release binaries + `brew` / `scoop`

## License

MIT
