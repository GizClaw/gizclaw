package gizrun

import (
	"flag"
	"log/slog"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/cmdhandler"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/configfile"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log/volclog"
	"github.com/gofiber/fiber/v2"
)

type RunContext struct {
	cmdHandler      *cmdhandler.CmdHandler
	debugApp        *fiber.App
	debugListening  bool
	logHandlers     []slog.Handler
	metricsPusher   *metricsPusher
	publicApp       *fiber.App
	publicListening bool
	volcLogHandler  *volclog.Handler
	configParser    *configfile.YamlParser

	configFile configfile.ConfigFile

	// flags
	configPath  string
	enablePprof bool
	debugPort   int
	addr        string
	logLevel    slog.LevelVar
	verbose     verboseFlag
	metricsPush metricsPushConfig
	volcLog     volcLogConfig
}

func newRunContext() *RunContext {
	ctx := &RunContext{
		cmdHandler:   cmdhandler.New(),
		configParser: configfile.NewYamlParser(),
		debugApp:     newDebugFiberApp(),
		publicApp:    newFiberApp(),
		debugPort:    6060,
	}
	ctx.logLevel.Set(slog.LevelInfo)
	return ctx
}

func (c *RunContext) loadConfig(fs *flag.FlagSet) error {
	config, err := c.configParser.ParseFile(c.configPathValue(fs))
	if err != nil {
		return err
	}
	c.configFile = config
	return nil
}

func (c *RunContext) config(name string) (any, bool) {
	if c == nil {
		return nil, false
	}
	return c.configFile.Config(name)
}

func (c *RunContext) configPathValue(fs *flag.FlagSet) string {
	if c == nil {
		return ""
	}
	if fs == nil {
		return c.configPath
	}
	flag := fs.Lookup("config")
	if flag == nil {
		return c.configPath
	}
	return flag.Value.String()
}

func (c *RunContext) shutdownTimeout() time.Duration {
	return 5 * time.Second
}
