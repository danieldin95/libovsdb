package server

import (
	"testing"

	"github.com/google/uuid"
	"github.com/ovn-org/libovsdb/mapper"
	"github.com/ovn-org/libovsdb/model"
	"github.com/ovn-org/libovsdb/ovsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutateOp(t *testing.T) {
	defDB, err := model.NewClientDBModel("Open_vSwitch", map[string]model.Model{
		"Open_vSwitch": &ovsType{},
		"Bridge":       &bridgeType{}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := getSchema()
	if err != nil {
		t.Fatal(err)
	}
	ovsDB := NewInMemoryDatabase(map[string]model.ClientDBModel{"Open_vSwitch": defDB})
	dbModel, errs := model.NewDatabaseModel(schema, defDB)
	require.Empty(t, errs)
	o, err := NewOvsdbServer(ovsDB, dbModel)
	require.Nil(t, err)

	ovsUUID := uuid.NewString()
	bridgeUUID := uuid.NewString()

	m := mapper.NewMapper(schema)

	ovs := ovsType{}
	info, err := dbModel.NewModelInfo(&ovs)
	require.NoError(t, err)
	ovsRow, err := m.NewRow(info)
	require.Nil(t, err)

	bridge := bridgeType{
		Name: "foo",
		ExternalIds: map[string]string{
			"foo":   "bar",
			"baz":   "quux",
			"waldo": "fred",
		},
	}
	bridgeInfo, err := dbModel.NewModelInfo(&bridge)
	require.NoError(t, err)
	bridgeRow, err := m.NewRow(bridgeInfo)
	require.Nil(t, err)

	transaction := o.NewTransaction(dbModel, "Open_vSwitch", o.db)

	res, updates := transaction.Insert("Open_vSwitch", ovsUUID, ovsRow)
	_, err = ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "insert"}})
	require.Nil(t, err)

	res, update2 := transaction.Insert("Bridge", bridgeUUID, bridgeRow)
	_, err = ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "insert"}})
	require.Nil(t, err)

	updates.Merge(update2)
	err = o.db.Commit("Open_vSwitch", uuid.New(), updates)
	require.NoError(t, err)

	gotResult, gotUpdate := transaction.Mutate(
		"Open_vSwitch",
		"Open_vSwitch",
		[]ovsdb.Condition{
			ovsdb.NewCondition("_uuid", ovsdb.ConditionEqual, ovsdb.UUID{GoUUID: ovsUUID}),
		},
		[]ovsdb.Mutation{
			*ovsdb.NewMutation("bridges", ovsdb.MutateOperationInsert, ovsdb.UUID{GoUUID: bridgeUUID}),
		},
	)
	assert.Equal(t, ovsdb.OperationResult{Count: 1}, gotResult)
	err = o.db.Commit("Open_vSwitch", uuid.New(), gotUpdate)
	require.NoError(t, err)

	bridgeSet, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: bridgeUUID}})
	assert.Nil(t, err)
	assert.Equal(t, ovsdb.TableUpdates2{
		"Open_vSwitch": ovsdb.TableUpdate2{
			ovsUUID: &ovsdb.RowUpdate2{
				Modify: &ovsdb.Row{
					"bridges": bridgeSet,
				},
				Old: &ovsdb.Row{
					// TODO: _uuid should be filtered
					"_uuid": ovsdb.UUID{GoUUID: ovsUUID},
				},
				New: &ovsdb.Row{
					// TODO: _uuid should be filtered
					"_uuid":   ovsdb.UUID{GoUUID: ovsUUID},
					"bridges": bridgeSet,
				},
			},
		},
	}, gotUpdate)

	keyDelete, err := ovsdb.NewOvsSet([]string{"foo"})
	assert.Nil(t, err)
	keyValueDelete, err := ovsdb.NewOvsMap(map[string]string{"baz": "quux"})
	assert.Nil(t, err)
	gotResult, gotUpdate = transaction.Mutate(
		"Open_vSwitch",
		"Bridge",
		[]ovsdb.Condition{
			ovsdb.NewCondition("_uuid", ovsdb.ConditionEqual, ovsdb.UUID{GoUUID: bridgeUUID}),
		},
		[]ovsdb.Mutation{
			*ovsdb.NewMutation("external_ids", ovsdb.MutateOperationDelete, keyDelete),
			*ovsdb.NewMutation("external_ids", ovsdb.MutateOperationDelete, keyValueDelete),
		},
	)
	assert.Equal(t, ovsdb.OperationResult{Count: 1}, gotResult)

	oldExternalIds, _ := ovsdb.NewOvsMap(bridge.ExternalIds)
	newExternalIds, _ := ovsdb.NewOvsMap(map[string]string{"waldo": "fred"})
	diffExternalIds, _ := ovsdb.NewOvsMap(map[string]string{"foo": "bar", "baz": "quux"})

	assert.Nil(t, err)

	gotModify := *gotUpdate["Bridge"][bridgeUUID].Modify
	gotOld := *gotUpdate["Bridge"][bridgeUUID].Old
	gotNew := *gotUpdate["Bridge"][bridgeUUID].New
	assert.Equal(t, diffExternalIds, gotModify["external_ids"])
	assert.Equal(t, oldExternalIds, gotOld["external_ids"])
	assert.Equal(t, newExternalIds, gotNew["external_ids"])
}

