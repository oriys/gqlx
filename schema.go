package gqlx

// Schema represents a GraphQL schema.
type Schema struct {
	QueryType        *ObjectType
	MutationType     *ObjectType
	SubscriptionType *ObjectType
	Directives_      []*DirectiveDefinition
	typeMap          map[string]GraphQLType
	implementations  map[string][]*ObjectType
}

// SchemaConfig is the configuration for building a schema.
type SchemaConfig struct {
	Query        *ObjectType
	Mutation     *ObjectType
	Subscription *ObjectType
	Types        []GraphQLType
	Directives   []*DirectiveDefinition
}

// NewSchema creates a new schema from the given configuration.
func NewSchema(config SchemaConfig) (*Schema, error) {
	if config.Query == nil {
		return nil, &GraphQLError{Message: "Must provide Query type"}
	}

	s := &Schema{
		QueryType:        config.Query,
		MutationType:     config.Mutation,
		SubscriptionType: config.Subscription,
		typeMap:          make(map[string]GraphQLType),
		implementations:  make(map[string][]*ObjectType),
	}

	if len(config.Directives) > 0 {
		s.Directives_ = config.Directives
	} else {
		s.Directives_ = BuiltInDirectives()
	}

	// Collect all types
	s.collectTypes(config.Query)
	if config.Mutation != nil {
		s.collectTypes(config.Mutation)
	}
	if config.Subscription != nil {
		s.collectTypes(config.Subscription)
	}
	for _, t := range config.Types {
		s.collectTypes(t)
	}

	// Always include built-in scalars
	for _, scalar := range []*ScalarType{IntScalar, FloatScalar, StringScalar, BooleanScalar, IDScalar} {
		if _, exists := s.typeMap[scalar.Name_]; !exists {
			s.typeMap[scalar.Name_] = scalar
		}
	}

	// Include introspection types so they pass validation.
	// Create types first, then wire up cross-references using actual instances.
	iSchema := &ObjectType{Name_: "__Schema", Description: "Built-in introspection type"}
	iType := &ObjectType{Name_: "__Type", Description: "Built-in introspection type"}
	iField := &ObjectType{Name_: "__Field", Description: "Built-in introspection type"}
	iInputValue := &ObjectType{Name_: "__InputValue", Description: "Built-in introspection type"}
	iEnumValue := &ObjectType{Name_: "__EnumValue", Description: "Built-in introspection type"}
	iDirective := &ObjectType{Name_: "__Directive", Description: "Built-in introspection type"}

	iSchema.Fields_ = FieldMap{
		"types":            {Type: NewNonNull(NewList(NewNonNull(iType)))},
		"queryType":        {Type: NewNonNull(iType)},
		"mutationType":     {Type: iType},
		"subscriptionType": {Type: iType},
		"directives":       {Type: NewNonNull(NewList(NewNonNull(iDirective)))},
		"description":      {Type: StringScalar},
	}
	iType.Fields_ = FieldMap{
		"kind":            {Type: NewNonNull(StringScalar)},
		"name":            {Type: StringScalar},
		"description":     {Type: StringScalar},
		"fields":          {Type: NewList(NewNonNull(iField)), Args: ArgumentMap{"includeDeprecated": {Name_: "includeDeprecated", Type: BooleanScalar}}},
		"interfaces":      {Type: NewList(NewNonNull(iType))},
		"possibleTypes":   {Type: NewList(NewNonNull(iType))},
		"enumValues":      {Type: NewList(NewNonNull(iEnumValue)), Args: ArgumentMap{"includeDeprecated": {Name_: "includeDeprecated", Type: BooleanScalar}}},
		"inputFields":     {Type: NewList(NewNonNull(iInputValue)), Args: ArgumentMap{"includeDeprecated": {Name_: "includeDeprecated", Type: BooleanScalar}}},
		"ofType":          {Type: iType},
		"specifiedByURL":  {Type: StringScalar},
	}
	iField.Fields_ = FieldMap{
		"name":              {Type: NewNonNull(StringScalar)},
		"description":       {Type: StringScalar},
		"args":              {Type: NewNonNull(NewList(NewNonNull(iInputValue)))},
		"type":              {Type: NewNonNull(iType)},
		"isDeprecated":      {Type: NewNonNull(BooleanScalar)},
		"deprecationReason": {Type: StringScalar},
	}
	iInputValue.Fields_ = FieldMap{
		"name":              {Type: NewNonNull(StringScalar)},
		"description":       {Type: StringScalar},
		"type":              {Type: NewNonNull(iType)},
		"defaultValue":      {Type: StringScalar},
		"isDeprecated":      {Type: NewNonNull(BooleanScalar)},
		"deprecationReason": {Type: StringScalar},
	}
	iEnumValue.Fields_ = FieldMap{
		"name":              {Type: NewNonNull(StringScalar)},
		"description":       {Type: StringScalar},
		"isDeprecated":      {Type: NewNonNull(BooleanScalar)},
		"deprecationReason": {Type: StringScalar},
	}
	iDirective.Fields_ = FieldMap{
		"name":         {Type: NewNonNull(StringScalar)},
		"description":  {Type: StringScalar},
		"locations":    {Type: NewNonNull(NewList(NewNonNull(StringScalar)))},
		"args":         {Type: NewNonNull(NewList(NewNonNull(iInputValue)))},
		"isRepeatable": {Type: NewNonNull(BooleanScalar)},
	}

	for _, obj := range []*ObjectType{iSchema, iType, iField, iInputValue, iEnumValue, iDirective} {
		if _, exists := s.typeMap[obj.Name_]; !exists {
			s.typeMap[obj.Name_] = obj
		}
	}

	// Build implementations map
	for _, t := range s.typeMap {
		if obj, ok := t.(*ObjectType); ok {
			for _, iface := range obj.Interfaces_ {
				s.implementations[iface.Name_] = append(s.implementations[iface.Name_], obj)
			}
		}
	}

	return s, nil
}

