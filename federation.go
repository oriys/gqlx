package gqlx

import "fmt"

// Federation directives following Apollo Federation v2 spec.
var (
	KeyDirective = &DirectiveDefinition{
		Name_:        "key",
		Description:  "Marks a type as an entity and specifies its key fields",
		Locations:    []DirectiveLocation{DirectiveLocationObject, DirectiveLocationInterface},
		Args:         ArgumentMap{"fields": {Name_: "fields", Type: NewNonNull(StringScalar)}},
		IsRepeatable: true,
	}

	ExternalDirective = &DirectiveDefinition{
		Name_:       "external",
		Description: "Marks a field as owned by another subgraph",
		Locations:   []DirectiveLocation{DirectiveLocationFieldDefinition, DirectiveLocationObject},
	}

	RequiresDirective = &DirectiveDefinition{
		Name_:       "requires",
		Description: "Specifies required fields from the base subgraph",
		Locations:   []DirectiveLocation{DirectiveLocationFieldDefinition},
		Args:        ArgumentMap{"fields": {Name_: "fields", Type: NewNonNull(StringScalar)}},
	}

	ProvidesDirective = &DirectiveDefinition{
		Name_:       "provides",
		Description: "Specifies fields provided for a related entity",
		Locations:   []DirectiveLocation{DirectiveLocationFieldDefinition},
		Args:        ArgumentMap{"fields": {Name_: "fields", Type: NewNonNull(StringScalar)}},
	}
)

// FederationDirectives returns all federation-specific directives.
func FederationDirectives() []*DirectiveDefinition {
	return []*DirectiveDefinition{KeyDirective, ExternalDirective, RequiresDirective, ProvidesDirective}
}

// ReferenceResolver resolves an entity from a representation containing __typename and key fields.
type ReferenceResolver func(representation map[string]interface{}) (interface{}, error)

// EntityDefinition defines how an entity type is resolved in a subgraph.
type EntityDefinition struct {
	TypeName  string
	KeyFields []string
	Resolver  ReferenceResolver
}

// Subgraph represents a federation subgraph service.
type Subgraph struct {
	Name     string
	Schema   *Schema
	Entities map[string]*EntityDefinition
}

// SubgraphConfig is the configuration for creating a subgraph.
type SubgraphConfig struct {
	Name     string
	Schema   SchemaConfig
	Entities []EntityConfig
}

// EntityConfig configures an entity type within a subgraph.
type EntityConfig struct {
	TypeName  string
	KeyFields []string
	Resolver  ReferenceResolver
}

// NewSubgraph creates a new federation subgraph.
func NewSubgraph(config SubgraphConfig) (*Subgraph, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("subgraph name is required")
	}

	config.Schema.Directives = append(config.Schema.Directives, FederationDirectives()...)
	schema, err := NewSchema(config.Schema)
	if err != nil {
		return nil, fmt.Errorf("subgraph %q: %w", config.Name, err)
	}

	entities := make(map[string]*EntityDefinition, len(config.Entities))
	for _, e := range config.Entities {
		if e.TypeName == "" {
			return nil, fmt.Errorf("subgraph %q: entity type name is required", config.Name)
		}
		if len(e.KeyFields) == 0 {
			return nil, fmt.Errorf("subgraph %q: entity %q must have at least one key field", config.Name, e.TypeName)
		}
		if e.Resolver == nil {
			return nil, fmt.Errorf("subgraph %q: entity %q must have a reference resolver", config.Name, e.TypeName)
		}
		entities[e.TypeName] = &EntityDefinition{
			TypeName:  e.TypeName,
			KeyFields: e.KeyFields,
			Resolver:  e.Resolver,
		}
	}

	return &Subgraph{
		Name:     config.Name,
		Schema:   schema,
		Entities: entities,
	}, nil
}
