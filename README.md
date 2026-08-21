# 🎱 ate-watch

`ate-watch` lets you monitor [Agent Substrate](https://github.com/agent-substrate/substrate) actors live from your terminal. It shows real-time status changes (running, suspended, paused, etc.) and lets you inspect actor details and endpoints.

## Features

- **Watcher**: Continuously refreshes a color-coded status table with active actors across all or specific atespaces.
- **Interactive Inspector (`d`)**: Select any actor using `↑`/`↓` (or `j`/`k`) and press `d` or `Enter` to inspect full details (metadata, worker placement, and snapshot states).
- **Filters**: Filter by atespace (`-a` / `-A`) or actor status (`--status=RUNNING,SUSPENDED`).

```text
Every 500ms: ate-watch (all) • 3 actors (2 running, 1 suspended) • 23:25:00 PDT

   │ STATUS    │ AGE │ ATESPACE │ NAME    │ TEMPLATE            
───┼───────────┼─────┼──────────┼─────────┼─────────────────────
 > │ RUNNING   │ 5m  │ default  │ env-1   │ ate-env/default-env 
   │ RUNNING   │ 12m │ default  │ agent-1 │ claude-code/agent   
   │ SUSPENDED │ 1h  │ team-a   │ crawler │ custom/scraper      

[↑/↓/j/k] Select • [d/Enter] Describe • [Esc] Exit
```

## Installation

```bash
go install github.com/rakyll/ate-watch/cmd/ate-watch@latest
```

## Usage

Watch actors using your current Kubernetes context:

```bash
ate-watch
```

Watch actors in a specific Kubernetes context:

```bash
ate-watch --context gke_my-project_us-central1_my-cluster
```

Watch actors in a specific atespace:

```bash
ate-watch -a default
```

Print actor statuses once and exit:

```bash
ate-watch --once
```

Connect directly to a control plane address:

```bash
ate-watch --endpoint localhost:8080
```

## License

Apache 2.0

