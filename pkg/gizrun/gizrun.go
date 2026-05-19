package gizrun

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/cmdhandler"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/configfile"
	"github.com/gofiber/fiber/v2"
)

var runCtx = newRunContext()

type (
	CmdHandler    = cmdhandler.Handler
	CmdHandleFunc = cmdhandler.HandleFunc
	ConfigParser  = configfile.Parser
)

func HandleCmd(path string, handler CmdHandler) error {
	return runCtx.cmdHandler.Handle(path, handler)
}

func RegisterConfigParser(name string, parser ConfigParser) {
	runCtx.configParser.Register(name, parser)
}

func Debug() *fiber.App {
	return runCtx.debugApp
}

func HTTP() *fiber.App {
	return runCtx.publicApp
}

func Config(name string) (any, bool) {
	return runCtx.configFile.Config(name)
}

func Serve() error {
	runInitHooks(initHooks.hooks)
	args, flags, err := cmdhandler.Parse(flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	if err := runCtx.loadConfig(flag.CommandLine); err != nil {
		return err
	}
	handler, ok := runCtx.cmdHandler.Lookup(strings.Join(args, "/"))
	if !ok {
		return errors.New("gizrun: command handler not found")
	}

	runPostInitHooks(runCtx, postInitHooks.hooks)
	defer runExitHooks(runCtx, exitHooks.hooks)
	return handler.Handle(context.Background(), args, flags)
}
