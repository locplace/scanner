// Package feeder provides batch production for the scanner queue.
// It downloads domain files from the tb0hdan/domains project, decompresses them
// in memory, and creates batches for scanners to process.
package feeder

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/ulikunitz/xz"

	"github.com/locplace/scanner/internal/coordinator/db"
)

// Config holds feeder configuration.
type Config struct {
	// BatchSize is the number of domains per batch.
	BatchSize int

	// MaxPendingBatches is the maximum number of pending batches to keep in the queue.
	// The feeder blocks when this limit is reached.
	MaxPendingBatches int

	// PollInterval is how often to check for pending batch capacity.
	PollInterval time.Duration

	// GitHubToken is an optional GitHub Personal Access Token for LFS downloads.
	// Using a token allows downloads to count against your account's LFS quota
	// instead of the repository owner's quota (which may be exceeded).
	GitHubToken string
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		BatchSize:         1000,
		MaxPendingBatches: 20,
		PollInterval:      5 * time.Second,
	}
}

// Feeder produces batches from domain files.
type Feeder struct {
	DB        *db.DB
	Config    Config
	LFSClient *LFSClient
}

// New creates a new Feeder with the given configuration.
func New(database *db.DB, cfg Config) *Feeder {
	var lfsClient *LFSClient
	if cfg.GitHubToken != "" {
		lfsClient = NewLFSClientWithToken(cfg.GitHubToken)
	} else {
		lfsClient = NewLFSClient()
	}

	return &Feeder{
		DB:        database,
		Config:    cfg,
		LFSClient: lfsClient,
	}
}

// Run starts the feeder loop. It processes files until all are complete,
// then waits for new files to be discovered.
func (f *Feeder) Run(ctx context.Context) {
	log.Printf("Feeder started: batch_size=%d, max_pending=%d",
		f.Config.BatchSize, f.Config.MaxPendingBatches)

	for {
		select {
		case <-ctx.Done():
			log.Println("Feeder stopped")
			return
		default:
		}

		// Get next file to process
		file, err := f.DB.GetNextFileToProcess(ctx)
		if err != nil {
			log.Printf("Feeder: error getting next file: %v", err)
			time.Sleep(f.Config.PollInterval)
			continue
		}

		if file == nil {
			// No files to process, wait and check again
			time.Sleep(f.Config.PollInterval)
			continue
		}

		log.Printf("Feeder: processing file %s (resuming from line %d)", file.Filename, file.ProcessedLines)

		err = f.processFile(ctx, file)
		if err != nil {
			if ctx.Err() != nil {
				return // Context canceled
			}
			log.Printf("Feeder: error processing file %s: %v", file.Filename, err)
			// File will be retried on next iteration since it's still in 'processing' state
			time.Sleep(f.Config.PollInterval)
		}
		// processFile marks feeding_complete and checks for file completion,
		// so we just continue to the next file
	}
}

