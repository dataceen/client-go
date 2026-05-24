package schema

import "strings"

// ModelSchema is the parser's output — the form Phase 3 emitters consume.
type ModelSchema struct {
	Domain   string
	Model    string
	Scope    string
	Entities []EntityInfo
	Enums    []EnumInfo
	// Scalars is a map from GraphQL scalar name to the Go type the emitters
	// should use. Only includes scalars that actually appear in entities.
	Scalars map[string]string
}

// EntityKind classifies an entity as a node or a relationship.
type EntityKind string

const (
	EntityNode         EntityKind = "node"
	EntityRelationship EntityKind = "relationship"
)

// EntityInfo describes one node or relationship.
type EntityInfo struct {
	Name string
	Kind EntityKind

	Read         ObjectType
	Create       *InputType // nil for relationships (compound-create lives separately)
	Update       *InputType
	Filter       InputType
	SearchFilter *InputType
	// Aggregation is the Search<Entity>_aggregation input type — populated
	// for entities that support Search. Each field is typed as
	// AggregationDetails (or a sibling Search<NestedType>_aggregation).
	Aggregation *InputType
	// AggregationDetails is the Search<Entity>AggregationDetails input type
	// (name/size/type/aggregations) referenced by Aggregation fields.
	AggregationDetails *InputType
	FindResultTypeName string
	OrderByTypeName    string

	// HasSearch is true if the schema exposes Search<Entity> as a query.
	HasSearch bool
	// IsTopLevel is true if Find<Entity> exists. Sub-entities (e.g.
	// CustomerName, CustomerContact) have a <Name>Filter sibling but no
	// Find<Name> query — they are reached via parent fields only.
	IsTopLevel bool

	// Relationship is non-nil when Kind == EntityRelationship. Captures the
	// {from, to, compoundCreate*} info needed to emit compound-create stubs.
	Relationship *RelationshipInfo
}

// RelationshipInfo describes how a relationship is created server-side.
type RelationshipInfo struct {
	From                   string
	To                     string
	CompoundCreateTypeName string
	CreateMutation         string
	CreateBulkMutation     string
	CompoundFields         []FieldRef
}

// ObjectType describes a read-model OBJECT.
type ObjectType struct {
	Name   string
	Fields []FieldRef
}

// InputType describes an INPUT_OBJECT used for create/update/filter input.
type InputType struct {
	Name   string
	Fields []FieldRef
}

// FieldRef is a fully-resolved field reference. NON_NULL/LIST wrappers are
// flattened into bools so emitters don't have to walk recursively.
type FieldRef struct {
	Name           string
	BaseType       string // innermost type name
	BaseKind       string // SCALAR | OBJECT | ENUM | INPUT_OBJECT
	NonNull        bool   // outer non-null
	IsList         bool
	ElementNonNull bool
	GoType         string // emitted Go type, e.g. "string", "[]Customer", "*Customer"
}

// EnumInfo lists the values of an ENUM type.
type EnumInfo struct {
	Name   string
	Values []string
}

// scalarToGo maps GraphQL scalars to Go types. Long/Decimal keep string for
// precision (decisions.md #29). Date/Time stay ISO strings on the wire
// (#25/#16); emit no time.Time conversion.
var scalarToGo = map[string]string{
	"String":         "string",
	"ID":             "string",
	"Boolean":        "bool",
	"Int":            "int",
	"Float":          "float64",
	"Double":         "float64",
	"Long":           "string", // precision-safe
	"Decimal":        "string", // precision-safe
	"Date":           "string",
	"DateTime":       "string",
	"DateTimeOffset": "string",
	"Time":           "string",
	"TimeSpan":       "string",
}

