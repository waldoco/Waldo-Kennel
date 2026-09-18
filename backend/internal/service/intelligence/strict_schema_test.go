package intelligence

import (
	"sort"
	"testing"
)

// assertStrictSchema walks a JSON schema recursively and enforces the rule
// Codex's (and OpenAI's) strict structured-output mode actually applies:
// every object with additionalProperties:false must list EVERY property key
// in required. A property that is only sometimes meaningful must be made
// nullable (its "type" includes "null") rather than omitted from required —
// omitting it is rejected outright with invalid_json_schema before the model
// ever runs, exactly the failure this test exists to catch ahead of a live
// call.
func assertStrictSchema(t *testing.T, path string, schema map[string]any) {
	t.Helper()
	if !schemaTypeIsObject(schema["type"]) {
		if props, ok := schema["properties"].(map[string]any); ok {
			walkStrictSchemaProperties(t, path, props)
		}
		if items, ok := schema["items"].(map[string]any); ok {
			assertStrictSchema(t, path+"[]", items)
		}
		return
	}
	additionalProps, _ := schema["additionalProperties"].(bool)
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	if additionalProps == false && properties != nil {
		requiredSet := map[string]bool{}
		for _, r := range required {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
		var missing []string
		for key := range properties {
			if !requiredSet[key] {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s: additionalProperties:false object is missing required entries for properties %v (strict structured-output mode rejects this with invalid_json_schema)", path, missing)
		}
	}
	walkStrictSchemaProperties(t, path, properties)
	if items, ok := schema["items"].(map[string]any); ok {
		assertStrictSchema(t, path+"[]", items)
	}
}

// schemaTypeIsObject reports whether a schema's "type" names "object",
// either directly or as one option of a nullable ["object","null"] type.
func schemaTypeIsObject(rawType any) bool {
	switch v := rawType.(type) {
	case string:
		return v == "object"
	case []any:
		for _, t := range v {
			if s, ok := t.(string); ok && s == "object" {
				return true
			}
		}
	}
	return false
}

func walkStrictSchemaProperties(t *testing.T, path string, properties map[string]any) {
	t.Helper()
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child, ok := properties[key].(map[string]any)
		if !ok {
			continue
		}
		assertStrictSchema(t, path+"."+key, child)
	}
}

func TestContractSchemaIsStrictModeValid(t *testing.T) {
	assertStrictSchema(t, "contractSchema", contractSchema())
}

func TestPlanSchemaIsStrictModeValid(t *testing.T) {
	assertStrictSchema(t, "planSchema", planSchema([]string{"C1"}))
}

func TestPlanningReadinessSchemaIsStrictModeValid(t *testing.T) {
	assertStrictSchema(t, "planningReadinessSchema", planningReadinessSchema([]string{"C1"}))
}
