package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/jmoiron/sqlx"
)

// ErrNotFound identifies a missing App record.
var ErrNotFound = errors.New("app: not found")

// ErrInvalid identifies an invalid App declaration or package.
var ErrInvalid = errors.New("app: invalid declaration")

// Server owns verified App records. HTTP is optional and supports package transport configuration.
type Server struct {
	DB   *sqlx.DB
	HTTP *http.Client
}

// Initialize creates the App catalog.
func (s *Server) Initialize(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS apps (id TEXT PRIMARY KEY, app_name TEXT NOT NULL, data TEXT NOT NULL)`)
	return err
}

// Get reads a canonical App resource.
func (s *Server) Get(ctx context.Context, id string) (apitypes.App, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, s.DB.Rebind(`SELECT data FROM apps WHERE id=?`), id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.App{}, ErrNotFound
	}
	if err != nil {
		return apitypes.App{}, err
	}
	var value apitypes.App
	err = json.Unmarshal([]byte(raw), &value)
	return value, err
}

// Put verifies a package and atomically stores its manifest, preserving app_name.
func (s *Server) Put(ctx context.Context, id string, spec apitypes.AppSpec, create bool) (apitypes.App, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return apitypes.App{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	manifest, err := Fetch(fetchCtx, s.HTTP, spec.Package)
	if err != nil {
		return apitypes.App{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	previous, err := s.Get(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return apitypes.App{}, err
	}
	exists := err == nil
	if !exists && !create {
		return apitypes.App{}, ErrNotFound
	}
	if exists && previous.AppName != manifest.AppName {
		return apitypes.App{}, fmt.Errorf("%w: app_name is immutable", ErrInvalid)
	}
	now := time.Now().UTC()
	value := apitypes.App{Id: id, Package: spec.Package, AppName: manifest.AppName, Runtime: manifest.Runtime, Entry: manifest.Entry, Methods: manifest.Methods, Requires: manifest.Requires, Sha256: spec.Package.Sha256, CreatedAt: now, UpdatedAt: now}
	if exists {
		value.CreatedAt = previous.CreatedAt
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return apitypes.App{}, err
	}
	if exists {
		result, err := s.DB.ExecContext(ctx, s.DB.Rebind(`UPDATE apps SET data=? WHERE id=? AND app_name=?`), string(raw), id, value.AppName)
		if err != nil {
			return apitypes.App{}, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return apitypes.App{}, err
		}
		if n != 1 {
			return apitypes.App{}, ErrNotFound
		}
	} else {
		if _, err := s.DB.ExecContext(ctx, s.DB.Rebind(`INSERT INTO apps (id,app_name,data) VALUES (?,?,?)`), id, value.AppName, string(raw)); err != nil {
			return apitypes.App{}, err
		}
	}
	return value, nil
}

// Delete removes a canonical App record, returning its previous value.
func (s *Server) Delete(ctx context.Context, id string) (apitypes.App, error) {
	value, err := s.Get(ctx, id)
	if err != nil {
		return value, err
	}
	_, err = s.DB.ExecContext(ctx, s.DB.Rebind(`DELETE FROM apps WHERE id=?`), id)
	return value, err
}
