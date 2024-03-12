package core

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nixmade/orchestrator/httpclient"
	"github.com/nixmade/orchestrator/server"
	"github.com/rs/zerolog"

	"github.com/go-chi/chi/v5"
)

type testContext struct {
	ctx *server.Context
	*httpclient.OrchestratorAPI
	bearerToken string
}

func createTestContext(_testName string) (*testContext, error) {
	t := &testContext{}

	if testing.Verbose() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
	var err error
	app := NewApp()
	t.ctx, err = server.Create(app)
	if err != nil {
		return t, err
	}

	t.OrchestratorAPI = httpclient.NewOrchestratorAPI("http://127.0.0.1:8080")

	time.Sleep(1 * time.Second)

	return t, nil
}

func cleanupTestContext(t *testContext) {
	// if ctx is not created its taken care of by this function
	if err := t.ctx.Delete(); err != nil {
		fmt.Printf("failed to cleanup context %v", err)
	}
}

func (t *testContext) setRolloutOptions(options *RolloutOptions) error {
	return httpclient.PostJSON(t.RolloutOptions("namespace", "entity"), t.bearerToken, options, nil)
}

func (t *testContext) setTargetController(controller *EntityWebTargetController) error {
	return httpclient.PostJSON(t.EntityTargetController("namespace", "entity"), t.bearerToken, controller, nil)
}

// func (t *testContext) setMonitoringController(controller *EntityWebMonitoringController) error {
// 	return httpclient.PostJSON(t.EntityMonitoringController("namespace", "entity"), t.bearerToken, controller, nil)
// }

func (t *testContext) setTargetVersion(targetVersion string) error {
	return httpclient.PostJSON(t.TargetVersion("namespace", "entity"), t.bearerToken, &EntityTargetVersion{Version: targetVersion}, nil)
}

func (t *testContext) postClientTargets(clientTargets []*ClientState) ([]*ClientState, error) {
	if err := httpclient.PostJSON(t.Orchestrate("namespace", "entity"), t.bearerToken, clientTargets, &clientTargets); err != nil {
		return nil, err
	}
	return clientTargets, nil
}

func (t *testContext) postClientTargetsAsync(clientTargets []*ClientState) ([]*ClientState, error) {
	if err := httpclient.PostJSON(t.Status("namespace", "entity"), t.bearerToken, clientTargets, nil); err != nil {
		return nil, err
	}
	time.Sleep(3 * time.Second)

	if err := httpclient.GetJSON(t.Status("namespace", "entity"), t.bearerToken, &clientTargets); err != nil {
		return nil, err
	}
	return clientTargets, nil
}

func (t *testContext) getClientTargetsAsync(groupName string) ([]*ClientState, error) {
	url := t.Status("namespace", "entity")

	if groupName != "" {
		url = t.GroupStatus("namespace", "entity", groupName)
	}

	var clientTargets []*ClientState
	if err := httpclient.GetJSON(url, t.bearerToken, &clientTargets); err != nil {
		return nil, err
	}

	return clientTargets, nil
}

func (t *testContext) establishLKG(numTargets int, version string) ([]*ClientState, error) {
	return t.establishGroupLKG(numTargets, version, "")
}

func (t *testContext) establishGroupLKG(numTargets int, version, groupName string) ([]*ClientState, error) {
	if err := t.setRolloutOptions(&RolloutOptions{
		BatchPercent:        100,
		SuccessPercent:      0,
		SuccessTimeoutSecs:  0,
		DurationTimeoutSecs: 0,
	}); err != nil {
		return nil, err
	}

	if err := t.setTargetVersion("v1"); err != nil {
		return nil, err
	}

	var clientTargets []*ClientState
	var err error

	for i := 0; i < numTargets; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Group:   groupName,
			Version: version,
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	clientTargets, err = t.postClientTargets(clientTargets)

	if err != nil {
		return nil, err
	}

	for _, clientTarget := range clientTargets {
		if clientTarget.Version != version {
			return nil, fmt.Errorf("Expected v1 but found %s", clientTarget.Version)
		}
	}

	options := DefaultRolloutOptions()
	options.SuccessTimeoutSecs = 0

	if err := t.setRolloutOptions(options); err != nil {
		return nil, err
	}

	return clientTargets, nil
}

func TestRolloutOrchestrate(t *testing.T) {
	tctx, err := createTestContext("TestRolloutOrchestrate")
	defer cleanupTestContext(tctx)

	if err != nil {
		t.Fatal(err)
		return
	}
	// Establish LKG
	var clientTargets []*ClientState
	clientTargets, err = tctx.establishLKG(10, "v1")

	if err != nil {
		t.Fatal(err)
		return
	}

	if err := tctx.setTargetVersion("v2"); err != nil {
		t.Fatal(err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	v1count := 0
	v2count := 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v1" {
			v1count++
		}
		if clientTarget.Version == "v2" {
			v2count++
		}
	}

	if v1count != 9 {
		t.Fatalf("Expected v1 count to be 9 but found %d", v1count)
		return
	}

	if v2count != 1 {
		t.Fatalf("Expected v2 count to be 1 but found %d", v2count)
		return
	}
}

