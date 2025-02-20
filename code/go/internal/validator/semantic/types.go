// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package semantic

import (
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	ve "github.com/elastic/package-spec/v2/code/go/internal/errors"
	"github.com/elastic/package-spec/v2/code/go/internal/fspath"
)

const dataStreamDir = "data_stream"

type fields []field

type field struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Unit       string `yaml:"unit"`
	MetricType string `yaml:"metric_type"`
	Dimension  bool   `yaml:"dimension"`
	External   string `yaml:"external"`

	Fields fields `yaml:"fields"`
}

type validateFunc func(fieldsFile string, f field) ve.ValidationErrors

func validateFields(fsys fspath.FS, validate validateFunc) ve.ValidationErrors {
	fieldsFiles, err := listFieldsFiles(fsys)
	if err != nil {
		return ve.ValidationErrors{errors.Wrap(err, "can't list fields files")}
	}

	var vErrs ve.ValidationErrors
	for _, fieldsFile := range fieldsFiles {
		unmarshaled, err := unmarshalFields(fsys, fieldsFile)
		if err != nil {
			vErrs = append(vErrs, errors.Wrapf(err, `file "%s" is invalid: can't unmarshal fields`, fsys.Path(fieldsFile)))
		}

		errs := validateNestedFields("", fsys.Path(fieldsFile), unmarshaled, validate)
		if len(errs) > 0 {
			vErrs = append(vErrs, errs...)
		}
	}
	return vErrs
}

func validateNestedFields(parent string, fieldsFile string, fields fields, validate validateFunc) ve.ValidationErrors {
	var result ve.ValidationErrors
	for _, field := range fields {
		if len(parent) > 0 {
			field.Name = parent + "." + field.Name
		}
		errs := validate(fieldsFile, field)
		if len(errs) > 0 {
			result = append(result, errs...)
		}
		if len(field.Fields) > 0 {
			errs := validateNestedFields(field.Name, fieldsFile, field.Fields, validate)
			if len(errs) > 0 {
				result = append(result, errs...)
			}
		}
	}
	return result
}

func listFieldsFiles(fsys fspath.FS) ([]string, error) {
	var fieldsFiles []string
	dataStreams, err := listDataStreams(fsys)
	if err != nil {
		return nil, err
	}

	for _, dataStream := range dataStreams {
		fieldsDir := path.Join(dataStreamDir, dataStream, "fields")
		fs, err := fs.ReadDir(fsys, fieldsDir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "can't list fields directory (path: %s)", fsys.Path(fieldsDir))
		}

		for _, f := range fs {
			fieldsFiles = append(fieldsFiles, path.Join(fieldsDir, f.Name()))
		}
	}

	return fieldsFiles, nil
}

func unmarshalFields(fsys fspath.FS, fieldsPath string) (fields, error) {
	content, err := fs.ReadFile(fsys, fieldsPath)
	if err != nil {
		return nil, errors.Wrapf(err, "can't read file (path: %s)", fieldsPath)
	}

	var f fields
	err = yaml.Unmarshal(content, &f)
	if err != nil {
		return nil, errors.Wrapf(err, "yaml.Unmarshal failed (path: %s)", fieldsPath)
	}
	return f, nil
}

func dataStreamFromFieldsPath(pkgRoot, fieldsFile string) (string, error) {
	dataStreamPath := path.Clean(path.Join(pkgRoot, "data_stream")) + "/"
	if !strings.HasPrefix(path.Clean(fieldsFile), dataStreamPath) {
		return "", errors.Errorf("%q is not a fields file in a data stream of %q", fieldsFile, dataStreamPath)
	}
	relPath := strings.TrimPrefix(path.Clean(fieldsFile), dataStreamPath)

	parts := strings.SplitN(relPath, "/", 2)
	if len(parts) != 2 {
		return "", errors.Errorf("could not find data stream for fields file %s", fieldsFile)
	}
	dataStream := parts[0]
	return dataStream, nil
}

func listDataStreams(fsys fspath.FS) ([]string, error) {
	dataStreams, err := fs.ReadDir(fsys, dataStreamDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "can't list data streams directory")
	}

	list := make([]string, len(dataStreams))
	for i, dataStream := range dataStreams {
		list[i] = dataStream.Name()
	}
	return list, nil
}
