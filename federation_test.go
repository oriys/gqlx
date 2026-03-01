package gqlx

import (
	"fmt"
	"testing"
)

// ============================================================
// Test data: 4 subgraphs for multi-level federation
// ============================================================

// Service A: Users - owns User type
var usersData = map[string]map[string]interface{}{
	"1": {"id": "1", "name": "Alice", "email": "alice@example.com"},
	"2": {"id": "2", "name": "Bob", "email": "bob@example.com"},
	"3": {"id": "3", "name": "Charlie", "email": "charlie@example.com"},
}

// Service B: Orders - extends User with orders, owns Order type
var ordersData = map[string][]map[string]interface{}{
	"1": {
		{"id": "o1", "userId": "1", "productId": "p1", "quantity": 2},
		{"id": "o2", "userId": "1", "productId": "p2", "quantity": 1},
	},
	"2": {
		{"id": "o3", "userId": "2", "productId": "p1", "quantity": 5},
	},
}

// Service C: Products - extends Order with product, owns Product type
var productsData = map[string]map[string]interface{}{
	"p1": {"id": "p1", "name": "Widget", "price": 9.99, "categoryId": "c1"},
	"p2": {"id": "p2", "name": "Gadget", "price": 24.99, "categoryId": "c2"},
}

// Mapping from orderId -> productId
var orderProductMap = map[string]string{
	"o1": "p1",
	"o2": "p2",
	"o3": "p1",
}

// Service D: Categories - extends Product with category, owns Category type
var categoriesData = map[string]map[string]interface{}{
	"c1": {"id": "c1", "label": "Hardware", "tier": "premium"},
	"c2": {"id": "c2", "label": "Electronics", "tier": "standard"},
}

var productCategoryMap = map[string]string{
	"p1": "c1",
	"p2": "c2",
}

func makeUserSubgraph() *Subgraph {
	userType := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id":    {Type: NewNonNull(IDScalar)},
			"name":  {Type: NewNonNull(StringScalar)},
			"email": {Type: NewNonNull(StringScalar)},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "users",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"user": {
						Type: userType,
						Args: ArgumentMap{"id": {Name_: "id", Type: NewNonNull(IDScalar)}},
						Resolve: func(p ResolveParams) (interface{}, error) {
							id, _ := p.Args["id"].(string)
							if u, ok := usersData[id]; ok {
								return copyMap(u), nil
							}
							return nil, nil
						},
					},
					"users": {
						Type: NewList(userType),
						Resolve: func(p ResolveParams) (interface{}, error) {
							var result []interface{}
							for _, u := range usersData {
								result = append(result, copyMap(u))
							}
							return result, nil
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return sg
}

func makeOrderSubgraph() *Subgraph {
	orderType := &ObjectType{
		Name_: "Order",
		Fields_: FieldMap{
			"id":        {Type: NewNonNull(IDScalar)},
			"userId":    {Type: NewNonNull(IDScalar)},
			"productId": {Type: NewNonNull(IDScalar)},
			"quantity":  {Type: NewNonNull(IntScalar)},
		},
	}
	userType := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"orders": {
				Type: NewList(NewNonNull(orderType)),
				Resolve: func(p ResolveParams) (interface{}, error) {
					source, _ := p.Source.(map[string]interface{})
					userId, _ := source["id"].(string)
					if orders, ok := ordersData[userId]; ok {
						result := make([]interface{}, len(orders))
						for i, o := range orders {
							result[i] = copyMap(o)
						}
						return result, nil
					}
					return []interface{}{}, nil
				},
			},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "orders",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"order": {
						Type: orderType,
						Args: ArgumentMap{"id": {Name_: "id", Type: NewNonNull(IDScalar)}},
						Resolve: func(p ResolveParams) (interface{}, error) {
							id, _ := p.Args["id"].(string)
							for _, orders := range ordersData {
								for _, o := range orders {
									if o["id"] == id {
										return copyMap(o), nil
									}
								}
							}
							return nil, nil
						},
					},
				},
			},
			Types: []GraphQLType{userType},
		},
		Entities: []EntityConfig{
			{
				TypeName:  "User",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					userId, _ := repr["id"].(string)
					orders := ordersData[userId]
					result := make([]interface{}, len(orders))
					for i, o := range orders {
						result[i] = copyMap(o)
					}
					return map[string]interface{}{
						"id":     userId,
						"orders": result,
					}, nil
				},
			},
			{
				TypeName:  "Order",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					orderId, _ := repr["id"].(string)
					for _, orders := range ordersData {
						for _, o := range orders {
							if o["id"] == orderId {
								return copyMap(o), nil
							}
						}
					}
					return nil, fmt.Errorf("order %q not found", orderId)
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return sg
}

