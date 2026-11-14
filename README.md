# Monogatari 物語

A multiplayer Japanese sentence-ordering game, played like a visual novel. Players of different JLPT levels join one room, and an AI writes a short story for them. Each player is one character. On your turn you put the words of your line in order, read it aloud, and answer a quiz about it. Everyone else watches live and helps by voice. It is made to be played on a voice call, for example on Discord, and a game takes about 10 to 15 minutes.

Go server, [Datastar](https://data-star.dev) UI, [templ](https://templ.guide) views, stories from [OpenRouter](https://openrouter.ai).

## How a game works

1. **Join.** Enter a name and pick a level on a 15-step scale, from Low N5 to High N1.
2. **Ready vote.** The game starts when 2 or more players are connected and all pressed Ready.
3. **Generation.** The server writes the story, then the full dialogue, with every line at its speaker's level.
4. **Briefing.** Each player sees a goal, one thing to teach the others, key words and a short reading.
5. **Turns.** Strict rotation. Order the word blocks, check, read the sentence aloud, answer a quiz. Wrong answers and wrong orders can be retried or shown, never punished.
6. **Wrap-up.** Full story, stars, missed lines and every player's word list.

The in-app **How to play** button explains the same in more detail.

## Requirements

- Go 1.26 or newer
- [templ](https://templ.guide/quick-start/installation) (`go install github.com/a-h/templ/cmd/templ@latest`), only to edit `views.templ`
- An OpenRouter API key for real stories. Without one the app plays a built-in mock story, which is enough to try the game and work on the UI.

## Setup

```sh
git clone https://ampcode.com/@elianiva/monogatari
cd monogatari
cp .env.example .env        # add OPENROUTER_API_KEY
set -a; . ./.env; set +a
make run                    # http://localhost:8080
```

Open the page in two browsers (or two tabs: every tab is its own player) and join with different names.

| Command | What it does |
|---|---|
| `make run` | Generate templates, build, and start the server |
| `make build` | Build `bin/monogatari` |
| `make test` | Generate templates and run the tests |
| `make generate` | Run `templ generate` |

The generated `*_templ.go` files are committed, so building does not need templ.

## Configuration

All settings are environment variables.

| Variable | Default | Meaning |
|---|---|---|
| `OPENROUTER_API_KEY` | none | Turns on real generation |
| `MODELS` | `deepseek/deepseek-v4.1-flash-1017` | Comma-separated model ids. Retries rotate through the list |
| `PROVIDER` | `stepfun` | OpenRouter provider to pin. Empty turns pinning off. If the provider is down, requests fail |
| `REASONING` | `low` | Reasoning effort sent to the model. `off` omits it |
| `JUDGE` | on | `off` skips the review call for each line |
| `ROOM_PASSPHRASE` | none | If set, joining needs this passphrase |
| `MAX_GAMES_PER_HOUR` | `10` | Cap on stories started per hour |
| `PORT` | `8080` | Listen port |

Set `ROOM_PASSPHRASE` before putting the app on a public address. Every story costs model calls on your key.

## How it works

- **One room, in memory.** There is a single global room. State is lost on restart, and the app must run as a single instance.
- **Server-driven UI.** Each browser opens one SSE stream (`/stream`). Whenever the room changes, the server re-renders that player's page and Datastar morphs it into the DOM. Buttons send small `POST` requests. The browser holds no game logic.
- **Identity.** A random token in `sessionStorage` identifies a player. It is never put in a page: pages only carry a separate public id. A refresh keeps your seat. A new tab is a new player.
- **Generation** (`llm.go`, `prompts/`):
  1. A plan call writes the scenario, characters, per-player briefings and one beat per line.
  2. A script call writes all lines in one pass so they read as one conversation.
  3. Each line is checked by code (furigana, chunk count for the level, one sentence, valid orders, quiz shape), then by a second review call. Lines that fail are rewritten one at a time with the earlier dialogue as context.
- **Dropouts.** If the active player disconnects, the server plays their turn with the right answers after 8 seconds. Auto-played turns earn no stars. The host (first connected player) can also skip a turn or remove someone in the lobby.

### Layout

| File | Contents |
|---|---|
| `main.go` | HTTP routes, SSE stream |
| `room.go` | Room state machine: lobby, turns, auto-play, scoring |
| `llm.go` | OpenRouter client, generation, validation retries, review call |
| `model.go` | Data types, levels, furigana parsing, line checks |
| `views.templ`, `views.go` | All pages |
| `prompts/` | Prompt templates for plan, script, single line and review |
| `mock.go` | Offline story |
| `static/` | CSS, Datastar, fonts |

## Testing

```sh
make test
```

The tests cover the turn flow, auto-play and scoring, ready-vote resets, host powers, the passphrase and game cap, page privacy, and the line checks. Prompt quality is not covered by tests: play a game and read the log (the **Show log** link on the loading screen).

## Deployment notes

- Run one instance and keep the process running between games.
- Behind a reverse proxy, turn off response buffering for `/stream`.
- Serve over HTTPS if the room is public.

## License

Copyright (C) 2026 elianiva

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version. It is distributed without any warranty. See [LICENSE](LICENSE).

Third-party components and their licenses are listed in [THIRD_PARTY.md](THIRD_PARTY.md).
