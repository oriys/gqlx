package gqlx

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

// ErrSubscriptionCanceled is returned by an AsyncIterator when the subscription
// has been canceled.
var ErrSubscriptionCanceled = errors.New("subscription canceled")

// AsyncIterator is the interface implemented by sources that produce a stream
// of subscription events. A subscription root field resolver may return either
// an AsyncIterator or a channel; channels are adapted automatically.
type AsyncIterator interface {
	// Next returns the next event. It returns io.EOF when the stream ends.
	Next() (interface{}, error)
	// Close releases any resources held by the iterator.
	Close() error
}

// SubscribeParams holds the parameters for executing a subscription operation.
type SubscribeParams struct {
	Schema        *Schema
	Document      *Document
	RootValue     interface{}
	Variables     map[string]interface{}
	OperationName string
	MaxDepth      int
}

// Subscription represents an active server-side subscription. Events are
// delivered on Results, which is closed when the source ends or Cancel is
// called.
type Subscription struct {
	Results <-chan *Result

	done   chan struct{}
	cancel func()
	once   sync.Once
}

// Cancel stops the subscription. It is safe to call multiple times.
func (s *Subscription) Cancel() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.done)
		if s.cancel != nil {
			s.cancel()
		}
	})
}

// Subscribe executes a subscription operation against the executor's schema.
// It returns a Subscription whose Results channel yields one Result per event.
//
// The subscription root field resolver must return either an AsyncIterator or a
// (possibly receive-only) channel of events. Each event is then resolved
// against the subscription selection set and delivered as a Result.
func (e *Executor) Subscribe(params SubscribeParams) (*Subscription, error) {
	schema := params.Schema
	if schema == nil {
		schema = e.schema
	}
	if schema == nil {
		return nil, errors.New("Must provide schema")
	}
	if params.Document == nil {
		return nil, errors.New("Must provide document")
	}

	operation, fragments, opErr := findOperation(params.Document, params.OperationName)
	if opErr != nil {
		return nil, opErr
	}
	if operation.Operation != OperationSubscription {
		return nil, fmt.Errorf("Subscribe requires a subscription operation, got %s", operation.Operation)
	}

	coercedVars, varErrors := coerceOperationVariables(schema, operation, params.Variables)
	if len(varErrors) > 0 {
		return nil, varErrors[0]
	}

	rootType := schema.SubscriptionType
	if rootType == nil {
		return nil, errors.New("Schema is not configured for subscriptions.")
	}

	ctx := newExecutionContext(schema, operation, fragments, coercedVars, params.MaxDepth)

	// A subscription must select exactly one root field.
	groupedFields := ctx.collectFields(rootType, operation.SelectionSet, nil)
	if len(groupedFields) == 0 {
		return nil, errors.New("Subscription must select a field.")
	}
	if len(groupedFields) > 1 {
		return nil, errors.New("Subscription must select only one root field.")
	}
	entry := groupedFields[0]
	fieldNode := entry.fields[0]

	fieldDef, ok := rootType.Fields_[fieldNode.Name]
	if !ok {
		return nil, fmt.Errorf("Cannot query field %q on type %q.", fieldNode.Name, rootType.Name_)
	}

	args, err := CoerceArgumentValues(fieldDef.Args, fieldNode.Arguments, ctx.variables)
	if err != nil {
		return nil, err
	}

	var stream interface{}
	if fieldDef.Resolve != nil {
		stream, err = fieldDef.Resolve(ResolveParams{
			Source: params.RootValue,
			Args:   args,
			Info: ResolveInfo{
				FieldName:  fieldNode.Name,
				ReturnType: fieldDef.Type,
				ParentType: rootType,
				Schema:     ctx.schema,
				Operation:  operation,
				Fragments:  fragments,
				Variables:  coercedVars,
			},
		})
		if err != nil {
			return nil, err
		}
	} else {
		stream = resolveFieldValue(params.RootValue, fieldNode.Name)
	}

	results := make(chan *Result)
	done := make(chan struct{})
	iter, err := newAsyncIterator(stream, done)
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		Results: results,
		done:    done,
		cancel:  func() { _ = iter.Close() },
	}

	go func() {
		defer close(results)
		defer iter.Close()

		for {
			event, err := iter.Next()
			if err != nil {
				if err == io.EOF || err == ErrSubscriptionCanceled {
					return
				}
				if !deliver(sub, results, &Result{Errors: []*GraphQLError{FormatError(err)}}) {
					return
				}
				return
			}

			eventCtx := newExecutionContext(schema, operation, fragments, coercedVars, params.MaxDepth)
			fieldPath := []interface{}{entry.key}
			value, cerr := eventCtx.completeValue(fieldDef.Type, entry.fields, event, fieldPath)
			data := interface{}(map[string]interface{}{entry.key: value})
			if cerr != nil {
				data = nil
			}
			if !deliver(sub, results, &Result{Data: data, Errors: eventCtx.Errors()}) {
				return
			}
		}
	}()

	return sub, nil
}

