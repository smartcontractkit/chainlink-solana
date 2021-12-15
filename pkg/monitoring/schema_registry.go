package monitoring

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riferrei/srclient"
	"github.com/stretchr/testify/assert"
)

type SchemaRegistry interface {
	// EnsureSchema handles three cases when pushing a schema spec to the SchemaRegistry:
	// 1. when the schema with a given subject does not exist, it will create it.
	// 2. if a schema with the given subject already exists but the spec is different, it will update it and bump the version.
	// 3. if the schema exists and the spec is the same, it will not do anything.
	EnsureSchema(subject, spec string) (Schema, error)
}

type schemaRegistry struct {
	backend *srclient.SchemaRegistryClient
}

func NewSchemaRegistry(cfg SchemaRegistryConfig) SchemaRegistry {
	backend := srclient.CreateSchemaRegistryClient(cfg.URL)
	if cfg.Username != "" && cfg.Password != "" {
		backend.SetCredentials(cfg.Username, cfg.Password)
	}
	return &schemaRegistry{backend}
}

func (s *schemaRegistry) EnsureSchema(subject, spec string) (Schema, error) {
	registeredSchema, err := s.backend.GetLatestSchema(subject)
	if err != nil && !isNotFoundErr(err) {
		return nil, fmt.Errorf("failed to read schema for subject '%s': %w", subject, err)
	}
	isEqualSchemas, errInIsEqualJSON := isEqualJSON(registeredSchema.Schema(), spec)
	if errInIsEqualJSON != nil {
		return nil, fmt.Errorf("failed to compare schama in registry with local schema: %w", errInIsEqualJSON)
	}
	if err == nil && isEqualSchemas {
		fmt.Printf("using existing schema for subject '%s'\n", subject)
		return wrapSchema{registeredSchema}, nil
	}
	fmt.Printf("creating new schema for subject '%s'\n", subject)
	newSchema, err := s.backend.CreateSchema(subject, spec, srclient.Avro)
	if err != nil {
		return nil, fmt.Errorf("unable to create new schema with subject '%s': %w", subject, err)
	}
	return wrapSchema{newSchema}, nil
}

type Schema interface {
	Encode(interface{}) ([]byte, error)
	Decode([]byte) (interface{}, error)
}

type wrapSchema struct {
	*srclient.Schema
}

func (w wrapSchema) Encode(value interface{}) ([]byte, error) {
	return w.Schema.Codec().BinaryFromNative(nil, value)
}

func (w wrapSchema) Decode(buf []byte) (interface{}, error) {
	value, _, err := w.Schema.Codec().NativeFromBinary(buf)
	return value, err
}

// Helpers

func isNotFoundErr(err error) bool {
	return strings.HasPrefix(err.Error(), "404 Not Found")
}

func isEqualJSON(a, b string) (bool, error) {
	var aUntyped, bUntyped interface{}

	if err := json.Unmarshal([]byte(a), &aUntyped); err != nil {
		return false, fmt.Errorf("failed to unmarshal first avro schema: %w", err)
	}
	if err := json.Unmarshal([]byte(b), &bUntyped); err != nil {
		return false, fmt.Errorf("failed to unmarshal second avro schema: %w", err)
	}

	return assert.ObjectsAreEqual(aUntyped, bUntyped), nil
}
