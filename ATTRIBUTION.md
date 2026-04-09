# Attribution

`slack同步器` is a derivative project built on top of
[`rusq/slackdump`](https://github.com/rusq/slackdump).

This repository keeps the upstream GPL-3.0 license because it includes and
modifies upstream source code. Credit for the original CLI, Slack scraping
logic, export pipeline, and related library code belongs to the upstream
project and its contributors.

Additions in this repository focus on a desktop workflow:

- a Wails desktop application in `SlackSyncGUI/`
- channel-based synchronization from a graphical interface
- direct thread capture from pasted Slack message URLs
- per-thread exports to HTML, Markdown, PDF, JSON, and SQLite

If you are looking for the original project or its full documentation, see:

- Upstream repository: <https://github.com/rusq/slackdump>
- Upstream package docs: <https://pkg.go.dev/github.com/rusq/slackdump/v3>
