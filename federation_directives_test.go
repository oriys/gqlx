package gqlx

import "testing"

// TestFederationExternalFieldOwnership verifies that a field declared
// @external in one subgraph is owned by the subgraph that actually defines it.
func TestFederationExternalFieldOwnership(t *testing.T) {
	// Subgraph "users" owns User.nickname.
	usersUser := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"nickname": {
				Type: StringScalar,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return "Ally", nil
				},
			},
		},
	}
	users, err := NewSubgraph(SubgraphConfig{
		Name: "users",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"user": {
						Type: usersUser,
						Args: ArgumentMap{"id": {Name_: "id", Type: NewNonNull(IDScalar)}},
						Resolve: func(p ResolveParams) (interface{}, error) {
							return map[string]interface{}{"id": "1"}, nil
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("users subgraph: %v", err)
	}

	// Subgraph "orders" declares nickname as @external and adds orders.
	ordersUser := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id":       {Type: NewNonNull(IDScalar)},
			"nickname": {Type: StringScalar, External: true},
			"orders": {
				Type: NewList(StringScalar),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return []interface{}{"o1"}, nil
				},
			},
		},
	}
	orders, err := NewSubgraph(SubgraphConfig{
		Name: "orders",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_:   "Query",
				Fields_: FieldMap{"noop": {Type: StringScalar}},
			},
			Types: []GraphQLType{ordersUser},
		},
		Entities: []EntityConfig{
			{
				TypeName:  "User",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					return map[string]interface{}{
						"id":     repr["id"],
						"orders": []interface{}{"o1"},
					}, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("orders subgraph: %v", err)
	}

	// Orders first, so the external placeholder is created before the owner.
	gw, err := NewGateway(GatewayConfig{Subgraphs: []*Subgraph{orders, users}})
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}

	userType, ok := gw.Schema().Type("User").(*ObjectType)
	if !ok {
		t.Fatal("User type missing from supergraph")
	}
	if fd := userType.Fields_["nickname"]; fd.External {
		t.Fatal("nickname should be owned (non-external) in the supergraph")
	}

	result := gw.Execute(`{ user(id: "1") { nickname } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("execution errors: %v", FormatErrors(result.Errors))
	}
	user := result.Data.(map[string]interface{})["user"].(map[string]interface{})
	if user["nickname"] != "Ally" {
		t.Fatalf("nickname = %v, want Ally", user["nickname"])
	}
}

func TestFederationRequiresUnknownField(t *testing.T) {
	thing := &ObjectType{
		Name_: "Thing",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"a":  {Type: StringScalar},
			"b":  {Type: StringScalar, Requires: []string{"missing"}},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "things",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_:   "Query",
				Fields_: FieldMap{"thing": {Type: thing}},
			},
		},
	})
	if err != nil {
		t.Fatalf("subgraph: %v", err)
	}
	if _, err := NewGateway(GatewayConfig{Subgraphs: []*Subgraph{sg}}); err == nil {
		t.Fatal("expected an error for @requires referencing an unknown field")
	}
}

func TestFederationProvidesUnknownField(t *testing.T) {
	detail := &ObjectType{
		Name_:   "Detail",
		Fields_: FieldMap{"id": {Type: NewNonNull(StringScalar)}},
	}
	thing := &ObjectType{
		Name_: "Thing",
		Fields_: FieldMap{
			"id":     {Type: NewNonNull(IDScalar)},
			"detail": {Type: detail, Provides: []string{"missing"}},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "things",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_:   "Query",
				Fields_: FieldMap{"thing": {Type: thing}},
			},
		},
	})
	if err != nil {
		t.Fatalf("subgraph: %v", err)
	}
	if _, err := NewGateway(GatewayConfig{Subgraphs: []*Subgraph{sg}}); err == nil {
		t.Fatal("expected an error for @provides referencing an unknown field")
	}
}

func TestFederationRequiresKnownFieldAccepted(t *testing.T) {
	thing := &ObjectType{
		Name_: "Thing",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"a":  {Type: StringScalar},
			"b":  {Type: StringScalar, Requires: []string{"a"}},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "things",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_:   "Query",
				Fields_: FieldMap{"thing": {Type: thing}},
			},
		},
	})
	if err != nil {
		t.Fatalf("subgraph: %v", err)
	}
	if _, err := NewGateway(GatewayConfig{Subgraphs: []*Subgraph{sg}}); err != nil {
		t.Fatalf("unexpected composition error: %v", err)
	}
}
