package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nixmade/orchestrator/response"
	"github.com/rs/zerolog"
)

func selection(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var tsRequest TargetSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&tsRequest); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var tsResponse TargetSelectionResponse

	//approve only 1 target if it exists
	for _, targetName := range tsRequest.Targets {
		if strings.ToLower(targetName.Name) == "clienttarget0" {
			tsResponse.Targets = append(tsResponse.Targets, targetName)
		}
	}

	response.JSON(w, http.StatusOK, tsResponse)
}

func approval(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var tsRequest TargetApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&tsRequest); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var tsResponse TargetApprovalResponse

	//approve only 1 target if it exists
	for _, targetName := range tsRequest.Targets {
		if strings.ToLower(targetName.Name) == "clienttarget0" {
			tsResponse.Targets = append(tsResponse.Targets, targetName)
		}
	}

	response.JSON(w, http.StatusOK, tsResponse)
}

func monitoring(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var tsRequest TargetMonitoringRequest
	if err := json.NewDecoder(r.Body).Decode(&tsRequest); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var tsResponse TargetMonitoringResponse

	if strings.ToLower(tsRequest.Target.Name) == "clienttarget0" {
		tsResponse.Status = "ok"
		tsResponse.Message = "monitoring successful"
	} else {
		tsResponse.Status = "error"
		tsResponse.Message = "Dummy error message"
	}

	response.JSON(w, http.StatusOK, tsResponse)
}

func removal(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var tsRequest TargetRemovalRequest
	if err := json.NewDecoder(r.Body).Decode(&tsRequest); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var tsResponse TargetRemovalResponse

	// Remove 1 target if it exists
	for _, targetName := range tsRequest.Targets {
		if strings.ToLower(targetName.Name) == "clienttarget0" {
			tsResponse.Targets = append(tsResponse.Targets, targetName)
		}
	}

	response.JSON(w, http.StatusOK, tsResponse)
}

func extmonitoring(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var tsRequest ExternalMonitoringRequest
	if err := json.NewDecoder(r.Body).Decode(&tsRequest); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var tsResponse ExternalMonitoringResponse

	tsResponse.Status = "error"
	tsResponse.Message = "Dummy error message"

	for _, targetName := range tsRequest.Targets {
		if strings.ToLower(targetName.Name) == "clienttarget0" {
			tsResponse.Status = "ok"
			tsResponse.Message = "monitoring successful"
		}
	}

	response.JSON(w, http.StatusOK, tsResponse)
}

func TestWebController(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/selection", selection)
	r.Post("/approval", approval)
	r.Post("/removal", removal)
	r.Post("/monitoring", monitoring)
	r.Post("/extmonitoring", extmonitoring)

	srv := &http.Server{
		Addr:         "127.0.0.1:8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	defer func() {
		if err := srv.Shutdown(context.TODO()); err != nil {
			fmt.Printf("Failed to shutdiwn %v", err)
		}
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			return
		}
	}()
	time.Sleep(3 * time.Second)

	if testing.Verbose() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	}
	logger := getLogger().With().Str("Client", "TestWebController").Logger()
	const entityName = "TestWebControllerSelection"
	e, err := createEntity(entityName, logger)

	if err != nil {
		t.Fatalf("Expected entity to be created, found error %s", err)
		return
	}
	defer e.store.Close()

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

	targetController := &EntityWebTargetController{
		SelectionEndpoint:  "http://127.0.0.1:8080/selection",
		ApprovalEndpoint:   "http://127.0.0.1:8080/approval",
		MonitoringEndpoint: "http://127.0.0.1:8080/monitoring",
		RemovalEndpoint:    "http://127.0.0.1:8080/removal",
	}

	monitoringController := &EntityWebMonitoringController{
		ExternalMonitoringEndpoint: "http://127.0.0.1:8080/extmonitoring",
	}
	err = e.setTargetController(targetController)
	if err != nil {
		t.Fatal(err)
		return
	}

	err = e.setMonitoringController(monitoringController)
	if err != nil {
		t.Fatal(err)
		return
	}

	rollout, err := e.findOrCreateRollout()
	if err != nil {
		t.Fatal(err)
		return
	}

	selectedTargets, err := rollout.TargetController.TargetSelection(clientTargets, 1)

	if err != nil {
		t.Fatal(err)
		return
	}

	if len(selectedTargets) != 1 {
		t.Fatalf("Expected just 1 target to be selected but found %d", len(selectedTargets))
		return
	}

	if selectedTargets[0].Name != "clientTarget0" {
		t.Fatalf("Expected clientTarget0 to be selection but found %s", selectedTargets[0].Name)
		return
	}

	approvedTargets, err := rollout.TargetController.TargetApproval(clientTargets[1:])

	if err != nil {
		t.Fatal(err)
		return
	}

	if len(approvedTargets) > 0 {
		t.Fatalf("Expected just nothing to be approved but found %d", len(approvedTargets))
		return
	}

	approvedTargets, err = rollout.TargetController.TargetApproval(clientTargets)

	if err != nil {
		t.Fatal(err)
		return
	}

	if len(approvedTargets) != 1 {
		t.Fatalf("Expected just clienttarget0 to be approved but found %d", len(approvedTargets))
		return
	}

	if approvedTargets[0].Name != "clientTarget0" {
		t.Fatalf("Expected just clienttarget0 to be approved but found %s", approvedTargets[0].Name)
		return
	}

	err = rollout.TargetController.TargetMonitoring(clientTargets[0])
	if err != nil {
		t.Fatalf("Expected success, but found an error %s", err)
		return
	}

	err = rollout.TargetController.TargetMonitoring(clientTargets[1])
	if err == nil {
		t.Fatal("Expected an error, but no error found")
		return
	}

	removedTargets, err := rollout.TargetController.TargetRemoval(clientTargets[1:], 2)

	if err != nil {
		t.Fatal(err)
		return
	}

	if len(removedTargets) > 0 {
		t.Fatalf("Expected just nothing to be removed but found %d", len(removedTargets))
		return
	}

	removedTargets, err = rollout.TargetController.TargetRemoval(clientTargets, 2)

	if err != nil {
		t.Fatal(err)
		return
	}

	if len(removedTargets) != 1 {
		t.Fatalf("Expected clientTarget0 to be removed but found %d", len(removedTargets))
		return
	}

	if removedTargets[0].Name != "clientTarget0" {
		t.Fatalf("Expected just clienttarget0 to be approved but found %s", removedTargets[0].Name)
		return
	}

	err = rollout.MonitoringController.ExternalMonitoring(clientTargets)
	if err != nil {
		t.Fatalf("Expected success, but found an error %s", err)
		return
	}

	err = rollout.MonitoringController.ExternalMonitoring(clientTargets[1:])
	if err == nil {
		t.Fatal("Expected an error, but no error found")
		return
	}
}
