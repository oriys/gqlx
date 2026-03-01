package gqlx

// DirectiveLocation represents where a directive can be applied.
type DirectiveLocation string

const (
	DirectiveLocationQuery                DirectiveLocation = "QUERY"
	DirectiveLocationMutation             DirectiveLocation = "MUTATION"
	DirectiveLocationSubscription         DirectiveLocation = "SUBSCRIPTION"
	DirectiveLocationField                DirectiveLocation = "FIELD"
	DirectiveLocationFragmentDefinition   DirectiveLocation = "FRAGMENT_DEFINITION"
	DirectiveLocationFragmentSpread       DirectiveLocation = "FRAGMENT_SPREAD"
	DirectiveLocationInlineFragment       DirectiveLocation = "INLINE_FRAGMENT"
	DirectiveLocationVariableDefinition   DirectiveLocation = "VARIABLE_DEFINITION"
	DirectiveLocationSchema               DirectiveLocation = "SCHEMA"
	DirectiveLocationScalar               DirectiveLocation = "SCALAR"
	DirectiveLocationObject               DirectiveLocation = "OBJECT"
	DirectiveLocationFieldDefinition      DirectiveLocation = "FIELD_DEFINITION"
	DirectiveLocationArgumentDefinition   DirectiveLocation = "ARGUMENT_DEFINITION"
	DirectiveLocationInterface            DirectiveLocation = "INTERFACE"
	DirectiveLocationUnion                DirectiveLocation = "UNION"
	DirectiveLocationEnum                 DirectiveLocation = "ENUM"
	DirectiveLocationEnumValue            DirectiveLocation = "ENUM_VALUE"
	DirectiveLocationInputObject          DirectiveLocation = "INPUT_OBJECT"
	DirectiveLocationInputFieldDefinition DirectiveLocation = "INPUT_FIELD_DEFINITION"
)

// DirectiveDefinition defines a directive in the schema.
type DirectiveDefinition struct {
	Name_        string
	Description  string
	Locations    []DirectiveLocation
	Args         ArgumentMap
	IsRepeatable bool
}

// Built-in directives
var (
	IncludeDirective = &DirectiveDefinition{
		Name_:       "include",
		Description: "Directs the executor to include this field or fragment only when the `if` argument is true.",
		Locations: []DirectiveLocation{
			DirectiveLocationField,
			DirectiveLocationFragmentSpread,
			DirectiveLocationInlineFragment,
		},
		Args: ArgumentMap{
			"if": {
				Name_:       "if",
				Description: "Included when true.",
				Type:        NewNonNull(BooleanScalar),
			},
		},
	}

	SkipDirective = &DirectiveDefinition{
		Name_:       "skip",
		Description: "Directs the executor to skip this field or fragment when the `if` argument is true.",
		Locations: []DirectiveLocation{
			DirectiveLocationField,
			DirectiveLocationFragmentSpread,
			DirectiveLocationInlineFragment,
		},
		Args: ArgumentMap{
			"if": {
				Name_:       "if",
				Description: "Skipped when true.",
				Type:        NewNonNull(BooleanScalar),
			},
		},
	}

	DeprecatedDirective = &DirectiveDefinition{
		Name_:       "deprecated",
		Description: "Marks an element of a GraphQL schema as no longer supported.",
		Locations: []DirectiveLocation{
			DirectiveLocationFieldDefinition,
			DirectiveLocationEnumValue,
			DirectiveLocationArgumentDefinition,
			DirectiveLocationInputFieldDefinition,
		},
		Args: ArgumentMap{
			"reason": {
				Name_:        "reason",
				Description:  "Explains why this element was deprecated.",
				Type:         StringScalar,
				DefaultValue: "No longer supported",
			},
		},
	}

	SpecifiedByDirective = &DirectiveDefinition{
		Name_:       "specifiedBy",
		Description: "Exposes a URL that specifies the behavior of this scalar.",
		Locations: []DirectiveLocation{
			DirectiveLocationScalar,
		},
		Args: ArgumentMap{
			"url": {
				Name_:       "url",
				Description: "The URL that specifies the behavior of this scalar.",
				Type:        NewNonNull(StringScalar),
			},
		},
	}
)

// BuiltInDirectives returns all built-in directives.
func BuiltInDirectives() []*DirectiveDefinition {
	return []*DirectiveDefinition{IncludeDirective, SkipDirective, DeprecatedDirective, SpecifiedByDirective}
}
