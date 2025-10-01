package core

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nixmade/orchestrator/store"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func createEntity(entityName string, logger zerolog.Logger) (*Entity, error) {
	if testing.Verbose() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
	var dbStore store.Store
	var err error
	if os.Getenv("DATABASE_URL") != "" {
		dbStore, err = store.NewPgxStoreWithTable(os.Getenv("DATABASE_URL"), entityName)
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

	namespace, err := e.createNamespace("EntityTest")
	if err != nil {
		return nil, err
	}

	return namespace.createEntity(entityName)
}

func getLogger() zerolog.Logger {
	return zerolog.New(os.Stderr).With().Caller().Timestamp().Logger().Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func TestCreateEntity(t *testing.T) {
	const entityName = "TestCreateEntity"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}

	if e == nil {
		t.Fatalf("Expected entity to be created, found nil")
		return
	}

	defer func() {
		assert.NoError(t, e.store.Close())
	}()

	if e.Name != entityName {
		t.Fatalf("Entity names do not match expected '%s', actual '%s'", entityName, e.Name)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be created, found nil")
		return
	}

	if rollout == nil {
		t.Fatalf("Expected rollout to be set, found nil")
		return
	}

	entityTargets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected entity targets to be returned, found error")
		return
	}

	if len(entityTargets) > 0 {
		t.Fatalf("Expected new entity to have no targets but found '%d'", len(entityTargets))
		return
	}
}

func TestSetTargetVersion(t *testing.T) {
	const entityName = "TestSetTargetVersion"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer func() {
		assert.NoError(t, e.store.Close())
	}()

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be created, found error %s", err)
		return
	}

	if rollout == nil {
		t.Fatalf("Expected rollout to be set, found nil")
		return
	}

	if err := e.setTargetVersion("v1", false); err != nil {
		t.Fatalf("Failed to set target version: %s", err)
		return
	}

	rollout, err = e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be found, found error %s", err)
		return
	}

	if rollout.State.TargetVersion != "v1" {
		t.Fatalf("Expected target version to be v1 but found %s", rollout.State.TargetVersion)
		return
	}
}

func TestSetRolloutOptions(t *testing.T) {
	const entityName = "TestSetRolloutOptions"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer func() {
		assert.NoError(t, e.store.Close())
	}()

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be created, found error %s", err)
		return
	}

	if rollout == nil {
		t.Fatalf("Expected rollout to be set, found nil")
		return
	}

	options := DefaultRolloutOptions()
	options.BatchPercent = 100
	options.DurationTimeoutSecs = 7200
	options.SuccessPercent = 80
	options.SuccessTimeoutSecs = 1800

	if err := e.setRolloutOptions(options); err != nil {
		t.Fatalf("Failed to set target version: %s", err)
		return
	}

	rollout, err = e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be created, found error %s", err)
		return
	}

	if rollout.State.Options.BatchPercent != 100 {
		t.Fatalf("Expected batch percent to be 100 but found %d", rollout.State.Options.BatchPercent)
		return
	}

	if rollout.State.Options.DurationTimeoutSecs != 7200 {
		t.Fatalf("Expected batch percent to be 7200 but found %d", rollout.State.Options.DurationTimeoutSecs)
		return
	}

	if rollout.State.Options.SuccessPercent != 80 {
		t.Fatalf("Expected batch percent to be 80 but found %d", rollout.State.Options.SuccessPercent)
		return
	}

	if rollout.State.Options.SuccessTimeoutSecs != 1800 {
		t.Fatalf("Expected batch percent to be 1800 but found %d", rollout.State.Options.SuccessTimeoutSecs)
		return
	}
}

type CustomEntityTestController struct {
	NoOpEntityTargetController
}

func (c *CustomEntityTestController) TargetSelection(targets []*ClientState, selection int) ([]*ClientState, error) {
	return nil, nil
}

func TestSetEntityController(t *testing.T) {
	RegisteredTargetControllers = append(RegisteredTargetControllers, &CustomEntityTestController{})
	const entityName = "TestSetEntityController"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer func() {
		assert.NoError(t, e.store.Close())
	}()

	if err := e.setTargetController(&CustomEntityTestController{}); err != nil {
		t.Fatalf("Failed to set rollout controller: %s", err)
		return
	}

	clientState := &ClientState{Name: "dummytarget"}
	targets := []*ClientState{clientState}
	_, err = e.findOrCreateEntityTarget(clientState)
	if err != nil {
		t.Fatalf("Expected entity target to be created, found error %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Expected rollout to be created, found error %s", err)
		return
	}

	if rollout == nil {
		t.Fatalf("Expected rollout to be set, found nil")
		return
	}

	selectedTargets, _ := rollout.TargetController.TargetSelection(targets, 5)

	if len(selectedTargets) > 0 {
		t.Fatalf("Expected no targets to be selection but found '%d'", len(selectedTargets))
		return
	}
}

