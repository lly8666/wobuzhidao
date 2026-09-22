// Package configfile applies a bounded, strict JSON object to the existing CLI
// FlagSet. There is one parameter namespace, not a second configuration model.
package configfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

const MaxBytes = 1 << 20

func ApplyFile(fs *flag.FlagSet, path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("config: cannot open file")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(raw) > MaxBytes {
		return errors.New("config: read failed or file exceeds 1 MiB")
	}
	return Apply(fs, raw)
}

func Apply(fs *flag.FlagSet, raw []byte) error {
	if len(raw) > MaxBytes {
		return errors.New("config: file exceeds 1 MiB")
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("config: expected JSON object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return errors.New("config: invalid key")
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return errors.New("config: invalid or duplicate key")
		}
		seen[name] = true
		if name == "config" || name == "version" || fs.Lookup(name) == nil {
			return fmt.Errorf("config: unsupported key %q", name)
		}
		var value any
		if err = d.Decode(&value); err != nil {
			return fmt.Errorf("config: invalid value for %q", name)
		}
		var text string
		switch v := value.(type) {
		case string:
			text = v
		case bool:
			text = strconv.FormatBool(v)
		case json.Number:
			text = string(v)
		default:
			return fmt.Errorf("config: %q must be a scalar", name)
		}
		if !explicit[name] {
			// Do not include flag.Set's error: it may echo a credential value.
			if err = fs.Set(name, text); err != nil {
				return fmt.Errorf("config: invalid value for %q", name)
			}
		}
	}
	if _, err = d.Token(); err != nil {
		return errors.New("config: incomplete object")
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("config: trailing content")
	}
	return nil
}
