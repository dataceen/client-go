package emit

import (
	"strings"

	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

const clientTemplate = `import (
	"context"
	"fmt"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/pkg/dataceen/dsl"
)

// {{ENTITY}}Client wraps dataceen.Client with {{ENTITY}}-specific helpers.
type {{ENTITY}}Client struct {
	client *dataceen.Client
}

// New{{ENTITY}}Client builds a typed client over the provided dataceen.Client.
func New{{ENTITY}}Client(c *dataceen.Client) *{{ENTITY}}Client {
	return &{{ENTITY}}Client{client: c}
}

// Find{{ENTITY}}Params are the inputs to {{ENTITY}}Client.Find.
type Find{{ENTITY}}Params struct {
	// Size caps results. Zero defaults to 100.
	Size int
	// Cursor is the pagination token. Empty defaults to "null" (start).
	Cursor string
	// Filter narrows results. Build via dsl.F/dsl.And/dsl.Or/dsl.Not. nil = no filter.
	Filter dsl.FilterExpr
	// Fields picks which fields to fetch. Zero value selects nothing.
	Fields {{ENTITY}}Fields
	// OrderBy is optional; nil = unordered.
	OrderBy *dsl.OrderByBuilder
}

// Find returns {{ENTITY}} entities matching the params. Maps onto the server's
// Find{{ENTITY}} GraphQL query.
func (c *{{ENTITY}}Client) Find(ctx context.Context, params Find{{ENTITY}}Params) (*dataceen.FindResult[{{ENTITY}}], error) {
	size := params.Size
	if size == 0 {
		size = 100
	}
	cursor := params.Cursor
	if cursor == "" {
		cursor = "null"
	}
	plan := dsl.QueryPlan{
		Entity: "{{ENTITY}}",
		Size:   size,
		Cursor: cursor,
		Filter: dsl.FilterToNode(params.Filter),
		Fields: params.Fields.ToNodes(),
	}
	if params.OrderBy != nil {
		plan.OrderBy = params.OrderBy.Build()
	}
	op, query := dsl.PlanToGraphQL(plan)

	var result dataceen.FindResult[{{ENTITY}}]
	if err := c.client.ExecuteQuery(ctx, dataceen.GraphQL{
		OperationName: op,
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FindByID fetches a single {{ENTITY}} by its _id. Returns (nil, nil) when no
// match exists.
func (c *{{ENTITY}}Client) FindByID(ctx context.Context, id string, fields {{ENTITY}}Fields) (*{{ENTITY}}, error) {
	res, err := c.Find(ctx, Find{{ENTITY}}Params{
		Size:   1,
		Filter: dsl.F("_id").Eq(id),
		Fields: fields,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, nil
	}
	return &res.Items[0], nil
}
`

const searchTemplate = `
// Search{{ENTITY}}Params are the inputs to {{ENTITY}}Client.Search.
//
// Search differs from Find in three ways: it goes through the Elasticsearch
// index (so behaviour for the like operator matches the documented
// token-match semantics), it exposes a resultlookup arg, and it returns
// aggregations alongside Items. SearchFilter accepts the same
// dsl.FilterExpr shapes as Find — server enforces the type-level differences.
type Search{{ENTITY}}Params struct {
	Size              int
	Cursor            string
	Filter            dsl.FilterExpr
	Fields            {{ENTITY}}Fields
	OrderBy           *dsl.OrderByBuilder
	// ResultLookup defaults to "Full". Other valid value: "WithinIndex".
	ResultLookup      string
	// Aggregations holds zero or more typed aggregation requests built
	// via the generated Search{{ENTITY}}_aggregation type. nil = no aggs.
	Aggregations      []Search{{ENTITY}}Aggregation
	// AggregationDepth caps the recursive expansion of the Aggregations
	// result fragment. Zero defaults to 3.
	AggregationDepth  int
}

// Search runs Search{{ENTITY}} and returns the typed envelope (Items +
// Aggregations + Cursor).
func (c *{{ENTITY}}Client) Search(ctx context.Context, params Search{{ENTITY}}Params) (*dataceen.SearchResult[{{ENTITY}}], error) {
	size := params.Size
	if size == 0 {
		size = 100
	}
	cursor := params.Cursor
	if cursor == "" {
		cursor = "null"
	}
	resultLookup := params.ResultLookup
	if resultLookup == "" {
		resultLookup = "Full"
	}

	whereStr := dsl.RenderFilter(dsl.FilterToNode(params.Filter))
	fieldsStr := dsl.RenderFields(params.Fields.ToNodes())

	q := fmt.Sprintf("query {Search{{ENTITY}}(size: %d, resultlookup: %s, cursor: %q, where: %s",
		size, resultLookup, cursor, whereStr)

	if params.OrderBy != nil {
		if ob := dsl.RenderOrderBy(params.OrderBy.Build()); ob != "" {
			q += ", order_by: " + ob
		}
	}

	hasAggs := len(params.Aggregations) > 0
	if hasAggs {
		q += ", aggregations: " + dsl.EncodeInput(params.Aggregations)
	}

	q += ") { Items { " + fieldsStr + " } Cursor ResponseTime ResponseCode Message"
	if hasAggs {
		depth := params.AggregationDepth
		if depth <= 0 {
			depth = 3
		}
		q += " " + dsl.AggregationFragment(depth)
	}
	q += "}}"

	var result dataceen.SearchResult[{{ENTITY}}]
	if err := c.client.ExecuteQuery(ctx, dataceen.GraphQL{
		OperationName: "Search{{ENTITY}}",
		Query:         q,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const bulkNodeTemplate = `
// CreateBulk inserts many {{ENTITY}} entities in one request. Returns a
// MutationResult whose Result slice contains the assigned _ids in input
// order.
func (c *{{ENTITY}}Client) CreateBulk(ctx context.Context, inputs []*{{ENTITY}}Create) (*dataceen.MutationResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("dataceen: Create{{ENTITY}}Bulk: inputs is empty")
	}
	query := "mutation {Create{{ENTITY}}Bulk({{ENTITY}}List: " + dsl.EncodeInput(inputs) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Create{{ENTITY}}Bulk",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateBulk applies many {{ENTITY}}Update payloads in one request. Each
// element must include MetaId so the server can identify the row to
// modify.
func (c *{{ENTITY}}Client) UpdateBulk(ctx context.Context, inputs []*{{ENTITY}}Update) (*dataceen.MutationResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("dataceen: Update{{ENTITY}}Bulk: inputs is empty")
	}
	query := "mutation {Update{{ENTITY}}Bulk({{ENTITY}}List: " + dsl.EncodeInput(inputs) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Update{{ENTITY}}Bulk",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const bulkRelationTemplate = `
// CreateBulk inserts many {{ENTITY}} relationships in one request.
// Each input must have _fromId / _toId set; _fromLabel / _toLabel /
// _label default to {{FROM}} / {{TO}} / {{ENTITY}} when empty.
//
// Server mutation: {{MUTATION}}Bulk
func (c *{{ENTITY}}Client) CreateBulk(ctx context.Context, inputs []*{{ENTITY}}RelationCreate) (*dataceen.MutationResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("dataceen: {{MUTATION}}Bulk: inputs is empty")
	}
	// Mutate a local copy so default labels are applied without touching
	// the caller's slice.
	prepared := make([]*{{ENTITY}}RelationCreate, len(inputs))
	for i, in := range inputs {
		if in == nil {
			return nil, fmt.Errorf("dataceen: {{MUTATION}}Bulk: inputs[%d] is nil", i)
		}
		if in.FromID == "" || in.ToID == "" {
			return nil, fmt.Errorf("dataceen: {{MUTATION}}Bulk: inputs[%d] missing _fromId or _toId", i)
		}
		clone := *in
		if clone.FromLabel == "" {
			clone.FromLabel = "{{FROM}}"
		}
		if clone.ToLabel == "" {
			clone.ToLabel = "{{TO}}"
		}
		if clone.Label == "" {
			clone.Label = "{{ENTITY}}"
		}
		prepared[i] = &clone
	}
	query := "mutation {{{MUTATION}}Bulk({{ENTITY}}List: " + dsl.EncodeInput(prepared) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "{{MUTATION}}Bulk",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const createDeleteTemplate = `
// Create inserts a new {{ENTITY}}. Returns the resulting MutationResult whose
// Result slice contains the assigned _id.
func (c *{{ENTITY}}Client) Create(ctx context.Context, input *{{ENTITY}}Create) (*dataceen.MutationResult, error) {
	if input == nil {
		return nil, fmt.Errorf("dataceen: Create{{ENTITY}}: input is required")
	}
	query := "mutation {Create{{ENTITY}}({{ENTITY}}: " + dsl.EncodeInput(input) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Create{{ENTITY}}",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete removes every {{ENTITY}} matching the filter. Returns the resulting
// MutationResult whose Result slice contains the deleted _ids.
func (c *{{ENTITY}}Client) Delete(ctx context.Context, filter dsl.FilterExpr) (*dataceen.MutationResult, error) {
	if filter == nil {
		return nil, fmt.Errorf("dataceen: Delete{{ENTITY}}: filter is required (nil would delete everything)")
	}
	whereStr := dsl.RenderFilter(dsl.FilterToNode(filter))
	query := "mutation {Delete{{ENTITY}}(where: " + whereStr + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Delete{{ENTITY}}",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const deleteOnlyTemplate = `
// Delete removes every {{ENTITY}} matching the filter. Returns the resulting
// MutationResult whose Result slice contains the deleted _ids.
func (c *{{ENTITY}}Client) Delete(ctx context.Context, filter dsl.FilterExpr) (*dataceen.MutationResult, error) {
	if filter == nil {
		return nil, fmt.Errorf("dataceen: Delete{{ENTITY}}: filter is required (nil would delete everything)")
	}
	whereStr := dsl.RenderFilter(dsl.FilterToNode(filter))
	query := "mutation {Delete{{ENTITY}}(where: " + whereStr + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Delete{{ENTITY}}",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const relationCreateTemplate = `
// Create inserts a new {{ENTITY}} relationship between two existing nodes.
// _fromId / _toId in the input must reference existing {{FROM}} / {{TO}}
// _ids. Returns the resulting MutationResult whose Result slice contains
// the assigned _id.
//
// Server mutation: {{MUTATION}}
func (c *{{ENTITY}}Client) Create(ctx context.Context, input *{{ENTITY}}RelationCreate) (*dataceen.MutationResult, error) {
	if input == nil {
		return nil, fmt.Errorf("dataceen: {{MUTATION}}: input is required")
	}
	if input.FromID == "" || input.ToID == "" {
		return nil, fmt.Errorf("dataceen: {{MUTATION}}: _fromId and _toId are required")
	}
	// Apply default labels if caller omitted them — the server expects them
	// even though introspection doesn't advertise them.
	in := *input
	if in.FromLabel == "" {
		in.FromLabel = "{{FROM}}"
	}
	if in.ToLabel == "" {
		in.ToLabel = "{{TO}}"
	}
	if in.Label == "" {
		in.Label = "{{ENTITY}}"
	}
	query := "mutation {{{MUTATION}}({{ENTITY}}: " + dsl.EncodeInput(&in) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "{{MUTATION}}",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

const updateTemplate = `
// Update applies the non-nil fields of input to every {{ENTITY}} matching
// filter. Returns the resulting MutationResult whose Result slice contains
// the affected _ids.
func (c *{{ENTITY}}Client) Update(ctx context.Context, filter dsl.FilterExpr, input *{{ENTITY}}Update) (*dataceen.MutationResult, error) {
	if filter == nil {
		return nil, fmt.Errorf("dataceen: Update{{ENTITY}}: filter is required (nil would update everything)")
	}
	if input == nil {
		return nil, fmt.Errorf("dataceen: Update{{ENTITY}}: input is required")
	}
	whereStr := dsl.RenderFilter(dsl.FilterToNode(filter))
	query := "mutation {Update{{ENTITY}}(where: " + whereStr + ", {{ENTITY}}: " + dsl.EncodeInput(input) + ") { Result ResponseTime ResponseCode Message }}"
	var result dataceen.MutationResult
	if err := c.client.ExecuteMutation(ctx, dataceen.GraphQL{
		OperationName: "Update{{ENTITY}}",
		Query:         query,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
`

// EmitClient produces `<Entity>Client.go` — a typed wrapper around
// dataceen.Client.ExecuteQuery.
//
// Emits Find + FindByID always; Create + Delete for entities with a
// CreateXX input type (i.e. nodes); Update only when an Update input
// type exists. Search/Bulk/compound-create are scheduled for a later
// extension.
//
// Returns ("", nil) for sub-entities that aren't IsTopLevel — they are
// not independently findable.
func EmitClient(_ schema.ModelSchema, entity schema.EntityInfo, packageName string) (string, error) {
	if !entity.IsTopLevel {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(FileHeader(entity.Name+"Client", packageName))
	b.WriteString(strings.ReplaceAll(clientTemplate, "{{ENTITY}}", entity.Name))

	switch {
	case entity.Kind == schema.EntityRelationship && entity.Relationship != nil:
		// Compound-create + CreateBulk + Delete. Relationships have no
		// Update method (server requires editing via the node's update path).
		applyRelTpl := func(tpl string) {
			body := strings.ReplaceAll(tpl, "{{ENTITY}}", entity.Name)
			body = strings.ReplaceAll(body, "{{FROM}}", entity.Relationship.From)
			body = strings.ReplaceAll(body, "{{TO}}", entity.Relationship.To)
			body = strings.ReplaceAll(body, "{{MUTATION}}", entity.Relationship.CreateMutation)
			b.WriteString(body)
		}
		applyRelTpl(relationCreateTemplate)
		applyRelTpl(bulkRelationTemplate)
		b.WriteString(strings.ReplaceAll(deleteOnlyTemplate, "{{ENTITY}}", entity.Name))

	case entity.Create != nil:
		// Nodes get Create + Delete + Bulk variants + (optional) Update.
		b.WriteString(strings.ReplaceAll(createDeleteTemplate, "{{ENTITY}}", entity.Name))
		b.WriteString(strings.ReplaceAll(bulkNodeTemplate, "{{ENTITY}}", entity.Name))
		if entity.Update != nil {
			b.WriteString(strings.ReplaceAll(updateTemplate, "{{ENTITY}}", entity.Name))
		}
	}

	// Search is emitted for entities with both HasSearch (a Search<E> query)
	// and an Aggregation type (so Search<E>Aggregation is a real type the
	// generated method can reference). Both are always set together for
	// CandyShopModel; the guard is defensive.
	if entity.HasSearch && entity.Aggregation != nil {
		b.WriteString(strings.ReplaceAll(searchTemplate, "{{ENTITY}}", entity.Name))
	}

	return FormatGo(b.String())
}
