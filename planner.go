package gqlx

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ============================================================
// Query plan IR
// ============================================================

// QueryPlan is an executable federated query plan produced by Gateway.Plan.
type QueryPlan struct {
	Operation OperationType
	Root      PlanNode
}

// PlanNode is a node in a query plan.
type PlanNode interface{ planNode() }

// ParallelNode executes its steps independently and merges the results.
type ParallelNode struct{ Steps []PlanNode }

func (*ParallelNode) planNode() {}

// SequenceNode executes its steps in order, merging the results.
type SequenceNode struct{ Steps []PlanNode }

func (*SequenceNode) planNode() {}

// FetchNode fetches a selection from a single subgraph. For a root fetch the
// selection is resolved against the subgraph's Query/Mutation type; for an
// entity fetch it is resolved against an entity returned by a reference
// resolver.
type FetchNode struct {
	Subgraph  *Subgraph
	RootType  *ObjectType // non-nil for root fetches
	Selection []Selection
	Entity    *EntityFetch   // non-nil for entity fetches
	Flatten   []*FlattenNode // dependent entity fetches
}

func (*FetchNode) planNode() {}

// EntityFetch describes the representation required to fetch an entity.
type EntityFetch struct {
	TypeName   string
	KeyFields  []string
	Definition *EntityDefinition
}

// FlattenNode applies a dependent fetch to the entities found at Path in the
// result of the enclosing fetch.
type FlattenNode struct {
	Path []string
	Node *FetchNode
}

// Describe renders a human readable plan, useful for debugging and tests.
func (q *QueryPlan) Describe() string {
	if q == nil || q.Root == nil {
		return ""
	}
	var b strings.Builder
	describeNode(&b, q.Root, 0)
	return b.String()
}

func describeNode(b *strings.Builder, node PlanNode, depth int) {
	indent := strings.Repeat("  ", depth)
	switch n := node.(type) {
	case *ParallelNode:
		b.WriteString(indent + "Parallel\n")
		for _, s := range n.Steps {
			describeNode(b, s, depth+1)
		}
	case *SequenceNode:
		b.WriteString(indent + "Sequence\n")
		for _, s := range n.Steps {
			describeNode(b, s, depth+1)
		}
	case *FetchNode:
		kind := "Fetch"
		if n.Entity != nil {
			kind = fmt.Sprintf("Fetch[entity %s]", n.Entity.TypeName)
		}
		b.WriteString(fmt.Sprintf("%s%s(service=%s, selection=%s)\n", indent, kind, n.Subgraph.Name, selectionString(n.Selection)))
		for _, fl := range n.Flatten {
			b.WriteString(fmt.Sprintf("%s  Flatten(path=%v)\n", indent, fl.Path))
			describeNode(b, fl.Node, depth+2)
		}
	}
}

