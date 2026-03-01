package gqlx

import (
	"fmt"
	"strings"
	"testing"
)

// Complex introspection query (real-world)
const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
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
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
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
            ofType {
              kind
              name
            }
          }
        }
      }
    }
  }
}
`

// Deeply nested query
const deepNestedQuery = `
query DeepNested($id: ID!, $first: Int = 10, $after: String) {
  user(id: $id) {
    id
    name
    email
    profile {
      bio
      avatar { url width height }
      socialLinks { platform url }
    }
    posts(first: $first, after: $after) {
      edges {
        cursor
        node {
          id
          title
          body
          createdAt
          updatedAt
          author {
            id
            name
            avatar { url }
          }
          comments(first: 5) {
            edges {
              node {
                id
                text
                author { id name }
                replies(first: 3) {
                  edges {
                    node {
                      id
                      text
                      author { id name }
                    }
                  }
                  pageInfo { hasNextPage endCursor }
                }
              }
            }
            pageInfo { hasNextPage endCursor }
          }
          tags { id name color }
          likes { totalCount }
        }
      }
      pageInfo {
        hasNextPage
        hasPreviousPage
        startCursor
        endCursor
      }
      totalCount
    }
    followers(first: 10) {
      edges {
        node { id name avatar { url } }
      }
      totalCount
    }
    following(first: 10) {
      edges {
        node { id name avatar { url } }
      }
      totalCount
    }
  }
}
`

// Query with many arguments, directives, inline fragments
const complexMixedQuery = `
query HeroSearch($text: String!, $ep: Episode = NEWHOPE, $withFriends: Boolean!, $skipDroid: Boolean!) {
  hero(episode: $ep) {
    name
    ... on Human @skip(if: $skipDroid) {
      height(unit: METER)
      homePlanet
      friends @include(if: $withFriends) {
        name
        ... on Human { mass }
        ... on Droid { primaryFunction }
      }
    }
    ... on Droid {
      primaryFunction
      friends @include(if: $withFriends) {
        name
      }
    }
  }
  search(text: $text) {
    ... on Human {
      name
      height(unit: FOOT)
    }
    ... on Droid {
      name
      primaryFunction
    }
    ... on Starship {
      name
      length(unit: METER)
    }
  }
}
`

// Generate a wide query with N fields
func generateWideQuery(n int) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "  field%d: hero(id: %d) { name age }\n", i, i)
	}
	sb.WriteString("}\n")
	return sb.String()
}

// Generate a deeply nested query of depth N
func generateDeepQuery(depth int) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&sb, "%slevel%d {\n", strings.Repeat("  ", i+1), i)
	}
	fmt.Fprintf(&sb, "%svalue\n", strings.Repeat("  ", depth+1))
	for i := depth - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, "%s}\n", strings.Repeat("  ", i+1))
	}
	sb.WriteString("}\n")
	return sb.String()
}

// --- Lexer Benchmarks ---

func BenchmarkLexerIntrospection(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l := NewLexer(introspectionQuery)
		_, err := l.ReadAllTokens()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLexerDeepNested(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l := NewLexer(deepNestedQuery)
		_, err := l.ReadAllTokens()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLexerWide100(b *testing.B) {
	q := generateWideQuery(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := NewLexer(q)
		_, err := l.ReadAllTokens()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLexerWide500(b *testing.B) {
	q := generateWideQuery(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := NewLexer(q)
		_, err := l.ReadAllTokens()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- Parser Benchmarks ---

func BenchmarkParseIntrospection(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Parse(introspectionQuery)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDeepNested(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Parse(deepNestedQuery)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseComplexMixed(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Parse(complexMixedQuery)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseWide100(b *testing.B) {
	q := generateWideQuery(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(q)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseWide500(b *testing.B) {
	q := generateWideQuery(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(q)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDeep50(b *testing.B) {
	q := generateDeepQuery(50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(q)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDeep100(b *testing.B) {
	q := generateDeepQuery(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Parse(q)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- Full Pipeline Benchmark ---

func BenchmarkFullPipelineIntrospection(b *testing.B) {
	schema := heroSchemaForBench()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := Do(schema, introspectionQuery, nil, "IntrospectionQuery")
		if result.HasErrors() {
			b.Fatalf("errors: %v", FormatErrors(result.Errors))
		}
	}
}

func heroSchemaForBench() *Schema {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name":    {Type: StringScalar},
			"age":     {Type: IntScalar},
			"friends": nil,
		},
	}
	humanType.Fields_["friends"] = &FieldDefinition{Type: NewList(humanType)}

	queryType := &ObjectType{
		Name_: "Query",
		Fields_: FieldMap{
			"hero": {
				Type: humanType,
				Args: ArgumentMap{
					"id": {Name_: "id", Type: IDScalar},
				},
				Resolve: func(p ResolveParams) (interface{}, error) {
					return map[string]interface{}{
						"name": "Luke",
						"age":  25,
					}, nil
				},
			},
		},
	}
	schema, _ := NewSchema(SchemaConfig{Query: queryType})
	return schema
}
