package core

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nixmade/orchestrator/store"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var persist = flag.Bool("persist", false, "optional persist to disk")

func setupTestEngineWithStore(testName string, dbstore store.Store) (*Engine, error) {
	if testing.Verbose() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
	logger := getLogger().With().Str("Test", testName).Logger()

	app := NewApp()
	app.dbStore = dbstore
	app.logger = logger

	return NewOrchestratorEngineWithApp(app)
}

func setupTestEngine(testName string) (*Engine, error) {
	if os.Getenv("DATABASE_URL") != "" {
		dbstore, err := store.NewPgxStoreWithTable(os.Getenv("DATABASE_URL"), testName)
		if err != nil {
			return nil, err
		}
		return setupTestEngineWithStore(testName, dbstore)
	}
	testDir := ""
	if *persist {
		testDir = "./" + testName
	}
	aesKey := make([]byte, 32)
	_, err := rand.Read(aesKey)
	if err != nil {
		return nil, err
	}
	dbstore, err := store.NewBadgerDBStore(testDir, string(aesKey))
	if err != nil {
		return nil, err
	}
	return setupTestEngineWithStore(testName, dbstore)
}

func cleanupTestEngine(engine *Engine, testName string) {
	if err := engine.Shutdown(); err != nil {
		fmt.Println(err.Error())
	}
	if err := engine.store.Close(); err != nil {
		fmt.Println(err.Error())
	}
	os.RemoveAll("./" + testName)
}

func setupNamespace(engine *Engine, namespaceName, entityName string, numTargets int) ([]*ClientState, error) {
	options := DefaultRolloutOptions()
	options.SuccessTimeoutSecs = 0

	if err := engine.SetRolloutOptions(namespaceName, entityName, &RolloutOptions{
		BatchPercent:        100,
		SuccessPercent:      0,
		SuccessTimeoutSecs:  0,
		DurationTimeoutSecs: 0,
	}); err != nil {
		return nil, err
	}

	if err := engine.SetTargetVersion(namespaceName, entityName, EntityTargetVersion{Version: "v1"}); err != nil {
		return nil, err
	}

	var clientTargets []*ClientState

	for i := 0; i < numTargets; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	// Set LKG to v1
	if _, err := engine.Orchestrate(namespaceName, entityName, clientTargets); err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	if err := engine.SetRolloutOptions(namespaceName, entityName, options); err != nil {
		return nil, err
	}

	if err := engine.SetTargetVersion(namespaceName, entityName, EntityTargetVersion{Version: "v2"}); err != nil {
		return nil, err
	}

	// Following updates rolling version to v2
	if _, err := engine.Orchestrate(namespaceName, entityName, clientTargets); err != nil {
		return nil, err
	}

	return clientTargets, nil
}

func getTargetVersionCount(clientTargets []*ClientState, targetVersion string) []*ClientState {
	var inRolloutTargets []*ClientState
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == targetVersion {
			inRolloutTargets = append(inRolloutTargets, clientTarget)
		}
	}
	return inRolloutTargets
}

func markTargetVersionBad(clientTargets []*ClientState, targetVersion string) []*ClientState {
	inRolloutTargets := getTargetVersionCount(clientTargets, targetVersion)

	// lets post some errors
	for _, inRolloutTarget := range inRolloutTargets {
		inRolloutTarget.IsError = true
		inRolloutTarget.Message = "New version just doesnt work"
	}

	return inRolloutTargets
}

func testRolloutOrchestrate(t *testing.T, engine *Engine, namespaceName, entityName string, clientTargets []*ClientState) {
	var err error
	options := DefaultRolloutOptions()
	options.DurationTimeoutSecs = 10
	options.SuccessTimeoutSecs = 0

	if err := engine.SetRolloutOptions(namespaceName, entityName, options); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	for i := 1; i <= 3; i++ {
		clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)

		if err != nil {
			t.Fatalf("Entity Orchestrate failed with error %s", err)
			return
		}

		if inRolloutTargets := getTargetVersionCount(clientTargets, "v2"); len(inRolloutTargets) != i {
			t.Fatalf("Expected inRollout '%d' but found '%d'", i, len(inRolloutTargets))
			return
		}

		time.Sleep(1 * time.Second)

		clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)

		if err != nil {
			t.Fatalf("Entity Orchestrate failed with error %s", err)
			return
		}

		time.Sleep(1 * time.Second)
	}

	_, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	n, err := engine.getNamespace(namespaceName)
	if err != nil {
		t.Fatalf("Error getting namespace %s", err)
		return
	}

	e, err := n.findorCreateEntity(entityName)
	if err != nil {
		t.Fatalf("Error getting entity %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Error getting rollout %s", err)
		return
	}

	lkg := rollout.State.LastKnownGoodVersion

	if lkg != "v2" {
		t.Fatalf("Expect lkg '%s' to be v2", lkg)
		return
	}
}