func TestDiff(t *testing.T) {
	originSet, _ := ovsdb.NewOvsSet([]interface{}{"foo"})

	originSetAdd, _ := ovsdb.NewOvsSet([]interface{}{"bar"})
	setAddDiff, _ := ovsdb.NewOvsSet([]interface{}{"foo", "bar"})

	originSetDel, _ := ovsdb.NewOvsSet([]interface{}{})
	setDelDiff, _ := ovsdb.NewOvsSet([]interface{}{"foo"})

	originMap, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"foo": "bar"})

	originMapAdd, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"foo": "bar", "baz": "quux"})
	originMapAddDiff, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"baz": "quux"})

	originMapDel, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{})
	originMapDelDiff, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"foo": "bar"})

	originMapReplace, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"foo": "baz"})
	originMapReplaceDiff, _ := ovsdb.NewOvsMap(map[interface{}]interface{}{"foo": "baz"})

	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected interface{}
	}{
		{
			"add to set",
			originSet,
			originSetAdd,
			setAddDiff,
		},
		{
			"delete from set",
			originSet,
			originSetDel,
			setDelDiff,
		},
		{
			"noop set",
			originSet,
			originSet,
			nil,
		},
		{
			"add to map",
			originMap,
			originMapAdd,
			originMapAddDiff,
		},
		{
			"delete from map",
			originMap,
			originMapDel,
			originMapDelDiff,
		},
		{
			"replace in map",
			originMap,
			originMapReplace,
			originMapReplaceDiff,
		},
		{
			"noop map",
			originMap,
			originMap,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := diff(tt.a, tt.b)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestOvsdbServerInsert(t *testing.T) {
	t.Skip("need a helper for comparing rows as map elements aren't in same order")
	defDB, err := model.NewClientDBModel("Open_vSwitch", map[string]model.Model{
		"Open_vSwitch": &ovsType{},
		"Bridge":       &bridgeType{}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := getSchema()
	if err != nil {
		t.Fatal(err)
	}
	ovsDB := NewInMemoryDatabase(map[string]model.ClientDBModel{"Open_vSwitch": defDB})
	dbModel, errs := model.NewDatabaseModel(schema, defDB)
	require.Empty(t, errs)
	o, err := NewOvsdbServer(ovsDB, dbModel)
	require.Nil(t, err)
	m := mapper.NewMapper(schema)

	gromit := "gromit"
	bridge := bridgeType{
		Name:         "foo",
		DatapathType: "bar",
		DatapathID:   &gromit,
		ExternalIds: map[string]string{
			"foo":   "bar",
			"baz":   "qux",
			"waldo": "fred",
		},
	}
	bridgeUUID := uuid.NewString()
	bridgeInfo, err := dbModel.NewModelInfo(&bridge)
	require.NoError(t, err)
	bridgeRow, err := m.NewRow(bridgeInfo)
	require.Nil(t, err)

	transaction := o.NewTransaction(dbModel, "Open_vSwitch", o.db)

	res, updates := transaction.Insert("Bridge", bridgeUUID, bridgeRow)
	_, err = ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "insert"}})
	require.NoError(t, err)

	err = ovsDB.Commit("Open_vSwitch", uuid.New(), updates)
	assert.NoError(t, err)

	bridge.UUID = bridgeUUID
	br, err := o.db.Get("Open_vSwitch", "Bridge", bridgeUUID)
	assert.NoError(t, err)
	assert.Equal(t, &bridge, br)
	assert.Equal(t, ovsdb.TableUpdates2{
		"Bridge": {
			bridgeUUID: &ovsdb.RowUpdate2{
				Insert: &bridgeRow,
				New:    &bridgeRow,
			},
		},
	}, updates)
}

func TestOvsdbServerUpdate(t *testing.T) {
	defDB, err := model.NewClientDBModel("Open_vSwitch", map[string]model.Model{
		"Open_vSwitch": &ovsType{},
		"Bridge":       &bridgeType{}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := getSchema()
	if err != nil {
		t.Fatal(err)
	}
	ovsDB := NewInMemoryDatabase(map[string]model.ClientDBModel{"Open_vSwitch": defDB})
	dbModel, errs := model.NewDatabaseModel(schema, defDB)
	require.Empty(t, errs)
	o, err := NewOvsdbServer(ovsDB, dbModel)
	require.Nil(t, err)
	m := mapper.NewMapper(schema)

	bridge := bridgeType{
		Name: "foo",
		ExternalIds: map[string]string{
			"foo":   "bar",
			"baz":   "qux",
			"waldo": "fred",
		},
	}
	bridgeUUID := uuid.NewString()
	bridgeInfo, err := dbModel.NewModelInfo(&bridge)
	require.NoError(t, err)
	bridgeRow, err := m.NewRow(bridgeInfo)
	require.Nil(t, err)

	transaction := o.NewTransaction(dbModel, "Open_vSwitch", o.db)

	res, updates := transaction.Insert("Bridge", bridgeUUID, bridgeRow)
	_, err = ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "insert"}})
	require.NoError(t, err)

	err = ovsDB.Commit("Open_vSwitch", uuid.New(), updates)
	assert.NoError(t, err)

	halloween, _ := ovsdb.NewOvsSet([]string{"halloween"})
	tests := []struct {
		name     string
		row      ovsdb.Row
		expected *ovsdb.RowUpdate2
	}{
		{
			"update single field",
			ovsdb.Row{"datapath_type": "waldo"},
			&ovsdb.RowUpdate2{
				Modify: &ovsdb.Row{
					"datapath_type": "waldo",
				},
			},
		},
		{
			"update single optional field",
			ovsdb.Row{"datapath_id": "halloween"},
			&ovsdb.RowUpdate2{
				Modify: &ovsdb.Row{
					"datapath_id": halloween,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, updates := transaction.Update(
				"Open_vSwitch", "Bridge",
				[]ovsdb.Condition{{
					Column: "_uuid", Function: ovsdb.ConditionEqual, Value: ovsdb.UUID{GoUUID: bridgeUUID},
				}}, tt.row)
			errs, err := ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "update"}})
			require.NoErrorf(t, err, "%+v", errs)

			bridge.UUID = bridgeUUID
			row, err := o.db.Get("Open_vSwitch", "Bridge", bridgeUUID)
			assert.NoError(t, err)
			br := row.(*bridgeType)
			assert.NotEqual(t, br, bridgeRow)
			assert.Equal(t, tt.expected.Modify, updates["Bridge"][bridgeUUID].Modify)
		})
	}
}

