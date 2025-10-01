package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func setupEntity() (*Entity, []*ClientState, error) {
	if testing.Verbose() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
	const entityName = "TestCreateEntity"
	e, err := createEntity(entityName, getLogger())

	if err != nil {
		return nil, nil, fmt.Errorf("Expected entity to be created, %s", err)
	}

	var clientTargets []*ClientState

	for i := 0; i < 10; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		return nil, nil, fmt.Errorf("Expected rollout to be created, %s", err)
	}

	rollout.State.LastKnownGoodVersion = "v1"
	if err := e.store.SaveJSON(e.rolloutKey(), rollout); err != nil {
		return nil, nil, fmt.Errorf("Expected rollout to be saved, %s", err)
	}

	if err := e.updateEntityTargets(clientTargets); err != nil {
		return nil, nil, fmt.Errorf("Failed to update Entity Targets: '%s'", err)
	}

	return e, clientTargets, nil
}

func TestUpdateLastKnownVersions(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	state := createRolloutInfo(targets)

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	// Set up last known versions
	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = targets
	state.failedTargets = nil

	if err := rollout.updateLastKnownVersions(state); err != nil {
		t.Fatalf("Failed to updateLastKnownVersions '%s'", err)
		return
	}

	// Above should lkg to v2
	if rollout.State.LastKnownGoodVersion != "v2" {
		t.Fatalf("Expected lkg to be v2 instead found '%s'", rollout.State.LastKnownGoodVersion)
		return
	}

	if rollout.State.LastKnownBadVersion != "" {
		t.Fatalf("Expected lkb to be empty instead found '%s'", rollout.State.LastKnownBadVersion)
		return
	}

	// Set up last known versions
	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = nil
	state.failedTargets = targets

	if err := rollout.updateLastKnownVersions(state); err != nil {
		t.Fatalf("Failed to updateLastKnownVersions '%s'", err)
		return
	}

	// Above should have lkg to be v1
	if rollout.State.LastKnownGoodVersion != "v1" {
		t.Fatalf("Expected lkg to be v1 instead found '%s'", rollout.State.LastKnownGoodVersion)
		return
	}

	// lkb to be v2
	if rollout.State.LastKnownBadVersion != "v2" {
		t.Fatalf("Expected lkb to be v2 instead found '%s'", rollout.State.LastKnownBadVersion)
		return
	}
}

func TestSetAllLastKnownGood(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout.State.RollingVersion = "v5"

	if err := rollout.setAllLastKnownGood(targets); err != nil {
		t.Fatalf("Error in setAllLastKnownGood: '%s'", err)
		return
	}

	clientTargets, err := e.returnClientState()
	if err != nil {
		t.Fatalf("Error in returnClientState: '%s'", err)
		return
	}

	for _, clientTarget := range clientTargets {
		if clientTarget.Version != "v5" {
			t.Fatalf("Expected version to be v5 but found '%s'", clientTarget.Version)
			return
		}
	}
}

func TestDetermineCurrentState(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout.State.RollingVersion = "v1"

	// establish an lkg
	if err := rollout.setAllLastKnownGood(targets); err != nil {
		t.Fatalf("Error in setAllLastKnownGood: '%s'", err)
		return
	}

	rollout.State.RollingVersion = "v5"
	state := createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	if len(state.availableTargets) != len(targets) {
		t.Fatalf("Expected '%d' to be available but found '%d'", len(targets), len(state.availableTargets))
		return
	}

	// lets set rolling version as last bad, which should put everything in rollout state
	rollout.State.LastKnownBadVersion = "v5"
	state = createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	if len(state.availableTargets) != 0 {
		t.Fatalf("Expected no targets to be available but found '%d'", len(state.availableTargets))
		return
	}

	if len(state.inRolloutTargets) != len(targets) {
		t.Fatalf("Expected '%d' to be inRollout but found '%d'", len(targets), len(state.inRolloutTargets))
		return
	}
}

func TestIsStateChanged(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	state := createRolloutInfo(targets)

	// Set up last known versions
	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = targets
	state.failedTargets = nil

	if changed, err := rollout.isStateChanged(state); err != nil {
		t.Fatalf("Failed to isStateChanged '%s'", err)
		return
	} else if changed {
		t.Fatalf("Execpted to return false but state change returned true")
		return
	}

	// Set up last known versions
	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = nil
	state.failedTargets = targets

	// Above should have lkg to be v1
	if changed, err := rollout.isStateChanged(state); err != nil {
		t.Fatalf("Failed to isStateChanged '%s'", err)
		return
	} else if !changed {
		t.Fatalf("Execpted to return true but state change returned false")
		return
	}
}

