package gqlx

import (
	"fmt"
	"strings"
)

// Gateway composes multiple subgraphs into a unified federated API.
type Gateway struct {
	subgraphs  []*Subgraph
	supergraph *Schema
	maxDepth   int
	// fieldOwner tracks which subgraph provides each field: typeName -> fieldName -> subgraph
	fieldOwner map[string]map[string]*Subgraph
}

// GatewayConfig is the configuration for creating a gateway.
type GatewayConfig struct {
	Subgraphs []*Subgraph
	MaxDepth  int // default 12; max 12
}

// DefaultMaxFederationDepth is the default (and maximum) allowed query depth.
const DefaultMaxFederationDepth = 12

// NewGateway creates a federated gateway by composing multiple subgraph schemas.
func NewGateway(config GatewayConfig) (*Gateway, error) {
	if len(config.Subgraphs) == 0 {
		return nil, fmt.Errorf("gateway requires at least one subgraph")
	}

	maxDepth := config.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxFederationDepth
	}
	if maxDepth > DefaultMaxFederationDepth {
		maxDepth = DefaultMaxFederationDepth
	}

	g := &Gateway{
		subgraphs:  config.Subgraphs,
		maxDepth:   maxDepth,
		fieldOwner: make(map[string]map[string]*Subgraph),
	}

	schema, err := g.compose()
	if err != nil {
		return nil, fmt.Errorf("gateway composition failed: %w", err)
	}
	g.supergraph = schema
	return g, nil
}

// Schema returns the composed supergraph schema.
func (g *Gateway) Schema() *Schema {
	return g.supergraph
}

// Execute parses, validates, and executes a query against the federated supergraph.
func (g *Gateway) Execute(query string, variables map[string]interface{}, operationName string) *Result {
	if g.supergraph == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide schema"}}}
	}

	doc, err := Parse(query)
	if err != nil {
		return &Result{Errors: []*GraphQLError{FormatError(err)}}
	}

	validationErrors := Validate(g.supergraph, doc)
	if len(validationErrors) > 0 {
		return &Result{Errors: validationErrors}
	}

	return Execute(ExecuteParams{
		Schema:        g.supergraph,
		Document:      doc,
		Variables:     variables,
		OperationName: operationName,
		MaxDepth:      g.maxDepth,
	})
}

