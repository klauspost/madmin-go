/*
 * Project Tricorder - Interactive MinIO Metrics Navigator
 *
 * An interactive text-based user interface for browsing MinIO cluster metrics
 * in real-time using path-based navigation through the metric hierarchy.
 *
 * Usage:
 *   go run . -endpoint localhost:9001 -access-key minio -secret-key minio123
 *   go run . -types scanner,cpu,mem -refresh 5s
 *   go run . --help
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/klauspost/compress/zstd"
	"github.com/minio/madmin-go/v4"
	"github.com/tinylib/msgp/msgp"
)

type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	MetricTypes   []string
	RefreshPeriod time.Duration
	InputFile     string // Path to import compressed metrics file
}

func parseFlags() Config {
	var cfg Config

	flag.StringVar(&cfg.Endpoint, "endpoint", "127.0.0.1:9001", "MinIO server endpoint (host:port)")
	flag.StringVar(&cfg.AccessKey, "access-key", "minio", "MinIO access key")
	flag.StringVar(&cfg.SecretKey, "secret-key", "minio123", "MinIO secret key")
	flag.BoolVar(&cfg.UseSSL, "tls", false, "Use SSL/TLS connection")
	flag.StringVar(&cfg.InputFile, "in", "", "Import compressed metrics from file instead of connecting to server")

	var types string
	flag.StringVar(&types, "types", "", "Comma-separated metric types (scanner,cpu,mem,disk,os,net,rpc,api,runtime,process)")
	var refresh string
	flag.StringVar(&refresh, "refresh", "3s", "Refresh interval (e.g., 1s, 30s, 1m)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Project Tricorder - Interactive MinIO Metrics Navigator\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nNavigation:\n")
		fmt.Fprintf(os.Stderr, "  ↑/↓     Navigate between options\n")
		fmt.Fprintf(os.Stderr, "  Enter   Navigate into selected item\n")
		fmt.Fprintf(os.Stderr, "  Esc     Go back to parent\n")
		fmt.Fprintf(os.Stderr, "  Home    Go to first item (..)\n")
		fmt.Fprintf(os.Stderr, "  End     Go to last item\n")
		fmt.Fprintf(os.Stderr, "  Ctrl+R  Refresh current data\n")
		fmt.Fprintf(os.Stderr, "  q       Quit application\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                                    # Connect to default MinIO\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -endpoint prod.example.com:9000    # Custom endpoint\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -types scanner,cpu,mem             # Specific metrics only\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -in metrics_25-11-21-22_31.msgp.zst # Import from file\n", os.Args[0])
	}

	flag.Parse()

	if types != "" {
		cfg.MetricTypes = strings.Split(types, ",")
		for i, t := range cfg.MetricTypes {
			cfg.MetricTypes[i] = strings.TrimSpace(t)
		}
	}

	var err error
	cfg.RefreshPeriod, err = time.ParseDuration(refresh)
	if err != nil {
		log.Fatalf("Invalid refresh period '%s': %v", refresh, err)
	}

	return cfg
}

func createAdminClient(cfg Config) (*madmin.AdminClient, error) {
	adminClient, err := madmin.New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, cfg.UseSSL)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %v", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to get server info as a connection test
	_, err = adminClient.ServerInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MinIO server: %v", err)
	}

	return adminClient, nil
}

func getMetricOptions(cfg Config) madmin.MetricsOptions {
	var opts madmin.MetricsOptions

	// Convert string types to MetricType
	if len(cfg.MetricTypes) > 0 {
		for _, t := range cfg.MetricTypes {
			switch strings.ToLower(t) {
			case "scanner":
				opts.Type |= madmin.MetricsScanner
			case "disk":
				opts.Type |= madmin.MetricsDisk
			case "os":
				opts.Type |= madmin.MetricsOS
			case "batch", "batchjobs":
				opts.Type |= madmin.MetricsBatchJobs
			case "resync", "siteresync":
				opts.Type |= madmin.MetricsSiteResync
			case "net", "network":
				opts.Type |= madmin.MetricNet
			case "mem", "memory":
				opts.Type |= madmin.MetricsMem
			case "cpu":
				opts.Type |= madmin.MetricsCPU
			case "rpc":
				opts.Type |= madmin.MetricsRPC
			case "runtime", "go":
				opts.Type |= madmin.MetricsRuntime
			case "api":
				opts.Type |= madmin.MetricsAPI
			case "replication", "repl":
				opts.Type |= madmin.MetricsReplication
			case "process":
				opts.Type |= madmin.MetricsProcess
			default:
				log.Printf("Warning: unknown metric type '%s'", t)
			}
		}
	} else {
		// Default to all metrics
		opts.Type = madmin.MetricsAll
	}

	// Enable aggregation flags for better navigation
	opts.Flags.Add(madmin.MetricsByHost, madmin.MetricsByDisk, madmin.MetricsByDiskSet)

	return opts
}

func main() {
	cfg := parseFlags()

	var adminClient *madmin.AdminClient
	var initialMetrics *madmin.RealtimeMetrics

	if cfg.InputFile != "" {
		// Load metrics from file instead of connecting to server
		fmt.Printf("Loading metrics from file: %s\n", cfg.InputFile)
		metrics, err := loadMetricsFromFile(cfg.InputFile)
		if err != nil {
			log.Fatalf("Failed to load metrics from file: %v", err)
		}
		initialMetrics = metrics
		fmt.Printf("Loaded metrics from file successfully\n")
	} else {
		// Create MinIO admin client for live connection
		client, err := createAdminClient(cfg)
		if err != nil {
			log.Fatalf("Failed to setup MinIO connection: %v", err)
		}
		adminClient = client
		fmt.Printf("Connected to MinIO at %s\n", cfg.Endpoint)
	}

	fmt.Printf("Starting Project Tricorder...\n")

	// Create the TUI model
	model := NewTricorderModel(adminClient, initialMetrics, cfg)

	// Start the TUI with alt screen buffer and full screen
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}

// collectAndSaveMetrics collects metrics data and saves it to a compressed file
func collectAndSaveMetrics(adminClient *madmin.AdminClient, config Config, issueNum string) (string, error) {
	// Create filename with timestamp
	now := time.Now()
	filename := fmt.Sprintf("metrics_%s.msgp.zst", now.Format("06-01-02-15_04"))

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Create zstd compressor
	encoder, err := zstd.NewWriter(file, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		return "", fmt.Errorf("failed to create zstd encoder: %v", err)
	}
	defer encoder.Close()

	// Collect metrics with required flags
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var opts madmin.MetricsOptions
	opts.Type = madmin.MetricsAll
	opts.Flags.Add(madmin.MetricsDayStats, madmin.MetricsByHost, madmin.MetricsByDisk, madmin.MetricsByDiskSet)
	opts.N = 1

	// Collect single metrics sample
	var metricsData *madmin.RealtimeMetrics

	err = adminClient.Metrics(ctx, opts, func(m madmin.RealtimeMetrics) {
		metricsData = &m
	})

	if err != nil {
		return "", fmt.Errorf("failed to collect metrics: %v", err)
	}

	if metricsData == nil {
		return "", fmt.Errorf("no metrics data received")
	}

	// Stream msgpack encoding directly to compressed writer
	writer := msgp.NewWriter(encoder)
	if err := metricsData.EncodeMsg(writer); err != nil {
		return "", fmt.Errorf("failed to encode metrics: %v", err)
	}

	if err := writer.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush msgpack writer: %v", err)
	}

	// Ensure compressor flushes all data
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("failed to close compressor: %v", err)
	}

	return filename, nil
}

// loadMetricsFromFile loads metrics data from a compressed msgpack file
func loadMetricsFromFile(filename string) (*madmin.RealtimeMetrics, error) {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create zstd decompressor
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd decoder: %v", err)
	}
	defer decoder.Close()

	// Create msgpack reader
	reader := msgp.NewReader(decoder)

	// Decode the metrics
	var metrics madmin.RealtimeMetrics
	if err := metrics.DecodeMsg(reader); err != nil {
		return nil, fmt.Errorf("failed to decode metrics: %v", err)
	}

	return &metrics, nil
}