func makeProductSubgraph() *Subgraph {
	productType := &ObjectType{
		Name_: "Product",
		Fields_: FieldMap{
			"id":         {Type: NewNonNull(IDScalar)},
			"name":       {Type: NewNonNull(StringScalar)},
			"price":      {Type: NewNonNull(FloatScalar)},
			"categoryId": {Type: NewNonNull(IDScalar)},
		},
	}
	orderType := &ObjectType{
		Name_: "Order",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"product": {
				Type: productType,
				Resolve: func(p ResolveParams) (interface{}, error) {
					source, _ := p.Source.(map[string]interface{})
					productId, _ := source["productId"].(string)
					if productId == "" {
						orderId, _ := source["id"].(string)
						productId = orderProductMap[orderId]
					}
					if prod, ok := productsData[productId]; ok {
						return copyMap(prod), nil
					}
					return nil, nil
				},
			},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "products",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"product": {
						Type: productType,
						Args: ArgumentMap{"id": {Name_: "id", Type: NewNonNull(IDScalar)}},
						Resolve: func(p ResolveParams) (interface{}, error) {
							id, _ := p.Args["id"].(string)
							if prod, ok := productsData[id]; ok {
								return copyMap(prod), nil
							}
							return nil, nil
						},
					},
				},
			},
			Types: []GraphQLType{orderType},
		},
		Entities: []EntityConfig{
			{
				TypeName:  "Order",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					orderId, _ := repr["id"].(string)
					productId := orderProductMap[orderId]
					prod := copyMap(productsData[productId])
					return map[string]interface{}{
						"id":      orderId,
						"product": prod,
					}, nil
				},
			},
			{
				TypeName:  "Product",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					productId, _ := repr["id"].(string)
					if prod, ok := productsData[productId]; ok {
						return copyMap(prod), nil
					}
					return nil, fmt.Errorf("product %q not found", productId)
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return sg
}

func makeCategorySubgraph() *Subgraph {
	categoryType := &ObjectType{
		Name_: "Category",
		Fields_: FieldMap{
			"id":    {Type: NewNonNull(IDScalar)},
			"label": {Type: NewNonNull(StringScalar)},
			"tier":  {Type: NewNonNull(StringScalar)},
		},
	}
	productType := &ObjectType{
		Name_: "Product",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
			"category": {
				Type: categoryType,
				Resolve: func(p ResolveParams) (interface{}, error) {
					source, _ := p.Source.(map[string]interface{})
					catId, _ := source["categoryId"].(string)
					if catId == "" {
						prodId, _ := source["id"].(string)
						catId = productCategoryMap[prodId]
					}
					if cat, ok := categoriesData[catId]; ok {
						return copyMap(cat), nil
					}
					return nil, nil
				},
			},
		},
	}
	sg, err := NewSubgraph(SubgraphConfig{
		Name: "categories",
		Schema: SchemaConfig{
			Query: &ObjectType{
				Name_: "Query",
				Fields_: FieldMap{
					"category": {
						Type: categoryType,
						Args: ArgumentMap{"id": {Name_: "id", Type: NewNonNull(IDScalar)}},
						Resolve: func(p ResolveParams) (interface{}, error) {
							id, _ := p.Args["id"].(string)
							if cat, ok := categoriesData[id]; ok {
								return copyMap(cat), nil
							}
							return nil, nil
						},
					},
				},
			},
			Types: []GraphQLType{productType},
		},
		Entities: []EntityConfig{
			{
				TypeName:  "Product",
				KeyFields: []string{"id"},
				Resolver: func(repr map[string]interface{}) (interface{}, error) {
					productId, _ := repr["id"].(string)
					catId := productCategoryMap[productId]
					cat := copyMap(categoriesData[catId])
					return map[string]interface{}{
						"id":       productId,
						"category": cat,
					}, nil
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return sg
}

// ============================================================
// Tests
// ============================================================

func TestNewSubgraph(t *testing.T) {
	t.Run("valid subgraph", func(t *testing.T) {
		sg := makeUserSubgraph()
		if sg.Name != "users" {
			t.Errorf("expected name %q, got %q", "users", sg.Name)
		}
		if sg.Schema == nil {
			t.Fatal("schema should not be nil")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := NewSubgraph(SubgraphConfig{
			Schema: SchemaConfig{
				Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"x": {Type: StringScalar}}},
			},
		})
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("entity without key fields", func(t *testing.T) {
		_, err := NewSubgraph(SubgraphConfig{
			Name: "test",
			Schema: SchemaConfig{
				Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"x": {Type: StringScalar}}},
			},
			Entities: []EntityConfig{{TypeName: "Foo", KeyFields: []string{}}},
		})
		if err == nil {
			t.Fatal("expected error for entity without key fields")
		}
	})

	t.Run("entity without resolver", func(t *testing.T) {
		_, err := NewSubgraph(SubgraphConfig{
			Name: "test",
			Schema: SchemaConfig{
				Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"x": {Type: StringScalar}}},
			},
			Entities: []EntityConfig{{TypeName: "Foo", KeyFields: []string{"id"}}},
		})
		if err == nil {
			t.Fatal("expected error for entity without resolver")
		}
	})

	t.Run("entity without type name", func(t *testing.T) {
		_, err := NewSubgraph(SubgraphConfig{
			Name: "test",
			Schema: SchemaConfig{
				Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"x": {Type: StringScalar}}},
			},
			Entities: []EntityConfig{{
				KeyFields: []string{"id"},
				Resolver:  func(r map[string]interface{}) (interface{}, error) { return nil, nil },
			}},
		})
		if err == nil {
			t.Fatal("expected error for entity without type name")
		}
	})
}

