package kvmigratecmd

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/cmd/internal/server"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	clientlog "github.com/pingcap/log"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
)

// NewCmd creates the offline Redis-to-TiKV KV migration command.
func NewCmd() *cobra.Command {
	var source, target string
	var names []string
	var options kv.RedisMigrationOptions
	command := &cobra.Command{
		SilenceUsage: true,
		Use:          "kv-migrate <dir>", Short: "Import Redis KV namespaces into TiKV and verify their contents",
		Long: "Stop every source and destination writer, including Server background workers. Keep both connector definitions in config.yaml; this command opens only the Redis primary and TiKV. It preserves binary values, absolute string deadlines, Sets and zero-score ordered Sets. It never deletes Redis data or overwrites differing live TiKV records. Use --dry-run before importing and --verify-only before changing Store bindings. Interrupted imports can be rerun; each rerun rescans and verifies completed keys.",
		Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
			restore, err := redirectClientLogs(command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer restore()
			result, err := server.MigrateRedisKV(command.Context(), args[0], source, target, names, options)
			if outputErr := json.NewEncoder(command.OutOrStdout()).Encode(result); outputErr != nil {
				return outputErr
			}
			return err
		},
	}
	command.Flags().StringVar(&source, "source", "", "source Redis connector name in config.yaml")
	command.Flags().StringVar(&target, "target", "", "target TiKV connector name in config.yaml")
	command.Flags().StringSliceVar(&names, "stores", nil, "logical KV Store names; default all Stores bound to source or target")
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "validate source and existing target data without writing")
	command.Flags().BoolVar(&options.VerifyOnly, "verify-only", false, "require exact source/target contents without writing")
	command.Flags().IntVar(&options.ScanCount, "scan-count", 128, "Redis scan count hint (1..1024); collection writes are bounded to 128 members")
	command.Flags().IntVar(&options.Workers, "workers", 8, "concurrent keys per scan page (1..32)")
	command.Flags().IntVar(&options.MaxValueBytes, "max-value-bytes", 1<<20, "maximum string payload bytes (1..4194304)")
	command.MarkFlagsMutuallyExclusive("dry-run", "verify-only")
	_ = command.MarkFlagRequired("source")
	_ = command.MarkFlagRequired("target")
	return command
}

// SDK logging belongs to this CLI process. Reusable storage and migration
// packages leave global loggers untouched; the command keeps stdout JSON-only.
func redirectClientLogs(output io.Writer) (func(), error) {
	sink := zapcore.AddSync(&clientLogWriter{output: output})
	logger, properties, err := clientlog.InitLoggerWithWriteSyncer(&clientlog.Config{
		Level: "info", Format: "text", DisableErrorVerbose: true,
	}, sink, sink)
	if err != nil {
		return nil, err
	}
	return clientlog.ReplaceGlobals(logger, properties), nil
}

// Each SDK record is one synchronous Write. Serialize access to caller-owned
// writers such as bytes.Buffer without owning their flushing or closing.
type clientLogWriter struct {
	mu     sync.Mutex
	output io.Writer
}

func (writer *clientLogWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.output.Write(data)
}
