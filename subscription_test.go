package gqlx

import (
	"io"
	"testing"
	"time"
)

func subscriptionSchema() *Schema {
	messageType := &ObjectType{
		Name_: "Message",
		Fields_: FieldMap{
			"id":   {Type: NewNonNull(IntScalar)},
			"text": {Type: NewNonNull(StringScalar)},
		},
	}

	query := &ObjectType{
		Name_:   "Query",
		Fields_: FieldMap{"noop": {Type: StringScalar}},
	}

	subscription := &ObjectType{
		Name_: "Subscription",
		Fields_: FieldMap{
			"tick": {
				Type: IntScalar,
				Resolve: func(p ResolveParams) (interface{}, error) {
					ch := make(chan interface{}, 3)
					ch <- 1
					ch <- 2
					ch <- 3
					close(ch)
					return ch, nil
				},
			},
			"messages": {
				Type: messageType,
				Resolve: func(p ResolveParams) (interface{}, error) {
					ch := make(chan interface{}, 2)
					ch <- map[string]interface{}{"id": 1, "text": "hello"}
					ch <- map[string]interface{}{"id": 2, "text": "world"}
					close(ch)
					return ch, nil
				},
			},
			"badSource": {
				Type: IntScalar,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return 42, nil // not a channel or iterator
				},
			},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query:        query,
		Subscription: subscription,
		Types:        []GraphQLType{messageType},
	})
	return schema
}

func collectSubscription(t *testing.T, sub *Subscription) []*Result {
	t.Helper()
	var results []*Result
	timeout := time.After(5 * time.Second)
	for {
		select {
		case r, ok := <-sub.Results:
			if !ok {
				return results
			}
			results = append(results, r)
		case <-timeout:
			t.Fatal("timed out waiting for subscription results")
		}
	}
}

func TestSubscribeScalarStream(t *testing.T) {
	sub, err := DoSubscribe(subscriptionSchema(), `subscription { tick }`, nil, "")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	defer sub.Cancel()

	results := collectSubscription(t, sub)
	if len(results) != 3 {
		t.Fatalf("expected 3 events, got %d", len(results))
	}
	for i, r := range results {
		if r.HasErrors() {
			t.Fatalf("event %d errors: %v", i, FormatErrors(r.Errors))
		}
		data := r.Data.(map[string]interface{})
		if data["tick"] != i+1 {
			t.Errorf("event %d tick = %v", i, data["tick"])
		}
	}
}

func TestSubscribeObjectStream(t *testing.T) {
	sub, err := DoSubscribe(subscriptionSchema(), `subscription { messages { id text } }`, nil, "")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	defer sub.Cancel()

	results := collectSubscription(t, sub)
	if len(results) != 2 {
		t.Fatalf("expected 2 events, got %d", len(results))
	}
	first := results[0].Data.(map[string]interface{})["messages"].(map[string]interface{})
	if first["id"] != 1 || first["text"] != "hello" {
		t.Fatalf("unexpected first event: %#v", first)
	}
}

func TestSubscribeWithNamedOperationAndVariables(t *testing.T) {
	schema := subscriptionSchema()
	// Add a field with an argument to exercise variable coercion.
	schema.SubscriptionType.Fields_["limited"] = &FieldDefinition{
		Type: IntScalar,
		Args: ArgumentMap{"count": {Name_: "count", Type: NewNonNull(IntScalar)}},
		Resolve: func(p ResolveParams) (interface{}, error) {
			count, _ := p.Args["count"].(int)
			ch := make(chan interface{}, count)
			for i := 0; i < count; i++ {
				ch <- i
			}
			close(ch)
			return ch, nil
		},
	}

	sub, err := DoSubscribe(schema, `subscription Watch($n: Int!) { limited(count: $n) }`,
		map[string]interface{}{"n": 2}, "Watch")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	defer sub.Cancel()

	results := collectSubscription(t, sub)
	if len(results) != 2 {
		t.Fatalf("expected 2 events, got %d", len(results))
	}
}

func TestSubscribeRejectsNonSubscriptionOperation(t *testing.T) {
	_, err := DoSubscribe(subscriptionSchema(), `{ noop }`, nil, "")
	if err == nil {
		t.Fatal("expected an error for a query operation")
	}
}

func TestSubscribeRejectsInvalidSource(t *testing.T) {
	_, err := DoSubscribe(subscriptionSchema(), `subscription { badSource }`, nil, "")
	if err == nil {
		t.Fatal("expected an error for a non-stream source")
	}
}

func TestSubscribeCancel(t *testing.T) {
	schema := subscriptionSchema()
	schema.SubscriptionType.Fields_["infinite"] = &FieldDefinition{
		Type: IntScalar,
		Resolve: func(p ResolveParams) (interface{}, error) {
			ch := make(chan interface{})
			go func() {
				defer close(ch)
				i := 0
				for {
					select {
					case ch <- i:
						i++
					case <-time.After(10 * time.Millisecond):
						return
					}
				}
			}()
			return ch, nil
		},
	}

	sub, err := DoSubscribe(schema, `subscription { infinite }`, nil, "")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	// Consume one event, then cancel.
	select {
	case <-sub.Results:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first event")
	}
	sub.Cancel()

	// The results channel should eventually close.
	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-sub.Results:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("results channel was not closed after Cancel")
		}
	}
}

// staticIterator is a custom AsyncIterator used to verify the interface path.
type staticIterator struct {
	values []interface{}
	idx    int
	closed bool
}

func (it *staticIterator) Next() (interface{}, error) {
	if it.idx >= len(it.values) {
		return nil, io.EOF
	}
	v := it.values[it.idx]
	it.idx++
	return v, nil
}

func (it *staticIterator) Close() error {
	it.closed = true
	return nil
}

func TestSubscribeCustomAsyncIterator(t *testing.T) {
	it := &staticIterator{values: []interface{}{10, 20}}
	schema := subscriptionSchema()
	schema.SubscriptionType.Fields_["custom"] = &FieldDefinition{
		Type: IntScalar,
		Resolve: func(p ResolveParams) (interface{}, error) {
			return it, nil
		},
	}

	sub, err := DoSubscribe(schema, `subscription { custom }`, nil, "")
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	results := collectSubscription(t, sub)
	if len(results) != 2 {
		t.Fatalf("expected 2 events, got %d", len(results))
	}
	if results[1].Data.(map[string]interface{})["custom"] != 20 {
		t.Fatalf("unexpected event: %#v", results[1].Data)
	}
	if !it.closed {
		t.Fatal("iterator should be closed after stream end")
	}
}

func TestSubscribeMultipleRootFieldsRejected(t *testing.T) {
	_, err := DoSubscribe(subscriptionSchema(), `subscription { tick messages { id } }`, nil, "")
	if err == nil {
		t.Fatal("expected an error for multiple root fields")
	}
}