func TestUpdateEntityTargets(t *testing.T) {
	const entityName = "TestUpdateEntityTargets"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer func() {
		assert.NoError(t, e.store.Close())
	}()
	var clientTargets []*ClientState

	for i := 0; i < 5; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	if err := e.updateEntityTargets(clientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	entityTargets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected to get entitiy targets, found error %s", err)
		return
	}

	if len(entityTargets) != len(clientTargets) {
		t.Fatalf("Entity Targets '%d', do not match Client Targets '%d'", len(entityTargets), len(clientTargets))
		return
	}

	changeTimestamps := make(map[string]time.Time)
	updateTimestamps := make(map[string]time.Time)

	for _, entityTarget := range entityTargets {
		if entityTarget.State.CurrentVersion.Version != "v1" {
			t.Fatalf("Expected Entity Target to be v1 but found '%s'", entityTarget.State.CurrentVersion.Version)
			return
		}
		changeTimestamps[entityTarget.Name] = entityTarget.State.CurrentVersion.ChangeTimestamp
		updateTimestamps[entityTarget.Name] = entityTarget.State.CurrentVersion.LastMessage.Timestamp
	}

	time.Sleep(1 * time.Second)

	if err := e.updateEntityTargets(clientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	entityTargets, err = e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected to get entitiy targets, found error %s", err)
		return
	}

	for _, entityTarget := range entityTargets {
		if entityTarget.State.CurrentVersion.Version != "v1" {
			t.Fatalf("Expected Entity Target to be v1 but found '%s'", entityTarget.State.CurrentVersion.Version)
			return
		}
		if changeTimestamps[entityTarget.Name] != entityTarget.State.CurrentVersion.ChangeTimestamp {
			t.Fatalf("Expected change timestamp '%+v' to match previous time stamp '%+v', since there are no updates",
				entityTarget.State.CurrentVersion.ChangeTimestamp, changeTimestamps[entityTarget.Name])
			return
		}
		if updateTimestamps[entityTarget.Name] != entityTarget.State.CurrentVersion.LastMessage.Timestamp {
			t.Fatalf("Expected update timestamp '%+v' to match previous time stamp '%+v', since there are no updates",
				entityTarget.State.CurrentVersion.LastMessage.Timestamp, updateTimestamps[entityTarget.Name])
			return
		}
	}

	for _, clientTarget := range clientTargets {
		clientTarget.IsError = true
		clientTarget.Message = "reporting failure"
	}

	time.Sleep(1 * time.Second)

	if err := e.updateEntityTargets(clientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	entityTargets, err = e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected to get entitiy targets, found error %s", err)
		return
	}

	for _, entityTarget := range entityTargets {
		if entityTarget.State.CurrentVersion.Version != "v1" {
			t.Fatalf("Expected Entity Target to be v1 but found '%s'", entityTarget.State.CurrentVersion.Version)
			return
		}
		if changeTimestamps[entityTarget.Name] != entityTarget.State.CurrentVersion.ChangeTimestamp {
			t.Fatalf("Expected change timestamp '%+v' to match previous time stamp '%+v', since there are no updates",
				entityTarget.State.CurrentVersion.ChangeTimestamp, changeTimestamps[entityTarget.Name])
			return
		}
		if time.Time.Equal(updateTimestamps[entityTarget.Name], entityTarget.State.CurrentVersion.LastMessage.Timestamp) {
			t.Fatalf("Expected update timestamp '%+v' not to match previous time stamp '%+v', since there are error updates",
				entityTarget.State.CurrentVersion.LastMessage.Timestamp, updateTimestamps[entityTarget.Name])
			return
		}
		updateTimestamps[entityTarget.Name] = entityTarget.State.CurrentVersion.LastMessage.Timestamp
	}

	// Let report error
	var newclientTargets []*ClientState

	for i := 0; i < 5; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v2",
			Message: "reporting failure",
			IsError: true,
		}
		newclientTargets = append(newclientTargets, clientTarget)
	}

	time.Sleep(1 * time.Second)

	if err := e.updateEntityTargets(newclientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	entityTargets, err = e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected to get entitiy targets, found error %s", err)
		return
	}

	for _, entityTarget := range entityTargets {
		if entityTarget.State.CurrentVersion.Version != "v2" {
			t.Fatalf("Expected Entity Target to be v2 but found '%s'", entityTarget.State.CurrentVersion.Version)
			return
		}
		if time.Time.Equal(changeTimestamps[entityTarget.Name], entityTarget.State.CurrentVersion.ChangeTimestamp) {
			t.Fatalf("Expected change timestamp '%+v' not to match previous time stamp '%+v' for target '%s', since there is a change in version",
				entityTarget.State.CurrentVersion.ChangeTimestamp, changeTimestamps[entityTarget.Name], entityTarget.Name)
			return
		}
		if time.Time.Equal(updateTimestamps[entityTarget.Name], entityTarget.State.CurrentVersion.LastMessage.Timestamp) {
			t.Fatalf("Expected update timestamp '%+v' not to match previous time stamp '%+v' for target '%s', since there is an error report",
				entityTarget.State.CurrentVersion.LastMessage.Timestamp, updateTimestamps[entityTarget.Name], entityTarget.Name)
			return
		}
	}
}

func TestReturnClientState(t *testing.T) {
	const entityName = "TestReturnClientState"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer func() {
		assert.NoError(t, e.store.Close())
	}()

	var clientTargets []*ClientState

	for i := 0; i < 20; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	if err := e.updateEntityTargets(clientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	entityTargets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Expected to get entitiy targets, found error %s", err)
		return
	}

	for _, entityTarget := range entityTargets {
		if entityTarget.State.CurrentVersion.Version != "v1" {
			t.Fatalf("Expected Entity Target to be v1 but found '%s'", entityTarget.State.CurrentVersion.Version)
			return
		}
		entityTarget.State.TargetVersion.Version = "v2"

		if err := e.store.SaveJSON(e.entityTargetKey(entityTarget.Group, entityTarget.Name), entityTarget); err != nil {
			t.Fatalf("Expected to save entitiy target %s, found error %s", entityTarget.Name, err)
			return
		}
	}

	clientTargets, err = e.returnClientState()
	if err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	for _, clientTarget := range clientTargets {
		if clientTarget.Version != "v2" {
			t.Fatalf("Expected Client Target to be v2 but found '%s'", clientTarget.Version)
			return
		}
	}
}
