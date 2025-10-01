package core

import (
	"context"
	"os"
	"testing"

	"github.com/nixmade/orchestrator/store"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func createEngine(logger zerolog.Logger, testName string) (*Engine, error) {
	var dbStore store.Store
	var err error
	if os.Getenv("DATABASE_URL") != "" {
		dbStore, err = store.NewPgxStoreWithTable(os.Getenv("DATABASE_URL"), testName)
	} else {
		dbStore, err = store.NewBadgerDBStore("", "")
	}
	if err != nil {
		return nil, err
	}
	e := &Engine{
		ctx:    context.Background(),
		logger: logger,
		store:  dbStore,
	}

	return e, nil
}

func TestCreateNamespace(t *testing.T) {
	e, err := createEngine(getLogger(), "TestCreateNamespace")
	if err != nil {
		t.Fatalf("Error creating engine '%s'", err)
		return
	}
	defer func() {
		assert.Error(t, e.store.Close())
	}()

	const namespaceName = "TestCreateNamespace"
	n, err := e.createNamespace(namespaceName)

	if err != nil {
		t.Fatalf("Error creating namesapce '%s'", err)
		return
	}

	if n.Name != namespaceName {
		t.Fatalf("Namespace names do not match expected '%s', actual '%s'", namespaceName, n.Name)
		return
	}

	entities, err := n.getEntities()
	if err != nil {
		t.Fatalf("failed to get entities '%s'", err)
		return
	}

	if len(entities) > 0 {
		t.Fatalf("Expected new namespace to have no entities but found '%d'", len(entities))
		return
	}
}

func TestFindorCreateEntity(t *testing.T) {
	const namespaceName = "TestCreateNamespace"
	const entityName = "DummyEntityName"
	e, err := createEngine(getLogger(), "TestFindorCreateEntity")
	if err != nil {
		t.Fatalf("Error creating engine '%s'", err)
		return
	}
	defer func() {
		assert.Error(t, e.store.Close())
	}()

	n, err := e.createNamespace(namespaceName)

	if err != nil {
		t.Fatalf("Error creating namesapce '%s'", err)
		return
	}

	entity, err := n.findorCreateEntity(entityName)

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}

	if entity.Name != entityName {
		t.Fatalf("Entity names do not match expected '%s', actual '%s'", entityName, entity.Name)
		return
	}

	entities, err := n.getEntities()
	if err != nil {
		t.Fatalf("failed to get entities '%s'", err)
		return
	}

	if len(entities) != 1 {
		t.Fatalf("Expected namespace to have only one entity but found '%d'", len(entities))
		return
	}
}
