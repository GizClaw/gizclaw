package rpcgen

import "fmt"

func Run(cfg Config) error {
	if cfg.Package == "" {
		cfg.Package = "gzc"
	}
	if cfg.OutDir == "" {
		return fmt.Errorf("-out is required")
	}
	model, err := loadProtoModel(cfg)
	if err != nil {
		return err
	}
	files, err := emitAll(model)
	if err != nil {
		return err
	}
	return writeFiles(cfg, files)
}