func TestMonitorTargets(t *testing.T) {
	e, clientTargets, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout.State.RollingVersion = "v1"
	rollout.State.LastKnownGoodVersion = "v1"

	state := createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	if err := rollout.monitorTargets(state); err != nil {
		t.Fatalf("Failed to monitorTargets '%s'", err)
		return
	}

	if len(targets) != len(state.inRolloutTargets) {
		t.Fatalf("Expected inRolloutTargets '%d' to be equal to total targets '%d'", len(state.inRolloutTargets), len(targets))
		return
	}

	time.Sleep(1 * time.Second)

	rollout.State.Options.SuccessTimeoutSecs = 0

	if err := rollout.monitorTargets(state); err != nil {
		t.Fatalf("Failed to monitorTargets '%s'", err)
		return
	}

	if len(state.inRolloutTargets) != 0 {
		t.Fatalf("Expected In rollout targets '%d' to be empty", len(state.inRolloutTargets))
		return
	}

	if len(targets) != len(state.successTargets) {
		t.Fatalf("Expected Success Targets '%d' to be equal to total targets '%d'", len(state.successTargets), len(targets))
		return
	}

	for _, clientTarget := range clientTargets {
		clientTarget.IsError = true
		clientTarget.Message = "Simulating Failure"
	}

	if err := e.updateEntityTargets(clientTargets); err != nil {
		t.Fatalf("Failed to update Entity Targets: '%s'", err)
		return
	}

	state = createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	rollout.State.Options.SuccessTimeoutSecs = 900
	rollout.State.Options.DurationTimeoutSecs = 0

	if err := rollout.monitorTargets(state); err != nil {
		t.Fatalf("Failed to monitorTargets '%s'", err)
		return
	}

	if len(state.successTargets) != 0 {
		t.Fatalf("Expected In success targets '%d' to be empty", len(state.successTargets))
		return
	}

	if len(state.failedTargets) != len(state.totalTargets) {
		t.Fatalf("Expected In failures targets '%d' to be equal to all targets '%d'", len(state.successTargets), len(state.totalTargets))
		return
	}
}

func TestSelectTargets(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"

	state := createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	rollout.State.Options.BatchPercent = 50

	if err := rollout.selectTargets(state); err != nil {
		t.Fatalf("Error in selectTargets '%s'", err)
		return
	}

	if len(state.availableTargets) != 5 {
		t.Fatalf("Expected In available targets '%d' to be equal to 5", len(state.availableTargets))
		return
	}
}

func TestRollouNewTargets(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout.State.RollingVersion = "v2"
	rollout.State.LastKnownGoodVersion = "v1"

	state := createRolloutInfo(targets)

	if err := rollout.determineCurrentState(state); err != nil {
		t.Fatalf("Error in determineCurrentState '%s'", err)
		return
	}

	rollout.State.Options.BatchPercent = 50

	if err := rollout.selectTargets(state); err != nil {
		t.Fatalf("Error in selectTargets '%s'", err)
		return
	}

	if len(state.availableTargets) != 5 {
		t.Fatalf("Expected In available targets '%d' to be equal to 5", len(state.availableTargets))
		return
	}

	if err := rollout.rolloutNewTargets(state); err != nil {
		t.Fatalf("Error in rolloutNewTargets '%s'", err)
		return
	}

	if err := e.saveEntityTargets(targets); err != nil {
		t.Fatalf("Error in saveEntityTargets: '%s'", err)
		return
	}

	clientTargets, err := e.returnClientState()
	if err != nil {
		t.Fatalf("Error in returnClientState: '%s'", err)
		return
	}

	v2Count := 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v2" {
			v2Count++
		}
	}

	if v2Count != 5 {
		t.Fatalf("Expected In in rollout targets '%d' to be equal to 5", v2Count)
		return
	}
}

func TestUpdateRollingVersion(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	state := createRolloutInfo(targets)

	// Set up last known versions
	rollout.State.TargetVersion = "v2"
	rollout.State.RollingVersion = "v1"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = targets
	state.failedTargets = nil

	if err := rollout.updateRollingVersion(state); err != nil {
		t.Fatalf("Failed to updateRollingVersion '%s'", err)
		return
	}

	// Above should lkg to v2
	if rollout.State.RollingVersion != "v2" {
		t.Fatalf("Expected rolling to be v2 instead found '%s'", rollout.State.RollingVersion)
		return
	}

	// Set up last known versions
	rollout.State.RollingVersion = "v1"
	rollout.State.LastKnownGoodVersion = "v1"
	rollout.State.LastKnownBadVersion = ""

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = nil
	state.failedTargets = targets

	if err := rollout.updateRollingVersion(state); err != nil {
		t.Fatalf("Failed to updateLastKnownVersions '%s'", err)
		return
	}

	// Above should have lkg to be v1
	if rollout.State.RollingVersion != "v2" {
		t.Fatalf("Expected rollingversion change to v2 instead found '%s'", rollout.State.RollingVersion)
		return
	}
}

func TestForceRollingVersion(t *testing.T) {
	e, _, err := setupEntity()

	if err != nil {
		t.Fatalf("Failed to setup entity '%s'", err)
		return
	}
	defer func() {
		assert.Nil(t, e.store.Close())
	}()

	targets, err := e.getEntityTargets()
	if err != nil {
		t.Fatalf("Failed to get entity targets'%s'", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	state := createRolloutInfo(targets)

	// Set up last known versions
	rollout.State.RollingVersion = "v1"
	rollout.State.LastKnownGoodVersion = "v0"
	rollout.State.LastKnownBadVersion = ""

	if err := rollout.setTargetVersion("v2", true); err != nil {
		t.Fatalf("Failed to forceRollingVersion '%s'", err)
		return
	}

	// Set up state to mimic actual rollout
	state.totalTargets = targets
	state.successTargets = nil
	state.failedTargets = nil

	if err := rollout.updateRollingVersion(state); err != nil {
		t.Fatalf("Failed to updateRollingVersion '%s'", err)
		return
	}

	if rollout.State.RollingVersion != "v2" {
		t.Fatalf("Expected rolling to be v2 instead found '%s'", rollout.State.RollingVersion)
		return
	}

	if rollout.State.LastKnownBadVersion != "v1" {
		t.Fatalf("Expected last bad version to be v1 instead found '%s'", rollout.State.LastKnownBadVersion)
		return
	}
}
