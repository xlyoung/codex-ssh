package hosts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"codex-ssh-skill/pkg/model"
)

func Load(path string) (model.Inventory, error) {
	inv := model.Inventory{
		Version: 1,
		Hosts:   map[string]model.Host{},
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return inv, nil
	} else if err != nil {
		return model.Inventory{}, err
	}

	if _, err := toml.DecodeFile(path, &inv); err != nil {
		return model.Inventory{}, err
	}
	if inv.Hosts == nil {
		inv.Hosts = map[string]model.Host{}
	}
	return inv, nil
}

func Save(path string, inv model.Inventory) error {
	pattern := filepath.Base(path) + ".tmp-*"
	file, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := toml.NewEncoder(file).Encode(inv); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func Resolve(inv model.Inventory, cfg model.Config, alias string) (model.ResolvedHost, error) {
	return resolve(inv, cfg, alias, map[string]bool{})
}

func resolve(inv model.Inventory, cfg model.Config, alias string, stack map[string]bool) (model.ResolvedHost, error) {
	if stack[alias] {
		return model.ResolvedHost{}, fmt.Errorf("cyclic via reference detected for %s", alias)
	}

	host, ok := inv.Hosts[alias]
	if !ok {
		return model.ResolvedHost{}, fmt.Errorf("host not found: %s", alias)
	}

	stack[alias] = true
	defer delete(stack, alias)

	resolved := model.ResolvedHost{
		Alias:        alias,
		Host:         host.Host,
		User:         firstNonEmpty(host.User, cfg.DefaultUser),
		Port:         firstNonZero(host.Port, cfg.DefaultPort),
		Tags:         append([]string(nil), host.Tags...),
		Workdir:      host.Workdir,
		Auth:         firstNonEmpty(host.Auth, cfg.DefaultAuth),
		IdentityFile: host.IdentityFile,
		SecretRef:    host.SecretRef,
	}

	for _, viaAlias := range host.Via {
		viaHost, err := resolve(inv, cfg, viaAlias, stack)
		if err != nil {
			return model.ResolvedHost{}, err
		}
		resolved.Via = append(resolved.Via, viaHost.Via...)
		resolved.Via = append(resolved.Via, stripVia(viaHost))
	}

	return resolved, nil
}

func stripVia(host model.ResolvedHost) model.ResolvedHost {
	host.Via = nil
	return host
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if item != "" {
			return item
		}
	}
	return ""
}

func firstNonZero(items ...int) int {
	for _, item := range items {
		if item != 0 {
			return item
		}
	}
	return 0
}
