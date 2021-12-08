package monitoring

/*
import (
	"fmt"

	"github.com/riferrei/srclient"
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
	if err != nil {
		return nil, fmt.Errorf("failed to read schema for subject '%s' with error: %w", subject, err)
	}
	if registeredSchema.Schema() == spec {
		return wrapSchema{registeredSchema}, nil
	}
	newSchema, err := s.backend.CreateSchema(subject, spec, srclient.Avro)
	if err != nil {
		return nil, fmt.Errorf("unale to create new schema with subject '%s' and spec\n%s\n with error: %w", subject, spec, err)
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
*/
