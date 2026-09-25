package gqlx

import (
	"reflect"
	"strings"
	"testing"
)

func threeLevelGateway(t *testing.T) *Gateway {
	t.Helper()
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph(), makeOrderSubgraph(), makeProductSubgraph()},
	})
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}
	return gw
}

func TestPlannerThreeLevel(t *testing.T) {
	gw := threeLevelGateway(t)
	query := `{
		user(id: "1") {
			name
			orders {
				quantity
				product {
					name
					price
				}
			}
		}
	}`

	result := gw.ExecutePlanned(query, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}

	user := result.Data.(map[string]interface{})["user"].(map[string]interface{})
	if user["name"] != "Alice" {
		t.Fatalf("name = %v", user["name"])
	}
	orders := user["orders"].([]interface{})
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
	product := orders[0].(map[string]interface{})["product"].(map[string]interface{})
	if product["name"] != "Widget" || product["price"] != 9.99 {
		t.Fatalf("unexpected product: %#v", product)
	}
	// Injected fields must not leak into the response.
	if _, ok := user["id"]; ok {
		t.Fatal("injected id leaked into response")
	}
	if _, ok := user["__typename"]; ok {
		t.Fatal("injected __typename leaked into response")
	}
	order0 := orders[0].(map[string]interface{})
	if _, ok := order0["id"]; ok {
		t.Fatal("injected order id leaked into response")
	}
}

func TestPlannerMatchesDirectExecution(t *testing.T) {
	gw := threeLevelGateway(t)
	queries := []string{
		`{ user(id: "1") { name } }`,
		`{ user(id: "1") { name orders { id quantity } } }`,
		`{ user(id: "1") { name orders { quantity product { name price } } } }`,
		`{ user(id: "1") { orders { product { name } } } }`,
		`{ product(id: "p1") { name price } }`,
		`{ user(id: "3") { name orders { id } } }`,
		`query Q($id: ID!) { user(id: $id) { name } }`,
	}
	for _, q := range queries {
		direct := gw.Execute(q, map[string]interface{}{"id": "1"}, "")
		planned := gw.ExecutePlanned(q, map[string]interface{}{"id": "1"}, "")
		if direct.HasErrors() {
			t.Fatalf("%s direct errors: %v", q, FormatErrors(direct.Errors))
		}
		if planned.HasErrors() {
			t.Fatalf("%s planned errors: %v", q, FormatErrors(planned.Errors))
		}
		if !reflect.DeepEqual(direct.Data, planned.Data) {
			t.Fatalf("%s\n direct:  %#v\n planned: %#v", q, direct.Data, planned.Data)
		}
	}
}

func TestPlannerMultipleRootFieldsParallel(t *testing.T) {
	gw := threeLevelGateway(t)
	query := `{
		user(id: "1") { name }
		product(id: "p1") { name }
	}`
	plan, err := gw.Plan(query)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	root, ok := plan.Root.(*ParallelNode)
	if !ok {
		t.Fatalf("expected parallel root, got %T", plan.Root)
	}
	if len(root.Steps) != 2 {
		t.Fatalf("expected 2 root fetches, got %d", len(root.Steps))
	}

	result := gw.ExecutePlanned(query, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["user"].(map[string]interface{})["name"] != "Alice" {
		t.Fatal("user lookup failed")
	}
	if data["product"].(map[string]interface{})["name"] != "Widget" {
		t.Fatal("product lookup failed")
	}
}

func TestPlannerPlanShape(t *testing.T) {
	gw := threeLevelGateway(t)
	plan, err := gw.Plan(`{ user(id: "1") { name orders { quantity product { name } } } }`)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	desc := plan.Describe()
	for _, want := range []string{"Parallel", "service=users", "Fetch[entity User]", "service=orders", "Fetch[entity Order]", "service=products"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("plan description missing %q:\n%s", want, desc)
		}
	}
}

