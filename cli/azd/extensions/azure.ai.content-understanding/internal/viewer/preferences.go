// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"azure.ai.content-understanding/internal/cuclient"

	azdconfig "github.com/azure/azure-dev/cli/azd/pkg/config"
	"github.com/gofrs/flock"
)

const maxFieldOrderSize = 1 << 20

type FieldOrder map[string][]string

type FieldOrderStore interface {
	Load(context.Context) (FieldOrder, error)
	Save(context.Context, FieldOrder) error
}

type fieldOrderStore struct {
	path     string
	analyzer string
}

var fieldOrderPointer = regexp.MustCompile(`^(/([^~/]|~[01])*)+$`)

func NewFieldOrderStore(analyzer string) (FieldOrderStore, error) {
	if err := cuclient.ValidateAnalyzerID(analyzer); err != nil {
		return nil, err
	}
	path, err := azdconfig.GetUserConfigFilePath()
	if err != nil {
		return nil, err
	}
	return &fieldOrderStore{path: path, analyzer: analyzer}, nil
}

func validateFieldOrder(order FieldOrder) error {
	if order == nil {
		return errors.New("field order must be a parent-path mapping")
	}
	for parent, children := range order {
		if (parent != "$root" && !fieldOrderPointer.MatchString(parent)) || children == nil {
			return errors.New("field order must contain arrays of direct child JSON-pointer paths")
		}
		seen := make(map[string]bool, len(children))
		for _, child := range children {
			index := strings.LastIndex(child, "/")
			directParent := "$root"
			if index > 0 {
				directParent = child[:index]
			}
			if !fieldOrderPointer.MatchString(child) || directParent != parent || seen[child] {
				return errors.New("field order must contain unique direct child JSON-pointer paths")
			}
			seen[child] = true
		}
	}
	body, err := json.Marshal(order)
	if err != nil {
		return err
	}
	if len(body) > maxFieldOrderSize {
		return errors.New("field order exceeds the 1 MiB limit")
	}
	return nil
}

var fieldOrderConfigKeys = []string{
	"extension", "installed", "azure.ai.content-understanding", "Viewer", "fieldOrderByAnalyzer",
}

func decodeSavedFieldOrder(raw json.RawMessage) (FieldOrder, error) {
	var order FieldOrder
	if err := json.Unmarshal(raw, &order); err != nil {
		return nil, errors.New("saved analyzer field order must be a parent-path mapping")
	}
	if err := validateFieldOrder(order); err != nil {
		return nil, err
	}
	return order, nil
}

func migrateFieldOrderKeys(orders map[string]json.RawMessage) (bool, error) {
	migrated := false
	for key, raw := range orders {
		order, err := decodeSavedFieldOrder(raw)
		if err != nil {
			return false, err
		}
		if cuclient.ValidateAnalyzerID(key) == nil {
			continue
		}
		separator := strings.LastIndex(key, "#")
		if !strings.HasPrefix(strings.ToLower(key), "https://") || separator < 0 {
			return false, errors.New("saved field order keys must be analyzer IDs")
		}
		analyzer := key[separator+1:]
		if err := cuclient.ValidateAnalyzerID(analyzer); err != nil {
			return false, errors.New("legacy field order key must end with a valid analyzer ID")
		}
		if existing, found := orders[analyzer]; found {
			saved, err := decodeSavedFieldOrder(existing)
			if err != nil {
				return false, err
			}
			if !reflect.DeepEqual(saved, order) {
				return false, fmt.Errorf(
					"conflicting saved field orders for analyzer %q; keep one order before migrating", analyzer)
			}
		} else {
			orders[analyzer] = raw
		}
		delete(orders, key)
		migrated = true
	}
	return migrated, nil
}

func (s *fieldOrderStore) load() ([]map[string]json.RawMessage, error) {
	root := make(map[string]json.RawMessage)
	body, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(body, &root); err != nil || root == nil {
			return nil, errors.New("azd user configuration must be a JSON object")
		}
	}
	layers := []map[string]json.RawMessage{root}
	for _, key := range fieldOrderConfigKeys {
		child := make(map[string]json.RawMessage)
		if raw, found := layers[len(layers)-1][key]; found {
			if err := json.Unmarshal(raw, &child); err != nil || child == nil {
				return nil, errors.New("azd Viewer settings must contain JSON objects")
			}
		}
		layers = append(layers, child)
	}
	return layers, nil
}

func (s *fieldOrderStore) Load(ctx context.Context) (FieldOrder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	layers, err := s.load()
	if err != nil {
		return nil, err
	}
	migrated, err := migrateFieldOrderKeys(layers[len(layers)-1])
	if err != nil {
		return nil, err
	}
	if migrated {
		if err := s.update(ctx, migrateFieldOrderKeys); err != nil {
			return nil, err
		}
		layers, err = s.load()
		if err != nil {
			return nil, err
		}
	}
	if raw, found := layers[len(layers)-1][s.analyzer]; found {
		return decodeSavedFieldOrder(raw)
	}
	return make(FieldOrder), nil
}

func (s *fieldOrderStore) Save(ctx context.Context, order FieldOrder) error {
	if err := validateFieldOrder(order); err != nil {
		return err
	}
	return s.update(ctx, func(orders map[string]json.RawMessage) (bool, error) {
		migrated, err := migrateFieldOrderKeys(orders)
		if err != nil {
			return false, err
		}
		customized := make(FieldOrder)
		for parent, children := range order {
			if len(children) > 0 {
				customized[parent] = children
			}
		}
		if len(customized) == 0 {
			if _, found := orders[s.analyzer]; !found {
				return migrated, nil
			}
			delete(orders, s.analyzer)
		} else {
			body, err := json.Marshal(customized)
			if err != nil {
				return false, err
			}
			orders[s.analyzer] = body
		}
		return true, nil
	})
}

func (s *fieldOrderStore) update(
	ctx context.Context, edit func(map[string]json.RawMessage) (bool, error),
) (resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := flock.New(s.path + ".viewer.lock")
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("azd Viewer settings could not be locked")
	}
	defer func() { resultErr = errors.Join(resultErr, lock.Unlock()) }()

	// Reload after locking: the azd RPC config is a command-lifetime snapshot.
	layers, err := s.load()
	if err != nil {
		return err
	}
	changed, err := edit(layers[len(layers)-1])
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	// Merge literal keys instead of passing the dotted extension ID to Config.Set.
	for index := len(fieldOrderConfigKeys) - 1; index >= 0; index-- {
		if index >= 3 && len(layers[index+1]) == 0 {
			delete(layers[index], fieldOrderConfigKeys[index])
			continue
		}
		body, err := json.Marshal(layers[index+1])
		if err != nil {
			return err
		}
		layers[index][fieldOrderConfigKeys[index]] = body
	}
	body, err := json.MarshalIndent(layers[0], "", "  ")
	if err != nil {
		return err
	}
	return replaceViewerConfig(ctx, s.path, body)
}

func replaceViewerConfig(ctx context.Context, path string, body []byte) (resultErr error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".config-viewer-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			resultErr = errors.Join(resultErr, err)
		}
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace azd Viewer settings: %w", err)
	}
	return nil
}