func TestMultipleOps(t *testing.T) {
	defDB, err := model.NewClientDBModel("Open_vSwitch", map[string]model.Model{
		"Open_vSwitch": &ovsType{},
		"Bridge":       &bridgeType{}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := getSchema()
	if err != nil {
		t.Fatal(err)
	}
	ovsDB := NewInMemoryDatabase(map[string]model.ClientDBModel{"Open_vSwitch": defDB})
	dbModel, errs := model.NewDatabaseModel(schema, defDB)
	require.Empty(t, errs)
	o, err := NewOvsdbServer(ovsDB, dbModel)
	require.Nil(t, err)
	m := mapper.NewMapper(schema)

	bridgeUUID := uuid.NewString()
	bridge := bridgeType{
		Name: "a_bridge_to_nowhere",
		Ports: []string{
			"port1",
			"port10",
		},
	}
	bridgeInfo, err := dbModel.NewModelInfo(&bridge)
	require.NoError(t, err)
	bridgeRow, err := m.NewRow(bridgeInfo)
	require.Nil(t, err)

	transaction := o.NewTransaction(dbModel, "Open_vSwitch", o.db)

	res, updates := transaction.Insert("Bridge", bridgeUUID, bridgeRow)
	_, err = ovsdb.CheckOperationResults([]ovsdb.OperationResult{res}, []ovsdb.Operation{{Op: "insert"}})
	require.Nil(t, err)

	err = o.db.Commit("Open_vSwitch", uuid.New(), updates)
	require.NoError(t, err)

	var ops []ovsdb.Operation
	var op ovsdb.Operation

	portA, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: "portA"}})
	require.NoError(t, err)
	op = ovsdb.Operation{
		Table: "Bridge",
		Where: []ovsdb.Condition{
			ovsdb.NewCondition("_uuid", ovsdb.ConditionEqual, ovsdb.UUID{GoUUID: bridgeUUID}),
		},
		Op: ovsdb.OperationMutate,
		Mutations: []ovsdb.Mutation{
			*ovsdb.NewMutation("ports", ovsdb.MutateOperationInsert, portA),
		},
	}
	ops = append(ops, op)

	portBC, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: "portB"}, {GoUUID: "portC"}})
	require.NoError(t, err)
	op2 := ovsdb.Operation{
		Table: "Bridge",
		Where: []ovsdb.Condition{
			ovsdb.NewCondition("_uuid", ovsdb.ConditionEqual, ovsdb.UUID{GoUUID: bridgeUUID}),
		},
		Op: ovsdb.OperationMutate,
		Mutations: []ovsdb.Mutation{
			*ovsdb.NewMutation("ports", ovsdb.MutateOperationInsert, portBC),
		},
	}
	ops = append(ops, op2)

	results, updates := o.transact("Open_vSwitch", ops)
	require.Len(t, results, len(ops))
	for _, result := range results {
		assert.Equal(t, "", result.Error)
	}

	err = o.db.Commit("Open_vSwitch", uuid.New(), updates)
	require.NoError(t, err)

	modifiedPorts, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: "portA"}, {GoUUID: "portB"}, {GoUUID: "portC"}})
	assert.Nil(t, err)

	oldPorts, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: "port1"}, {GoUUID: "port10"}})
	assert.Nil(t, err)

	newPorts, err := ovsdb.NewOvsSet([]ovsdb.UUID{{GoUUID: "port1"}, {GoUUID: "port10"}, {GoUUID: "portA"}, {GoUUID: "portB"}, {GoUUID: "portC"}})
	assert.Nil(t, err)

	assert.Equal(t, ovsdb.TableUpdates2{
		"Bridge": ovsdb.TableUpdate2{
			bridgeUUID: &ovsdb.RowUpdate2{
				Modify: &ovsdb.Row{
					"ports": modifiedPorts,
				},
				Old: &ovsdb.Row{
					"_uuid": ovsdb.UUID{GoUUID: bridgeUUID},
					"name":  "a_bridge_to_nowhere",
					"ports": oldPorts,
				},
				New: &ovsdb.Row{
					"_uuid": ovsdb.UUID{GoUUID: bridgeUUID},
					"name":  "a_bridge_to_nowhere",
					"ports": newPorts,
				},
			},
		},
	}, updates)

}