// Parse turns a CachedSchema into the ModelSchema the emitters consume.
func Parse(cached *CachedSchema) ModelSchema {
	types := cached.Data.Schema.Types

	// Dedupe by name — server returns some input types twice (decisions.md #31).
	typeMap := make(map[string]*Type, len(types))
	for i := range types {
		t := &types[i]
		if _, exists := typeMap[t.Name]; !exists {
			typeMap[t.Name] = t
		}
	}

	queryRoot := lookupRoot(typeMap, cached.Data.Schema.QueryType)
	mutationRoot := lookupRoot(typeMap, cached.Data.Schema.MutationType)

	topLevelNames := map[string]bool{}
	searchableNames := map[string]bool{}
	if queryRoot != nil {
		for _, f := range queryRoot.Fields {
			switch {
			case strings.HasPrefix(f.Name, "Find"):
				topLevelNames[strings.TrimPrefix(f.Name, "Find")] = true
			case strings.HasPrefix(f.Name, "Search"):
				searchableNames[strings.TrimPrefix(f.Name, "Search")] = true
			}
		}
	}

	usedScalars := map[string]bool{}
	relationshipInfo := collectRelationshipInfo(mutationRoot, typeMap, usedScalars)

	// Sub-entities: OBJECT types with a <Name>Filter sibling but no Find<Name>.
	subEntityNames := map[string]bool{}
	for name, t := range typeMap {
		if t.Kind != "OBJECT" || strings.HasPrefix(name, "__") {
			continue
		}
		if topLevelNames[name] {
			continue
		}
		if _, ok := typeMap[name+"Filter"]; !ok {
			continue
		}
		if strings.HasSuffix(name, "FindResult") ||
			strings.HasSuffix(name, "List") ||
			strings.HasSuffix(name, "SearchResult") {
			continue
		}
		subEntityNames[name] = true
	}

	all := make([]string, 0, len(topLevelNames)+len(subEntityNames))
	for n := range topLevelNames {
		all = append(all, n)
	}
	for n := range subEntityNames {
		all = append(all, n)
	}

	var entities []EntityInfo
	for _, name := range all {
		readType, ok := typeMap[name]
		if !ok {
			continue
		}
		filterType, ok := typeMap[name+"Filter"]
		if !ok {
			continue
		}

		kind := EntityNode
		if isRelationship(readType) {
			kind = EntityRelationship
		}

		info := EntityInfo{
			Name:               name,
			Kind:               kind,
			Read:               toObjectType(readType, typeMap, usedScalars),
			Filter:             toInputType(filterType, typeMap, usedScalars),
			FindResultTypeName: name + "FindResult",
			OrderByTypeName:    name + "_order_by",
			HasSearch:          searchableNames[name],
			IsTopLevel:         topLevelNames[name],
		}
		if t, ok := typeMap[name+"Create"]; ok && kind == EntityNode {
			it := toInputType(t, typeMap, usedScalars)
			info.Create = &it
		}
		if t, ok := typeMap[name+"Update"]; ok {
			it := toInputType(t, typeMap, usedScalars)
			info.Update = &it
		}
		if t, ok := typeMap[name+"SearchFilter"]; ok {
			it := toInputType(t, typeMap, usedScalars)
			info.SearchFilter = &it
		}
		if t, ok := typeMap["Search"+name+"_aggregation"]; ok {
			it := toInputType(t, typeMap, usedScalars)
			info.Aggregation = &it
		}
		if t, ok := typeMap["Search"+name+"AggregationDetails"]; ok {
			it := toInputType(t, typeMap, usedScalars)
			info.AggregationDetails = &it
		}
		if rel, ok := relationshipInfo[name]; ok {
			info.Relationship = rel
		}

		entities = append(entities, info)
	}

	var enums []EnumInfo
	for name, t := range typeMap {
		if t.Kind != "ENUM" || strings.HasPrefix(name, "__") {
			continue
		}
		vals := make([]string, 0, len(t.EnumValues))
		for _, v := range t.EnumValues {
			vals = append(vals, v.Name)
		}
		enums = append(enums, EnumInfo{Name: name, Values: vals})
	}

	scalars := map[string]string{}
	for s := range usedScalars {
		if g, ok := scalarToGo[s]; ok {
			scalars[s] = g
		} else {
			scalars[s] = "any"
		}
	}

	return ModelSchema{
		Domain:   cached.Metadata.Domain,
		Model:    cached.Metadata.Model,
		Scope:    cached.Metadata.Scope,
		Entities: entities,
		Enums:    enums,
		Scalars:  scalars,
	}
}

// FindEntity returns the entity with the given name, or nil if absent.
// Linear search is fine — schemas have <100 entities.
func (m ModelSchema) FindEntity(name string) *EntityInfo {
	for i := range m.Entities {
		if m.Entities[i].Name == name {
			return &m.Entities[i]
		}
	}
	return nil
}

func lookupRoot(typeMap map[string]*Type, ref *NamedRef) *Type {
	if ref == nil || ref.Name == "" {
		return nil
	}
	return typeMap[ref.Name]
}

// isRelationship detects relationship OBJECTs by their edge metadata
// (decisions.md #32) rather than by name suffix.
func isRelationship(t *Type) bool {
	hasFromID, hasToID := false, false
	for _, f := range t.Fields {
		switch f.Name {
		case "_fromId":
			hasFromID = true
		case "_toId":
			hasToID = true
		}
	}
	return hasFromID && hasToID
}