func selectionString(selection []Selection) string {
	parts := make([]string, 0, len(selection))
	for _, sel := range selection {
		if f, ok := sel.(*Field); ok {
			parts = append(parts, responseKey(f))
		}
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

// ============================================================
// Plan builder
// ============================================================

// Plan parses, validates and plans a federated operation.
func (g *Gateway) Plan(query string) (*QueryPlan, error) {
	doc, err := Parse(query)
	if err != nil {
		return nil, FormatError(err)
	}
	if validationErrors := Validate(g.supergraph, doc); len(validationErrors) > 0 {
		return nil, validationErrors[0]
	}
	return g.PlanDocument(doc, "")
}

// PlanDocument builds a query plan for an already parsed document.
func (g *Gateway) PlanDocument(doc *Document, operationName string) (*QueryPlan, error) {
	if g.supergraph == nil {
		return nil, errors.New("gateway has no supergraph")
	}
	operation, fragments, opErr := findOperation(doc, operationName)
	if opErr != nil {
		return nil, opErr
	}
	if operation.Operation == OperationSubscription {
		return nil, errors.New("federation planner does not support subscriptions")
	}

	p := &planner{gw: g, fragments: fragments}

	rootType := g.supergraph.QueryType
	if operation.Operation == OperationMutation {
		rootType = g.supergraph.MutationType
	}
	if rootType == nil {
		return nil, fmt.Errorf("schema is not configured for %ss", operation.Operation)
	}

	rootFields := p.expandFields(rootType, operation.SelectionSet)
	if len(rootFields) == 0 {
		return nil, errors.New("operation selects no root fields")
	}

	// Group root fields by owning subgraph, preserving first-seen order.
	type rootGroup struct {
		sg     *Subgraph
		fields []*Field
	}
	var groups []*rootGroup
	bySubgraph := map[*Subgraph]*rootGroup{}
	for _, f := range rootFields {
		owner := g.fieldOwnerOf(rootType.Name_, f.Name)
		if owner == nil {
			if len(g.subgraphs) == 0 {
				return nil, errors.New("gateway has no subgraphs")
			}
			owner = g.subgraphs[0]
		}
		grp := bySubgraph[owner]
		if grp == nil {
			grp = &rootGroup{sg: owner}
			bySubgraph[owner] = grp
			groups = append(groups, grp)
		}
		grp.fields = append(grp.fields, f)
	}

	steps := make([]PlanNode, 0, len(groups))
	for _, grp := range groups {
		node := &FetchNode{Subgraph: grp.sg, RootType: rootType}
		for _, f := range grp.fields {
			newF, childFlattens, err := p.planField(grp.sg, rootType, f)
			if err != nil {
				return nil, err
			}
			node.Selection = append(node.Selection, newF)
			node.Flatten = append(node.Flatten, childFlattens...)
		}
		steps = append(steps, node)
	}

	var root PlanNode = &ParallelNode{Steps: steps}
	if operation.Operation == OperationMutation {
		root = &SequenceNode{Steps: steps}
	}
	return &QueryPlan{Operation: operation.Operation, Root: root}, nil
}

type planner struct {
	gw        *Gateway
	fragments map[string]*FragmentDefinition
}

// planField plans the selection set beneath a single field. It returns a copy of
// the field whose selection only contains fields owned by sg, plus the flatten
// steps (paths relative to the field's parent) required for cross-subgraph
// fields.
func (p *planner) planField(sg *Subgraph, parentType *ObjectType, f *Field) (*Field, []*FlattenNode, error) {
	copyF := *f
	if len(f.SelectionSet) == 0 || f.Name == "__typename" {
		return &copyF, nil, nil
	}
	fd := p.gw.fieldDef(parentType.Name_, f.Name)
	if fd == nil {
		return &copyF, nil, nil
	}
	objType := namedObjectType(fd.Type)
	if objType == nil {
		// Leaf or abstract type: keep the selection as-is.
		return &copyF, nil, nil
	}

	childFields := p.expandFields(objType, f.SelectionSet)
	local, flattens, err := p.planSelection(sg, objType, childFields)
	if err != nil {
		return nil, nil, err
	}
	copyF.SelectionSet = local

	// Paths returned by planSelection are relative to objType. Re-anchor them to
	// the field's parent by prefixing the field's response key.
	prefixed := make([]*FlattenNode, 0, len(flattens))
	for _, fl := range flattens {
		path := append([]string{responseKey(f)}, fl.Path...)
		prefixed = append(prefixed, &FlattenNode{Path: path, Node: fl.Node})
	}
	return &copyF, prefixed, nil
}

// planSelection splits a selection set of one object type into fields resolved
// by sg and entity fetches to other subgraphs. Returned flatten paths are
// relative to the object itself.
func (p *planner) planSelection(sg *Subgraph, objType *ObjectType, fields []*Field) ([]Selection, []*FlattenNode, error) {
	var local []Selection
	var flattens []*FlattenNode

	type crossGroup struct {
		sg     *Subgraph
		entity *EntityDefinition
		fields []*Field
	}
	var crosses []*crossGroup
	bySubgraph := map[*Subgraph]*crossGroup{}

	for _, f := range fields {
		owner := p.gw.fieldOwnerOf(objType.Name_, f.Name)
		if owner != nil && owner != sg {
			entity := owner.Entities[objType.Name_]
			if entity == nil {
				return nil, nil, fmt.Errorf("federation: %s.%s is owned by subgraph %q where %s is not an entity", objType.Name_, f.Name, owner.Name, objType.Name_)
			}
			cg := bySubgraph[owner]
			if cg == nil {
				cg = &crossGroup{sg: owner, entity: entity}
				bySubgraph[owner] = cg
				crosses = append(crosses, cg)
			}
			cg.fields = append(cg.fields, f)
			continue
		}

		newF, childFlattens, err := p.planField(sg, objType, f)
		if err != nil {
			return nil, nil, err
		}
		local = append(local, newF)
		flattens = append(flattens, childFlattens...)
	}

	if len(crosses) > 0 {
		var keyFields []string
		for _, c := range crosses {
			keyFields = append(keyFields, c.entity.KeyFields...)
		}
		injected, err := p.injectKeys(sg, objType, keyFields, local)
		if err != nil {
			return nil, nil, err
		}
		local = injected

		for _, c := range crosses {
			node, err := p.planEntityFetch(c.sg, objType, c.fields, c.entity)
			if err != nil {
				return nil, nil, err
			}
			flattens = append(flattens, &FlattenNode{Path: nil, Node: node})
		}
	}

	return local, flattens, nil
}

func (p *planner) planEntityFetch(sg *Subgraph, objType *ObjectType, fields []*Field, entity *EntityDefinition) (*FetchNode, error) {
	local, flattens, err := p.planSelection(sg, objType, fields)
	if err != nil {
		return nil, err
	}
	return &FetchNode{
		Subgraph:  sg,
		Selection: local,
		Entity: &EntityFetch{
			TypeName:   objType.Name_,
			KeyFields:  entity.KeyFields,
			Definition: entity,
		},
		Flatten: flattens,
	}, nil
}

// injectKeys adds __typename and the entity key fields to a selection so the
// parent fetch can build representations for dependent entity fetches.
func (p *planner) injectKeys(sg *Subgraph, objType *ObjectType, keyFields []string, local []Selection) ([]Selection, error) {
	needed := map[string]bool{"__typename": true}
	for _, k := range keyFields {
		needed[k] = true
	}
	present := map[string]bool{}
	for _, sel := range local {
		if f, ok := sel.(*Field); ok {
			present[responseKey(f)] = true
		}
	}

	sgObj, _ := sg.Schema.Type(objType.Name_).(*ObjectType)
	names := make([]string, 0, len(needed))
	for n := range needed {
		names = append(names, n)
	}
	sort.Strings(names)

	result := local
	for _, name := range names {
		if present[name] {
			continue
		}
		if name == "__typename" {
			result = append(result, &Field{Name: "__typename"})
			continue
		}
		if sgObj == nil || sgObj.Fields_[name] == nil {
			return nil, fmt.Errorf("federation: cannot build representation for %s: key field %q is not available from subgraph %q", objType.Name_, name, sg.Name)
		}
		result = append(result, &Field{Name: name})
	}
	return result, nil
}

// expandFields flattens fragments into a list of fields that apply to objType.
func (p *planner) expandFields(objType *ObjectType, selections []Selection) []*Field {
	var out []*Field
	for _, sel := range selections {
		switch s := sel.(type) {
		case *Field:
			out = append(out, s)
		case *InlineFragment:
			if s.TypeCondition != "" && !p.typeConditionApplies(objType, s.TypeCondition) {
				continue
			}
			out = append(out, withDirectives(p.expandFields(objType, s.SelectionSet), s.Directives)...)
		case *FragmentSpread:
			frag := p.fragments[s.Name]
			if frag == nil {
				continue
			}
			if frag.TypeCondition != "" && !p.typeConditionApplies(objType, frag.TypeCondition) {
				continue
			}
			dirs := append(append([]*Directive{}, frag.Directives...), s.Directives...)
			out = append(out, withDirectives(p.expandFields(objType, frag.SelectionSet), dirs)...)
		}
	}
	return out
}

func (p *planner) typeConditionApplies(objType *ObjectType, condition string) bool {
	if condition == objType.Name_ {
		return true
	}
	if t := p.gw.supergraph.Type(condition); t != nil {
		if iface, ok := t.(*InterfaceType); ok {
			return p.gw.supergraph.IsPossibleType(iface, objType)
		}
	}
	return false
}

func withDirectives(fields []*Field, dirs []*Directive) []*Field {
	if len(dirs) == 0 {
		return fields
	}
	out := make([]*Field, len(fields))
	for i, f := range fields {
		c := *f
		c.Directives = append(append([]*Directive{}, f.Directives...), dirs...)
		out[i] = &c
	}
	return out
}

// ============================================================
// Plan executor
// ============================================================

// ExecutePlanned parses, validates, plans and executes a federated operation
// using the query planner. It is an alternative to Gateway.Execute.
func (g *Gateway) ExecutePlanned(query string, variables map[string]interface{}, operationName string) *Result {
	doc, err := Parse(query)
	if err != nil {
		return &Result{Errors: []*GraphQLError{FormatError(err)}}
	}
	if validationErrors := Validate(g.supergraph, doc); len(validationErrors) > 0 {
		return &Result{Errors: validationErrors}
	}
	plan, err := g.PlanDocument(doc, operationName)
	if err != nil {
		return &Result{Errors: []*GraphQLError{FormatError(err)}}
	}
	return g.ExecutePlan(plan, doc, variables, operationName)
}

// ExecutePlan executes a previously built plan against a parsed document.
func (g *Gateway) ExecutePlan(plan *QueryPlan, doc *Document, variables map[string]interface{}, operationName string) *Result {
	if plan == nil {
		return &Result{Errors: []*GraphQLError{{Message: "Must provide a query plan"}}}
	}
	operation, fragments, opErr := findOperation(doc, operationName)
	if opErr != nil {
		return &Result{Errors: []*GraphQLError{opErr}}
	}
	coerced, varErrors := coerceOperationVariables(g.supergraph, operation, variables)
	if len(varErrors) > 0 {
		return &Result{Errors: varErrors}
	}

	pe := &planExecutor{
		gw:        g,
		operation: operation,
		fragments: fragments,
		variables: coerced,
	}

	data := pe.runPlan(plan.Root, nil)

	rootType := g.supergraph.QueryType
	if operation.Operation == OperationMutation {
		rootType = g.supergraph.MutationType
	}
	shaped := pe.shape(data, rootType, operation.SelectionSet)

	return &Result{Data: shaped, Errors: pe.Errors()}
}

type planExecutor struct {
	gw        *Gateway
	operation *OperationDefinition
	fragments map[string]*FragmentDefinition
	variables map[string]interface{}

	mu     sync.Mutex
	errors []*GraphQLError
}

func (pe *planExecutor) addErrors(errs []*GraphQLError) {
	if len(errs) == 0 {
		return
	}
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.errors = append(pe.errors, errs...)
}

func (pe *planExecutor) Errors() []*GraphQLError {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	return pe.errors
}

func (pe *planExecutor) runPlan(node PlanNode, rootValue interface{}) map[string]interface{} {
	switch n := node.(type) {
	case *ParallelNode:
		result := map[string]interface{}{}
		for _, step := range n.Steps {
			deepMerge(result, pe.runPlan(step, rootValue))
		}
		return result
	case *SequenceNode:
		result := map[string]interface{}{}
		for _, step := range n.Steps {
			deepMerge(result, pe.runPlan(step, rootValue))
		}
		return result
	case *FetchNode:
		return pe.runRootFetch(n, rootValue)
	}
	return nil
}

func (pe *planExecutor) runRootFetch(node *FetchNode, rootValue interface{}) map[string]interface{} {
	data := pe.executeSelection(node.Subgraph, node.RootType, rootValue, node.Selection)
	if data == nil {
		return nil
	}
	for _, fl := range node.Flatten {
		pe.applyFlatten(fl, data)
	}
	return data
}

// executeSelection resolves a selection against a subgraph using its own schema.
func (pe *planExecutor) executeSelection(sg *Subgraph, objType *ObjectType, source interface{}, selection []Selection) map[string]interface{} {
	if objType == nil {
		return nil
	}
	ctx := newExecutionContext(sg.Schema, pe.operation, pe.fragments, pe.variables, 0)
	data, _ := ctx.executeFields(objType, source, selection, []interface{}{})
	pe.addErrors(ctx.Errors())
	return data
}

// applyFlatten resolves the entities found at fl.Path and merges their fields
// (and any deeper flattens) into the surrounding result.
func (pe *planExecutor) applyFlatten(fl *FlattenNode, data interface{}) {
	entities := collectAtPath(data, fl.Path)
	if len(entities) == 0 {
		return
	}

	ef := fl.Node.Entity
	if ef == nil || ef.Definition == nil {
		return
	}

	targets := make([]map[string]interface{}, 0, len(entities))
	reprs := make([]map[string]interface{}, 0, len(entities))
	for _, e := range entities {
		m, ok := e.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		repr := map[string]interface{}{"__typename": ef.TypeName}
		complete := true
		for _, k := range ef.KeyFields {
			v, exists := m[k]
			if !exists {
				complete = false
				break
			}
			repr[k] = v
		}
		if !complete {
			continue
		}
		targets = append(targets, m)
		reprs = append(reprs, repr)
	}
	if len(reprs) == 0 {
		return
	}

	resolved := make([]interface{}, len(reprs))
	if ef.Definition.BatchResolver != nil {
		batch, err := ef.Definition.BatchResolver(reprs)
		if err != nil {
			pe.addErrors([]*GraphQLError{FormatError(err)})
			return
		}
		copy(resolved, batch)
	} else {
		for i, repr := range reprs {
			v, err := ef.Definition.Resolver(repr)
			if err != nil {
				pe.addErrors([]*GraphQLError{FormatError(err)})
				continue
			}
			resolved[i] = v
		}
	}

	entityType := namedObjectType(fl.Node.Subgraph.Schema.Type(ef.TypeName))
	for i, target := range targets {
		if i >= len(resolved) || resolved[i] == nil {
			continue
		}
		completed := pe.executeSelection(fl.Node.Subgraph, entityType, resolved[i], fl.Node.Selection)
		if completed == nil {
			continue
		}
		deepMerge(target, completed)
		for _, child := range fl.Node.Flatten {
			pe.applyFlatten(child, target)
		}
	}
}

// collectAtPath returns the entity objects/values located at a response path.
func collectAtPath(node interface{}, path []string) []interface{} {
	if len(path) == 0 {
		if list, ok := node.([]interface{}); ok {
			return list
		}
		return []interface{}{node}
	}
	switch v := node.(type) {
	case map[string]interface{}:
		return collectAtPath(v[path[0]], path[1:])
	case []interface{}:
		var out []interface{}
		for _, item := range v {
			out = append(out, collectAtPath(item, path)...)
		}
		return out
	}
	return nil
}

func deepMerge(dst, src map[string]interface{}) {
	for k, sv := range src {
		if dv, ok := dst[k].(map[string]interface{}); ok {
			if sm, ok := sv.(map[string]interface{}); ok {
				deepMerge(dv, sm)
				continue
			}
		}
		dst[k] = sv
	}
}

// ============================================================
// Response shaping
// ============================================================

// shape rebuilds the response from the original operation so injected key
// fields and __typename never leak into the result.
func (pe *planExecutor) shape(data interface{}, typ GraphQLType, selections []Selection) interface{} {
	if data == nil {
		return nil
	}
	if list, ok := data.([]interface{}); ok {
		out := make([]interface{}, len(list))
		for i, item := range list {
			out[i] = pe.shape(item, typ, selections)
		}
		return out
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}

	objType := namedObjectType(typ)
	if objType == nil {
		if tn, ok := m["__typename"].(string); ok {
			if t := pe.gw.supergraph.Type(tn); t != nil {
				objType, _ = t.(*ObjectType)
			}
		}
	}

	result := map[string]interface{}{}
	pe.shapeInto(result, m, objType, selections)
	return result
}

func (pe *planExecutor) shapeInto(result, data map[string]interface{}, objType *ObjectType, selections []Selection) {
	for _, sel := range selections {
		if selectionSkipped(sel, pe.variables) {
			continue
		}
		switch s := sel.(type) {
		case *Field:
			key := responseKey(s)
			val := data[key]
			if len(s.SelectionSet) > 0 && val != nil && objType != nil {
				if fd := pe.gw.fieldDef(objType.Name_, s.Name); fd != nil {
					val = pe.shape(val, fd.Type, s.SelectionSet)
				}
			}
			result[key] = val
		case *InlineFragment:
			if s.TypeCondition != "" && !pe.typeConditionApplies(data, objType, s.TypeCondition) {
				continue
			}
			pe.shapeInto(result, data, objType, s.SelectionSet)
		case *FragmentSpread:
			frag := pe.fragments[s.Name]
			if frag == nil {
				continue
			}
			if frag.TypeCondition != "" && !pe.typeConditionApplies(data, objType, frag.TypeCondition) {
				continue
			}
			pe.shapeInto(result, data, objType, frag.SelectionSet)
		}
	}
}

func (pe *planExecutor) typeConditionApplies(data map[string]interface{}, objType *ObjectType, condition string) bool {
	if objType != nil && condition == objType.Name_ {
		return true
	}
	if tn, ok := data["__typename"].(string); ok && tn == condition {
		return true
	}
	if objType != nil {
		if t := pe.gw.supergraph.Type(condition); t != nil {
			if iface, ok := t.(*InterfaceType); ok {
				return pe.gw.supergraph.IsPossibleType(iface, objType)
			}
		}
	}
	return false
}

func selectionSkipped(sel Selection, variables map[string]interface{}) bool {
	var dirs []*Directive
	switch s := sel.(type) {
	case *Field:
		dirs = s.Directives
	case *FragmentSpread:
		dirs = s.Directives
	case *InlineFragment:
		dirs = s.Directives
	}
	for _, d := range dirs {
		switch d.Name {
		case "skip":
			if args, err := CoerceArgumentValues(SkipDirective.Args, d.Arguments, variables); err == nil {
				if b, ok := args["if"].(bool); ok && b {
					return true
				}
			}
		case "include":
			if args, err := CoerceArgumentValues(IncludeDirective.Args, d.Arguments, variables); err == nil {
				if b, ok := args["if"].(bool); ok && !b {
					return true
				}
			}
		}
	}
	return false
}

// ============================================================
// Helpers
// ============================================================

func responseKey(f *Field) string {
	if f.Alias != "" {
		return f.Alias
	}
	return f.Name
}

func namedObjectType(t GraphQLType) *ObjectType {
	if t == nil {
		return nil
	}
	obj, _ := UnwrapType(t).(*ObjectType)
	return obj
}

// fieldOwnerOf returns the subgraph that owns a field, or nil.
func (g *Gateway) fieldOwnerOf(typeName, fieldName string) *Subgraph {
	if m := g.fieldOwner[typeName]; m != nil {
		return m[fieldName]
	}
	return nil
}

// fieldDef returns the merged supergraph field definition.
func (g *Gateway) fieldDef(typeName, fieldName string) *FieldDefinition {
	t, ok := g.supergraph.Type(typeName).(*ObjectType)
	if !ok {
		return nil
	}
	return t.Fields_[fieldName]
}