// processFile downloads and processes a single domain file.
func (f *Feeder) processFile(ctx context.Context, file *db.DomainFile) error {
	log.Printf("Feeder: downloading %s via GitHub web interface", file.Filename)

	// Use the web-based download which may bypass LFS quota issues
	// The file.Filename is like "data/afghanistan/domain2multi-af00.txt.xz"
	body, err := f.LFSClient.DownloadViaWeb(ctx, "tb0hdan", "domains", "master", file.Filename)
	if err != nil {
		return fmt.Errorf("web download: %w", err)
	}
	defer body.Close() //nolint:errcheck // Close error not actionable

	// Create XZ decompressor
	xzReader, err := xz.NewReader(body)
	if err != nil {
		return fmt.Errorf("xz reader: %w", err)
	}

	// Process lines
	scanner := bufio.NewScanner(xzReader)
	// Increase buffer size for potentially long lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var (
		lineNum    int64
		batch      []string
		batchStart int64
		batchCount int
		skipToLine = file.ProcessedLines
	)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lineNum++

		// Skip already processed lines (for resume)
		if lineNum <= skipToLine {
			continue
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Start a new batch if needed
		if len(batch) == 0 {
			batchStart = lineNum
		}

		batch = append(batch, line)

		// Batch is full, insert it
		if len(batch) >= f.Config.BatchSize {
			if insertErr := f.insertBatch(ctx, file.ID, batchStart, lineNum, batch); insertErr != nil {
				return fmt.Errorf("insert batch: %w", insertErr)
			}
			batchCount++
			batch = batch[:0]

			// Log progress periodically
			if batchCount%100 == 0 {
				log.Printf("Feeder: %s progress: %d batches created, line %d", file.Filename, batchCount, lineNum)
			}
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("scan: %w", scanErr)
	}

	// Insert final partial batch
	if len(batch) > 0 {
		if insertErr := f.insertBatch(ctx, file.ID, batchStart, lineNum, batch); insertErr != nil {
			return fmt.Errorf("insert final batch: %w", insertErr)
		}
		batchCount++
	}

	log.Printf("Feeder: %s feeding done: %d batches created", file.Filename, batchCount)

	// Mark feeding complete now that we've read all lines
	if markErr := f.DB.MarkFeedingComplete(ctx, file.ID); markErr != nil {
		return fmt.Errorf("mark feeding complete: %w", markErr)
	}

	// Try to mark file complete if all batches are already done
	completed, err := f.DB.CheckAndMarkFileComplete(ctx, file.ID)
	if err != nil {
		log.Printf("Feeder: error checking file completion %s: %v", file.Filename, err)
	}
	if completed {
		log.Printf("Feeder: %s complete (all batches done)", file.Filename)
	} else if batchCount > 0 {
		log.Printf("Feeder: %s has %d batches pending, moving to next file", file.Filename, batchCount)
	}

	return nil
}

// insertBatch waits for queue capacity and inserts a batch.
func (f *Feeder) insertBatch(ctx context.Context, fileID int, lineStart, lineEnd int64, domains []string) error {
	// Wait for queue capacity
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pending, err := f.DB.GetPendingBatchCount(ctx)
		if err != nil {
			return fmt.Errorf("get pending count: %w", err)
		}

		if pending < f.Config.MaxPendingBatches {
			break
		}

		// Queue is full, wait
		time.Sleep(f.Config.PollInterval)
	}

	// Insert batch
	domainsStr := strings.Join(domains, "\n")
	return f.DB.CreateBatchAndUpdateProgress(ctx, fileID, lineStart, lineEnd, domainsStr)
}

// ProcessFileByID processes a specific file by ID (for manual triggering).
func (f *Feeder) ProcessFileByID(ctx context.Context, fileID int) error {
	var file db.DomainFile
	err := f.DB.Pool.QueryRow(ctx, `
		SELECT id, filename, url, size_bytes, processed_lines, batches_created, batches_completed, feeding_complete, status, started_at, completed_at
		FROM domain_files
		WHERE id = $1
	`, fileID).Scan(&file.ID, &file.Filename, &file.URL, &file.SizeBytes, &file.ProcessedLines,
		&file.BatchesCreated, &file.BatchesCompleted, &file.FeedingComplete, &file.Status, &file.StartedAt, &file.CompletedAt)
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}

	return f.processFile(ctx, &file)
}

// WaitForCapacity blocks until there's room in the batch queue.
// Useful for startup to ensure we don't create too many batches.
func (f *Feeder) WaitForCapacity(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pending, err := f.DB.GetPendingBatchCount(ctx)
		if err != nil {
			return err
		}

		if pending < f.Config.MaxPendingBatches {
			return nil
		}

		time.Sleep(f.Config.PollInterval)
	}
}

// StreamingReader wraps an io.Reader with context cancellation support.
type StreamingReader struct {
	ctx    context.Context
	reader io.Reader
}

// NewStreamingReader creates a reader that respects context cancellation.
func NewStreamingReader(ctx context.Context, r io.Reader) *StreamingReader {
	return &StreamingReader{ctx: ctx, reader: r}
}

// Read implements io.Reader with context cancellation.
func (r *StreamingReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}