// collectRelationshipInfo scans mutation root for `Create{From}{Rel}{To}`
// patterns. The arg name is the relation; the rest of the mutation name
// encodes from/to.
//
//	CreateCustomerCustomerPlacedOrderOrder
//	  arg: CustomerPlacedOrder: CustomerCustomerPlacedOrderOrderCreate!
//	  → { relation: "CustomerPlacedOrder", from: "Customer", to: "Order" }
func collectRelationshipInfo(
	mutationRoot *Type,
	typeMap map[string]*Type,
	usedScalars map[string]bool,
) map[string]*RelationshipInfo {
	out := map[string]*RelationshipInfo{}
	if mutationRoot == nil {
		return out
	}
	for _, op := range mutationRoot.Fields {
		if !strings.HasPrefix(op.Name, "Create") || strings.HasSuffix(op.Name, "Bulk") {
			continue
		}
		if len(op.Args) == 0 {
			continue
		}
		first := op.Args[0]
		relationName := first.Name

		base := strings.TrimPrefix(op.Name, "Create")
		idx := strings.Index(base, relationName)
		// Must appear in the middle (idx>0); idx==0 would be a node-create
		// like CreateCustomer (where arg is the entity itself).
		if idx <= 0 {
			continue
		}
		from := base[:idx]
		to := base[idx+len(relationName):]
		if from == "" || to == "" {
			continue
		}

		compoundTypeName := unwrapTypeName(first.Type)
		if compoundTypeName == "" {
			continue
		}
		var compoundFields []FieldRef
		if t, ok := typeMap[compoundTypeName]; ok {
			compoundFields = make([]FieldRef, 0, len(t.InputFields))
			for _, f := range t.InputFields {
				compoundFields = append(compoundFields, toFieldRef(f.Name, f.Type, typeMap, usedScalars))
			}
		}

		out[relationName] = &RelationshipInfo{
			From:                   from,
			To:                     to,
			CompoundCreateTypeName: compoundTypeName,
			CreateMutation:         op.Name,
			CreateBulkMutation:     op.Name + "Bulk",
			CompoundFields:         compoundFields,
		}
	}
	return out
}

func unwrapTypeName(ref TypeRef) string {
	cur := &ref
	for cur != nil {
		if cur.Name != "" {
			return cur.Name
		}
		cur = cur.OfType
	}
	return ""
}

func toObjectType(t *Type, typeMap map[string]*Type, usedScalars map[string]bool) ObjectType {
	out := ObjectType{Name: t.Name, Fields: make([]FieldRef, 0, len(t.Fields))}
	for _, f := range t.Fields {
		out.Fields = append(out.Fields, toFieldRef(f.Name, f.Type, typeMap, usedScalars))
	}
	return out
}

func toInputType(t *Type, typeMap map[string]*Type, usedScalars map[string]bool) InputType {
	out := InputType{Name: t.Name, Fields: make([]FieldRef, 0, len(t.InputFields))}
	for _, f := range t.InputFields {
		out.Fields = append(out.Fields, toFieldRef(f.Name, f.Type, typeMap, usedScalars))
	}
	return out
}

func toFieldRef(name string, ref TypeRef, typeMap map[string]*Type, usedScalars map[string]bool) FieldRef {
	cur := ref
	nonNull := false
	if cur.Kind == "NON_NULL" {
		nonNull = true
		cur = *cur.OfType
	}
	isList := false
	elementNonNull := false
	if cur.Kind == "LIST" {
		isList = true
		cur = *cur.OfType
		if cur.Kind == "NON_NULL" {
			elementNonNull = true
			cur = *cur.OfType
		}
	}
	baseType := cur.Name
	if baseType == "" {
		baseType = cur.Kind
	}
	baseKind := resolveKind(cur, typeMap)
	if baseKind == "SCALAR" {
		usedScalars[baseType] = true
	}

	goBase := baseType
	if baseKind == "SCALAR" {
		if g, ok := scalarToGo[baseType]; ok {
			goBase = g
		} else {
			goBase = "any"
		}
	}
	goType := goBase
	if isList {
		goType = "[]" + goType
	} else if !nonNull && (baseKind == "OBJECT" || baseKind == "INPUT_OBJECT") {
		// Optional nested objects emit as pointers so callers can distinguish
		// "absent" from "zero".
		goType = "*" + goType
	}

	return FieldRef{
		Name:           name,
		BaseType:       baseType,
		BaseKind:       baseKind,
		NonNull:        nonNull,
		IsList:         isList,
		ElementNonNull: elementNonNull,
		GoType:         goType,
	}
}

func resolveKind(ref TypeRef, typeMap map[string]*Type) string {
	if ref.Kind != "" && ref.Kind != "NON_NULL" && ref.Kind != "LIST" {
		return ref.Kind
	}
	if ref.Name != "" {
		if t, ok := typeMap[ref.Name]; ok {
			return t.Kind
		}
	}
	return "UNKNOWN"
}
