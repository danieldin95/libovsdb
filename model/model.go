package model

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/ovsdb"
)

// A Model is the base interface used to build Database Models. It is used
// to express how data from a specific Database Table shall be translated into structs
// A Model is a struct with at least one (most likely more) field tagged with the 'ovs' tag
// The value of 'ovs' field must be a valid column name in the OVS Database
// A field associated with the "_uuid" column mandatory. The rest of the columns are optional
// The struct may also have non-tagged fields (which will be ignored by the API calls)
// The Model interface must be implemented by the pointer to such type
// Example:
//type MyLogicalRouter struct {
//	UUID          string            `ovsdb:"_uuid"`
//	Name          string            `ovsdb:"name"`
//	ExternalIDs   map[string]string `ovsdb:"external_ids"`
//	LoadBalancers []string          `ovsdb:"load_balancer"`
//}
type Model interface{}

// Clone creates a deep copy of a model
func Clone(a Model) Model {
	val := reflect.Indirect(reflect.ValueOf(a))
	b := reflect.New(val.Type()).Interface()
	aBytes, _ := json.Marshal(a)
	_ = json.Unmarshal(aBytes, b)
	return b
}

func modelSetUUID(model Model, uuid string) error {
	modelVal := reflect.ValueOf(model).Elem()
	for i := 0; i < modelVal.NumField(); i++ {
		if field := modelVal.Type().Field(i); field.Tag.Get("ovsdb") == "_uuid" &&
			field.Type.Kind() == reflect.String {
			modelVal.Field(i).Set(reflect.ValueOf(uuid))
			return nil
		}
	}
	return fmt.Errorf("model is expected to have a string field mapped to column _uuid")
}

// Condition is a model-based representation of an OVSDB Condition
type Condition struct {
	// Pointer to the field of the model where the operation applies
	Field interface{}
	// Condition function
	Function ovsdb.ConditionFunction
	// Value to use in the condition
	Value interface{}
}

// Mutation is a model-based representation of an OVSDB Mutation
type Mutation struct {
	// Pointer to the field of the model that shall be mutated
	Field interface{}
	// String representing the mutator (as per RFC7047)
	Mutator ovsdb.Mutator
	// Value to use in the mutation
	Value interface{}
}
