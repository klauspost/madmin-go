# Project Tricorder 📡

An interactive text-based user interface for browsing MinIO cluster metrics in real-time. Navigate through the complete metrics hierarchy using an intuitive path-based system.

## Features

- 🧭 **Interactive Navigation**: Browse metrics using arrow keys and hierarchical paths
- 🔄 **Real-time Updates**: Auto-refresh metrics with configurable intervals
- 📊 **Rich Display**: Formatted values with human-readable numbers, bytes, and timestamps
- 🎯 **Selective Loading**: Choose specific metric types for focused analysis
- 🖥️  **Responsive UI**: Clean terminal interface that adapts to window size
- 📍 **Breadcrumbs**: Always know where you are in the metrics tree

## Installation

```bash
cd examples/tricorder
go mod tidy
go build
```

## Usage

### Basic Usage

Connect to default MinIO instance (localhost:9001):
```bash
go run .
```

### Custom Configuration

```bash
# Custom MinIO endpoint
go run . -endpoint prod.minio.com:9000 -access-key admin -secret-key secretkey

# Specific metric types only
go run . -types scanner,cpu,mem,disk

# Custom refresh interval
go run . -refresh 10s

# SSL connection
go run . -endpoint secure.minio.com:9000 -ssl -access-key admin -secret-key secretkey
```

### Command Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `-endpoint` | `127.0.0.1:9001` | MinIO server endpoint (host:port) |
| `-access-key` | `minio` | MinIO access key |
| `-secret-key` | `minio123` | MinIO secret key |
| `-ssl` | `false` | Use SSL/TLS connection |
| `-refresh` | `3s` | Auto-refresh interval |

## Navigation Controls

| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `Enter` / `Space` | Navigate into selected item |
| `Esc` / `Backspace` | Go back to parent |
| `Home` | Go to first item (..) |
| `End` | Go to last item |
| `F5` | Refresh current data |
| `q` / `Ctrl+C` | Quit application |

- Double press `Esc` on the main page to exit.
- Press a letter to go to the next item starting with that letter
 
## Navigation Paths

The interface follows a hierarchical path structure:

```
root
├── aggregated/              # Cluster-wide aggregated metrics
│   ├── scanner/             # Scanner metrics
│   │   ├── buckets/         # Per-bucket scan statistics
│   │   ├── lifetime_ops/    # Operations since start
│   │   └── last_minute/     # Recent activity
│   ├── cpu/                 # CPU usage metrics
│   ├── mem/                 # Memory statistics
│   └── disk/                # Disk I/O and usage
├── by_host/                 # Metrics broken down by host
│   └── [hostname]/          # Individual host metrics
└── by_disk/                 # Metrics broken down by disk
    └── [disk_id]/           # Individual disk metrics
```

## Examples

### Monitor Scanner Activity
```bash
# Navigate to: root › aggregated › scanner › buckets
go run . -types scanner
# Use arrow keys to navigate to "aggregated" → Enter
# Navigate to "scanner" → Enter
# Navigate to "buckets" → Enter
# View per-bucket scanning statistics
```

### Check CPU Usage Across Hosts
```bash
# Navigate to: root › by_host › [hostname] › cpu
go run . -types cpu -refresh 1s
# Navigate to "by_host" → Enter
# Select a host → Enter
# Navigate to "cpu" → Enter
# View real-time CPU metrics for that host
```

### Monitor Memory Usage
```bash
# Navigate to: root › aggregated › mem
go run . -types mem
# Navigate to "aggregated" → Enter
# Navigate to "mem" → Enter
# View cluster memory statistics
```

## Sample Output

```
📡 Project Tricorder - MinIO Metrics Navigator
Path: root › aggregated › scanner

Last refresh: 2s ago | Type: Scanner

Available options (6):

► buckets          - Per-bucket scanning statistics
  lifetime_ops     - Accumulated operations since server start
  lifetime_ilm     - Accumulated ILM operations since server start
  last_minute      - Last minute operation statistics
  active_paths     - Currently active scan paths
  excessive_paths  - Paths marked as having excessive entries

Navigation: ↑/↓ Move  Enter Select  Esc Back  F5 Refresh  q Quit
```

## Development

The application is structured into several modules:

- `main.go` - CLI argument parsing and initialization
- `ui.go` - Bubbletea TUI implementation
- `navigator.go` - Navigation state management
- `renderer.go` - Display formatting and styling

### Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [madmin-go](https://github.com/minio/madmin-go) - MinIO admin client

## Troubleshooting

### Connection Issues

- Verify MinIO endpoint is accessible
- Check access key and secret key credentials
- Ensure MinIO admin API is enabled
- For SSL connections, verify certificates are valid

### Performance

- Increase `-refresh` interval for slower systems
- Some metrics may take longer to collect on large clusters


## License

This project follows the same license as madmin-go (GNU Affero General Public License v3.0).