// compose builds the supergraph schema by merging all subgraph schemas.
func (g *Gateway) compose() (*Schema, error) {
	// Phase 1: Collect field ownership and merge types
	mergedObjects := make(map[string]*ObjectType)
	mergedEnums := make(map[string]*EnumType)
	mergedInterfaces := make(map[string]*InterfaceType)

	for _, sg := range g.subgraphs {
		for name, typ := range sg.Schema.TypeMap() {
			if isBuiltInType(name) {
				continue
			}
			if g.fieldOwner[name] == nil {
				g.fieldOwner[name] = make(map[string]*Subgraph)
			}

			switch t := typ.(type) {
			case *ObjectType:
				if existing, ok := mergedObjects[name]; ok {
					for fn, fd := range t.Fields_ {
						if _, exists := existing.Fields_[fn]; !exists {
							existing.Fields_[fn] = fd
						}
						if g.fieldOwner[name][fn] == nil {
							g.fieldOwner[name][fn] = sg
						}
					}
				} else {
					clonedFields := make(FieldMap, len(t.Fields_))
					for fn, fd := range t.Fields_ {
						clonedFields[fn] = fd
						if g.fieldOwner[name][fn] == nil {
							g.fieldOwner[name][fn] = sg
						}
					}
					mergedObjects[name] = &ObjectType{
						Name_:       t.Name_,
						Description: t.Description,
						Fields_:     clonedFields,
						Interfaces_: t.Interfaces_,
						IsTypeOf:    t.IsTypeOf,
					}
				}
			case *EnumType:
				if _, ok := mergedEnums[name]; !ok {
					mergedEnums[name] = t
				}
			case *InterfaceType:
				if _, ok := mergedInterfaces[name]; !ok {
					mergedInterfaces[name] = t
				}
			}
		}
	}

	// Phase 2: Rewrite type references to point to merged types
	for _, obj := range mergedObjects {
		for fn, fd := range obj.Fields_ {
			obj.Fields_[fn] = &FieldDefinition{
				Name_:             fd.Name_,
				Description:       fd.Description,
				Type:              rewriteTypeRef(fd.Type, mergedObjects),
				Args:              rewriteArgs(fd.Args, mergedObjects),
				Resolve:           fd.Resolve,
				DeprecationReason: fd.DeprecationReason,
			}
		}
	}

	// Phase 3: Wire up entity-crossing resolvers
	for typeName, obj := range mergedObjects {
		for fieldName, fd := range obj.Fields_ {
			ownerSg := g.fieldOwner[typeName][fieldName]
			if ownerSg == nil {
				continue
			}
			entity, isEntity := ownerSg.Entities[typeName]
			if !isEntity {
				continue
			}
			// This field is on an entity type contributed by ownerSg.
			// Set up a resolver that handles cross-subgraph entity resolution.
			obj.Fields_[fieldName] = &FieldDefinition{
				Name_:             fd.Name_,
				Description:       fd.Description,
				Type:              fd.Type,
				Args:              fd.Args,
				DeprecationReason: fd.DeprecationReason,
				Resolve:           makeEntityFieldResolver(typeName, fieldName, ownerSg, entity),
			}
		}
	}

	// Phase 4: Detect entity reference cycles
	if err := g.detectEntityCycles(mergedObjects); err != nil {
		return nil, err
	}

	// Phase 5: Build merged Query and Mutation types
	var queryType, mutationType *ObjectType

	for _, sg := range g.subgraphs {
		if sg.Schema.QueryType != nil {
			name := sg.Schema.QueryType.Name_
			if merged, ok := mergedObjects[name]; ok {
				queryType = merged
			}
		}
		if sg.Schema.MutationType != nil {
			name := sg.Schema.MutationType.Name_
			if merged, ok := mergedObjects[name]; ok {
				mutationType = merged
			}
		}
	}

	if queryType == nil {
		return nil, fmt.Errorf("no query type found in subgraphs")
	}

	// Wire up root Query/Mutation field resolvers from their owning subgraphs
	wireRootResolvers(queryType, g.fieldOwner, g.subgraphs, true)
	if mutationType != nil {
		wireRootResolvers(mutationType, g.fieldOwner, g.subgraphs, false)
	}

	// Phase 6: Collect additional types for schema
	var additionalTypes []GraphQLType
	for name, obj := range mergedObjects {
		if queryType != nil && name == queryType.Name_ {
			continue
		}
		if mutationType != nil && name == mutationType.Name_ {
			continue
		}
		additionalTypes = append(additionalTypes, obj)
	}
	for _, e := range mergedEnums {
		additionalTypes = append(additionalTypes, e)
	}
	for _, iface := range mergedInterfaces {
		additionalTypes = append(additionalTypes, iface)
	}

	return NewSchema(SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
		Types:    additionalTypes,
	})
}

// detectEntityCycles checks for circular entity references across subgraphs.
// For example: User has orders (→Order), Order has product (→Product), Product has user (→User) = cycle.
// It builds a directed graph of entity type references and detects cycles using DFS.
func (g *Gateway) detectEntityCycles(mergedObjects map[string]*ObjectType) error {
	// Build adjacency list: typeName -> set of referenced entity type names
	adj := make(map[string]map[string]bool)
	entityTypes := make(map[string]bool)

	// Collect all entity type names across subgraphs
	for _, sg := range g.subgraphs {
		for typeName := range sg.Entities {
			entityTypes[typeName] = true
		}
	}

	// For each entity type, find which other entity types it references via object fields
	for typeName := range entityTypes {
		obj, ok := mergedObjects[typeName]
		if !ok {
			continue
		}
		refs := make(map[string]bool)
		for _, fd := range obj.Fields_ {
			refType := UnwrapType(fd.Type).TypeName()
			if refType != "" && refType != typeName && entityTypes[refType] {
				refs[refType] = true
			}
		}
		if len(refs) > 0 {
			adj[typeName] = refs
		}
	}

	// DFS cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully processed
	)
	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string) []string
	dfs = func(node string) []string {
		color[node] = gray
		for neighbor := range adj[node] {
			if color[neighbor] == gray {
				// Found cycle - reconstruct it
				cycle := []string{neighbor, node}
				cur := node
				for cur != neighbor {
					cur = parent[cur]
					if cur == "" {
						break
					}
					cycle = append(cycle, cur)
				}
				// Reverse to get readable order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return cycle
			}
			if color[neighbor] == white {
				parent[neighbor] = node
				if cycle := dfs(neighbor); cycle != nil {
					return cycle
				}
			}
		}
		color[node] = black
		return nil
	}

	for typeName := range entityTypes {
		if color[typeName] == white {
			if cycle := dfs(typeName); cycle != nil {
				return fmt.Errorf("federation: entity reference cycle detected: %s", formatCycle(cycle))
			}
		}
	}

	return nil
}

