package store

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T, store Store) error {
	err := store.SaveJSON("Dummy", map[string]string{"Key": "Value"})
	require.NoError(t, err)

	val := map[string]string{"Key": "Value"}
	err = store.SaveJSON("Dummy3", val)
	require.NoError(t, err)

	var jsonVal map[string]string
	err = store.LoadJSON("Dummy3", &jsonVal)
	require.NoError(t, err)
	require.Equal(t, val, jsonVal)

	keys, err := store.LoadKeys("")
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Contains(t, keys, "Dummy3")

	err = store.Delete("Dummy3")
	require.NoError(t, err)

	err = store.LoadJSON("Dummy3", &jsonVal)
	require.Equal(t, ErrKeyNotFound, err)

	keys, err = store.LoadKeys("")
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Contains(t, keys, "Dummy")

	itr := func(key any, value interface{}) error {
		require.Equal(t, "Dummy", key)
		var jsonVal map[string]string

		require.NoError(t, json.Unmarshal([]byte(value.(string)), &jsonVal))
		require.Equal(t, val, jsonVal)
		return nil
	}

	require.NoError(t, store.LoadValues("Dummy", itr))

	c, err := store.Count("")
	require.NoError(t, err)
	require.Equal(t, uint64(1), c)

	// lets do a bit fancy testing

	nowTime := time.Now().UTC()
	err = store.SaveJSON("jsonpathtest1", map[string]interface{}{"state": "Success", "locked": false, "approval": true, "time": nowTime})
	require.NoError(t, err)

	err = store.SaveJSON("jsonpathtest2", map[string]interface{}{"state": "Failed", "locked": false, "approval": false, "time": nowTime.AddDate(0, 0, -1)})
	require.NoError(t, err)

	err = store.SaveJSON("jsonpathtest3", map[string]interface{}{"state": "Unknown", "locked": true, "approval": false, "time": nowTime.AddDate(0, 0, 1)})
	require.NoError(t, err)

	call := make(map[string]any)
	qItr := func(key any, value any) error {
		call[key.(string)] = value
		return nil
	}

	require.NoError(t, store.QueryJsonPath("jsonpathtest", "$.state", qItr))
	require.True(t, reflect.DeepEqual(call, map[string]interface{}{"jsonpathtest1": "Success", "jsonpathtest2": "Failed", "jsonpathtest3": "Unknown"}))

	call = make(map[string]interface{})
	require.NoError(t, store.QueryJsonPath("jsonpathtest", "$.locked", qItr))
	require.True(t, reflect.DeepEqual(call, map[string]interface{}{"jsonpathtest1": false, "jsonpathtest2": false, "jsonpathtest3": true}))

	call = make(map[string]interface{})
	require.NoError(t, store.CountJsonPath("jsonpathtest", "$.state", qItr))
	require.True(t, reflect.DeepEqual(call, map[string]interface{}{"Failed": int64(1), "Success": int64(1), "Unknown": int64(1)}))

	var sorted []string
	sItr := func(key any, value any) error {
		sorted = append(sorted, key.(string))
		return nil
	}
	require.NoError(t, store.SortedAscN("jsonpathtest", "$.time", -1, sItr))
	require.True(t, reflect.DeepEqual(sorted, []string{"jsonpathtest2", "jsonpathtest1", "jsonpathtest3"}))

	sorted = nil
	require.NoError(t, store.SortedDescN("jsonpathtest", "$.time", -1, sItr))
	require.True(t, reflect.DeepEqual(sorted, []string{"jsonpathtest3", "jsonpathtest1", "jsonpathtest2"}))

	sorted = nil
	require.NoError(t, store.SortedAscN("jsonpathtest", "$.time", 2, sItr))
	require.True(t, reflect.DeepEqual(sorted, []string{"jsonpathtest2", "jsonpathtest1"}))

	sorted = nil
	require.NoError(t, store.SortedDescN("jsonpathtest", "$.time", 2, sItr))
	require.True(t, reflect.DeepEqual(sorted, []string{"jsonpathtest3", "jsonpathtest1"}))

	for i := 0; i < 10; i++ {
		err = store.SaveJSON(fmt.Sprintf("PrefixedKey%d", i), val)
		require.NoError(t, err)
	}

	require.NoError(t, store.DeletePrefix("PrefixedKey"))
	keys, err = store.LoadKeys("PrefixedKey")
	require.NoError(t, err)
	require.Empty(t, keys)

	return nil
}

func TestInMemoryStore(t *testing.T) {
	store, err := NewBadgerDBStore("", "")
	if err != nil {
		t.Fatalf("Found create store error %s", err)
		return
	}

	defer func() {
		assert.Error(t, store.Close())
	}()

	if err := testStore(t, store); err != nil {
		t.Fatalf("Found error %s", err)
		return
	}
}

func TestBadgerDBStore(t *testing.T) {
	storeLocation := "./TestBadgerDBStore"
	aesKey := make([]byte, 32)
	_, err := rand.Read(aesKey)
	if err != nil {
		t.Fatalf("failed to created random aeskey %s", err)
		return
	}
	store, err := NewBadgerDBStore(storeLocation, string(aesKey))

	if err != nil {
		t.Fatalf("Found create store error %s", err)
		return
	}

	defer func() {
		assert.Error(t, os.RemoveAll(storeLocation))
	}()
	defer func() {
		assert.Error(t, store.Close())
	}()

	if err := testStore(t, store); err != nil {
		t.Fatalf("Found error %s", err)
		return
	}
}
func TestPgxStore(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		// skip testing if database url if not defined
		return
	}

	store, err := NewPgxStoreWithTable(os.Getenv("DATABASE_URL"), "TestPgxStore")
	if err != nil {
		t.Fatalf("failed to create store error %s", err)
		return
	}
	defer func() {
		assert.Error(t, store.Close())
	}()

	if err := testStore(t, store); err != nil {
		t.Fatalf("Found error %s", err)
		return
	}
}