func testRollbackOrchestrate(t *testing.T, engine *Engine, namespaceName, entityName string, clientTargets []*ClientState) {
	var err error
	options := DefaultRolloutOptions()
	options.DurationTimeoutSecs = 2
	options.SuccessTimeoutSecs = 0

	if err := engine.SetRolloutOptions(namespaceName, entityName, options); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	inRolloutTargets := markTargetVersionBad(clientTargets, "v2")
	if len(inRolloutTargets) != 1 {
		t.Fatalf("Expected inRollout 1 but found '%d'", len(inRolloutTargets))
		return
	}

	// update client targets
	clientTargets = inRolloutTargets

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	clientTargets = markTargetVersionBad(clientTargets, "v2")

	time.Sleep(3 * time.Second)

	// This would mark it as last bad
	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	clientTargets = markTargetVersionBad(clientTargets, "v2")

	// This would start the rollback process
	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	if lkgTargets := getTargetVersionCount(clientTargets, "v1"); len(lkgTargets) != len(clientTargets) {
		t.Fatalf("Expected '%d' to rolled back to lkg but found '%d'", len(clientTargets), len(lkgTargets))
		return
	}

	n, err := engine.getNamespace(namespaceName)
	if err != nil {
		t.Fatalf("Error getting namespace %s", err)
		return
	}

	e, err := n.findorCreateEntity(entityName)
	if err != nil {
		t.Fatalf("Error getting entity %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Error getting rollout %s", err)
		return
	}

	lkg := rollout.State.LastKnownGoodVersion

	if lkg != "v1" {
		t.Fatalf("Expect lkg '%s' to be v1", lkg)
		return
	}

	lkb := rollout.State.LastKnownBadVersion

	if lkb != "v2" {
		t.Fatalf("Expect lkb '%s' to be v2", lkb)
		return
	}
}

// TestRollbackEntityOrchestrate tests rollback functionality
func TestRollbackEntityOrchestrate(t *testing.T) {

	const numTargets = 3
	const namespaceName = "TestRollbackEntityOrchestrate"
	const entityName = "NewEntity"

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRollbackOrchestrate(t, engine, namespaceName, entityName, clientTargets)
}

// TestRolloutEntityOrchestrate 99% rollout case scenario
func TestRolloutEntityOrchestrate(t *testing.T) {

	const numTargets = 3
	const namespaceName = "TestBasicEntityOrchestrate"
	const entityName = "NewEntity"

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRolloutOrchestrate(t, engine, namespaceName, entityName, clientTargets)
}

type CustomEngineTestController struct {
	CurTargetNum int            `json:"curtargetnum"`
	logger       zerolog.Logger `json:"-"`
	NoOpEntityTargetController
}

func (c *CustomEngineTestController) TargetSelection(targets []*ClientState, selection int) ([]*ClientState, error) {
	c.logger.Info().Msgf("TargetSelection called with '%d' targets", len(targets))

	var newClientTargets []*ClientState
	for i := c.CurTargetNum; i < (c.CurTargetNum + selection); i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		c.logger.Info().Msgf("TargetSelection created new '%s'", clientTarget.Name)
		newClientTargets = append(newClientTargets, clientTarget)
	}

	c.CurTargetNum = c.CurTargetNum + selection

	return newClientTargets, nil
}

func (c *CustomEngineTestController) TargetRemoval(targets []*ClientState, selection int) ([]*ClientState, error) {
	c.logger.Info().Msgf("TargetRemoval called with '%d' targets", len(targets))
	var retClientTargets []*ClientState
	for _, removeTarget := range targets {
		if selection > 0 {
			//nolint:ineffassign
			selection--
			retClientTargets = append(retClientTargets, removeTarget)
			c.logger.Info().Msgf("TargetRemoval removed old '%s'", removeTarget.Name)
			break
		}
	}

	return retClientTargets, nil
}