// formatCycle formats a cycle path for error messages.
func formatCycle(cycle []string) string {
	return strings.Join(append(cycle, cycle[0]), " → ")
}

// wireRootResolvers ensures root query/mutation fields have resolvers from their owning subgraphs.
func wireRootResolvers(rootType *ObjectType, fieldOwner map[string]map[string]*Subgraph, subgraphs []*Subgraph, isQuery bool) {
	typeName := rootType.Name_
	for fieldName, fd := range rootType.Fields_ {
		if fd.Resolve != nil {
			continue
		}
		ownerSg := fieldOwner[typeName][fieldName]
		if ownerSg == nil {
			continue
		}
		var srcType *ObjectType
		if isQuery {
			srcType = ownerSg.Schema.QueryType
		} else {
			srcType = ownerSg.Schema.MutationType
		}
		if srcType == nil {
			continue
		}
		if srcField, ok := srcType.Fields_[fieldName]; ok && srcField.Resolve != nil {
			fd.Resolve = srcField.Resolve
		}
	}
}

// makeEntityFieldResolver creates a field resolver that handles cross-subgraph entity resolution.
// When the field's data isn't in the source (came from a different subgraph), it resolves the
// entity via the owning subgraph's reference resolver.
func makeEntityFieldResolver(typeName, fieldName string, sg *Subgraph, entity *EntityDefinition) ResolveFunc {
	return func(p ResolveParams) (interface{}, error) {
		source, ok := p.Source.(map[string]interface{})
		if !ok {
			return resolveFieldValue(p.Source, fieldName), nil
		}

		// Fast path: field already in source (same subgraph provided it)
		if val, exists := source[fieldName]; exists {
			return val, nil
		}

		// Check entity resolution cache for this subgraph+type
		cacheKey := "__entity_" + sg.Name + "_" + typeName
		if cached, ok := source[cacheKey]; ok {
			if m, ok := cached.(map[string]interface{}); ok {
				return m[fieldName], nil
			}
		}

		// Build entity representation from key fields
		repr := map[string]interface{}{"__typename": typeName}
		for _, kf := range entity.KeyFields {
			val, exists := source[kf]
			if !exists {
				return nil, fmt.Errorf("federation: missing key field %q on %s for entity resolution in subgraph %q", kf, typeName, sg.Name)
			}
			repr[kf] = val
		}

		// Resolve entity from the owning subgraph
		resolved, err := entity.Resolver(repr)
		if err != nil {
			return nil, fmt.Errorf("federation: entity resolution failed for %s in subgraph %q: %w", typeName, sg.Name, err)
		}
		if resolved == nil {
			return nil, nil
		}

		// Cache the resolved entity to avoid re-resolving for sibling fields
		if resolvedMap, ok := resolved.(map[string]interface{}); ok {
			source[cacheKey] = resolvedMap
			return resolvedMap[fieldName], nil
		}

		return resolveFieldValue(resolved, fieldName), nil
	}
}

// rewriteTypeRef rewrites type references to use merged ObjectType instances.
func rewriteTypeRef(typ GraphQLType, merged map[string]*ObjectType) GraphQLType {
	switch t := typ.(type) {
	case *ObjectType:
		if m, ok := merged[t.Name_]; ok {
			return m
		}
		return t
	case *NonNullOfType:
		return NewNonNull(rewriteTypeRef(t.OfType, merged))
	case *ListOfType:
		return NewList(rewriteTypeRef(t.OfType, merged))
	default:
		return typ
	}
}

// rewriteArgs rewrites argument type references.
func rewriteArgs(args ArgumentMap, merged map[string]*ObjectType) ArgumentMap {
	if len(args) == 0 {
		return args
	}
	result := make(ArgumentMap, len(args))
	for k, v := range args {
		result[k] = &ArgumentDefinition{
			Name_:        v.Name_,
			Description:  v.Description,
			Type:         rewriteTypeRef(v.Type, merged),
			DefaultValue: v.DefaultValue,
		}
	}
	return result
}

func isBuiltInType(name string) bool {
	switch name {
	case "Int", "Float", "String", "Boolean", "ID",
		"__Schema", "__Type", "__Field", "__InputValue", "__EnumValue", "__Directive":
		return true
	}
	return false
}