func TestGatewayCreation(t *testing.T) {
	t.Run("no subgraphs", func(t *testing.T) {
		_, err := NewGateway(GatewayConfig{})
		if err == nil {
			t.Fatal("expected error for empty subgraphs")
		}
	})

	t.Run("single subgraph", func(t *testing.T) {
		gw, err := NewGateway(GatewayConfig{
			Subgraphs: []*Subgraph{makeUserSubgraph()},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gw.Schema() == nil {
			t.Fatal("supergraph schema should not be nil")
		}
	})

	t.Run("two subgraphs", func(t *testing.T) {
		gw, err := NewGateway(GatewayConfig{
			Subgraphs: []*Subgraph{makeUserSubgraph(), makeOrderSubgraph()},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Verify merged User type has fields from both subgraphs
		userType := gw.Schema().Type("User")
		if userType == nil {
			t.Fatal("User type should exist in supergraph")
		}
		obj := userType.(*ObjectType)
		for _, f := range []string{"id", "name", "email", "orders"} {
			if _, ok := obj.Fields_[f]; !ok {
				t.Errorf("User type should have field %q", f)
			}
		}
	})
}

func TestFederationDirectives(t *testing.T) {
	directives := FederationDirectives()
	if len(directives) != 4 {
		t.Fatalf("expected 4 federation directives, got %d", len(directives))
	}
	names := map[string]bool{}
	for _, d := range directives {
		names[d.Name_] = true
	}
	for _, name := range []string{"key", "external", "requires", "provides"} {
		if !names[name] {
			t.Errorf("missing directive @%s", name)
		}
	}
}

func TestFederationSingleSubgraph(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := gw.Execute(`{ user(id: "1") { name email } }`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	user := getPath(result.Data, "user").(map[string]interface{})
	assertEqual(t, user["name"], "Alice")
	assertEqual(t, user["email"], "alice@example.com")
}

func TestFederationTwoLevel(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph(), makeOrderSubgraph()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("user with orders", func(t *testing.T) {
		result := gw.Execute(`{
			user(id: "1") {
				name
				orders {
					id
					quantity
				}
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Alice")
		orders := user["orders"].([]interface{})
		if len(orders) != 2 {
			t.Fatalf("expected 2 orders, got %d", len(orders))
		}
	})

	t.Run("user with no orders", func(t *testing.T) {
		result := gw.Execute(`{
			user(id: "3") {
				name
				orders {
					id
				}
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Charlie")
		orders := user["orders"].([]interface{})
		if len(orders) != 0 {
			t.Fatalf("expected 0 orders, got %d", len(orders))
		}
	})

	t.Run("direct order query", func(t *testing.T) {
		result := gw.Execute(`{
			order(id: "o1") {
				id
				quantity
				userId
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		order := getPath(result.Data, "order").(map[string]interface{})
		assertEqual(t, order["id"], "o1")
		assertEqual(t, order["quantity"], 2)
	})
}

func TestFederationThreeLevel(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("user -> orders -> product (3-level)", func(t *testing.T) {
		result := gw.Execute(`{
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
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Alice")

		orders := user["orders"].([]interface{})
		if len(orders) != 2 {
			t.Fatalf("expected 2 orders, got %d", len(orders))
		}

		order0 := orders[0].(map[string]interface{})
		product0 := order0["product"].(map[string]interface{})
		assertEqual(t, product0["name"], "Widget")
		assertEqual(t, product0["price"], 9.99)

		order1 := orders[1].(map[string]interface{})
		product1 := order1["product"].(map[string]interface{})
		assertEqual(t, product1["name"], "Gadget")
		assertEqual(t, product1["price"], 24.99)
	})

	t.Run("mixed fields from multiple subgraphs", func(t *testing.T) {
		result := gw.Execute(`{
			user(id: "1") {
				name
				email
				orders {
					id
					quantity
					userId
					product {
						id
						name
						price
					}
				}
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Alice")
		assertEqual(t, user["email"], "alice@example.com")

		orders := user["orders"].([]interface{})
		order0 := orders[0].(map[string]interface{})
		assertEqual(t, order0["id"], "o1")
		assertEqual(t, order0["quantity"], 2)
		assertEqual(t, order0["userId"], "1")

		product0 := order0["product"].(map[string]interface{})
		assertEqual(t, product0["id"], "p1")
		assertEqual(t, product0["name"], "Widget")
	})

	t.Run("direct product query", func(t *testing.T) {
		result := gw.Execute(`{
			product(id: "p1") {
				name
				price
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		product := getPath(result.Data, "product").(map[string]interface{})
		assertEqual(t, product["name"], "Widget")
		assertEqual(t, product["price"], 9.99)
	})
}

func TestFederationFourLevel(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
			makeCategorySubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("user -> orders -> product -> category (4-level)", func(t *testing.T) {
		result := gw.Execute(`{
			user(id: "1") {
				name
				orders {
					quantity
					product {
						name
						price
						category {
							label
							tier
						}
					}
				}
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Alice")

		orders := user["orders"].([]interface{})
		order0 := orders[0].(map[string]interface{})
		product0 := order0["product"].(map[string]interface{})
		assertEqual(t, product0["name"], "Widget")

		category0 := product0["category"].(map[string]interface{})
		assertEqual(t, category0["label"], "Hardware")
		assertEqual(t, category0["tier"], "premium")

		order1 := orders[1].(map[string]interface{})
		product1 := order1["product"].(map[string]interface{})
		category1 := product1["category"].(map[string]interface{})
		assertEqual(t, category1["label"], "Electronics")
		assertEqual(t, category1["tier"], "standard")
	})

	t.Run("all four levels with all fields", func(t *testing.T) {
		result := gw.Execute(`{
			user(id: "2") {
				name
				email
				orders {
					id
					quantity
					product {
						id
						name
						price
						category {
							id
							label
							tier
						}
					}
				}
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("errors: %v", formatErrors(result.Errors))
		}

		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Bob")
		assertEqual(t, user["email"], "bob@example.com")

		orders := user["orders"].([]interface{})
		if len(orders) != 1 {
			t.Fatalf("expected 1 order for Bob, got %d", len(orders))
		}
		order := orders[0].(map[string]interface{})
		assertEqual(t, order["id"], "o3")
		assertEqual(t, order["quantity"], 5)

		product := order["product"].(map[string]interface{})
		assertEqual(t, product["id"], "p1")
		assertEqual(t, product["name"], "Widget")
		assertEqual(t, product["price"], 9.99)

		category := product["category"].(map[string]interface{})
		assertEqual(t, category["id"], "c1")
		assertEqual(t, category["label"], "Hardware")
		assertEqual(t, category["tier"], "premium")
	})
}

func TestFederationMultipleRootFields(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := gw.Execute(`{
		user(id: "1") {
			name
		}
		order(id: "o1") {
			id
			quantity
		}
		product(id: "p1") {
			name
			price
		}
	}`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	data := result.Data.(map[string]interface{})
	user := data["user"].(map[string]interface{})
	assertEqual(t, user["name"], "Alice")

	order := data["order"].(map[string]interface{})
	assertEqual(t, order["id"], "o1")
	assertEqual(t, order["quantity"], 2)

	product := data["product"].(map[string]interface{})
	assertEqual(t, product["name"], "Widget")
	assertEqual(t, product["price"], 9.99)
}

func TestFederationEntityCaching(t *testing.T) {
	// Verify that entity resolution is cached (resolver called once per entity per subgraph)
	resolveCount := 0
	orderSg := makeOrderSubgraph()
	originalResolver := orderSg.Entities["User"].Resolver
	orderSg.Entities["User"].Resolver = func(repr map[string]interface{}) (interface{}, error) {
		resolveCount++
		return originalResolver(repr)
	}

	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph(), orderSg},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Access multiple fields from the Order subgraph's User entity
	result := gw.Execute(`{
		user(id: "1") {
			name
			orders {
				id
				quantity
			}
		}
	}`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	// The User entity resolver should only be called once (cached for sibling fields)
	if resolveCount != 1 {
		t.Errorf("expected entity resolver to be called once, got %d", resolveCount)
	}
}

func TestFederationErrorHandling(t *testing.T) {
	t.Run("entity resolver error", func(t *testing.T) {
		userType := &ObjectType{
			Name_: "User",
			Fields_: FieldMap{
				"id":     {Type: NewNonNull(IDScalar)},
				"secret": {Type: StringScalar},
			},
		}
		failSg, _ := NewSubgraph(SubgraphConfig{
			Name: "failing",
			Schema: SchemaConfig{
				Query: &ObjectType{
					Name_: "Query",
					Fields_: FieldMap{
						"_placeholder": {Type: StringScalar},
					},
				},
				Types: []GraphQLType{userType},
			},
			Entities: []EntityConfig{
				{
					TypeName:  "User",
					KeyFields: []string{"id"},
					Resolver: func(repr map[string]interface{}) (interface{}, error) {
						return nil, fmt.Errorf("access denied: classified data")
					},
				},
			},
		})

		gw, err := NewGateway(GatewayConfig{
			Subgraphs: []*Subgraph{makeUserSubgraph(), failSg},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := gw.Execute(`{
			user(id: "1") {
				name
				secret
			}
		}`, nil, "")
		// Should have an error for the secret field
		if len(result.Errors) == 0 {
			t.Fatal("expected errors for failed entity resolution")
		}
	})

	t.Run("null entity resolution", func(t *testing.T) {
		userType := &ObjectType{
			Name_: "User",
			Fields_: FieldMap{
				"id":     {Type: NewNonNull(IDScalar)},
				"avatar": {Type: StringScalar},
			},
		}
		nilSg, _ := NewSubgraph(SubgraphConfig{
			Name: "nilservice",
			Schema: SchemaConfig{
				Query: &ObjectType{
					Name_: "Query",
					Fields_: FieldMap{
						"_placeholder": {Type: StringScalar},
					},
				},
				Types: []GraphQLType{userType},
			},
			Entities: []EntityConfig{
				{
					TypeName:  "User",
					KeyFields: []string{"id"},
					Resolver: func(repr map[string]interface{}) (interface{}, error) {
						return nil, nil // user not found
					},
				},
			},
		})

		gw, err := NewGateway(GatewayConfig{
			Subgraphs: []*Subgraph{makeUserSubgraph(), nilSg},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := gw.Execute(`{
			user(id: "1") {
				name
				avatar
			}
		}`, nil, "")
		if len(result.Errors) > 0 {
			t.Fatalf("unexpected errors: %v", formatErrors(result.Errors))
		}
		user := getPath(result.Data, "user").(map[string]interface{})
		assertEqual(t, user["name"], "Alice")
		if user["avatar"] != nil {
			t.Errorf("expected nil avatar, got %v", user["avatar"])
		}
	})
}

func TestFederationTypename(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := gw.Execute(`{
		user(id: "1") {
			__typename
			name
			orders {
				__typename
				id
			}
		}
	}`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	user := getPath(result.Data, "user").(map[string]interface{})
	assertEqual(t, user["__typename"], "User")

	orders := user["orders"].([]interface{})
	order0 := orders[0].(map[string]interface{})
	assertEqual(t, order0["__typename"], "Order")
}

func TestFederationParallelEntities(t *testing.T) {
	// Test multiple entities at the same level (User "1" has 2 orders)
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := gw.Execute(`{
		user(id: "1") {
			orders {
				product {
					name
				}
			}
		}
	}`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	user := getPath(result.Data, "user").(map[string]interface{})
	orders := user["orders"].([]interface{})
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}

	productNames := make([]string, len(orders))
	for i, o := range orders {
		order := o.(map[string]interface{})
		product := order["product"].(map[string]interface{})
		productNames[i] = product["name"].(string)
	}

	// Verify both products resolved correctly
	if productNames[0] != "Widget" && productNames[0] != "Gadget" {
		t.Errorf("unexpected product name: %s", productNames[0])
	}
	if productNames[1] != "Widget" && productNames[1] != "Gadget" {
		t.Errorf("unexpected product name: %s", productNames[1])
	}
	if productNames[0] == productNames[1] {
		t.Errorf("expected different products, got same: %s", productNames[0])
	}
}

func TestFederationNullParent(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{makeUserSubgraph(), makeOrderSubgraph()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Query a user that doesn't exist
	result := gw.Execute(`{
		user(id: "999") {
			name
			orders {
				id
			}
		}
	}`, nil, "")
	if len(result.Errors) > 0 {
		t.Fatalf("errors: %v", formatErrors(result.Errors))
	}

	data := result.Data.(map[string]interface{})
	if data["user"] != nil {
		t.Errorf("expected nil user, got %v", data["user"])
	}
}

func TestFederationGatewaySchema(t *testing.T) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
			makeCategorySubgraph(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := gw.Schema()

	// Verify all types exist
	for _, typeName := range []string{"User", "Order", "Product", "Category"} {
		if schema.Type(typeName) == nil {
			t.Errorf("expected type %q in supergraph", typeName)
		}
	}

	// Verify User has merged fields from users + orders subgraphs
	userType := schema.Type("User").(*ObjectType)
	expectedFields := []string{"id", "name", "email", "orders"}
	for _, f := range expectedFields {
		if _, ok := userType.Fields_[f]; !ok {
			t.Errorf("User should have field %q", f)
		}
	}

	// Verify Order has merged fields from orders + products subgraphs
	orderType := schema.Type("Order").(*ObjectType)
	expectedFields = []string{"id", "userId", "productId", "quantity", "product"}
	for _, f := range expectedFields {
		if _, ok := orderType.Fields_[f]; !ok {
			t.Errorf("Order should have field %q", f)
		}
	}

	// Verify Product has merged fields from products + categories subgraphs
	productType := schema.Type("Product").(*ObjectType)
	expectedFields = []string{"id", "name", "price", "category"}
	for _, f := range expectedFields {
		if _, ok := productType.Fields_[f]; !ok {
			t.Errorf("Product should have field %q", f)
		}
	}
}

// ============================================================
// Benchmark: 4-level federation query
// ============================================================

func BenchmarkFederationFourLevel(b *testing.B) {
	gw, err := NewGateway(GatewayConfig{
		Subgraphs: []*Subgraph{
			makeUserSubgraph(),
			makeOrderSubgraph(),
			makeProductSubgraph(),
			makeCategorySubgraph(),
		},
	})
	if err != nil {
		b.Fatalf("gateway setup failed: %v", err)
	}

	query := `{
		user(id: "1") {
			name
			email
			orders {
				id
				quantity
				product {
					name
					price
					category {
						label
						tier
					}
				}
			}
		}
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := gw.Execute(query, nil, "")
		if len(result.Errors) > 0 {
			b.Fatalf("errors: %v", formatErrors(result.Errors))
		}
	}
}

// ============================================================
// Helpers
// ============================================================

func getPath(data interface{}, keys ...string) interface{} {
	current := data
	for _, key := range keys {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func assertEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("got %v (%T), want %v (%T)", got, got, want, want)
	}
}

func formatErrors(errs []*GraphQLError) string {
	s := ""
	for _, e := range errs {
		s += "\n- " + e.Message
	}
	return s
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	c := make(map[string]interface{}, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
