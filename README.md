# Monogatari

Multiplayer Japanese sentence-ordering visual novel. Go server, Datastar UI, templ views, stories generated through OpenRouter.

```
make run        # generate templ, build, run on :8080
make test
```

Without `OPENROUTER_API_KEY` the app plays an offline mock story.

| Variable | Default | Meaning |
|---|---|---|
| `OPENROUTER_API_KEY` | none | Enables real generation |
| `MODELS` | `deepseek/deepseek-v4.1-flash-1017` | Comma-separated model ids. Retries rotate through the list |
| `PROVIDER` | `stepfun` | OpenRouter provider to pin. Empty turns pinning off |
| `REASONING` | `low` | Reasoning effort sent to the model. `off` omits it |
| `JUDGE` | on | Set to `off` to skip the second review call per line |
| `ROOM_PASSPHRASE` | none | When set, joining needs this passphrase |
| `MAX_GAMES_PER_HOUR` | `10` | Cap on generations started per hour |
| `PORT` | `8080` | Listen port |

Prompts are in `prompts/`. Views are in `views.templ`; run `templ generate` after editing (`make` does it). The generated `*_templ.go` files are committed.