func TestBlueGreenRollout(t *testing.T) {
	const numTargets = 3
	const namespaceName = "TestBlueGreenRollout"
	const entityName = "NewEntity"

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)
	RegisteredTargetControllers = append(RegisteredTargetControllers, &CustomEngineTestController{logger: engine.logger})

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	options := DefaultRolloutOptions()
	options.DurationTimeoutSecs = 10
	options.SuccessTimeoutSecs = 0

	if err := engine.SetRolloutOptions(namespaceName, entityName, options); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	controller := &CustomEngineTestController{CurTargetNum: numTargets}

	if err := engine.SetEntityTargetController(namespaceName, entityName, controller); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	for i := 1; i <= 3; i++ {
		if inRolloutTargets := getTargetVersionCount(clientTargets, "v2"); len(inRolloutTargets) != i {
			t.Fatalf("Expected inRollout '%d' but found '%d'", i, len(inRolloutTargets))
			return
		}

		time.Sleep(1 * time.Second)

		clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
		if err != nil {
			t.Fatalf("Entity Orchestrate failed with error %s", err)
			return
		}

		time.Sleep(1 * time.Second)

		clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
		if err != nil {
			t.Fatalf("Entity Orchestrate failed with error %s", err)
			return
		}
	}

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	// At end of the rollout we want any new targets to be removed
	// Expect all of them to be new version which is already tested above
	if len(clientTargets) != numTargets {
		t.Fatalf("Expected total clientTargets '%d' to be '%d'", len(clientTargets), numTargets)
		return
	}

	for i := numTargets; i < controller.CurTargetNum; i++ {
		found := false
		targetName := fmt.Sprintf("clientTarget%d", i)
		for _, clientTarget := range clientTargets {
			if clientTarget.Name == targetName {
				found = true
			}
		}

		if !found {
			t.Fatalf("Expected clientTarget '%s' to be present but not found", targetName)
			return
		}
	}

	n, err := engine.getNamespace(namespaceName)
	if err != nil {
		t.Fatalf("Error getting namespace %s", err)
		return
	}

	e, err := n.findorCreateEntity(entityName)
	if err != nil {
		t.Fatalf("Error getting entity %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Error getting rollout %s", err)
		return
	}

	lkg := rollout.State.LastKnownGoodVersion

	if lkg != "v2" {
		t.Fatalf("Expect lkg '%s' to be v2", lkg)
		return
	}
}