// deliver sends a result unless the subscription was canceled.
func deliver(sub *Subscription, results chan<- *Result, r *Result) bool {
	select {
	case results <- r:
		return true
	case <-sub.done:
		return false
	}
}

// Subscribe executes a subscription against the provided schema.
func Subscribe(params SubscribeParams) (*Subscription, error) {
	if params.Schema == nil {
		return nil, errors.New("Must provide schema")
	}
	return NewExecutor(params.Schema).Subscribe(params)
}

// DoSubscribe parses, validates, and subscribes to a GraphQL subscription in
// one call. The returned subscription must be consumed and eventually canceled.
func DoSubscribe(schema *Schema, query string, variables map[string]interface{}, operationName string) (*Subscription, error) {
	if schema == nil {
		return nil, errors.New("Must provide schema")
	}
	doc, err := Parse(query)
	if err != nil {
		return nil, FormatError(err)
	}
	if validationErrors := Validate(schema, doc); len(validationErrors) > 0 {
		return nil, validationErrors[0]
	}
	return Subscribe(SubscribeParams{
		Schema:        schema,
		Document:      doc,
		Variables:     variables,
		OperationName: operationName,
	})
}

// newAsyncIterator adapts an AsyncIterator or channel into an AsyncIterator.
func newAsyncIterator(stream interface{}, done <-chan struct{}) (AsyncIterator, error) {
	if stream == nil {
		return nil, errors.New("subscription resolver returned nil")
	}
	if it, ok := stream.(AsyncIterator); ok {
		return &doneAwareIterator{iter: it, done: done}, nil
	}

	rv := reflect.ValueOf(stream)
	if rv.Kind() == reflect.Chan {
		if rv.Type().ChanDir() == reflect.SendDir {
			return nil, errors.New("subscription resolver returned a send-only channel")
		}
		return &channelIterator{ch: rv, done: done}, nil
	}

	return nil, fmt.Errorf("subscription resolver must return a channel or AsyncIterator, got %T", stream)
}

// channelIterator adapts a Go channel to the AsyncIterator interface. Close
// signals cancellation but does not close the underlying channel, since the
// producer owns it.
type channelIterator struct {
	ch     reflect.Value
	done   <-chan struct{}
	mu     sync.Mutex
	closed bool
}

func (it *channelIterator) Next() (interface{}, error) {
	it.mu.Lock()
	if it.closed {
		it.mu.Unlock()
		return nil, io.EOF
	}
	it.mu.Unlock()

	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(it.done)},
		{Dir: reflect.SelectRecv, Chan: it.ch},
	}
	chosen, val, ok := reflect.Select(cases)
	if chosen == 0 {
		return nil, ErrSubscriptionCanceled
	}
	if !ok {
		it.mu.Lock()
		it.closed = true
		it.mu.Unlock()
		return nil, io.EOF
	}
	return val.Interface(), nil
}

func (it *channelIterator) Close() error {
	it.mu.Lock()
	it.closed = true
	it.mu.Unlock()
	return nil
}

// doneAwareIterator wraps a user supplied AsyncIterator and stops calling it
// once the subscription is canceled.
type doneAwareIterator struct {
	iter AsyncIterator
	done <-chan struct{}
}

func (it *doneAwareIterator) Next() (interface{}, error) {
	select {
	case <-it.done:
		return nil, ErrSubscriptionCanceled
	default:
	}
	return it.iter.Next()
}

func (it *doneAwareIterator) Close() error {
	return it.iter.Close()
}
