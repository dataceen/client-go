// Package schema fetches, parses and caches the Dataceen GraphQL schema for
// the Phase 3 codegen.
package schema

// IntrospectionQuery is the standard GraphQL introspection query. ofType is
// nested 7 levels deep to comfortably cover NON_NULL(LIST(NON_NULL(T))).
//
// Reference: https://github.com/graphql/graphql-js/blob/main/src/utilities/getIntrospectionQuery.ts
const IntrospectionQuery = `query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { ...FullType }
    directives {
      name
      description
      locations
      args { ...InputValue }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args { ...InputValue }
    type { ...TypeRef }
    isDeprecated
    deprecationReason
  }
  inputFields { ...InputValue }
  interfaces { ...TypeRef }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes { ...TypeRef }
}

fragment InputValue on __InputValue {
  name
  description
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType { kind name }
          }
        }
      }
    }
  }
}
`

// IntrospectionResponse is the typed view of the standard introspection data.
// Mirrors the GraphQL spec. We keep it lean — only fields the parser uses.
type IntrospectionResponse struct {
	Schema Schema `json:"__schema"`
}

// Schema is the introspection root.
type Schema struct {
	QueryType        *NamedRef `json:"queryType"`
	MutationType     *NamedRef `json:"mutationType"`
	SubscriptionType *NamedRef `json:"subscriptionType"`
	Types            []Type    `json:"types"`
	// Directives intentionally untyped — we don't use them yet.
}

// NamedRef is just `{ name: ... }`.
type NamedRef struct {
	Name string `json:"name"`
}

// Type is one entry in __schema.types.
type Type struct {
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	Fields        []Field      `json:"fields,omitempty"`
	InputFields   []InputValue `json:"inputFields,omitempty"`
	Interfaces    []TypeRef    `json:"interfaces,omitempty"`
	EnumValues    []EnumValue  `json:"enumValues,omitempty"`
	PossibleTypes []TypeRef    `json:"possibleTypes,omitempty"`
}

// Field is one field on an OBJECT or INTERFACE type.
type Field struct {
	Name              string       `json:"name"`
	Description       string       `json:"description,omitempty"`
	Args              []InputValue `json:"args,omitempty"`
	Type              TypeRef      `json:"type"`
	IsDeprecated      bool         `json:"isDeprecated,omitempty"`
	DeprecationReason string       `json:"deprecationReason,omitempty"`
}

// InputValue is an argument or input-object field.
type InputValue struct {
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	Type         TypeRef `json:"type"`
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// TypeRef is the recursive type reference (NON_NULL(LIST(NON_NULL(T)))).
type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name,omitempty"`
	OfType *TypeRef `json:"ofType,omitempty"`
}

// EnumValue is one variant of an ENUM type.
type EnumValue struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	IsDeprecated      bool   `json:"isDeprecated,omitempty"`
	DeprecationReason string `json:"deprecationReason,omitempty"`
}

// CachedSchema is the on-disk format produced by `tools/codegen schema`.
// metadata helps debug staleness; data is the full introspection payload.
type CachedSchema struct {
	Metadata Metadata               `json:"metadata"`
	Data     IntrospectionResponse  `json:"data"`
}

// Metadata records when, where and against which model the schema was fetched.
type Metadata struct {
	FetchedAt string         `json:"fetchedAt"`
	Endpoint  string         `json:"endpoint"`
	Domain    string         `json:"domain"`
	Model     string         `json:"model"`
	Scope     string         `json:"scope"`
	Stats     SchemaStats    `json:"stats"`
}

// SchemaStats captures the high-level shape of the schema (types per kind).
type SchemaStats struct {
	TotalTypes int            `json:"totalTypes"`
	ByKind     map[string]int `json:"byKind"`
}