func TestBlueGreenRollback(t *testing.T) {
	const numTargets = 3
	const namespaceName = "TestBlueGreenRollback"
	const entityName = "NewEntity"

	RegisteredTargetControllers = append(RegisteredTargetControllers, &CustomEngineTestController{})

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	options := DefaultRolloutOptions()
	options.DurationTimeoutSecs = 2
	options.SuccessTimeoutSecs = 0

	if err := engine.SetRolloutOptions(namespaceName, entityName, options); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	controller := &CustomEngineTestController{CurTargetNum: numTargets}

	if err := engine.SetEntityTargetController(namespaceName, entityName, controller); err != nil {
		t.Fatalf("Failed to set rollout options %s", err)
		return
	}

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	if inRolloutTargets := getTargetVersionCount(clientTargets, "v2"); len(inRolloutTargets) != 1 {
		t.Fatalf("Expected inRollout '%d' but found '%d'", 1, len(inRolloutTargets))
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	if inRolloutTargets := getTargetVersionCount(clientTargets, "v2"); len(inRolloutTargets) != 2 {
		t.Fatalf("Expected inRollout '%d' but found '%d'", 2, len(inRolloutTargets))
		return
	}

	time.Sleep(3 * time.Second)

	clientTargets = markTargetVersionBad(clientTargets, "v2")
	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	if inRolloutTargets := getTargetVersionCount(clientTargets, "v1"); len(inRolloutTargets) != 2 {
		t.Fatalf("Expected inRollout '%d' but found '%d'", 2, len(inRolloutTargets))
		return
	}

	clientTargets = markTargetVersionBad(clientTargets, "v2")

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	clientTargets, err = engine.Orchestrate(namespaceName, entityName, clientTargets)
	if err != nil {
		t.Fatalf("Entity Orchestrate failed with error %s", err)
		return
	}

	// At end of the rollout we want any new targets to be removed
	// Expect all of them to be new version which is already tested above
	if len(clientTargets) != numTargets {
		t.Fatalf("Expected total clientTargets '%d' to be '%d'", len(clientTargets), numTargets)
		return
	}

	targetCount := 0
	for i := 0; i < numTargets; i++ {
		targetName := fmt.Sprintf("clientTarget%d", i)
		for _, clientTarget := range clientTargets {
			if clientTarget.Name == targetName {
				targetCount++
			}
		}
	}

	if targetCount != 2 {
		t.Fatalf("Expected old clientTargets of count '%d' to be '%d'", targetCount, 2)
		return
	}

	n, err := engine.getNamespace(namespaceName)
	if err != nil {
		t.Fatalf("Error getting namespace %s", err)
		return
	}

	e, err := n.findorCreateEntity(entityName)
	if err != nil {
		t.Fatalf("Error getting entity %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Error getting rollout %s", err)
		return
	}

	lkg := rollout.State.LastKnownGoodVersion

	if lkg != "v1" {
		t.Fatalf("Expect lkg '%s' to be v1", lkg)
		return
	}

	lkb := rollout.State.LastKnownBadVersion

	if lkb != "v2" {
		t.Fatalf("Expect lkb '%s' to be v2", lkb)
		return
	}
}

func TestMultipleNamespaceEntityRollout(t *testing.T) {
	const numTargets = 3
	namespaceName := "TestMultipleNamespaceEntityRollout"
	entityName := "NewEntity"

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRolloutOrchestrate(t, engine, namespaceName, entityName, clientTargets)

	namespaceName = "TestMultipleNamespaceEntityRollout"
	entityName = "NewEntity2"

	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRolloutOrchestrate(t, engine, namespaceName, entityName, clientTargets)

	namespaceName = "TestMultipleNamespaceEntityRollout2"
	entityName = "NewEntity"

	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRolloutOrchestrate(t, engine, namespaceName, entityName, clientTargets)
}

func TestMultipleNamespaceEntityRollback(t *testing.T) {
	const numTargets = 3
	namespaceName := "TestMultipleNamespaceEntityRollback"
	entityName := "NewEntity"

	engine, err := setupTestEngine(namespaceName)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}
	defer cleanupTestEngine(engine, namespaceName)

	var clientTargets []*ClientState
	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRollbackOrchestrate(t, engine, namespaceName, entityName, clientTargets)

	namespaceName = "TestMultipleNamespaceEntityRollback"
	entityName = "NewEntity2"

	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRollbackOrchestrate(t, engine, namespaceName, entityName, clientTargets)

	namespaceName = "TestMultipleNamespaceEntityRollback2"
	entityName = "NewEntity"

	clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	testRollbackOrchestrate(t, engine, namespaceName, entityName, clientTargets)
}

// Test Store save and load
func TestEngineSaveLoad(t *testing.T) {
	const numTargets = 3
	const namespaceName = "TestBasicEntityOrchestrate"
	const entityName = "NewEntity"

	var dbstore store.Store
	var err error
	if os.Getenv("DATABASE_URL") != "" {
		dbstore, err = store.NewPgxStoreWithTable(os.Getenv("DATABASE_URL"), "TestEngineSaveLoad")
	} else {
		dbstore, err = store.NewBadgerDBStore("", "")
	}
	require.NoError(t, err)
	// Call Orchestrate and save the state
	defer dbstore.Close()
	{
		engine, err := setupTestEngineWithStore(namespaceName, dbstore)
		if err != nil {
			t.Fatalf("Failed to setup Test Engine %s", err)
			return
		}

		var clientTargets []*ClientState
		clientTargets, err = setupNamespace(engine, namespaceName, entityName, numTargets)

		if err != nil {
			t.Fatalf("Failed to setup Test Engine %s", err)
			return
		}

		testRolloutOrchestrate(t, engine, namespaceName, entityName, clientTargets)
	}

	// Create the same engine should load up the state

	engine, err := setupTestEngineWithStore(namespaceName, dbstore)
	if err != nil {
		t.Fatalf("Failed to setup Test Engine %s", err)
		return
	}

	n, err := engine.getNamespace(namespaceName)
	if err != nil {
		t.Fatalf("Error getting namespace %s", err)
		return
	}

	e, err := n.findorCreateEntity(entityName)
	if err != nil {
		t.Fatalf("Error getting entity %s", err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatalf("Error getting rollout %s", err)
		return
	}

	lkg := rollout.State.LastKnownGoodVersion

	if lkg != "v2" {
		t.Fatalf("Expect lkg '%s' to be v2", lkg)
		return
	}

	if err := dbstore.Close(); err != nil {
		t.Fatalf("Failed to close store %v", err)
		return
	}
}