func (s *Schema) collectTypes(t GraphQLType) {
	if t == nil {
		return
	}

	switch typ := t.(type) {
	case *ListOfType:
		s.collectTypes(typ.OfType)
		return
	case *NonNullOfType:
		s.collectTypes(typ.OfType)
		return
	}

	name := t.TypeName()
	if name == "" {
		return
	}
	if _, exists := s.typeMap[name]; exists {
		return
	}
	s.typeMap[name] = t

	switch typ := t.(type) {
	case *ObjectType:
		for _, iface := range typ.Interfaces_ {
			s.collectTypes(iface)
		}
		for _, field := range typ.Fields_ {
			s.collectTypes(field.Type)
			for _, arg := range field.Args {
				s.collectTypes(arg.Type)
			}
		}
	case *InterfaceType:
		for _, field := range typ.Fields_ {
			s.collectTypes(field.Type)
			for _, arg := range field.Args {
				s.collectTypes(arg.Type)
			}
		}
	case *UnionType:
		for _, member := range typ.Types {
			s.collectTypes(member)
		}
	case *InputObjectType:
		for _, field := range typ.Fields_ {
			s.collectTypes(field.Type)
		}
	case *EnumType:
		// no further types to collect
	case *ScalarType:
		// no further types to collect
	}
}

// TypeMap returns all types in the schema.
func (s *Schema) TypeMap() map[string]GraphQLType {
	return s.typeMap
}

// Type returns a type by name.
func (s *Schema) Type(name string) GraphQLType {
	return s.typeMap[name]
}

// GetImplementations returns all object types that implement the given interface.
func (s *Schema) GetImplementations(iface *InterfaceType) []*ObjectType {
	return s.implementations[iface.Name_]
}

// IsPossibleType returns true if the given type is a possible type for the abstract type.
func (s *Schema) IsPossibleType(abstractType GraphQLType, objectType *ObjectType) bool {
	switch at := abstractType.(type) {
	case *InterfaceType:
		for _, impl := range s.implementations[at.Name_] {
			if impl == objectType {
				return true
			}
		}
	case *UnionType:
		for _, member := range at.Types {
			if member == objectType {
				return true
			}
		}
	}
	return false
}

// Directives returns all directives in the schema.
func (s *Schema) Directives() []*DirectiveDefinition {
	return s.Directives_
}

// Directive returns a directive by name.
func (s *Schema) Directive(name string) *DirectiveDefinition {
	for _, d := range s.Directives_ {
		if d.Name_ == name {
			return d
		}
	}
	return nil
}