func TestRollbackOrchestrate(t *testing.T) {
	tctx, err := createTestContext("TestRollbackOrchestrate")
	defer cleanupTestContext(tctx)

	if err != nil {
		t.Fatal(err)
		return
	}

	// Establish LKG
	var clientTargets []*ClientState
	clientTargets, err = tctx.establishLKG(10, "v1")

	if err != nil {
		t.Fatal(err)
		return
	}

	if err := tctx.setTargetVersion("v2"); err != nil {
		t.Fatal(err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	options := DefaultRolloutOptions()
	options.DurationTimeoutSecs = 3
	options.SuccessTimeoutSecs = 2
	if err := tctx.setRolloutOptions(options); err != nil {
		t.Fatal(err)
		return
	}

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	for i := 0; i < 6; i++ {
		for _, clientTarget := range clientTargets {
			if clientTarget.Version == "v2" {
				clientTarget.Message = "Error with this version"
				clientTarget.IsError = true
				continue
			}
			clientTarget.Message = "running successfully"
			clientTarget.IsError = false
		}

		clientTargets, err = tctx.postClientTargets(clientTargets)

		if err != nil {
			t.Fatal(err)
			return
		}
		time.Sleep(1 * time.Second)
	}

	for _, clientTarget := range clientTargets {
		if clientTarget.Version != "v1" {
			t.Fatalf("Do not expect any client to have version v2 but found %s %s", clientTarget.Name, clientTarget.Version)
			return
		}
	}
}

func TestRolloutOrchestrateAsync(t *testing.T) {
	tctx, err := createTestContext("TestRolloutOrchestrateAsync")
	defer cleanupTestContext(tctx)

	if err != nil {
		t.Fatal(err)
		return
	}
	// Establish LKG
	var clientTargets []*ClientState
	clientTargets, err = tctx.establishLKG(10, "v1")

	if err != nil {
		t.Fatal(err)
		return
	}

	if err := tctx.setTargetVersion("v2"); err != nil {
		t.Fatal(err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	clientTargets, err = tctx.postClientTargetsAsync(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	v1count := 0
	v2count := 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v1" {
			v1count++
		}
		if clientTarget.Version == "v2" {
			v2count++
		}
	}

	if v1count != 9 {
		t.Fatalf("Expected v1 count to be 9 but found %d", v1count)
		return
	}

	if v2count != 1 {
		t.Fatalf("Expected v2 count to be 1 but found %d", v2count)
		return
	}
}

func TestRolloutGroupOrchestrateAsync(t *testing.T) {
	tctx, err := createTestContext("TestRolloutGroupOrchestrateAsync")
	defer cleanupTestContext(tctx)

	if err != nil {
		t.Fatal(err)
		return
	}
	// Establish LKG
	var clientTargets []*ClientState
	clientTargets, err = tctx.establishGroupLKG(10, "v1", "Group1")

	if err != nil {
		t.Fatal(err)
		return
	}

	if err := tctx.setTargetVersion("v2"); err != nil {
		t.Fatal(err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	clientTargets, err = tctx.postClientTargetsAsync(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	v1count := 0
	v2count := 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v1" {
			v1count++
		}
		if clientTarget.Version == "v2" {
			v2count++
		}
	}

	if v1count != 9 {
		t.Fatalf("Expected v1 count to be 9 but found %d", v1count)
		return
	}

	if v2count != 1 {
		t.Fatalf("Expected v2 count to be 1 but found %d", v2count)
		return
	}

	clientTargets = nil

	for i := 0; i < 10; i++ {
		clientTarget := &ClientState{
			Name:    fmt.Sprintf("clientTarget%d", i),
			Group:   "Group2",
			Version: "v1",
			Message: "running successfully",
			IsError: false,
		}
		clientTargets = append(clientTargets, clientTarget)
	}

	clientTargets, err = tctx.postClientTargetsAsync(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	v1count = 0
	v2count = 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v1" {
			v1count++
		}
		if clientTarget.Version == "v2" {
			v2count++
		}
	}

	if v1count != 19 {
		t.Fatalf("Expected v1 count to be 19 but found %d", v1count)
		return
	}

	if v2count != 1 {
		t.Fatalf("Expected v2 count to be 1 but found %d", v2count)
		return
	}

	clientTargets, err = tctx.getClientTargetsAsync("Group2")

	if err != nil {
		t.Fatal(err)
		return
	}

	for _, clientTarget := range clientTargets {
		if clientTarget.Group != "Group2" {
			t.Fatalf("Expected client group name to be Group2 but found %s", clientTarget.Group)
			return
		}

		if clientTarget.Version != "v1" {
			t.Fatalf("Expected client group Group2 to be all v1 but found %s", clientTarget.Version)
			return
		}
	}
}

func TestRolloutOrchestrateSelection(t *testing.T) {
	tctx, err := createTestContext("TestRolloutOrchestrateSelection")
	defer cleanupTestContext(tctx)

	if err != nil {
		t.Fatal(err)
		return
	}
	// Establish LKG
	var clientTargets []*ClientState
	clientTargets, err = tctx.establishLKG(10, "v1")

	if err != nil {
		t.Fatal(err)
		return
	}

	if err := tctx.setTargetVersion("v2"); err != nil {
		t.Fatal(err)
		return
	}

	time.Sleep(1 * time.Second)

	clientTargets, err = tctx.postClientTargets(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	r := chi.NewRouter()
	r.Post("/selection", selection)

	srv := &http.Server{
		Addr:         "127.0.0.1:8081",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	defer func() {
		if err := srv.Shutdown(context.TODO()); err != nil {
			t.Fatal(err)
			return
		}
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			return
		}
	}()

	controller := &EntityWebTargetController{SelectionEndpoint: "http://127.0.0.1:8081/selection"}

	if err := tctx.setTargetController(controller); err != nil {
		t.Fatal(err)
		return
	}

	clientTargets, err = tctx.postClientTargetsAsync(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	v1count := 0
	v2count := 0
	for _, clientTarget := range clientTargets {
		if clientTarget.Version == "v1" {
			v1count++
		}
		if clientTarget.Version == "v2" {
			v2count++
		}
	}

	if v1count != 9 {
		t.Fatalf("Expected v1 count to be 9 but found %d", v1count)
		return
	}

	if v2count != 1 {
		t.Fatalf("Expected v2 count to be 1 but found %d", v2count)
		return
	}
}