func TestPlannerAliasesAndDirectives(t *testing.T) {
	gw := threeLevelGateway(t)
	query := `{
		u: user(id: "1") {
			n: name
			orders @include(if: true) { id }
			orders @skip(if: true) { quantity }
		}
	}`
	result := gw.ExecutePlanned(query, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	u := result.Data.(map[string]interface{})["u"].(map[string]interface{})
	if u["n"] != "Alice" {
		t.Fatalf("alias n = %v", u["n"])
	}
	orders, ok := u["orders"].([]interface{})
	if !ok || len(orders) == 0 {
		t.Fatalf("orders missing: %#v", u)
	}
	// The @skip'd duplicate must not contribute a quantity field.
	if _, ok := orders[0].(map[string]interface{})["quantity"]; ok {
		t.Fatal("skipped selection leaked quantity")
	}
}

func TestPlannerBatchedEntityResolution(t *testing.T) {
	var batchCalls int
	userType := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id":   {Type: NewNonNull(IDScalar)},
			"name": {Type: NewNonNull(StringScalar)},
		},
	}
	orderType := &ObjectType{
		Name_: "Order",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
		},
	}
	users, err := NewSubgraph(SubgraphConfig{
		Name: "users",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"users": {
						Type: NewList(userType),
						Resolve: func(p ResolveParams) (interface{}, error) {
							return []interface{}{
								map[string]interface{}{"id": "1", "name": "Alice"},
								map[string]interface{}{"id": "2", "name": "Bob"},
								map[string]interface{}{"id": "3", "name": "Carol"},
							}, nil
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("users: %v", err)
	}

	userWithOrders := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id":     {Type: NewNonNull(IDScalar)},
			"orders": {Type: NewList(orderType)},
		},
	}
	orders, err := NewSubgraph(SubgraphConfig{
		Name: "orders",
		Schema: SchemaConfig{
			Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"noop": {Type: StringScalar}}},
			Types: []GraphQLType{userWithOrders, orderType},
		},
		Entities: []EntityConfig{
			{
				TypeName:  "User",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					return map[string]interface{}{"id": repr["id"], "orders": []interface{}{map[string]interface{}{"id": "o"}}}, nil
				},
				BatchResolver: func(reprs []map[string]interface{}) ([]interface{}, error) {
					batchCalls++
					out := make([]interface{}, len(reprs))
					for i, r := range reprs {
						out[i] = map[string]interface{}{
							"id":     r["id"],
							"orders": []interface{}{map[string]interface{}{"id": "o"}},
						}
					}
					return out, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("orders: %v", err)
	}

	gw, err := NewGateway(GatewayConfig{Subgraphs: []*Subgraph{users, orders}})
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}

	result := gw.ExecutePlanned(`{ users { name orders { id } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	if batchCalls != 1 {
		t.Fatalf("expected 1 batched entity call, got %d", batchCalls)
	}
	usersList := result.Data.(map[string]interface{})["users"].([]interface{})
	if len(usersList) != 3 {
		t.Fatalf("expected 3 users, got %d", len(usersList))
	}
	for _, u := range usersList {
		if _, ok := u.(map[string]interface{})["orders"]; !ok {
			t.Fatalf("orders missing on %#v", u)
		}
	}
}

func TestPlannerNestedFragments(t *testing.T) {
	gw := threeLevelGateway(t)
	query := `{
		user(id: "1") {
			...UserBits
		}
	}
	fragment UserBits on User {
		name
		orders { id }
	}`
	result := gw.ExecutePlanned(query, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	user := result.Data.(map[string]interface{})["user"].(map[string]interface{})
	if user["name"] != "Alice" {
		t.Fatalf("name = %v", user["name"])
	}
	if _, ok := user["orders"]; !ok {
		t.Fatal("orders missing")
	}
}

func TestPlannerFourLevelAgainstDirect(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph(), makeOrderSubgraph(), makeProductSubgraph(), makeCategorySubgraph()},
	})
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}
	query := `{
		user(id: "1") {
			name
			orders {
				quantity
				product {
					name
					price
					category { label tier }
				}
			}
		}
	}`
	direct := gw.Execute(query, nil, "")
	planned := gw.ExecutePlanned(query, nil, "")
	if direct.HasErrors() {
		t.Fatalf("direct errors: %v", FormatErrors(direct.Errors))
	}
	if planned.HasErrors() {
		t.Fatalf("planned errors: %v", FormatErrors(planned.Errors))
	}
	if !reflect.DeepEqual(direct.Data, planned.Data) {
		t.Fatalf("mismatch\n direct:  %#v\n planned: %#v", direct.Data, planned.Data)
	}
}
